package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBuildPOSIXFileUpdatePortableGeneration(t *testing.T) {
	posixScript := buildPOSIXFileUpdateWithHelperSync(
		"/tmp/stage",
		"/usr/bin",
		[]string{"tunnelkit", "tunnelkit-helper"},
		"/usr/bin/install",
		"/usr/bin/tunnelkit-helper",
		nil,
	)
	for _, want := range []string{
		"'/usr/bin/install' -m 0755 '/tmp/stage/tunnelkit' '/usr/bin/tunnelkit.new'",
		"'/usr/bin/install' -m 0755 '/tmp/stage/tunnelkit-helper' '/usr/bin/tunnelkit-helper.new'",
		"if [ -x '/usr/bin/tunnelkit-helper' ]; then '/usr/bin/tunnelkit-helper' 'install' >/dev/null 2>&1 || true; fi",
	} {
		if !strings.Contains(posixScript, want) {
			t.Fatalf("expected POSIX update script to contain %q, got:\n%s", want, posixScript)
		}
	}
	if strings.Contains(posixScript, `\`) {
		t.Fatalf("expected POSIX update script not to contain Windows backslashes:\n%s", posixScript)
	}

	if _, err := os.Stat("/bin/sh"); err == nil {
		if out, err := exec.Command("/bin/sh", "-n", "-c", posixScript).CombinedOutput(); err != nil {
			t.Fatalf("POSIX update script syntax error: %s (%v)", out, err)
		}
	}
}

func TestBuildPOSIXFileUpdateSuccessAndRollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell test")
	}
	installBin, err := exec.LookPath("install")
	if err != nil {
		t.Skip("install binary not found")
	}

	tmp := t.TempDir()
	stageDir := filepath.Join(tmp, "stage")
	targetDir := filepath.Join(tmp, "target")
	_ = os.MkdirAll(stageDir, 0o755)
	_ = os.MkdirAll(targetDir, 0o755)

	_ = os.WriteFile(filepath.Join(targetDir, "tunnelkit"), []byte("old-gui"), 0o755)
	_ = os.WriteFile(filepath.Join(targetDir, "tunnelkit-helper"), []byte("old-helper"), 0o755)

	// 1. 缺少第二个暂存文件时，应清理第一个 *.new 且原文件完好无损。
	_ = os.WriteFile(filepath.Join(stageDir, "tunnelkit"), []byte("new-gui"), 0o755)
	script := buildPOSIXFileUpdate(stageDir, targetDir, []string{"tunnelkit", "tunnelkit-helper"}, installBin)
	if err := exec.Command("/bin/sh", "-c", script).Run(); err == nil {
		t.Fatal("expected script to fail when tunnelkit-helper is missing in stageDir")
	}
	gotGUI, err := os.ReadFile(filepath.Join(targetDir, "tunnelkit"))
	if err != nil || string(gotGUI) != "old-gui" {
		t.Fatalf("expected rollback to restore old-gui, got %q (err=%v)", gotGUI, err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "tunnelkit.new")); !os.IsNotExist(err) {
		t.Fatalf("expected staged tunnelkit.new to be cleaned up on failure, err=%v", err)
	}

	// 2. 暂存文件齐全但校验和不匹配时，应阻止替换并清理全部 *.new 残留。
	_ = os.WriteFile(filepath.Join(stageDir, "tunnelkit-helper"), []byte("new-helper"), 0o755)
	guiSum, err := fileSHA256Hex(filepath.Join(stageDir, "tunnelkit"))
	if err != nil {
		t.Fatal(err)
	}
	helperSum, err := fileSHA256Hex(filepath.Join(stageDir, "tunnelkit-helper"))
	if err != nil {
		t.Fatal(err)
	}
	badVerified := buildPOSIXFileUpdateWithHelperSync(
		stageDir,
		targetDir,
		[]string{"tunnelkit", "tunnelkit-helper"},
		installBin,
		"",
		map[string]string{
			"tunnelkit":        guiSum,
			"tunnelkit-helper": strings.Repeat("0", 64),
		},
	)
	if err := exec.Command("/bin/sh", "-c", badVerified).Run(); err == nil {
		t.Fatal("expected POSIX update script to fail on SHA-256 mismatch")
	}
	gotGUI, _ = os.ReadFile(filepath.Join(targetDir, "tunnelkit"))
	gotHelper, _ := os.ReadFile(filepath.Join(targetDir, "tunnelkit-helper"))
	if string(gotGUI) != "old-gui" || string(gotHelper) != "old-helper" {
		t.Fatalf("SHA-256 mismatch must leave originals intact: gui=%q helper=%q", gotGUI, gotHelper)
	}
	for _, leftover := range []string{"tunnelkit.new", "tunnelkit-helper.new"} {
		if _, err := os.Stat(filepath.Join(targetDir, leftover)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be cleaned up on checksum failure, err=%v", leftover, err)
		}
	}

	// 3. 校验和全部匹配时，应全部成功更新。
	goodVerified := buildPOSIXFileUpdateWithHelperSync(
		stageDir,
		targetDir,
		[]string{"tunnelkit", "tunnelkit-helper"},
		installBin,
		"",
		map[string]string{
			"tunnelkit":        guiSum,
			"tunnelkit-helper": helperSum,
		},
	)
	if out, err := exec.Command("/bin/sh", "-c", goodVerified).CombinedOutput(); err != nil {
		t.Fatalf("expected verified script to succeed, got %v: %s", err, out)
	}
	gotGUI, _ = os.ReadFile(filepath.Join(targetDir, "tunnelkit"))
	gotHelper, _ = os.ReadFile(filepath.Join(targetDir, "tunnelkit-helper"))
	if string(gotGUI) != "new-gui" || string(gotHelper) != "new-helper" {
		t.Fatalf("unexpected updated contents: gui=%q helper=%q", gotGUI, gotHelper)
	}

	helperPath := filepath.Join(targetDir, "tunnelkit-helper")
	elevated := buildPOSIXFileUpdateWithHelperSync(
		stageDir,
		targetDir,
		[]string{"tunnelkit", "tunnelkit-helper"},
		installBin,
		helperPath,
		map[string]string{"tunnelkit": guiSum, "tunnelkit-helper": helperSum},
	)
	wantSync := "if [ -x " + shellQuote(helperPath) + " ]; then " + shellQuote(helperPath) + " 'install'"
	if !strings.Contains(elevated, wantSync) {
		t.Fatalf("expected elevated POSIX update script to sync helper, got:\n%s", elevated)
	}
	if out, err := exec.Command("/bin/sh", "-n", "-c", elevated).CombinedOutput(); err != nil {
		t.Fatalf("verified POSIX update script syntax error: %s (%v)", out, err)
	}
}

func TestBuildMacElevatedUpdateShellCmdSuccessAndRollback(t *testing.T) {
	script := buildMacElevatedUpdateShellCmd(
		"/Volumes/TunnelKit/TunnelKit's.app",
		"/Applications/TunnelKit.app",
		"/Applications/TunnelKit.app.bak",
		map[string]string{
			"Contents/MacOS/tunnelkit":        strings.Repeat("a", 64),
			"Contents/MacOS/tunnelkit-helper": strings.Repeat("b", 64),
		},
	)
	for _, want := range []string{
		"set -eu",
		`'/Volumes/TunnelKit/TunnelKit'\''s.app'`,
		`'/Applications/TunnelKit.app'`,
		`'/Applications/TunnelKit.app.bak'`,
		`'/Applications/TunnelKit.app/Contents/MacOS/tunnelkit-helper' 'install'`,
		"/usr/bin/ditto",
		"/usr/bin/shasum -a 256 '/Applications/TunnelKit.app.stage/Contents/MacOS/tunnelkit'",
		"/usr/bin/shasum -a 256 '/Applications/TunnelKit.app.stage/Contents/MacOS/tunnelkit-helper'",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("expected macOS update script to contain %q, got:\n%s", want, script)
		}
	}
	chownIdx := strings.Index(script, "/usr/sbin/chown -R 0:0")
	shasumIdx := strings.Index(script, "/usr/bin/shasum -a 256")
	mvDstIdx := strings.Index(script, "/bin/mv '/Applications/TunnelKit.app' '/Applications/TunnelKit.app.bak'")
	if chownIdx <= 0 || shasumIdx <= chownIdx || mvDstIdx <= shasumIdx {
		t.Fatalf("expected chown -> shasum -> move order, got chown=%d shasum=%d mv=%d", chownIdx, shasumIdx, mvDstIdx)
	}
	if _, err := os.Stat("/bin/sh"); err == nil {
		if out, err := exec.Command("/bin/sh", "-n", "-c", script).CombinedOutput(); err != nil {
			t.Fatalf("macOS update script syntax error: %s (%v)", out, err)
		}
	}

	if runtime.GOOS != "darwin" {
		return
	}

	tmp := t.TempDir()
	srcApp := filepath.Join(tmp, "src.app")
	dstApp := filepath.Join(tmp, "TunnelKit.app")
	bakApp := dstApp + ".bak"

	_ = os.MkdirAll(dstApp, 0o755)
	_ = os.WriteFile(filepath.Join(dstApp, "version.txt"), []byte("old-bundle"), 0o644)

	// 1. 源 .app 不存在时，ditto 暂存失败应保持原 .app 不变且无 .stage/.bak 残留。
	cmdText := buildMacElevatedUpdateShellCmd(srcApp, dstApp, bakApp, nil)
	if err := exec.Command("/bin/sh", "-c", cmdText).Run(); err == nil {
		t.Fatal("expected script to fail when srcApp does not exist")
	}
	gotOld, err := os.ReadFile(filepath.Join(dstApp, "version.txt"))
	if err != nil || string(gotOld) != "old-bundle" {
		t.Fatalf("expected rollback to preserve old-bundle, got %q (err=%v)", gotOld, err)
	}
	if _, err := os.Stat(dstApp + ".stage"); !os.IsNotExist(err) {
		t.Fatalf("expected .stage directory to be cleaned up on failure, err=%v", err)
	}

	// 2. 源 .app 存在但二进制 SHA-256 不匹配时，应中止替换、清理 .stage 且保持原 .app 完整。
	_ = os.MkdirAll(filepath.Join(srcApp, "Contents", "MacOS"), 0o755)
	_ = os.WriteFile(filepath.Join(srcApp, "version.txt"), []byte("new-bundle"), 0o644)
	_ = os.WriteFile(filepath.Join(srcApp, "Contents", "MacOS", "tunnelkit"), []byte("new-mac-gui"), 0o755)
	_ = os.WriteFile(filepath.Join(srcApp, "Contents", "MacOS", "tunnelkit-helper"), []byte("new-mac-helper"), 0o644)

	badCmd := buildMacElevatedUpdateShellCmd(srcApp, dstApp, bakApp, map[string]string{
		"Contents/MacOS/tunnelkit":        strings.Repeat("0", 64),
		"Contents/MacOS/tunnelkit-helper": strings.Repeat("1", 64),
	})
	if err := exec.Command("/bin/sh", "-c", badCmd).Run(); err == nil {
		t.Fatal("expected macOS update script to fail on SHA-256 mismatch")
	}
	gotOld, err = os.ReadFile(filepath.Join(dstApp, "version.txt"))
	if err != nil || string(gotOld) != "old-bundle" {
		t.Fatalf("SHA-256 mismatch must preserve old-bundle, got %q (err=%v)", gotOld, err)
	}
	if _, err := os.Stat(dstApp + ".stage"); !os.IsNotExist(err) {
		t.Fatalf("expected .stage directory to be cleaned up on checksum failure, err=%v", err)
	}

	// 3. 二进制 SHA-256 全部匹配时，应完成替换并清理 .stage 与 .bak。
	guiSum, err := fileSHA256Hex(filepath.Join(srcApp, "Contents", "MacOS", "tunnelkit"))
	if err != nil {
		t.Fatal(err)
	}
	helperSum, err := fileSHA256Hex(filepath.Join(srcApp, "Contents", "MacOS", "tunnelkit-helper"))
	if err != nil {
		t.Fatal(err)
	}
	goodCmd := buildMacElevatedUpdateShellCmd(srcApp, dstApp, bakApp, map[string]string{
		"Contents/MacOS/tunnelkit":        guiSum,
		"Contents/MacOS/tunnelkit-helper": helperSum,
	})
	if out, err := exec.Command("/bin/sh", "-c", goodCmd).CombinedOutput(); err != nil {
		t.Fatalf("expected verified macOS update script to succeed, got %v: %s", err, out)
	}
	gotNew, err := os.ReadFile(filepath.Join(dstApp, "version.txt"))
	if err != nil || string(gotNew) != "new-bundle" {
		t.Fatalf("expected updated bundle content 'new-bundle', got %q (err=%v)", gotNew, err)
	}
	for _, artifact := range []string{bakApp, dstApp + ".stage"} {
		if _, err := os.Stat(artifact); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed after success, err=%v", artifact, err)
		}
	}
}

