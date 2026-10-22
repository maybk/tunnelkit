package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/internal/netcfg"
	"tunnelkit/fortivpn/internal/netdev"
	"tunnelkit/fortivpn/internal/sslvpn"
)

type session struct {
	cfg       fortivpn.Config
	handler   Handler
	opts      options
	log       *slog.Logger
	checkOnly bool

	mu         sync.Mutex
	dev        Device
	applied    netcfg.Applied
	devWg      sync.WaitGroup
	activeLink atomic.Pointer[link]
	lastInfo   fortivpn.TunnelInfo

	rxBytes   atomic.Uint64
	txBytes   atomic.Uint64
	rxPackets atomic.Uint64
	txPackets atomic.Uint64
	rttMillis int64
}

func newSession(cfg fortivpn.Config, h Handler, opts options, checkOnly bool) *session {
	return &session{
		cfg:       cfg,
		handler:   h,
		opts:      opts,
		log:       opts.logger,
		checkOnly: checkOnly,
	}
}

func (s *session) run(ctx context.Context) error {
	cfg := s.cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		fe := asFortiError(fortivpn.CodeInvalidConfig, err)
		s.emit(fortivpn.Event{Phase: fortivpn.PhaseDisconnected, Err: fe})
		return fe
	}
	s.cfg = cfg

	var clientOpts []sslvpn.ClientOption
	if s.opts.rootCAs != nil {
		clientOpts = append(clientOpts, sslvpn.WithRootCAs(s.opts.rootCAs))
	}
	if s.opts.samlListenAddr != "" {
		clientOpts = append(clientOpts, sslvpn.WithSAMLListenAddr(s.opts.samlListenAddr))
	}
	if s.opts.dumpConfigXML != nil {
		clientOpts = append(clientOpts, sslvpn.WithConfigXMLHook(s.opts.dumpConfigXML))
	}

	client, err := sslvpn.NewClient(cfg, s.opts.dialer, clientOpts...)
	if err != nil {
		fe := asFortiError(fortivpn.CodeInvalidConfig, err)
		s.emit(fortivpn.Event{Phase: fortivpn.PhaseDisconnected, Err: fe})
		return fe
	}

	s.emit(fortivpn.Event{Phase: fortivpn.PhaseConnecting})

	var (
		cookie        string
		reusingCookie bool
		everConnected bool
		attempt       int
	)

	const (
		dialTimeout       = 15 * time.Second
		authTimeout       = 30 * time.Second
		promptTimeout     = 2 * time.Minute
		handshakeTimeout  = 15 * time.Second
		disconnectTimeout = 3 * time.Second
	)

	for {
		if ctx.Err() != nil {
			s.teardown(client, cookie)
			s.emit(fortivpn.Event{Phase: fortivpn.PhaseDisconnected})
			return nil
		}

		if cookie == "" {
			s.emit(fortivpn.Event{Phase: fortivpn.PhaseAuthenticating})

			probeCtx, probeCancel := context.WithTimeout(ctx, dialTimeout)
			err = client.Probe(probeCtx)
			probeCancel()
			if err != nil {
				if retry := s.shouldRetry(ctx, err, everConnected, &attempt); retry {
					continue
				}
				return s.failAndCleanup(client, cookie, err)
			}
			s.log.Info("Connected to gateway.")

			authCtx, authCancel := context.WithTimeout(ctx, authTimeout+promptTimeout)
			cookie, err = client.Login(authCtx, func(pCtx context.Context, p fortivpn.Prompt) (string, error) {
				return s.promptUser(ctx, p)
			})
			authCancel()
			if err != nil {
				if retry := s.shouldRetry(ctx, err, everConnected, &attempt); retry {
					continue
				}
				return s.failAndCleanup(client, cookie, err)
			}
			reusingCookie = false
			s.log.Info("Authenticated.")
		}

		s.emit(fortivpn.Event{Phase: fortivpn.PhaseNegotiating})

		xmlCtx, xmlCancel := context.WithTimeout(ctx, authTimeout)
		tunCfg, err := client.FetchTunnelConfig(xmlCtx, cookie)
		xmlCancel()
		if err != nil {
			if reusingCookie && (fortivpn.IsCode(err, fortivpn.CodeTunnelDenied) || fortivpn.IsCode(err, fortivpn.CodeAuthFailed)) {
				cookie = ""
				reusingCookie = false
				continue
			}
			if retry := s.shouldRetry(ctx, err, everConnected, &attempt); retry {
				continue
			}
			return s.failAndCleanup(client, cookie, err)
		}
		s.log.Info("Remote gateway has allocated a VPN.")

		dialCtx, dialCancel := context.WithTimeout(ctx, dialTimeout)
		tconn, err := client.OpenTunnel(dialCtx, cookie)
		dialCancel()
		if err != nil {
			if retry := s.shouldRetry(ctx, err, everConnected, &attempt); retry {
				continue
			}
			return s.failAndCleanup(client, cookie, err)
		}

		lnk := newLink(s, tconn, tunCfg.AssignedIP)
		lnk.start()

		neg, err := lnk.waitNegotiated(ctx, handshakeTimeout)
		if err != nil {
			lnk.close()
			if reusingCookie && (fortivpn.IsCode(err, fortivpn.CodeTunnelDenied) || fortivpn.IsCode(err, fortivpn.CodeAuthFailed)) {
				cookie = ""
				reusingCookie = false
				continue
			}
			if retry := s.shouldRetry(ctx, err, everConnected, &attempt); retry {
				continue
			}
			return s.failAndCleanup(client, cookie, err)
		}

		s.log.Info(fmt.Sprintf("Got addresses: [%s], ns [%s, %s]", neg.ipCfg.LocalIP, neg.ns1, neg.ns2))
		s.log.Info("Negotiation complete.")

		if s.checkOnly {
			_, info, buildErr := s.buildDesiredAndInfo(cfg, client.GatewayIP(), tunCfg, neg, "check0")
			if buildErr != nil {
				lnk.close()
				return s.failAndCleanup(client, cookie, buildErr)
			}
			s.lastInfo = info
			lnk.gracefulClose(disconnectTimeout)
			s.teardown(client, cookie)
			s.emit(fortivpn.Event{Phase: fortivpn.PhaseDisconnected})
			return nil
		}

		s.emit(fortivpn.Event{Phase: fortivpn.PhaseConfiguring})

		if err := s.ensureDevice(neg.mru); err != nil {
			lnk.close()
			return s.failAndCleanup(client, cookie, err)
		}

		desired, info, err := s.buildDesiredAndInfo(cfg, client.GatewayIP(), tunCfg, neg, s.dev.Name())
		if err != nil {
			lnk.close()
			return s.failAndCleanup(client, cookie, err)
		}

		if err := s.applyOrUpdateNetcfg(ctx, desired); err != nil {
			lnk.close()
			return s.failAndCleanup(client, cookie, err)
		}

		s.lastInfo = info
		everConnected = true
		attempt = 0
		s.activeLink.Store(lnk)

		s.emit(fortivpn.Event{
			Phase: fortivpn.PhaseConnected,
			Info:  &info,
		})
		s.log.Info("Tunnel is up and running.")

		linkErr := lnk.runConnectedLoops(ctx)
		s.activeLink.CompareAndSwap(lnk, nil)

		if ctx.Err() != nil {
			s.emit(fortivpn.Event{Phase: fortivpn.PhaseDisconnecting})
			s.log.Info("Cancelling threads...")
			lnk.gracefulClose(disconnectTimeout)
			s.teardown(client, cookie)
			s.emit(fortivpn.Event{Phase: fortivpn.PhaseDisconnected})
			return nil
		}

		lnk.close()
		reusingCookie = cookie != ""
		if retry := s.shouldRetry(ctx, linkErr, everConnected, &attempt); retry {
			continue
		}
		return s.failAndCleanup(client, cookie, linkErr)
	}
}

