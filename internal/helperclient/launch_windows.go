//go:build windows

package helperclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"tunnelkit/internal/privilege"
)

func defaultEndpoint() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return `\\.\pipe\TunnelKit-` + hex.EncodeToString(id[:]), nil
}

func launchElevated(ctx context.Context, source, endpoint, owner, tokenHash string) (<-chan error, error) {
	if _, err := installedHelperPath(ctx, source); err != nil {
		return nil, err
	}
	s, err := privilege.OpenWindowsService(windows.SERVICE_QUERY_STATUS | windows.SERVICE_START)
	if err != nil {
		return nil, err
	}
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := s.Query()
		if err != nil {
			_ = s.Close()
			return nil, err
		}
		if status.State == svc.Stopped {
			break
		}
		if status.State != svc.StopPending {
			_ = s.Close()
			return nil, errors.New("辅助组件已有活动会话，请先断开该会话")
		}
		select {
		case <-ctx.Done():
			_ = s.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
	if err := s.Start(buildServeArgs("--owner", endpoint, owner, tokenHash)...); err != nil {
		_ = s.Close()
		return nil, err
	}
	return watchServiceExit(s), nil
}

func watchServiceExit(s *mgr.Service) <-chan error {
	exited := make(chan error, 1)
	go func() {
		defer close(exited)
		defer s.Close()

		ticker := time.NewTicker(40 * time.Millisecond)
		defer ticker.Stop()

		var servicePID int
		for {
			status, err := s.Query()
			if err != nil {
				exited <- err
				return
			}
			if status.ProcessId != 0 {
				servicePID = int(status.ProcessId)
			}
			if status.State == svc.Stopped {
				waitServiceProcessRelease(servicePID, 5*time.Second)
				exited <- serviceExitError(status)
				return
			}
			<-ticker.C
		}
	}()
	return exited
}

func waitServiceProcessRelease(pid int, maxWait time.Duration) {
	if pid <= 0 {
		return
	}
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		alive, err := privilege.IsProcessAlive(pid)
		if err != nil || !alive {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func serviceExitError(status svc.Status) error {
	if status.Win32ExitCode == uint32(windows.ERROR_SERVICE_SPECIFIC_ERROR) {
		return fmt.Errorf("辅助服务退出（服务错误码 %d）", status.ServiceSpecificExitCode)
	}
	if status.Win32ExitCode != 0 && status.Win32ExitCode != uint32(windows.NO_ERROR) {
		return fmt.Errorf("辅助服务异常退出（Win32 错误码 %d: %v）", status.Win32ExitCode, syscall.Errno(status.Win32ExitCode))
	}
	return nil
}
