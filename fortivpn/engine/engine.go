// Package engine orchestrates the Fortinet SSL-VPN lifecycle: HTTPS control
// plane authentication, PPP negotiation, TUN device creation, host route/DNS
// configuration, data-plane packet pumping, LCP Echo keepalives, and automatic
// reconnection.
package engine

import (
	"context"
	"crypto/x509"
	"io"
	"log/slog"
	"net"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/internal/netcfg"
)

// Handler receives lifecycle events and answers interactive authentication prompts.
type Handler interface {
	// OnEvent is invoked synchronously on phase transitions, stats updates, and errors.
	// Implementations must not block.
	OnEvent(fortivpn.Event)

	// Prompt blocks until the user supplies a response (OTP code, FTM push confirmation,
	// or SAML completion) or ctx is canceled.
	Prompt(ctx context.Context, p fortivpn.Prompt) (string, error)
}

// Device abstracts an OS TUN adapter (utun on macOS, Wintun on Windows) or an
// in-memory test device (fortitest.MemDevice).
type Device interface {
	Name() string
	MTU() int
	ReadPacket(buf []byte) (int, error)
	WritePacket(pkt []byte) error
	Close() error
}

type options struct {
	logger          *slog.Logger
	journalPath     string
	device          Device
	dialer          *net.Dialer
	rootCAs         *x509.CertPool
	samlListenAddr  string
	echoInterval    time.Duration
	echoTimeout     time.Duration
	maxEchoFailures int
	statsInterval   time.Duration
	dumpConfigXML   func(raw []byte)
}

func defaultOptions() options {
	return options{
		logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		echoInterval:    10 * time.Second,
		echoTimeout:     5 * time.Second,
		maxEchoFailures: 3,
		statsInterval:   2 * time.Second,
	}
}

// Option customizes engine execution.
type Option func(*options)

// WithLogger sets the structured logger used by the engine.
func WithLogger(l *slog.Logger) Option {
	return func(o *options) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithJournal sets the file path used to persist write-ahead network rollback entries.
func WithJournal(path string) Option {
	return func(o *options) { o.journalPath = path }
}

// WithDevice injects a pre-opened TUN Device (such as fortitest.MemDevice).
// When set, the engine skips OS TUN creation and host network configuration.
func WithDevice(d Device) Option {
	return func(o *options) { o.device = d }
}

// WithDialer sets a custom underlying TCP dialer.
func WithDialer(d *net.Dialer) Option {
	return func(o *options) { o.dialer = d }
}

// WithRootCAs sets a custom X.509 root CA pool (primarily for testing).
func WithRootCAs(pool *x509.CertPool) Option {
	return func(o *options) { o.rootCAs = pool }
}

// WithSAMLListenAddr overrides the local HTTP callback address used during SAML login.
func WithSAMLListenAddr(addr string) Option {
	return func(o *options) { o.samlListenAddr = addr }
}

// WithEchoPolicy customizes the LCP Echo keepalive interval, per-probe timeout,
// and failure threshold.
func WithEchoPolicy(interval, timeout time.Duration, maxFailures int) Option {
	return func(o *options) {
		if interval > 0 {
			o.echoInterval = interval
		}
		if timeout > 0 {
			o.echoTimeout = timeout
		}
		if maxFailures > 0 {
			o.maxEchoFailures = maxFailures
		}
	}
}

// WithStatsInterval customizes how frequently Stats events are emitted while connected.
func WithStatsInterval(interval time.Duration) Option {
	return func(o *options) {
		if interval > 0 {
			o.statsInterval = interval
		}
	}
}

// WithConfigXMLHook registers a callback invoked with the raw fortisslvpn_xml response body.
func WithConfigXMLHook(fn func(raw []byte)) Option {
	return func(o *options) {
		o.dumpConfigXML = fn
	}
}

// Run establishes the VPN session and blocks until ctx is canceled (graceful
// disconnect) or an unrecoverable error occurs. Before returning, Run always
// executes teardown in reverse order: LCP Terminate -> /remote/logout ->
// host network rollback -> TUN device close.
func Run(ctx context.Context, cfg fortivpn.Config, h Handler, opts ...Option) error {
	o := defaultOptions()
	for _, fn := range opts {
		fn(&o)
	}
	s := newSession(cfg, h, o, false)
	return s.run(ctx)
}

// Check executes control-plane authentication, fetches tunnel XML config,
// upgrades to the TLS tunnel, completes LCP + IPCP negotiation, and immediately
// terminates PPP and logs out without opening a TUN device or mutating host routes.
func Check(ctx context.Context, cfg fortivpn.Config, h Handler, opts ...Option) (fortivpn.TunnelInfo, error) {
	o := defaultOptions()
	for _, fn := range opts {
		fn(&o)
	}
	s := newSession(cfg, h, o, true)
	err := s.run(ctx)
	return s.lastInfo, err
}

// Recover rolls back any residual host routes or DNS policies left behind in
// journalPath by a previously crashed process.
func Recover(ctx context.Context, journalPath string) error {
	return netcfg.Recover(ctx, journalPath)
}
