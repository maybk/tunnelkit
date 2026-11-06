//go:build darwin || linux

package helperclient

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"syscall"

	"tunnelkit/internal/ipc"
	"tunnelkit/internal/privilege"
)

func defaultEndpoint() (string, error) {
	owner, err := ipc.CurrentOwnerID()
	if err != nil {
		return "", err
	}
	return privilege.Endpoint(owner)
}

func launchElevated(ctx context.Context, source, endpoint, owner, tokenHash string) (<-chan error, error) {
	target, err := installedHelperPath(ctx, source)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(target, buildServeArgs("--owner", endpoint, owner, tokenHash)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C"}
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// The outer launcher authenticates the socket, and uses IPC to cancel. Never
	// kill a root child or start a second helper when readiness takes longer.
	exited := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		if err != nil {
			err = fmt.Errorf("%s: %w", stderr.String(), err)
		}
		exited <- err
		close(exited)
	}()
	return exited, nil
}