func TestBuildMacRelaunchShellCmdWaitsForExitAndLaunchServices(t *testing.T) {
	relaunchScript := buildMacRelaunchShellCmd()
	if !strings.HasSuffix(relaunchScript, "\nexec \"$open_bin\" \"$bundle_path\"") {
		t.Fatalf("expected macOS relaunch script to end with plain exec open without -n, got:\n%s", relaunchScript)
	}
	if _, err := os.Stat("/bin/sh"); err == nil {
		if out, err := exec.Command("/bin/sh", "-n", "-c", relaunchScript).CombinedOutput(); err != nil {
			t.Fatalf("macOS relaunch script syntax error: %s (%v)", out, err)
		}
	}

	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell test")
	}

	for _, badArgs := range [][]string{
		{"0", "/Applications/TunnelKit.app"},
		{"1", "/Applications/TunnelKit.app"},
		{"abc", "/Applications/TunnelKit.app"},
		{"99999", ""},
	} {
		cmd := exec.Command("/bin/sh", append([]string{"-c", relaunchScript, "tunnelkit-relaunch"}, badArgs...)...)
		if err := cmd.Run(); err == nil {
			t.Fatalf("expected invalid relaunch args %v to fail", badArgs)
		}
	}

	tmp := t.TempDir()
	openArgsFile := filepath.Join(tmp, "open_args.txt")
	lsCountFile := filepath.Join(tmp, "ls_count.txt")

	fakeOpen := filepath.Join(tmp, "fake-open.sh")
	fakeOpenContent := "#!/bin/sh\nset -eu\nfor arg in \"$@\"; do printf '%s\\n' \"$arg\"; done > " + shellQuote(openArgsFile) + "\n"
	if err := os.WriteFile(fakeOpen, []byte(fakeOpenContent), 0o755); err != nil {
		t.Fatalf("write fakeOpen: %v", err)
	}

	// 模拟进程退出后 LaunchServices 仍短暂保留旧 ASN（前两次返回非空，第三次起返回空）。
	fakeLSAppInfo := filepath.Join(tmp, "fake-lsappinfo.sh")
	countFileQ := shellQuote(lsCountFile)
	fakeLSContent := strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		"n=0",
		"if [ -f " + countFileQ + " ]; then n=$(cat " + countFileQ + "); fi",
		"n=$((n + 1))",
		"printf '%s' \"$n\" > " + countFileQ,
		"if [ \"$n\" -le 2 ]; then",
		"  printf 'ASN:0x0-0x12345-\"TunnelKit\":\\n'",
		"fi",
		"",
	}, "\n")
	if err := os.WriteFile(fakeLSAppInfo, []byte(fakeLSContent), 0o755); err != nil {
		t.Fatalf("write fakeLSAppInfo: %v", err)
	}

	oldProc := exec.Command("/bin/sh", "-c", "sleep 30")
	if err := oldProc.Start(); err != nil {
		t.Fatalf("start simulated old process: %v", err)
	}
	defer func() {
		_ = oldProc.Process.Kill()
		_ = oldProc.Wait()
	}()

	targetBundle := "/Applications/TunnelKit's App.app"
	relauncher := exec.Command(
		"/bin/sh",
		"-c",
		relaunchScript,
		"tunnelkit-relaunch",
		strconv.Itoa(oldProc.Process.Pid),
		targetBundle,
		fakeOpen,
		fakeLSAppInfo,
		"0.01",
	)
	if err := relauncher.Start(); err != nil {
		t.Fatalf("start relauncher: %v", err)
	}

	// 旧进程存活期间，open 绝不能提前触发。
	time.Sleep(120 * time.Millisecond)
	if _, err := os.Stat(openArgsFile); !os.IsNotExist(err) {
		t.Fatalf("expected open not to be called while old process is still alive (err=%v)", err)
	}

	// 终止并回收旧进程，随后 relauncher 应等待 fake-lsappinfo 清空后再调用 fake-open。
	_ = oldProc.Process.Kill()
	_ = oldProc.Wait()

	done := make(chan error, 1)
	go func() {
		done <- relauncher.Wait()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("relauncher exited with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = relauncher.Process.Kill()
		t.Fatal("timed out waiting for relauncher to finish")
	}

	lsCallsRaw, err := os.ReadFile(lsCountFile)
	if err != nil {
		t.Fatalf("read lsCountFile: %v", err)
	}
	lsCalls, err := strconv.Atoi(strings.TrimSpace(string(lsCallsRaw)))
	if err != nil || lsCalls < 3 {
		t.Fatalf("expected lsappinfo to be polled until empty (>=3 calls), got %q (err=%v)", lsCallsRaw, err)
	}

	openArgsRaw, err := os.ReadFile(openArgsFile)
	if err != nil {
		t.Fatalf("read openArgsFile: %v", err)
	}
	gotArgs := strings.Split(strings.TrimRight(string(openArgsRaw), "\n"), "\n")
	if len(gotArgs) != 1 || gotArgs[0] != targetBundle {
		t.Fatalf("expected open to receive solely [%q] without -n, got %v", targetBundle, gotArgs)
	}
}
