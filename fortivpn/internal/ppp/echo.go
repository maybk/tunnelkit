package ppp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrLinkClosed reports an operation on a closed PPP session.
var ErrLinkClosed = errors.New("ppp: link is closed")

// Events receives lifecycle callbacks from a PPP Session.
type Events struct {
	Send      func(frame []byte)
	LCPUp     func(mru int)
	NetworkUp func(cfg IPConfig, mru int, ns1, ns2 string)
	Closed    func(err error)
}

// Session orchestrates LCP, IPCP, Echo keepalive probes, and graceful termination
// without performing direct network I/O.
type Session struct {
	ev  Events
	cfg FSMConfig

	mu      sync.Mutex
	lcpH    *LCPHandler
	ipcpH   *IPCPHandler
	lcp     *FSM
	ipcp    *FSM
	closed  bool
	upFired bool

	echoSeq  byte
	echoID   byte
	echoSent time.Time
	echoWait chan time.Duration

	termWait chan struct{}
}

// NewSession constructs a PPP client session.
func NewSession(ev Events, cfg FSMConfig) *Session {
	s := &Session{
		ev:       ev,
		cfg:      cfg,
		termWait: make(chan struct{}),
	}
	s.lcpH = NewLCPHandler(DefaultMRU, s.onEchoReplyLocked)
	s.ipcpH = NewIPCPHandler()

	s.lcp = NewFSM(s.lcpH, FSMCallbacks{
		Send:              s.sendProtocolLocked,
		ThisLayerUp:       s.onLCPUpLocked,
		ThisLayerDown:     s.onLCPDownLocked,
		ThisLayerFinished: s.onLCPFinishedLocked,
	}, cfg)

	s.ipcp = NewFSM(s.ipcpH, FSMCallbacks{
		Send:              s.sendProtocolLocked,
		ThisLayerUp:       s.onIPCPUpLocked,
		ThisLayerDown:     func() {},
		ThisLayerFinished: s.onIPCPFinishedLocked,
	}, cfg)

	return s
}

// Start opens LCP and sends the initial LCP Configure-Request.
func (s *Session) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.lcp.Open(s.scheduleLCP)
	s.lcp.Up(s.scheduleLCP)
	s.ipcp.Open(s.scheduleIPCP)
}

// Input processes one inbound PPP frame (excluding the 6-byte Fortinet header).
func (s *Session) Input(frame []byte) {
	proto, payload, ok := DecodeFrame(frame)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}

	switch proto {
	case ProtocolLCP:
		s.lcp.Input(payload, s.scheduleLCP)
	case ProtocolIPCP:
		if s.lcp.State() == StateOpened {
			s.ipcp.Input(payload, s.scheduleIPCP)
		}
	case ProtocolIP:
		// Handled by caller on the fast path via ExtractIPv4.
	default:
		// Reject unsupported network protocols (e.g. CCP 0x80FD, IPv6CP 0x8057)
		// with LCP Protocol-Reject (RFC 1661 §5.7) when LCP is Opened.
		if s.lcp.State() == StateOpened {
			body := make([]byte, 2+len(payload))
			binary.BigEndian.PutUint16(body[0:2], proto)
			copy(body[2:], payload)
			mru := s.lcpH.NegotiatedMRU()
			if 4+len(body) > mru {
				body = body[:mru-4]
			}
			s.echoSeq++
			pkt := Packet{Code: CodeProtocolReject, ID: s.echoSeq, Body: body}
			s.sendProtocolLocked(ProtocolLCP, pkt.Marshal())
		}
	}
}

