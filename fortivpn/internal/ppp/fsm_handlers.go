package ppp

import (
	"fmt"
	"time"
)

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