func (s *session) shouldRetry(ctx context.Context, err error, everConnected bool, attempt *int) bool {
	if ctx.Err() != nil || s.checkOnly || !s.cfg.Reconnect.Enabled || !everConnected {
		return false
	}
	fe, ok := fortivpn.AsError(err)
	if !ok || !fe.Retryable() {
		return false
	}
	if s.cfg.Reconnect.MaxAttempts > 0 && *attempt >= s.cfg.Reconnect.MaxAttempts {
		return false
	}
	*attempt++
	s.emit(fortivpn.Event{
		Phase: fortivpn.PhaseReconnecting,
		Err:   fe,
	})

	delay := s.cfg.Reconnect.Delay
	if delay <= 0 {
		delay = 5 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *session) promptUser(ctx context.Context, p fortivpn.Prompt) (string, error) {
	if s.handler == nil {
		return "", fortivpn.NewError(fortivpn.CodeOTPRequired, p.Message)
	}
	s.emit(fortivpn.Event{
		Phase:  fortivpn.PhaseWaitingInput,
		Prompt: &p,
	})
	defer s.emit(fortivpn.Event{
		Phase: fortivpn.PhaseAuthenticating,
	})

	promptCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return s.handler.Prompt(promptCtx, p)
}

func (s *session) ensureDevice(mru int) error {
	s.mu.Lock()
	if s.dev != nil {
		dev := s.dev
		s.mu.Unlock()
		if setter, ok := dev.(interface{ SetMTU(int) }); ok && mru > 0 {
			setter.SetMTU(mru)
		}
		return nil
	}
	var (
		dev Device
		err error
	)
	if s.opts.device != nil {
		dev = s.opts.device
	} else {
		dev, err = netdev.Open(mru)
		if err != nil {
			s.mu.Unlock()
			return err
		}
	}
	s.dev = dev
	s.mu.Unlock()

	s.log.Info(fmt.Sprintf("Interface %s is UP.", dev.Name()))

	s.devWg.Add(1)
	go func() {
		defer s.devWg.Done()
		s.deviceReaderLoop(dev)
	}()
	return nil
}

func (s *session) deviceReaderLoop(dev Device) {
	buf := make([]byte, 65535)
	for {
		n, err := dev.ReadPacket(buf)
		if err != nil {
			return
		}
		if n <= 0 {
			continue
		}
		if lnk := s.activeLink.Load(); lnk != nil {
			lnk.enqueueData(buf[:n])
		}
	}
}

func (s *session) getDevice() Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dev
}

