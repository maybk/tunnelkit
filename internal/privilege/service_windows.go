//go:build windows

package privilege

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// Ordinary users can query/start, but cannot stop another user's session.
const serviceSDDL = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;CCLCRPRC;;;IU)"

// InstallService installs protected binaries and registers a manual-start service.
func InstallService(exePath string) error {
	if exePath == "" {
		var err error
		exePath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("resolve executable path: %w", err)
		}
	}
	absExe, err := filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("abs executable path: %w", err)
	}
	if info, statErr := os.Stat(absExe); statErr != nil || info.IsDir() {
		return fmt.Errorf("helper executable not found at %s", absExe)
	}

	if !windows.GetCurrentProcessToken().IsElevated() {
		return runElevatedAndWait(context.Background(), absExe, "install-service")
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(WindowsServiceName)
	if err == nil {
		defer s.Close()
		if err := stopServiceAndWait(s, 30*time.Second); err != nil {
			return err
		}
	} else if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}
	if err := Install(absExe); err != nil {
		return err
	}
	installed, err := ExecutablePath()
	if err != nil {
		return err
	}
	desiredCfg := mgr.Config{
		ServiceType:      windows.SERVICE_WIN32_OWN_PROCESS,
		StartType:        mgr.StartManual,
		ErrorControl:     mgr.ErrorNormal,
		BinaryPathName:   windows.EscapeArg(installed),
		ServiceStartName: "LocalSystem",
		DisplayName:      "TunnelKit VPN Helper",
		Description:      "On-demand single-session privileged helper for TunnelKit SSL-VPN",
	}
	if s != nil {
		if err := s.UpdateConfig(desiredCfg); err != nil {
			return fmt.Errorf("update helper service: %w", err)
		}
	} else {
		s, err = m.CreateService(WindowsServiceName, installed, desiredCfg)
		if err != nil {
			return err
		}
		defer s.Close()
	}
	if err := grantServiceStartACL(s.Handle); err != nil {
		return fmt.Errorf("grant service start ACL: %w", err)
	}

	return nil
}

// UninstallService stops and removes the service and its protected binaries.
func UninstallService() error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		source, err := os.Executable()
		if err != nil {
			return err
		}
		return runElevatedAndWait(context.Background(), source, "uninstall-service")
	}
	// The app's packaged helper performs removal; a running system copy cannot
	// synchronously delete its own executable on Windows.
	self, err := os.Executable()
	if err != nil {
		return err
	}
	target, err := ExecutablePath()
	if err != nil {
		return err
	}
	selfInfo, selfErr := os.Stat(self)
	targetInfo, targetErr := os.Stat(target)
	if selfErr == nil && targetErr == nil && os.SameFile(selfInfo, targetInfo) {
		return errors.New("请使用应用安装目录中的 tunnelkit-helper.exe uninstall-service 卸载系统组件")
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(WindowsServiceName)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return removeInstalledComponents()
		}
		return fmt.Errorf("open service %s: %w", WindowsServiceName, err)
	}
	defer s.Close()

	if err := stopServiceAndWait(s, 30*time.Second); err != nil {
		return err
	}
	if err := s.Delete(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
		return fmt.Errorf("delete service %s: %w", WindowsServiceName, err)
	}
	return removeInstalledComponents()
}

func Uninstall() error {
	return UninstallService()
}

func removeInstalledComponents() error {
	target, err := ExecutablePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(target)
	if err := verifyACL(dir, directorySDDL); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.RemoveAll(dir)
}

func stopServiceAndWait(s *mgr.Service, timeout time.Duration) error {
	status, err := s.Query()
	if err != nil {
		return err
	}
	if status.State == svc.Stopped {
		return nil
	}
	if status.State != svc.StopPending {
		if _, err := s.Control(svc.Stop); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err = s.Query()
		if err != nil || status.State == svc.Stopped {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("timed out waiting for service to stop")
}

func grantServiceStartACL(serviceHandle windows.Handle) error {
	sd, err := windows.SecurityDescriptorFromString(serviceSDDL)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	owner, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(
		serviceHandle,
		windows.SE_SERVICE,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner,
		nil,
		dacl,
		nil,
	)
}

func serviceCurrent() (bool, error) {
	s, err := openInstalledService(windows.SERVICE_QUERY_CONFIG | windows.READ_CONTROL)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer s.Close()
	sd, err := windows.GetSecurityInfo(s.Handle, windows.SE_SERVICE, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false, err
	}
	expected, err := windows.SecurityDescriptorFromString(serviceSDDL)
	if err != nil {
		return false, err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return false, err
	}
	if (!owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) && !owner.IsWellKnown(windows.WinLocalSystemSid)) || aclString(sd) != aclString(expected) {
		return false, nil
	}
	cfg, err := s.Config()
	if err != nil {
		return false, err
	}
	target, err := ExecutablePath()
	if err != nil {
		return false, err
	}
	return strings.EqualFold(cfg.BinaryPathName, windows.EscapeArg(target)) && cfg.StartType == mgr.StartManual && strings.EqualFold(cfg.ServiceStartName, "LocalSystem"), nil
}

func openInstalledService(access uint32) (*mgr.Service, error) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, err
	}
	defer windows.CloseServiceHandle(scm)
	name, err := windows.UTF16PtrFromString(WindowsServiceName)
	if err != nil {
		return nil, err
	}
	h, err := windows.OpenService(scm, name, access)
	if err != nil {
		return nil, err
	}
	return &mgr.Service{Name: WindowsServiceName, Handle: h}, nil
}
