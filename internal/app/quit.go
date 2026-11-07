package app

import (
	"context"
	"sync"
	"time"
)

// quitGuard 在单次有界断开清理期间保持原生退出回调响应：
// 清理成功后自动重入一次 Quit，清理失败则保留应用可继续交互重试。
type quitGuard struct {
	mu             sync.Mutex
	pending, ready bool
	stop           func(context.Context) error
	quit           func()
	onError        func(error)
}

func (q *quitGuard) shouldQuit() bool {
	q.mu.Lock()
	if q.ready {
		q.mu.Unlock()
		return true
	}
	if q.pending {
		q.mu.Unlock()
		return false
	}
	q.pending = true
	q.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := q.stop(ctx)
		cancel()
		if err != nil {
			if q.onError != nil {
				q.onError(err)
			}
			q.mu.Lock()
			q.pending = false
			q.mu.Unlock()
			return
		}
		q.mu.Lock()
		q.ready = true
		q.pending = false
		q.mu.Unlock()
		q.quit()
	}()
	return false
}