func (s *session) applyOrUpdateNetcfg(ctx context.Context, desired netcfg.Desired) error {
	if s.opts.device != nil {
		// Injected device mode (e.g. fortitest.MemDevice): do not touch host routes/DNS.
		return nil
	}

	s.mu.Lock()
	applied := s.applied
	s.mu.Unlock()

	if applied == nil {
		s.log.Info("Setting new routes...")
		if desired.DNS.Mode != fortivpn.DNSOff && len(desired.DNS.Servers) > 0 && !netcfg.SplitDNSNeedsDomains(desired.DNS) {
			s.log.Info("Adding VPN nameservers...")
		}
		ap, err := netcfg.Apply(ctx, desired, s.opts.journalPath)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.applied = ap
		s.mu.Unlock()
		return nil
	}

	return applied.Update(ctx, desired)
}

func (s *session) buildDesiredAndInfo(
	cfg fortivpn.Config,
	gatewayIP netip.Addr,
	tunCfg sslvpn.TunnelConfig,
	neg negotiatedPPP,
	ifaceName string,
) (netcfg.Desired, fortivpn.TunnelInfo, error) {
	localIP := neg.ipCfg.LocalIP
	if !localIP.IsValid() || localIP.IsUnspecified() {
		localIP = tunCfg.AssignedIP
	}
	peerIP := neg.ipCfg.PeerIP
	if !peerIP.IsValid() || peerIP.IsUnspecified() || peerIP == gatewayIP || peerIP == localIP {
		peerIP = netip.MustParseAddr("169.254.2.1")
	}

	excludes := append([]netip.Prefix(nil), tunCfg.ExcludeRoutes...)

	routes, _, err := netcfg.SanitizeRoutes(
		cfg.Routes,
		tunCfg.SplitRoutes,
		cfg.ExtraRoutes,
		excludes,
		gatewayIP,
	)
	if err != nil {
		return netcfg.Desired{}, fortivpn.TunnelInfo{}, err
	}

	dnsServers := mergeAddrs(neg.ipCfg.DNS, tunCfg.DNS)
	dnsDomains := mergeDomains(tunCfg.DNSDomains, cfg.DNSDomains)

	dnsPlan := netcfg.DNSPlan{
		Mode:    cfg.DNS,
		Servers: dnsServers,
		Domains: dnsDomains,
	}
	splitNeedsDomains := netcfg.SplitDNSNeedsDomains(dnsPlan)

	desired := netcfg.Desired{
		Iface:    ifaceName,
		Local:    localIP,
		Peer:     peerIP,
		MTU:      neg.mru,
		Routes:   routes,
		Excludes: excludes,
		Gateway:  gatewayIP,
		DNS:      dnsPlan,
	}

	routeStrs := make([]string, len(routes))
	for i, r := range routes {
		routeStrs[i] = r.String()
	}
	dnsServerStrs := make([]string, len(dnsServers))
	for i, a := range dnsServers {
		dnsServerStrs[i] = a.String()
	}

	var gwIPStr string
	if gatewayIP.IsValid() {
		gwIPStr = gatewayIP.String()
	}

	info := fortivpn.TunnelInfo{
		Interface:            ifaceName,
		LocalIP:              localIP.String(),
		PeerIP:               peerIP.String(),
		MTU:                  neg.mru,
		RouteCount:           len(routes),
		Routes:               routeStrs,
		DNSMode:              cfg.DNS,
		DNSServers:           dnsServerStrs,
		DNSDomains:           dnsDomains,
		SplitDNSNeedsDomains: splitNeedsDomains,
		GatewayHost:          cfg.Host,
		GatewayIP:            gwIPStr,
		FortiOS:              tunCfg.FortiOS,
	}
	return desired, info, nil
}

