package ppp

import (
	"fmt"
	"net/netip"
	"time"
)

// State represents one of the 10 states of the RFC 1661 §4.2 automaton.
type State int

const (
	StateInitial  State = 0
	StateStarting State = 1
	StateClosed   State = 2
	StateStopped  State = 3
	StateClosing  State = 4
	StateStopping State = 5
	StateReqSent  State = 6
	StateAckRcvd  State = 7
	StateAckSent  State = 8
	StateOpened   State = 9
)

func (s State) String() string {
	switch s {
	case StateInitial:
		return "Initial"
	case StateStarting:
		return "Starting"
	case StateClosed:
		return "Closed"
	case StateStopped:
		return "Stopped"
	case StateClosing:
		return "Closing"
	case StateStopping:
		return "Stopping"
	case StateReqSent:
		return "Req-Sent"
	case StateAckRcvd:
		return "Ack-Rcvd"
	case StateAckSent:
		return "Ack-Sent"
	case StateOpened:
		return "Opened"
	default:
		return fmt.Sprintf("State(%d)", int(s))
	}
}

// PeerEvaluation is the verdict of evaluating an incoming Configure-Request.
type PeerEvaluation int

const (
	PeerAck PeerEvaluation = iota
	PeerNak
	PeerReject
)

// OptionHandler specializes the generic RFC 1661 FSM for LCP or IPCP.
type OptionHandler interface {
	Protocol() uint16
	Name() string
	ResetOptions()
	BuildRequestOptions() []Option
	ApplyNak(opts []Option) error
	ApplyReject(opts []Option) error
	EvaluatePeerRequest(opts []Option) (verdict PeerEvaluation, responseOpts []Option, err error)
	HandleExtraCode(pkt Packet, send func(Packet)) (handled bool, err error)
}

// FSMCallbacks receives notifications from the RFC 1661 state machine.
type FSMCallbacks struct {
	Send              func(protocol uint16, payload []byte)
	ThisLayerUp       func()
	ThisLayerDown     func()
	ThisLayerFinished func(err error)
}

// FSMConfig tunes timers and retry budgets of an FSM.
type FSMConfig struct {
	RestartInterval time.Duration
	MaxConfigure    int
	MaxTerminate    int
	MaxFailure      int
	FallbackLocalIP netip.Addr
}

// DefaultFSMConfig returns the RFC 1661 §4.6 default parameters.
func DefaultFSMConfig() FSMConfig {
	return FSMConfig{
		RestartInterval: 3 * time.Second,
		MaxConfigure:    10,
		MaxTerminate:    2,
		MaxFailure:      5,
	}
}

// FSM implements the RFC 1661 §4 state table for a single control protocol.
// Callers must serialize calls to FSM methods (e.g. under Session's mutex).
type FSM struct {
	h     OptionHandler
	cb    FSMCallbacks
	cfg   FSMConfig
	state State

	reqID        byte
	lastReqID    byte
	lastTermID   byte
	restartCount int
	failureCount int
	nakCount     int
	timerGen     uint64
	timer        *time.Timer
}

// NewFSM constructs an FSM in StateInitial.
func NewFSM(h OptionHandler, cb FSMCallbacks, cfg FSMConfig) *FSM {
	if cfg.RestartInterval <= 0 {
		cfg.RestartInterval = 3 * time.Second
	}
	if cfg.MaxConfigure <= 0 {
		cfg.MaxConfigure = 10
	}
	if cfg.MaxTerminate <= 0 {
		cfg.MaxTerminate = 2
	}
	if cfg.MaxFailure <= 0 {
		cfg.MaxFailure = 5
	}
	return &FSM{
		h:     h,
		cb:    cb,
		cfg:   cfg,
		state: StateInitial,
	}
}

// State returns the current automaton state.
func (f *FSM) State() State { return f.state }

// TimerGeneration returns the current timer generation token for external timer scheduling.
func (f *FSM) TimerGeneration() uint64 { return f.timerGen }

// StopTimer disarms any pending restart timer.
func (f *FSM) StopTimer() {
	f.timerGen++
	if f.timer != nil {
		f.timer.Stop()
		f.timer = nil
	}
}

// ArmTimer schedules onExpire to be called after RestartInterval if the timer generation matches.
func (f *FSM) ArmTimer(schedule func(gen uint64, d time.Duration)) {
	f.StopTimer()
	gen := f.timerGen
	if schedule != nil {
		schedule(gen, f.cfg.RestartInterval)
	}
}

