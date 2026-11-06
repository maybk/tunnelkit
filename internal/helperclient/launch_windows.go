//go:build windows

package helperclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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

func openHelperService(access uint32) (*mgr.Service, error) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, err
	}
	defer windows.CloseServiceHandle(scm)
	name, err := windows.UTF16PtrFromString(privilege.WindowsServiceName)
	if err != nil {
		return nil, err
	}
	handle, err := windows.OpenService(scm, name, access)
	if err != nil {
		return nil, err
	}
	return &mgr.Service{Name: privilege.WindowsServiceName, Handle: handle}, nil
}

func launchElevated(ctx context.Context, source, endpoint, owner, tokenHash string) (<-chan error, error) {
	if _, err := installedHelperPath(ctx, source); err != nil {
		return nil, err
	}
	s, err := openHelperService(windows.SERVICE_QUERY_STATUS | windows.SERVICE_START)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := s.Query()
		if err != nil {
			return nil, err
		}
		if status.State == svc.Stopped {
			break
		}
		if status.State != svc.StopPending {
			return nil, errors.New("辅助组件已有活动会话，请先断开该会话")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
	return nil, s.Start(buildServeArgs("--owner", endpoint, owner, tokenHash)...)
}
