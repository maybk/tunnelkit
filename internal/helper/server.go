package helper

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/engine"
	"tunnelkit/internal/ipc"
	"tunnelkit/internal/privilege"
)

// Options configures a single-session tunnelkit-helper execution.
type Options struct {
	Endpoint      string
	OwnerID       string
	TokenSHA256   string
	LogPath       string
	JournalPath   string
	Version       string
	HelloTimeout  time.Duration
	PromptTimeout time.Duration
	ExitGrace     time.Duration
	ClientGrace   time.Duration
	EngineOptions []engine.Option
	streamLogs    bool
}

func (o Options) withDefaults(defaultExitGrace time.Duration) Options {
	if o.HelloTimeout <= 0 {
		o.HelloTimeout = 15 * time.Second
	}
	if o.ClientGrace <= 0 {
		o.ClientGrace = 30 * time.Second
	}
	if o.ExitGrace <= 0 {
		o.ExitGrace = defaultExitGrace
	}
	if o.Version == "" {
		o.Version = "dev"
	}
	return o
}

// Serve runs the single-session privileged helper lifecycle:
//  1. Validate the kernel caller identity and fixed endpoint, acquire the system lock.
//  2. Listen on Endpoint restricted to OwnerID and require hello within 15 seconds.
//  3. After authentication and start, recover the protected journal, run the engine,
//     stream redacted logs/events/prompts, and exit when the session terminates.
func Serve(ctx context.Context, opts Options) error {
	opts = opts.withDefaults(500 * time.Millisecond)
	if err := privilege.ValidateEndpoint(opts.OwnerID, opts.Endpoint); err != nil {
		return err
	}
	lock, err := privilege.AcquireSession()
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := privilege.PrepareEndpoint(opts.OwnerID); err != nil {
		return err
	}
	journal, err := privilege.JournalPath(opts.OwnerID)
	if err != nil {
		return err
	}
	opts.JournalPath = journal
	opts.LogPath = ""
	opts.streamLogs = true

	ln, err := ipc.Listen(opts.Endpoint, opts.OwnerID)
	if err != nil {
		return err
	}
	defer ln.Close()

	return ServeListener(ctx, ln, opts)
}

// ServeListener runs the helper loop on an already-opened net.Listener (used by unit tests).
func ServeListener(ctx context.Context, ln net.Listener, opts Options) error {
	opts = opts.withDefaults(100 * time.Millisecond)
	logFile, err := OpenLogFile(opts.LogPath)
	if err != nil {
		return fmt.Errorf("helper: open log file: %w", err)
	}
	if logFile != nil {
		defer logFile.Close()
	}
	return serveListener(ctx, ln, opts, NewLogger(logFile))
}

type sessionRunner struct {
	runCtx       context.Context
	cancelRun    context.CancelFunc
	ln           net.Listener
	opts         Options
	logger       *slog.Logger
	state        *sessionState
	helloTimer   *time.Timer
	started      atomic.Bool
	engineCancel atomic.Pointer[context.CancelFunc]
	engineDone   chan error
	leaseMu      sync.Mutex
	orphanTimer  *time.Timer
}

func serveListener(ctx context.Context, ln net.Listener, opts Options, logger *slog.Logger) error {
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	state := newSessionState(opts.PromptTimeout)
	if opts.streamLogs {
		logger = NewLogger(sessionLogWriter{state})
	}

	r := &sessionRunner{
		runCtx:     runCtx,
		cancelRun:  cancelRun,
		ln:         ln,
		opts:       opts,
		logger:     logger,
		state:      state,
		engineDone: make(chan error, 1),
	}

	// A successful hello alone must not leave an idle privileged process behind.
	r.helloTimer = time.AfterFunc(opts.HelloTimeout, func() {
		if !r.started.Load() {
			r.cancelRun()
			_ = r.ln.Close()
		}
	})
	defer r.helloTimer.Stop()

	go func() {
		<-runCtx.Done()
		_ = ln.Close()
		state.CloseClient()
	}()

	defer r.stopOrphanTimer()

	var wg sync.WaitGroup
	for {
		rawConn, err := ln.Accept()
		if err != nil {
			if runCtx.Err() != nil || errors.Is(err, net.ErrClosed) {
				break
			}
			return err
		}
		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			r.handleClient(ipc.NewConn(c))
		}(rawConn)
	}

	if r.started.Load() {
		if cancelFn := r.engineCancel.Load(); cancelFn != nil {
			(*cancelFn)()
		}
		<-r.engineDone
	}
	wg.Wait()
	return nil
}

func (r *sessionRunner) stopOrphanTimer() {
	r.leaseMu.Lock()
	defer r.leaseMu.Unlock()
	if r.orphanTimer != nil {
		r.orphanTimer.Stop()
		r.orphanTimer = nil
	}
}

func (r *sessionRunner) scheduleOrphanTimer() {
	r.leaseMu.Lock()
	defer r.leaseMu.Unlock()
	if r.orphanTimer != nil {
		r.orphanTimer.Stop()
	}
	r.orphanTimer = time.AfterFunc(r.opts.ClientGrace, func() {
		r.leaseMu.Lock()
		defer r.leaseMu.Unlock()
		if !r.state.HasClient() {
			r.cancelRun()
			_ = r.ln.Close()
		}
	})
}

