package helperclient

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProcessWatcherCompleteAndAbortSemantics(t *testing.T) {
	t.Run("complete reports exit error once", func(t *testing.T) {
		w := newProcessWatcher()
		if _, ok := w.finished(); ok {
			t.Fatal("fresh watcher should not report finished")
		}
		boom := errors.New("helper exited")
		w.complete(boom)
		w.abort() // late abort must not override completed exit

		if err, ok := w.finished(); !ok || !errors.Is(err, boom) {
			t.Fatalf("finished() = (%v, %v), want (%v, true)", err, ok, boom)
		}
		if err, decided := w.wait(context.Background()); !decided || !errors.Is(err, boom) {
			t.Fatalf("wait() = (%v, %v), want (%v, true)", err, decided, boom)
		}
	})

	t.Run("abort does not masquerade as process exit", func(t *testing.T) {
		w := newProcessWatcher()
		w.abort()
		w.complete(errors.New("ignored"))

		if err, ok := w.finished(); ok || err != nil {
			t.Fatalf("aborted watcher finished() = (%v, %v), want (nil, false)", err, ok)
		}
		if err, decided := w.wait(context.Background()); decided || err != nil {
			t.Fatalf("aborted watcher wait() = (%v, %v), want (nil, false)", err, decided)
		}
	})

	t.Run("wait honors context cancellation", func(t *testing.T) {
		w := newProcessWatcher()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if err, decided := w.wait(ctx); !decided || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("wait(expired) = (%v, %v), want (DeadlineExceeded, true)", err, decided)
		}
	})
}