func (s *session) failAndCleanup(client *sslvpn.Client, cookie string, err error) error {
	if errors.Is(err, context.Canceled) {
		s.log.Info("Session cancelled.")
		s.teardown(client, cookie)
		s.emit(fortivpn.Event{Phase: fortivpn.PhaseDisconnected})
		return nil
	}
	fe := asFortiError(fortivpn.CodeGatewayUnreachable, err)
	s.log.Error("Session failed.", "code", fe.Code, "error", fe.Error())
	s.teardown(client, cookie)
	s.emit(fortivpn.Event{
		Phase: fortivpn.PhaseDisconnected,
		Err:   fe,
	})
	return fe
}

func (s *session) teardown(client *sslvpn.Client, cookie string) {
	s.activeLink.Store(nil)

	s.mu.Lock()
	applied := s.applied
	s.applied = nil
	dev := s.dev
	s.dev = nil
	s.mu.Unlock()

	if applied != nil {
		s.log.Info("Restoring routes...")
		_ = applied.Close(context.Background())
	}

	if dev != nil {
		_ = dev.Close()
		s.devWg.Wait()
	}

	if cookie != "" && client != nil {
		_ = client.Logout(context.Background(), cookie)
		s.log.Info("Logged out.")
	}
}

func (s *session) snapshotStats() fortivpn.Stats {
	return fortivpn.Stats{
		RxBytes:   s.rxBytes.Load(),
		TxBytes:   s.txBytes.Load(),
		RxPackets: s.rxPackets.Load(),
		TxPackets: s.txPackets.Load(),
		RTTMillis: atomic.LoadInt64(&s.rttMillis),
	}
}

func (s *session) emit(ev fortivpn.Event) {
	if s.handler != nil {
		s.handler.OnEvent(ev)
	}
}

func asFortiError(fallback fortivpn.Code, err error) *fortivpn.Error {
	if fe, ok := fortivpn.AsError(err); ok {
		return fe
	}
	if err == nil {
		return fortivpn.NewError(fallback, "")
	}
	return fortivpn.WrapError(fallback, err.Error(), err)
}

func mergeAddrs(primary, secondary []netip.Addr) []netip.Addr {
	var out []netip.Addr
	for _, list := range [][]netip.Addr{primary, secondary} {
		for _, a := range list {
			if !a.IsValid() || a.IsUnspecified() {
				continue
			}
			if !slices.Contains(out, a) {
				out = append(out, a)
			}
		}
	}
	return out
}

func mergeDomains(primary, secondary []string) []string {
	var out []string
	for _, list := range [][]string{primary, secondary} {
		for _, d := range list {
			cleaned := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(d, ".")))
			if cleaned != "" && !slices.Contains(out, cleaned) {
				out = append(out, cleaned)
			}
		}
	}
	return out
}
