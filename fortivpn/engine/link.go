package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/internal/ppp"
	"tunnelkit/fortivpn/internal/sslvpn"
)

type negotiatedPPP struct {
	ipCfg ppp.IPConfig
	mru   int
	ns1   string
	ns2   string
}

type link struct {
	sess    *session
	tconn   *sslvpn.TunnelConn
	pppSess *ppp.Session

	ctrlCh    chan []byte
	dataCh    chan []byte
	doneCh    chan struct{}
	closeOnce sync.Once
	closing   atomic.Bool

	readyCh chan negotiatedPPP
	ioErrCh chan error
	ioWg    sync.WaitGroup
}

func newLink(sess *session, tconn *sslvpn.TunnelConn) *link {
	l := &link{
		sess:    sess,
		tconn:   tconn,
		ctrlCh:  make(chan []byte, ctrlQueueSize),
		dataCh:  make(chan []byte, dataQueueSize),
		doneCh:  make(chan struct{}),
		readyCh: make(chan negotiatedPPP, 1),
		ioErrCh: make(chan error, 3),
	}

	l.pppSess = ppp.NewSession(ppp.Events{
		Send: l.enqueueControl,
		NetworkUp: func(cfg ppp.IPConfig, mru int, ns1, ns2 string) {
			select {
			case l.readyCh <- negotiatedPPP{ipCfg: cfg, mru: mru, ns1: ns1, ns2: ns2}:
			default:
			}
		},
		Closed: func(err error) {
			if l.isClosing() {
				return
			}
			if err != nil {
				l.reportIOErr(fortivpn.WrapError(fortivpn.CodePPPFailed, err.Error(), err))
			} else {
				l.reportIOErr(fortivpn.NewError(fortivpn.CodeConnectionLost, "peer terminated PPP session"))
			}
		},
	}, ppp.FSMConfig{})

	return l
}

func (l *link) start() {
	l.pppSess.Start()
	l.ioWg.Add(2)
	go func() {
		defer l.ioWg.Done()
		if err := l.readerLoop(); err != nil {
			l.reportIOErr(err)
		}
	}()
	go func() {
		defer l.ioWg.Done()
		if err := l.writerLoop(); err != nil {
			l.reportIOErr(err)
		}
	}()
}

func (l *link) waitNegotiated(ctx context.Context, timeout time.Duration) (negotiatedPPP, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case neg := <-l.readyCh:
		return neg, nil
	case err := <-l.ioErrCh:
		if fortivpn.IsCode(err, fortivpn.CodeConnectionLost) {
			return negotiatedPPP{}, fortivpn.WrapError(fortivpn.CodePPPFailed, "tunnel closed during PPP negotiation", err)
		}
		return negotiatedPPP{}, err
	case <-timer.C:
		return negotiatedPPP{}, fortivpn.NewError(fortivpn.CodePPPFailed, "PPP negotiation timed out")
	case <-ctx.Done():
		return negotiatedPPP{}, ctx.Err()
	}
}

func (l *link) gracefulClose(timeout time.Duration) {
	l.closing.Store(true)
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	termCtx, cancel := context.WithTimeout(context.Background(), timeout)
	_ = l.pppSess.Terminate(termCtx)
	cancel()
	l.close()
}

func (l *link) close() {
	l.closing.Store(true)
	l.closeOnce.Do(func() {
		close(l.doneCh)
		l.pppSess.Close()
		_ = l.tconn.Close()
	})
	l.ioWg.Wait()
}

func (l *link) isClosing() bool {
	return l.closing.Load()
}

func (l *link) reportIOErr(err error) {
	if err == nil {
		return
	}
	select {
	case l.ioErrCh <- err:
	default:
	}
}