// SendEcho sends a single LCP Echo-Request and waits for the matching Echo-Reply,
// returning the measured round-trip time.
func (s *Session) SendEcho(ctx context.Context) (time.Duration, error) {
	s.mu.Lock()
	if s.closed || s.lcp.State() != StateOpened {
		s.mu.Unlock()
		return 0, ErrLinkClosed
	}
	wait := s.echoWait
	if wait == nil {
		wait = make(chan time.Duration, 1)
		s.echoSeq++
		s.echoID = s.echoSeq
		s.echoSent = time.Now()
		s.echoWait = wait
		var magic [4]byte
		binary.BigEndian.PutUint32(magic[:], s.lcpH.LocalMagic())
		pkt := Packet{Code: CodeEchoRequest, ID: s.echoID, Body: magic[:]}
		s.sendProtocolLocked(ProtocolLCP, pkt.Marshal())
	}
	s.mu.Unlock()

	select {
	case rtt, ok := <-wait:
		if !ok {
			return 0, ErrLinkClosed
		}
		return rtt, nil
	case <-ctx.Done():
		s.mu.Lock()
		if s.echoWait == wait {
			s.echoWait = nil
		}
		s.mu.Unlock()
		return 0, ctx.Err()
	}
}

// Terminate sends an LCP Terminate-Request and waits up to ctx deadline for
// the peer's Terminate-Ack.
func (s *Session) Terminate(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.ipcp.Down()
	s.lcp.Close(s.scheduleLCP)
	termWait := s.termWait
	s.mu.Unlock()

	select {
	case <-termWait:
		return nil
	case <-ctx.Done():
		s.Close()
		return ctx.Err()
	}
}

// Close immediately stops timers and marks the session closed.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finishLocked(nil)
}

func (s *Session) sendProtocolLocked(protocol uint16, payload []byte) {
	if s.ev.Send != nil {
		s.ev.Send(EncodeFrame(protocol, payload))
	}
}

func (s *Session) onLCPUpLocked() {
	if s.ev.LCPUp != nil {
		s.ev.LCPUp(s.lcpH.NegotiatedMRU())
	}
	s.ipcp.Up(s.scheduleIPCP)
}

func (s *Session) onLCPDownLocked() {
	s.ipcp.Down()
}

func (s *Session) onLCPFinishedLocked(err error) {
	s.finishLocked(err)
}

func (s *Session) onIPCPUpLocked() {
	cfg := s.ipcpH.Config()
	if !cfg.LocalIP.IsValid() || cfg.LocalIP.IsUnspecified() {
		s.finishLocked(fmt.Errorf("IPCP completed without a valid local IPv4 address"))
		return
	}
	s.upFired = true
	if s.ev.NetworkUp != nil {
		ns1, ns2 := s.ipcpH.RawDNSStrings()
		s.ev.NetworkUp(cfg, s.lcpH.NegotiatedMRU(), ns1, ns2)
	}
}

func (s *Session) onIPCPFinishedLocked(err error) {
	if err != nil {
		s.finishLocked(err)
	}
}

func (s *Session) onEchoReplyLocked(id byte) {
	if s.echoWait != nil && id == s.echoID {
		rtt := time.Since(s.echoSent)
		s.echoWait <- rtt
		close(s.echoWait)
		s.echoWait = nil
	}
}

func (s *Session) finishLocked(err error) {
	if s.closed {
		return
	}
	s.closed = true
	s.lcp.StopTimer()
	s.ipcp.StopTimer()
	if s.echoWait != nil {
		close(s.echoWait)
		s.echoWait = nil
	}
	select {
	case <-s.termWait:
	default:
		close(s.termWait)
	}
	if s.ev.Closed != nil {
		s.ev.Closed(err)
	}
}

func (s *Session) scheduleLCP(gen uint64, d time.Duration) {
	s.lcp.timer = time.AfterFunc(d, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed {
			return
		}
		s.lcp.Timeout(gen, s.scheduleLCP)
	})
}

func (s *Session) scheduleIPCP(gen uint64, d time.Duration) {
	s.ipcp.timer = time.AfterFunc(d, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed {
			return
		}
		s.ipcp.Timeout(gen, s.scheduleIPCP)
	})
}
