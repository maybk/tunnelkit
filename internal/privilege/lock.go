package privilege

import (
	"os"
	"path/filepath"
	"time"
)

// AcquireSession 获取跨用户与跨进程的系统级会话锁，串行化宿主网络变更与组件升级。
func AcquireSession() (*os.File, error) {
	base, err := stateRoot()
	if err != nil {
		return nil, err
	}
	if err := protectDirectory(base); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(base, "session.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := lockFile(f)
		if err == nil {
			return f, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, err
		}
		time.Sleep(40 * time.Millisecond)
	}
}