// Up signals that the lower layer is ready (RFC 1661 Up event).
func (f *FSM) Up(schedule func(gen uint64, d time.Duration)) {
	switch f.state {
	case StateInitial:
		f.state = StateClosed
	case StateStarting:
		f.h.ResetOptions()
		f.restartCount = f.cfg.MaxConfigure
		f.failureCount = 0
		f.nakCount = 0
		f.sendConfigureRequest(true)
		f.state = StateReqSent
		f.ArmTimer(schedule)
	}
}

// Down signals that the lower layer went down (RFC 1661 Down event).
func (f *FSM) Down() {
	f.StopTimer()
	switch f.state {
	case StateClosed, StateClosing:
		f.state = StateInitial
	case StateStopped:
		f.state = StateStarting
	case StateStopping, StateReqSent, StateAckRcvd, StateAckSent:
		f.state = StateStarting
	case StateOpened:
		if f.cb.ThisLayerDown != nil {
			f.cb.ThisLayerDown()
		}
		f.state = StateStarting
	}
}

// Open initiates administrative open of the protocol (RFC 1661 Open event).
func (f *FSM) Open(schedule func(gen uint64, d time.Duration)) {
	switch f.state {
	case StateInitial:
		f.state = StateStarting
	case StateClosed:
		f.h.ResetOptions()
		f.restartCount = f.cfg.MaxConfigure
		f.failureCount = 0
		f.nakCount = 0
		f.sendConfigureRequest(true)
		f.state = StateReqSent
		f.ArmTimer(schedule)
	case StateClosing:
		f.state = StateStopping
	}
}

// Close initiates administrative close of the protocol (RFC 1661 Close event).
func (f *FSM) Close(schedule func(gen uint64, d time.Duration)) {
	switch f.state {
	case StateStarting:
		f.state = StateInitial
		if f.cb.ThisLayerFinished != nil {
			f.cb.ThisLayerFinished(nil)
		}
	case StateStopped:
		f.state = StateClosed
	case StateStopping:
		f.state = StateClosing
	case StateOpened:
		if f.cb.ThisLayerDown != nil {
			f.cb.ThisLayerDown()
		}
		f.restartCount = f.cfg.MaxTerminate
		f.sendTerminateRequest()
		f.state = StateClosing
		f.ArmTimer(schedule)
	case StateReqSent, StateAckRcvd, StateAckSent:
		f.restartCount = f.cfg.MaxTerminate
		f.sendTerminateRequest()
		f.state = StateClosing
		f.ArmTimer(schedule)
	}
}

// Timeout handles the RFC 1661 TO+ / TO- events when the Restart timer fires.
func (f *FSM) Timeout(gen uint64, schedule func(gen uint64, d time.Duration)) {
	if gen != f.timerGen {
		return
	}
	switch f.state {
	case StateClosing, StateStopping:
		f.restartCount--
		if f.restartCount <= 0 {
			f.StopTimer()
			prev := f.state
			if prev == StateClosing {
				f.state = StateClosed
			} else {
				f.state = StateStopped
			}
			if f.cb.ThisLayerFinished != nil {
				f.cb.ThisLayerFinished(nil)
			}
			return
		}
		f.resendTerminateRequest()
		f.ArmTimer(schedule)

	case StateReqSent, StateAckRcvd, StateAckSent:
		f.restartCount--
		if f.restartCount <= 0 {
			f.StopTimer()
			f.state = StateStopped
			if f.cb.ThisLayerFinished != nil {
				f.cb.ThisLayerFinished(fmt.Errorf("%s negotiation timed out after %d attempts", f.h.Name(), f.cfg.MaxConfigure))
			}
			return
		}
		f.sendConfigureRequest(false)
		if f.state == StateAckRcvd {
			f.state = StateReqSent
		}
		f.ArmTimer(schedule)
	}
}

// Input processes an incoming control-protocol payload.
func (f *FSM) Input(payload []byte, schedule func(gen uint64, d time.Duration)) {
	pkt, ok := ParsePacket(payload)
	if !ok {
		return
	}
	if f.state == StateInitial || f.state == StateStarting {
		return
	}

	switch pkt.Code {
	case CodeConfigureRequest:
		f.handleConfigureRequest(pkt, schedule)
	case CodeConfigureAck:
		f.handleConfigureAck(pkt, schedule)
	case CodeConfigureNak, CodeConfigureReject:
		f.handleConfigureNakOrReject(pkt, schedule)
	case CodeTerminateRequest:
		f.handleTerminateRequest(pkt, schedule)
	case CodeTerminateAck:
		f.handleTerminateAck(pkt, schedule)
	case CodeCodeReject, CodeProtocolReject:
		if _, err := f.h.HandleExtraCode(pkt, f.sendPacket); err != nil {
			f.StopTimer()
			f.state = StateStopped
			if f.cb.ThisLayerFinished != nil {
				f.cb.ThisLayerFinished(err)
			}
		}
	default:
		handled, err := f.h.HandleExtraCode(pkt, f.sendPacket)
		if err != nil {
			f.StopTimer()
			f.state = StateStopped
			if f.cb.ThisLayerFinished != nil {
				f.cb.ThisLayerFinished(err)
			}
			return
		}
		if !handled {
			// Unknown code -> send Code-Reject (RFC 1661 §5.6).
			f.sendPacket(Packet{Code: CodeCodeReject, ID: f.nextID(), Body: payload})
		}
	}
}

