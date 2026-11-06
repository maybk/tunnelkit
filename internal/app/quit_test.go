package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestQuitWaitsAsynchronouslyAndCoalescesNativeRequests(t *testing.T) {
	started, release, quitted := make(chan struct{}), make(chan struct{}), make(chan bool, 1)
	var calls atomic.Int32
	q := &quitGuard{stop: func(ctx context.Context) error {
		calls.Add(1)
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	q.quit = func() { quitted <- q.shouldQuit() }
	if q.shouldQuit() {
		t.Fatal("allowed exit before network cleanup")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("cleanup not started")
	}
	for range 8 {
		if q.shouldQuit() {
			t.Fatal("pending native quit bypassed cleanup")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("repeated quit repeated cleanup")
	}
	close(release)
	select {
	case allowed := <-quitted:
		if !allowed {
			t.Fatal("completed quit was rejected")
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup did not quit")
	}
	if !q.shouldQuit() || calls.Load() != 1 {
		t.Fatal("successful re-entry repeated cleanup")
	}
}

func TestQuitFailureKeepsApplicationAvailableForRetry(t *testing.T) {
	var calls atomic.Int32
	failure := errors.New("isolated cleanup failure")
	failed, quitted := make(chan error, 1), make(chan bool, 1)
	q := &quitGuard{stop: func(context.Context) error {
		if calls.Add(1) == 1 {
			return failure
		}
		return nil
	}, onError: func(err error) { failed <- err }}
	q.quit = func() { quitted <- q.shouldQuit() }
	q.shouldQuit()
	select {
	case err := <-failed:
		if !errors.Is(err, failure) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("missing cleanup error")
	}
	deadline := time.Now().Add(time.Second)
	for {
		q.mu.Lock()
		pending := q.pending
		q.mu.Unlock()
		if !pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("failed quit remained locked")
		}
		time.Sleep(time.Millisecond)
	}
	if q.shouldQuit() {
		t.Fatal("retry bypassed asynchronous cleanup")
	}
	select {
	case allowed := <-quitted:
		if !allowed {
			t.Fatal("successful retry rejected")
		}
	case <-time.After(time.Second):
		t.Fatal("retry did not quit")
	}
}
