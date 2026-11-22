//go:build linux

package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"tunnelkit/internal/privilege"
)

func applyPlatformUpdate(ctx context.Context, archivePath string, _ ProgressFunc) (func() error, error) {
	selfExe, err := ResolveExecutablePath()
	if err != nil {
		return nil, fmt.Errorf("定位当前可执行文件失败: %w", err)
	}
	targetDir := filepath.Dir(selfExe)

	stageDir, err := os.MkdirTemp("", "tunnelkit-linux-stage-*")
	if err != nil {
		return nil, fmt.Errorf("创建解压临时目录失败: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(stageDir)
	}()

	required := map[string]os.FileMode{
		"tunnelkit":        0o755,
		"tunnelkit-helper": 0o755,
	}
	if err := ExtractTarGzTargets(archivePath, stageDir, required); err != nil {
		return nil, err
	}

	files := []string{"tunnelkit", "tunnelkit-helper"}
	if err := ReplaceFilesWithBackup(stageDir, targetDir, files, 0o755, false); err != nil {
		if errors.Is(err, os.ErrPermission) && !errors.Is(err, ErrRollbackFailed) {
			if elevErr := replaceLinuxFilesElevated(ctx, stageDir, targetDir); elevErr != nil {
				return nil, elevErr
			}
		} else {
			return nil, fmt.Errorf("替换可执行文件失败: %w", err)
		}
	}

	restart := func() error {
		return startDetachedProcess(selfExe, targetDir, restartArguments())
	}
	return restart, nil
}

func startDetachedProcess(exePath, workDir string, args []string) error {
	cmd := exec.Command(exePath, args...)
	cmd.Dir = workDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动新版本应用失败: %w", err)
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

func replaceLinuxFilesElevated(ctx context.Context, stageDir, targetDir string) error {
	installBin, ok := privilege.ResolveTrustedSystemBinary("/usr/bin/install", "/bin/install")
	if !ok {
		return errors.New("未找到受信任的系统 install 命令")
	}
	shBin, ok := privilege.ResolveTrustedSystemBinary("/bin/sh", "/usr/bin/sh")
	if !ok {
		return errors.New("未找到受信任的系统 sh 命令")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	names := []string{"tunnelkit", "tunnelkit-helper"}
	expectedSums := make(map[string]string, len(names))
	for _, name := range names {
		sum, err := fileSHA256Hex(filepath.Join(stageDir, name))
		if err != nil {
			return fmt.Errorf("校验暂存更新文件失败: %w", err)
		}
		expectedSums[name] = sum
	}
	script := buildPOSIXFileUpdateWithHelperSync(
		stageDir,
		targetDir,
		names,
		installBin,
		filepath.Join(targetDir, "tunnelkit-helper"),
		expectedSums,
	)
	// 授权后的文件事务必须完成或回滚，不因 GUI 请求取消而中断在两次替换之间。
	if err := privilege.RunElevatedLinux(context.WithoutCancel(ctx), shBin, "-c", script); err != nil {
		if errors.Is(err, privilege.ErrCancelled) {
			return ErrElevationCancelled
		}
		return fmt.Errorf("管理员更新失败: %w", err)
	}
	return nil
}