func (r *sessionRunner) authenticateClient(conn *ipc.Conn) bool {
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	env, err := conn.ReadEnvelope()
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil || env.Type != ipc.TypeHello {
		_ = conn.Send(ipc.TypeResult, env.ID, ipc.ResultData{ID: env.ID, Error: "expected hello message"})
		return false
	}

	hello, err := ipc.DecodeStrict[ipc.HelloData](env.Data)
	if err != nil {
		_ = conn.Send(ipc.TypeResult, env.ID, ipc.ResultData{ID: env.ID, Error: "invalid hello payload"})
		return false
	}
	if hello.Protocol != ipc.ProtocolVersion {
		_ = conn.Send(ipc.TypeResult, env.ID, ipc.ResultData{ID: env.ID, Error: "protocol_mismatch"})
		return false
	}
	if !ipc.VerifyTokenHash(hello.Token, r.opts.TokenSHA256) {
		_ = conn.Send(ipc.TypeResult, env.ID, ipc.ResultData{ID: env.ID, Error: "unauthorized"})
		return false
	}

	return r.state.AttachClient(conn, env.ID, r.opts.Version) == nil
}

func (r *sessionRunner) handleClient(conn *ipc.Conn) {
	defer conn.Close()
	if !r.authenticateClient(conn) {
		return
	}
	r.stopOrphanTimer()
	defer func() {
		if r.state.ClearClient(conn) && r.started.Load() {
			r.scheduleOrphanTimer()
		}
	}()

	for {
		msg, err := conn.ReadEnvelope()
		if err != nil {
			return
		}
		switch msg.Type {
		case ipc.TypeStart, ipc.TypeRecover:
			r.handleStartOrRecover(conn, msg)
		case ipc.TypeAnswer:
			r.handleAnswer(conn, msg)
		case ipc.TypeStop:
			if r.handleStop(conn, msg) {
				return
			}
		default:
			_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: "unsupported message type"})
		}
	}
}

func (r *sessionRunner) handleStartOrRecover(conn *ipc.Conn, msg ipc.Envelope) {
	recoveryOnly := msg.Type == ipc.TypeRecover
	var startData ipc.StartData
	var recoveryData ipc.RecoverData
	if recoveryOnly {
		var err error
		recoveryData, err = ipc.DecodeStrict[ipc.RecoverData](msg.Data)
		if err != nil {
			_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: "invalid recovery request"})
			return
		}
	} else {
		var err error
		startData, err = ipc.DecodeStrict[ipc.StartData](msg.Data)
		if err != nil {
			_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: err.Error()})
			return
		}
		if cert := startData.Config.ClientCert; cert != nil && (cert.CertPath != "" || cert.KeyPath != "") {
			_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: "client certificate files must be read by the desktop process"})
			return
		}
		if err := startData.Config.Validate(); err != nil {
			_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: err.Error()})
			return
		}
	}
	if !r.started.CompareAndSwap(false, true) {
		_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: "session already started"})
		return
	}
	r.helloTimer.Stop()
	_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID})

	eCtx, eCancel := context.WithCancel(r.runCtx)
	r.engineCancel.Store(&eCancel)

	engOpts := make([]engine.Option, 0, len(r.opts.EngineOptions)+2)
	if r.logger != nil {
		engOpts = append(engOpts, engine.WithLogger(r.logger))
	}
	if r.opts.JournalPath != "" {
		engOpts = append(engOpts, engine.WithJournal(r.opts.JournalPath))
	}
	engOpts = append(engOpts, r.opts.EngineOptions...)

	go func(cfg fortivpn.Config) {
		var err error
		if len(recoveryData.Journal) > 0 {
			err = engine.ImportRecoveryJournal(r.opts.JournalPath, recoveryData.Journal)
		}
		if err == nil && r.opts.JournalPath != "" {
			err = engine.Recover(eCtx, r.opts.JournalPath)
		}
		if err == nil && recoveryOnly {
			r.state.OnEvent(fortivpn.Event{Phase: fortivpn.PhaseDisconnected})
		} else if err == nil {
			err = engine.Run(eCtx, cfg, r.state, engOpts...)
		} else {
			r.state.OnEvent(fortivpn.Event{Phase: fortivpn.PhaseDisconnected, Err: fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "恢复网络配置失败", err)})
		}
		_ = r.ln.Close()
		r.engineDone <- err
		time.Sleep(r.opts.ExitGrace)
		r.cancelRun()
	}(startData.Config)
}

func (r *sessionRunner) handleAnswer(conn *ipc.Conn, msg ipc.Envelope) {
	ans, err := ipc.DecodeStrict[ipc.AnswerData](msg.Data)
	if err != nil {
		_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: err.Error()})
		return
	}
	if err := r.state.DeliverAnswer(ans.PromptID, ans.Value, ans.Cancel); err != nil {
		_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: err.Error()})
		return
	}
	_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID})
}

func (r *sessionRunner) handleStop(conn *ipc.Conn, msg ipc.Envelope) bool {
	_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID})
	if cancelFn := r.engineCancel.Load(); cancelFn != nil {
		(*cancelFn)()
		return false
	}
	r.cancelRun()
	_ = r.ln.Close()
	return true
}