func (f *FSM) handleConfigureRequest(pkt Packet, schedule func(gen uint64, d time.Duration)) {
	switch f.state {
	case StateClosed:
		f.sendPacket(Packet{Code: CodeTerminateAck, ID: pkt.ID})
		return
	case StateClosing, StateStopping:
		return
	}

	opts, ok := ParseOptions(pkt.Body)
	if !ok {
		return
	}

	verdict, respOpts, err := f.h.EvaluatePeerRequest(opts)
	if err != nil {
		f.StopTimer()
		f.state = StateStopped
		if f.cb.ThisLayerFinished != nil {
			f.cb.ThisLayerFinished(err)
		}
		return
	}

	if f.state == StateOpened {
		if f.cb.ThisLayerDown != nil {
			f.cb.ThisLayerDown()
		}
		f.restartCount = f.cfg.MaxConfigure
		f.sendConfigureRequest(true)
	} else if f.state == StateStopped {
		f.restartCount = f.cfg.MaxConfigure
		f.sendConfigureRequest(true)
	}

	switch verdict {
	case PeerAck:
		f.sendPacket(Packet{Code: CodeConfigureAck, ID: pkt.ID, Body: pkt.Body})
		switch f.state {
		case StateStopped, StateReqSent, StateOpened:
			f.state = StateAckSent
			f.ArmTimer(schedule)
		case StateAckRcvd:
			f.StopTimer()
			f.state = StateOpened
			if f.cb.ThisLayerUp != nil {
				f.cb.ThisLayerUp()
			}
		}
	case PeerNak, PeerReject:
		code := CodeConfigureNak
		if verdict == PeerReject {
			code = CodeConfigureReject
		}
		if verdict == PeerNak {
			f.failureCount++
			if f.failureCount >= f.cfg.MaxFailure {
				code = CodeConfigureReject
			}
		}
		f.sendPacket(Packet{Code: code, ID: pkt.ID, Body: MarshalOptions(respOpts)})
		switch f.state {
		case StateStopped, StateOpened:
			f.state = StateReqSent
			f.ArmTimer(schedule)
		case StateAckSent:
			f.state = StateReqSent
		}
	}
}

func (f *FSM) handleConfigureAck(pkt Packet, schedule func(gen uint64, d time.Duration)) {
	if pkt.ID != f.lastReqID {
		return
	}
	switch f.state {
	case StateClosed, StateStopped:
		f.sendPacket(Packet{Code: CodeTerminateAck, ID: pkt.ID})
	case StateReqSent:
		f.restartCount = f.cfg.MaxConfigure
		f.nakCount = 0
		f.state = StateAckRcvd
		f.ArmTimer(schedule)
	case StateAckRcvd:
		f.sendConfigureRequest(true)
		f.state = StateReqSent
		f.ArmTimer(schedule)
	case StateAckSent:
		f.StopTimer()
		f.restartCount = f.cfg.MaxConfigure
		f.nakCount = 0
		f.state = StateOpened
		if f.cb.ThisLayerUp != nil {
			f.cb.ThisLayerUp()
		}
	case StateOpened:
		if f.cb.ThisLayerDown != nil {
			f.cb.ThisLayerDown()
		}
		f.sendConfigureRequest(true)
		f.state = StateReqSent
		f.ArmTimer(schedule)
	}
}

