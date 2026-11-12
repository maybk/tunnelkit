//go:build windows

package helper

import (
	"context"
	"os"

	"golang.org/x/sys/windows/svc"

	"tunnelkit/internal/privilege"
)

// EnsureRootIdentity is a no-op on Windows (privileges come from the UAC token
// or the LocalSystem service account).
func EnsureRootIdentity() error {
	return nil
}

// RunIfWindowsService checks whether the current process is being invoked by the
// Windows Service Control Manager (SCM). If so, it runs the single-session helper
// lifecycle as an on-demand service and returns true.
func RunIfWindowsService(version string) (bool, error) {
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		return true, err
	}
	if !isSvc {
		return false, nil
	}
	return true, svc.Run(privilege.WindowsServiceName, &onDemandService{version: version})
}

type onDemandService struct {
	version string
}

func (s *onDemandService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}

	var serveArgs []string
	if len(args) > 1 {
		serveArgs = args[1:]
	} else if len(os.Args) > 1 {
		serveArgs = os.Args[1:]
	}

	opts, err := ParseServeArgs(serveArgs, s.version)
	if err != nil {
		changes <- svc.Status{State: svc.StopPending}
		return true, 2
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changes <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown,
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case c, ok := <-r:
				if !ok {
					return
				}
				switch c.Cmd {
				case svc.Interrogate:
					changes <- c.CurrentStatus
				case svc.Stop, svc.Shutdown:
					changes <- svc.Status{State: svc.StopPending}
					cancel()
					return
				}
			}
		}
	}()

	err = Serve(ctx, opts)
	changes <- svc.Status{State: svc.StopPending}
	if err != nil {
		return true, 1
	}
	return false, 0
}
