package helperclient

import (
	"context"
	"sync"
)

// processWatcher 跟踪由当前客户端启动的辅助进程或系统服务退出状态，
// 区分真实退出（complete）与主动取消监控（abort），避免释放句柄被误判为进程已退出。
type processWatcher struct {
	done    chan struct{}
	once    sync.Once
	err     error
	aborted bool
}

func newProcessWatcher() *processWatcher {
	return &processWatcher{done: make(chan struct{})}
}

func (w *processWatcher) complete(err error) {
	w.once.Do(func() {
		w.err = err
		close(w.done)
	})
}

func (w *processWatcher) abort() {
	w.once.Do(func() {
		w.aborted = true
		close(w.done)
	})
}

func (w *processWatcher) finished() (error, bool) {
	select {
	case <-w.done:
		if w.aborted {
			return nil, false
		}
		return w.err, true
	default:
		return nil, false
	}
}

func (w *processWatcher) wait(ctx context.Context) (error, bool) {
	select {
	case <-w.done:
		if w.aborted {
			return nil, false
		}
		return w.err, true
	case <-ctx.Done():
		return ctx.Err(), true
	}
}