func (f *FSM) handleConfigureNakOrReject(pkt Packet, schedule func(gen uint64, d time.Duration)) {
	if pkt.ID != f.lastReqID {
		return
	}
	switch f.state {
	case StateClosed, StateStopped:
		f.sendPacket(Packet{Code: CodeTerminateAck, ID: pkt.ID})
		return
	case StateClosing, StateStopping:
		return
	}

	opts, ok := ParseOptions(pkt.Body)
	if !ok {
		return
	}
	f.nakCount++
	if f.nakCount > f.cfg.MaxConfigure {
		f.StopTimer()
		f.state = StateStopped
		if f.cb.ThisLayerFinished != nil {
			f.cb.ThisLayerFinished(fmt.Errorf("%s negotiation exceeded maximum %d Configure-Nak/Reject responses", f.h.Name(), f.cfg.MaxConfigure))
		}
		return
	}
	var err error
	if pkt.Code == CodeConfigureNak {
		err = f.h.ApplyNak(opts)
	} else {
		err = f.h.ApplyReject(opts)
	}
	if err != nil {
		f.StopTimer()
		f.state = StateStopped
		if f.cb.ThisLayerFinished != nil {
			f.cb.ThisLayerFinished(err)
		}
		return
	}

	switch f.state {
	case StateReqSent, StateAckSent:
		f.restartCount = f.cfg.MaxConfigure
		f.sendConfigureRequest(true)
		f.ArmTimer(schedule)
	case StateAckRcvd:
		f.sendConfigureRequest(true)
		f.state = StateReqSent
		f.ArmTimer(schedule)
	case StateOpened:
		if f.cb.ThisLayerDown != nil {
			f.cb.ThisLayerDown()
		}
		f.restartCount = f.cfg.MaxConfigure
		f.sendConfigureRequest(true)
		f.state = StateReqSent
		f.ArmTimer(schedule)
	}
}

func (f *FSM) handleTerminateRequest(pkt Packet, schedule func(gen uint64, d time.Duration)) {
	switch f.state {
	case StateClosed, StateStopped, StateClosing, StateStopping, StateReqSent, StateAckSent:
		f.sendPacket(Packet{Code: CodeTerminateAck, ID: pkt.ID})
		if f.state == StateReqSent || f.state == StateAckSent {
			f.StopTimer()
			f.state = StateStopped
			if f.cb.ThisLayerFinished != nil {
				f.cb.ThisLayerFinished(fmt.Errorf("%s terminated by peer", f.h.Name()))
			}
		}
	case StateAckRcvd:
		f.sendPacket(Packet{Code: CodeTerminateAck, ID: pkt.ID})
		f.StopTimer()
		f.state = StateStopped
		if f.cb.ThisLayerFinished != nil {
			f.cb.ThisLayerFinished(fmt.Errorf("%s terminated by peer", f.h.Name()))
		}
	case StateOpened:
		if f.cb.ThisLayerDown != nil {
			f.cb.ThisLayerDown()
		}
		f.sendPacket(Packet{Code: CodeTerminateAck, ID: pkt.ID})
		f.StopTimer()
		f.state = StateStopping
		f.restartCount = 0
		if f.cb.ThisLayerFinished != nil {
			f.cb.ThisLayerFinished(fmt.Errorf("%s terminated by peer", f.h.Name()))
		}
	}
}

func (f *FSM) handleTerminateAck(pkt Packet, schedule func(gen uint64, d time.Duration)) {
	switch f.state {
	case StateClosing:
		f.StopTimer()
		f.state = StateClosed
		if f.cb.ThisLayerFinished != nil {
			f.cb.ThisLayerFinished(nil)
		}
	case StateStopping:
		f.StopTimer()
		f.state = StateStopped
		if f.cb.ThisLayerFinished != nil {
			f.cb.ThisLayerFinished(nil)
		}
	case StateAckRcvd:
		f.state = StateReqSent
	case StateOpened:
		if f.cb.ThisLayerDown != nil {
			f.cb.ThisLayerDown()
		}
		f.sendConfigureRequest(true)
		f.state = StateReqSent
		f.ArmTimer(schedule)
	}
}

func (f *FSM) sendConfigureRequest(newID bool) {
	if newID {
		f.lastReqID = f.nextID()
	}
	opts := f.h.BuildRequestOptions()
	f.sendPacket(Packet{
		Code: CodeConfigureRequest,
		ID:   f.lastReqID,
		Body: MarshalOptions(opts),
	})
}

func (f *FSM) sendTerminateRequest() {
	f.lastTermID = f.nextID()
	f.sendPacket(Packet{
		Code: CodeTerminateRequest,
		ID:   f.lastTermID,
	})
}

func (f *FSM) resendTerminateRequest() {
	f.sendPacket(Packet{
		Code: CodeTerminateRequest,
		ID:   f.lastTermID,
	})
}

func (f *FSM) sendPacket(pkt Packet) {
	if f.cb.Send != nil {
		f.cb.Send(f.h.Protocol(), pkt.Marshal())
	}
}

func (f *FSM) nextID() byte {
	f.reqID++
	return f.reqID
}
