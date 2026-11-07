package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/internal/netcfg"
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
			return s.failAndCleanup(client, cookie, nil)
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
			return s.failAndCleanup(client, cookie, nil)
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
			return s.failAndCleanup(client, cookie, nil)
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

func (s *session) failAndCleanup(client *sslvpn.Client, cookie string, cause error) error {
	if errors.Is(cause, context.Canceled) {
		s.log.Info("Session cancelled.")
		cause = nil
	}
	if cleanupErr := s.teardown(client, cookie); cleanupErr != nil {
		cause = fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "Network cleanup incomplete", errors.Join(cause, cleanupErr))
	}
	if cause == nil {
		s.emit(fortivpn.Event{Phase: fortivpn.PhaseDisconnected})
		return nil
	}
	fe := asFortiError(fortivpn.CodeGatewayUnreachable, cause)
	s.log.Error("Session failed.", "code", fe.Code, "error", fe.Error())
	s.emit(fortivpn.Event{Phase: fortivpn.PhaseDisconnected, Err: fe})
	return fe
}

func (s *session) teardown(client *sslvpn.Client, cookie string) error {
	var cleanupErrors []error
	s.activeLink.Store(nil)

	s.mu.Lock()
	applied := s.applied
	s.applied = nil
	dev := s.dev
	s.dev = nil
	s.mu.Unlock()

	if applied != nil {
		s.log.Info("Restoring routes...")
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		if err := applied.Close(cleanupCtx); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
		cancel()
	}

	if dev != nil {
		if err := dev.Close(); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
		s.devWg.Wait()
	}

	if cookie != "" && client != nil {
		logoutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := client.Logout(logoutCtx, cookie); err == nil {
			s.log.Info("Logged out.")
		} else {
			s.log.Warn("Gateway logout did not complete.")
		}
		cancel()
	}
	return errors.Join(cleanupErrors...)
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
