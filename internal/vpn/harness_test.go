package vpn

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/profile"
)

type fakeConn struct {
	eventsCh chan helperclient.Message
	mu       sync.Mutex
	closed   bool
	stopped  int
	stopErr  error
	answers  []struct {
		id     string
		value  string
		cancel bool
	}
	onStop   func(*fakeConn)
	onAnswer func(*fakeConn, string, string, bool) error
}

func newFakeConn() *fakeConn {
	return &fakeConn{
		eventsCh: make(chan helperclient.Message, 32),
	}
}

func (c *fakeConn) Events() <-chan helperclient.Message { return c.eventsCh }

func (c *fakeConn) Answer(_ context.Context, id, value string, cancel bool) error {
	c.mu.Lock()
	c.answers = append(c.answers, struct {
		id     string
		value  string
		cancel bool
	}{id, value, cancel})
	fn := c.onAnswer
	c.mu.Unlock()
	if fn != nil {
		return fn(c, id, value, cancel)
	}
	return nil
}

func (c *fakeConn) Stop(_ context.Context) error {
	c.mu.Lock()
	c.stopped++
	err := c.stopErr
	fn := c.onStop
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if fn != nil {
		fn(c)
	} else {
		c.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseDisconnecting})
		c.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseDisconnected})
		_ = c.Close()
	}
	return nil
}

func (c *fakeConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.eventsCh)
	}
	return nil
}

func (c *fakeConn) sendEvent(ev fortivpn.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.eventsCh <- helperclient.Message{Event: &ev}
	}
}

func (c *fakeConn) sendPrompt(p fortivpn.Prompt) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.eventsCh <- helperclient.Message{Prompt: &p}
	}
}

func (c *fakeConn) stopCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopped
}

type processAwareConn struct {
	*fakeConn
	waiting, exited chan struct{}
}

func (c *processAwareConn) WaitForExit(ctx context.Context) error {
	close(c.waiting)
	select {
	case <-c.exited:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type fakeLauncher struct {
	mu       sync.Mutex
	launchFn func(ctx context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error)
	attachFn func(ctx context.Context, t helperclient.Ticket) (Conn, *fortivpn.Event, error)
}

func (l *fakeLauncher) Launch(ctx context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
	l.mu.Lock()
	fn := l.launchFn
	l.mu.Unlock()
	if fn != nil {
		return fn(ctx, req)
	}
	conn := newFakeConn()
	conn.sendEvent(fortivpn.Event{
		Phase: fortivpn.PhaseConnected,
		Info:  &fortivpn.TunnelInfo{LocalIP: "10.212.0.10", RouteCount: 2},
	})
	return conn, helperclient.Ticket{
		Endpoint:    "/tmp/fake.sock",
		Token:       "tok",
		PID:         4242,
		ProfileID:   req.ProfileID,
		ProfileName: req.ProfileName,
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (l *fakeLauncher) Attach(ctx context.Context, t helperclient.Ticket) (Conn, *fortivpn.Event, error) {
	l.mu.Lock()
	fn := l.attachFn
	l.mu.Unlock()
	if fn != nil {
		return fn(ctx, t)
	}
	return nil, nil, errors.New("no running helper")
}

func setupTestService(t *testing.T, l *fakeLauncher) (*Service, profile.Profile) {
	t.Helper()
	root := t.TempDir()
	store := profile.NewStore(root)
	saved, err := store.Save(profile.Input{
		Name:     "公司 VPN",
		Host:     "vpn.example.com",
		Port:     443,
		Username: "alice",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("save profile failed: %v", err)
	}
	svc := NewService(root, store, l)
	svc.readyTimeout = 200 * time.Millisecond
	t.Cleanup(svc.detachConn)
	return svc, saved
}
