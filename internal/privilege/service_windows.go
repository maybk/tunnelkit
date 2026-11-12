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

// Ordinary interactive users can query config/status, read the DACL, and start
// the on-demand helper, but cannot stop or reconfigure another user's session.
// Explicit service-specific rights (SERVICE_ALL_ACCESS = 0x000F01FF) are used
// instead of generic GA so SCM's MapGenericMask preserves the canonical SDDL.
const serviceSDDL = "D:P(A;;CCDCLCSWRPWPDTLOCRSDRCWDWO;;;SY)(A;;CCDCLCSWRPWPDTLOCRSDRCWDWO;;;BA)(A;;CCLCRPRC;;;IU)"

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
	if reason, err := checkServiceHandle(s, installed); err != nil {
		return fmt.Errorf("verify helper service: %w", err)
	} else if reason != "" {
		return fmt.Errorf("helper service verification failed: %s", reason)
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
		if _, err := s.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
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
	owner, dacl, err := parseAdminDACL(serviceSDDL)
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

func isLocalSystemAccount(account string) bool {
	trimmed := strings.TrimSpace(account)
	return strings.EqualFold(trimmed, "LocalSystem") ||
		strings.EqualFold(trimmed, `.\LocalSystem`) ||
		strings.EqualFold(trimmed, `NT AUTHORITY\System`)
}

func checkServiceHandle(s *mgr.Service, target string) (string, error) {
	sd, err := windows.GetSecurityInfo(s.Handle, windows.SE_SERVICE, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return "", err
	}
	trustedOwner, aclOK, err := checkDescriptor(sd, serviceSDDL)
	if err != nil {
		return "", err
	}
	if !trustedOwner {
		return "服务所有者异常", nil
	}
	if !aclOK {
		return "服务安全访问控制列表未就绪", nil
	}
	cfg, err := s.Config()
	if err != nil {
		return "", err
	}
	if cfg.ServiceType != windows.SERVICE_WIN32_OWN_PROCESS {
		return "服务进程类型不匹配", nil
	}
	if cfg.StartType != mgr.StartManual {
		return "服务启动模式非手动按需启动", nil
	}
	if !isLocalSystemAccount(cfg.ServiceStartName) {
		return "服务运行账户非 LocalSystem", nil
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.BinaryPathName), windows.EscapeArg(target)) {
		return "服务可执行文件路径不匹配", nil
	}
	return "", nil
}

func checkService() (string, error) {
	elevated := windows.GetCurrentProcessToken().IsElevated()
	target, err := ExecutablePath()
	if err != nil {
		return "", err
	}
	s, err := OpenWindowsService(windows.SERVICE_QUERY_CONFIG | windows.READ_CONTROL)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return "系统服务未注册", nil
	}
	if !elevated && errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return "系统服务查询权限待修复", nil
	}
	if err != nil {
		return "", err
	}
	defer s.Close()

	reason, err := checkServiceHandle(s, target)
	if !elevated && errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return "系统服务安全描述符权限待修复", nil
	}
	return reason, err
}

// OpenWindowsService 以最小所需访问掩码打开已注册的 TunnelKitHelper 系统服务句柄。
func OpenWindowsService(access uint32) (*mgr.Service, error) {
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
