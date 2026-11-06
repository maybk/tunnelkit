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

func serveListener(ctx context.Context, ln net.Listener, opts Options, logger *slog.Logger) error {
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	state := newSessionState(opts.PromptTimeout)
	if opts.streamLogs {
		logger = NewLogger(sessionLogWriter{state})
	}

	var (
		started      atomic.Bool
		engineCancel atomic.Pointer[context.CancelFunc]
		engineDone   = make(chan error, 1)
		wg           sync.WaitGroup
	)

	// A successful hello alone must not leave an idle privileged process behind.
	helloTimer := time.AfterFunc(opts.HelloTimeout, func() {
		if !started.Load() {
			cancelRun()
			_ = ln.Close()
		}
	})
	defer helloTimer.Stop()

	// Close listener and active client when runCtx ends.
	go func() {
		<-runCtx.Done()
		_ = ln.Close()
		state.CloseClient()
	}()

	var leaseMu sync.Mutex
	var orphanTimer *time.Timer
	defer func() {
		leaseMu.Lock()
		if orphanTimer != nil {
			orphanTimer.Stop()
		}
		leaseMu.Unlock()
	}()

	handleClient := func(conn *ipc.Conn) {
		defer conn.Close()

		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		env, err := conn.ReadEnvelope()
		_ = conn.SetReadDeadline(time.Time{})
		if err != nil || env.Type != ipc.TypeHello {
			_ = conn.Send(ipc.TypeResult, env.ID, ipc.ResultData{ID: env.ID, Error: "expected hello message"})
			return
		}

		hello, err := ipc.DecodeStrict[ipc.HelloData](env.Data)
		if err != nil {
			_ = conn.Send(ipc.TypeResult, env.ID, ipc.ResultData{ID: env.ID, Error: "invalid hello payload"})
			return
		}
		if hello.Protocol != ipc.ProtocolVersion {
			_ = conn.Send(ipc.TypeResult, env.ID, ipc.ResultData{ID: env.ID, Error: "protocol_mismatch"})
			return
		}
		if !ipc.VerifyTokenHash(hello.Token, opts.TokenSHA256) {
			_ = conn.Send(ipc.TypeResult, env.ID, ipc.ResultData{ID: env.ID, Error: "unauthorized"})
			return
		}

		if err := state.AttachClient(conn, env.ID, opts.Version); err != nil {
			return
		}
		leaseMu.Lock()
		if orphanTimer != nil {
			orphanTimer.Stop()
			orphanTimer = nil
		}
		leaseMu.Unlock()
		defer func() {
			if !state.ClearClient(conn) || !started.Load() {
				return
			}
			leaseMu.Lock()
			defer leaseMu.Unlock()
			if orphanTimer != nil {
				orphanTimer.Stop()
			}
			orphanTimer = time.AfterFunc(opts.ClientGrace, func() {
				leaseMu.Lock()
				defer leaseMu.Unlock()
				if !state.HasClient() {
					cancelRun()
					_ = ln.Close()
				}
			})
		}()

		for {
			msg, err := conn.ReadEnvelope()
			if err != nil {
				return
			}
			switch msg.Type {
			case ipc.TypeStart, ipc.TypeRecover:
				recoveryOnly := msg.Type == ipc.TypeRecover
				var startData ipc.StartData
				if recoveryOnly {
					if _, err := ipc.DecodeStrict[ipc.StopData](msg.Data); err != nil {
						_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: "invalid recovery request"})
						continue
					}
				} else {
					var err error
					startData, err = ipc.DecodeStrict[ipc.StartData](msg.Data)
					if err != nil {
						_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: err.Error()})
						continue
					}
					if cert := startData.Config.ClientCert; cert != nil && (cert.CertPath != "" || cert.KeyPath != "") {
						_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: "client certificate files must be read by the desktop process"})
						continue
					}
					if err := startData.Config.Validate(); err != nil {
						_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: err.Error()})
						continue
					}
				}
				if !started.CompareAndSwap(false, true) {
					_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: "session already started"})
					continue
				}
				helloTimer.Stop()
				_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID})

				eCtx, eCancel := context.WithCancel(runCtx)
				engineCancel.Store(&eCancel)

				engOpts := make([]engine.Option, 0, len(opts.EngineOptions)+2)
				if logger != nil {
					engOpts = append(engOpts, engine.WithLogger(logger))
				}
				if opts.JournalPath != "" {
					engOpts = append(engOpts, engine.WithJournal(opts.JournalPath))
				}
				engOpts = append(engOpts, opts.EngineOptions...)

				go func(cfg fortivpn.Config) {
					var err error
					if opts.JournalPath != "" {
						err = engine.Recover(eCtx, opts.JournalPath)
					}
					if err == nil && recoveryOnly {
						state.OnEvent(fortivpn.Event{Phase: fortivpn.PhaseDisconnected})
					} else if err == nil {
						err = engine.Run(eCtx, cfg, state, engOpts...)
					} else {
						state.OnEvent(fortivpn.Event{Phase: fortivpn.PhaseDisconnected, Err: fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "恢复网络配置失败", err)})
					}
					_ = ln.Close()
					engineDone <- err
					time.Sleep(opts.ExitGrace)
					cancelRun()
				}(startData.Config)

			case ipc.TypeAnswer:
				ans, err := ipc.DecodeStrict[ipc.AnswerData](msg.Data)
				if err != nil {
					_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: err.Error()})
					continue
				}
				if err := state.DeliverAnswer(ans.PromptID, ans.Value, ans.Cancel); err != nil {
					_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: err.Error()})
					continue
				}
				_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID})

			case ipc.TypeStop:
				_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID})
				if cancelFn := engineCancel.Load(); cancelFn != nil {
					(*cancelFn)()
				} else {
					cancelRun()
					_ = ln.Close()
					return
				}

			default:
				_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: "unsupported message type"})
			}
		}
	}

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
			handleClient(ipc.NewConn(c))
		}(rawConn)
	}

	if started.Load() {
		if cancelFn := engineCancel.Load(); cancelFn != nil {
			(*cancelFn)()
		}
		<-engineDone
	}
	wg.Wait()
	return nil
}
