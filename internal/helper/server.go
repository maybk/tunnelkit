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
	EngineOptions []engine.Option
}

func (o Options) withDefaults(defaultExitGrace time.Duration) Options {
	if o.HelloTimeout <= 0 {
		o.HelloTimeout = 60 * time.Second
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
//  1. Roll back any stale journal entries via engine.Recover.
//  2. Listen on Endpoint restricted to OwnerID.
//  3. Require a valid hello handshake within HelloTimeout (default 60s).
//  4. Accept a single start command, execute engine.Run, stream Events/Prompts,
//     and exit after the VPN session terminates.
func Serve(ctx context.Context, opts Options) error {
	opts = opts.withDefaults(500 * time.Millisecond)
	if opts.JournalPath != "" {
		_ = engine.Recover(ctx, opts.JournalPath)
	}

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

	var (
		helloReceived atomic.Bool
		started       atomic.Bool
		engineCancel  atomic.Pointer[context.CancelFunc]
		engineDone    = make(chan error, 1)
		wg            sync.WaitGroup
	)

	// Initial 60s watchdog if no client ever completes hello.
	helloTimer := time.AfterFunc(opts.HelloTimeout, func() {
		if !helloReceived.Load() {
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

		helloReceived.Store(true)
		helloTimer.Stop()

		if err := state.AttachClient(conn, env.ID, opts.Version); err != nil {
			return
		}
		defer state.ClearClient(conn)

		for {
			msg, err := conn.ReadEnvelope()
			if err != nil {
				return
			}
			switch msg.Type {
			case ipc.TypeStart:
				if !started.CompareAndSwap(false, true) {
					_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: "session already started"})
					continue
				}
				startData, err := ipc.DecodeStrict[ipc.StartData](msg.Data)
				if err != nil {
					started.Store(false)
					_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: err.Error()})
					continue
				}
				if err := startData.Config.Validate(); err != nil {
					started.Store(false)
					_ = conn.Send(ipc.TypeResult, msg.ID, ipc.ResultData{ID: msg.ID, Error: err.Error()})
					continue
				}
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
					err := engine.Run(eCtx, cfg, state, engOpts...)
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
