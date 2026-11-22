package update

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"tunnelkit/internal/privilege"
)

func shellQuote(s string) string { return privilege.ShellQuote(s) }

// buildPOSIXFileUpdate 与普通安装器保持同一事务：先暂存至受保护目标目录，可选校验 SHA-256，
// 全部就绪后再备份原文件并原子替换，失败时按逆序回滚。
func buildPOSIXFileUpdate(stageDir, targetDir string, names []string, installBin string) string {
	return buildPOSIXFileUpdateWithHelperSync(stageDir, targetDir, names, installBin, "", nil)
}

func buildPOSIXFileUpdateWithHelperSync(
	stageDir, targetDir string,
	names []string,
	installBin, helperExe string,
	expectedSums map[string]string,
) string {
	lines := []string{"set -eu"}
	for i := range names {
		lines = append(lines, fmt.Sprintf("staged_%d=0; backed_%d=0; installed_%d=0", i, i, i))
	}
	lines = append(lines, "rollback() {", "status=$?", "trap - EXIT HUP INT TERM", "if [ \"$status\" -ne 0 ]; then")
	for i, name := range slices.Backward(names) {
		dst := path.Join(targetDir, name)
		lines = append(lines,
			fmt.Sprintf("if [ \"$staged_%d\" -eq 1 ]; then rm -f %s || status=1; fi", i, shellQuote(dst+".new")),
			fmt.Sprintf("if [ \"$installed_%d\" -eq 1 ]; then rm -f %s || status=1; fi", i, shellQuote(dst)),
			fmt.Sprintf("if [ \"$backed_%d\" -eq 1 ]; then mv -f %s %s || status=1; fi", i, shellQuote(dst+".old"), shellQuote(dst)),
		)
	}
	lines = append(lines, "fi", "exit \"$status\"", "}", "trap rollback EXIT", "trap 'exit 1' HUP INT TERM")
	// 1. 先拷贝至受保护目标目录 targetDir/<name>.new（root 环境下立即锁定 0:0 属主），消除非特权暂存目录替换窗口。
	for i, name := range names {
		dstNew := path.Join(targetDir, name+".new")
		dstNewQ := shellQuote(dstNew)
		lines = append(lines,
			fmt.Sprintf("staged_%d=1", i),
			shellQuote(installBin)+" -m 0755 "+shellQuote(path.Join(stageDir, name))+" "+dstNewQ,
			`if [ "$(/usr/bin/id -u 2>/dev/null || id -u)" -eq 0 ]; then chown 0:0 `+dstNewQ+`; fi`,
		)
	}
	// 2. 在触碰原可执行文件前，直接对受保护副本校验 SHA-256。
	for _, name := range names {
		sum := strings.ToLower(strings.TrimSpace(expectedSums[name]))
		if sum == "" {
			continue
		}
		dstNewQ := shellQuote(path.Join(targetDir, name+".new"))
		lines = append(lines,
			fmt.Sprintf("actual=$(/usr/bin/sha256sum %s 2>/dev/null || /bin/sha256sum %s 2>/dev/null || /usr/bin/shasum -a 256 %s) || exit 1", dstNewQ, dstNewQ, dstNewQ),
			fmt.Sprintf("actual=${actual%%%% *}; actual=${actual#\\\\}; [ \"$actual\" = %s ] || exit 1", shellQuote(sum)),
		)
	}
	// 3. 校验全部通过后再备份原文件并原子提交。
	for i, name := range names {
		dst := path.Join(targetDir, name)
		lines = append(lines,
			"rm -f "+shellQuote(dst+".old"),
			fmt.Sprintf("if [ -e %s ]; then mv %s %s; backed_%d=1; fi", shellQuote(dst), shellQuote(dst), shellQuote(dst+".old"), i),
		)
	}
	for i, name := range names {
		dst := path.Join(targetDir, name)
		lines = append(lines,
			fmt.Sprintf("installed_%d=1", i),
			"mv -f "+shellQuote(dst+".new")+" "+shellQuote(dst),
			fmt.Sprintf("staged_%d=0", i),
		)
	}
	lines = append(lines, "trap - EXIT HUP INT TERM")
	if helperExe != "" {
		helperQ := shellQuote(helperExe)
		lines = append(lines, "if [ -x "+helperQ+" ]; then "+privilege.ShellQuoteCommand(helperExe, []string{"install"})+" >/dev/null 2>&1 || true; fi")
	}
	return strings.Join(lines, "\n")
}

// buildMacElevatedUpdateShellCmd 生成 macOS .app 提权暂存、属主锁定、SHA-256 二次校验、隔离属性清理与失败回滚脚本。
func buildMacElevatedUpdateShellCmd(srcAppPath, dstAppPath, backupPath string, expectedSums map[string]string) string {
	srcQ := shellQuote(srcAppPath)
	dstQ := shellQuote(dstAppPath)
	bakQ := shellQuote(backupPath)
	stagePath := dstAppPath + ".stage"
	stageQ := shellQuote(stagePath)
	helperPath := path.Join(dstAppPath, "Contents", "MacOS", "tunnelkit-helper")
	helperQ := shellQuote(helperPath)
	lines := []string{
		"set -eu",
		"backed=0; installed=0",
		"rollback() {",
		"  status=$?",
		"  trap - EXIT HUP INT TERM",
		"  if [ \"$status\" -ne 0 ]; then",
		"    /bin/rm -rf " + stageQ,
		"    if [ \"$installed\" -eq 1 ]; then /bin/rm -rf " + dstQ + " || status=1; fi",
		"    if [ \"$backed\" -eq 1 ]; then /bin/mv -f " + bakQ + " " + dstQ + " || status=1; fi",
		"  fi",
		"  exit \"$status\"",
		"}",
		"trap rollback EXIT",
		"trap 'exit 1' HUP INT TERM",
		"/bin/rm -rf " + stageQ + " " + bakQ,
		"/usr/bin/ditto " + srcQ + " " + stageQ,
		"/usr/bin/xattr -dr com.apple.quarantine " + stageQ + " || true",
		`if [ "$(/usr/bin/id -u)" -eq 0 ]; then /usr/sbin/chown -R 0:0 ` + stageQ + `; fi`,
	}
	// 先完成属主锁定后再校验暂存 .app 内可执行文件 SHA-256，校验通过前不触碰原 .app。
	for _, rel := range macVerifiedBundleBinaries {
		sum := strings.ToLower(strings.TrimSpace(expectedSums[rel]))
		if sum == "" {
			continue
		}
		fileQ := shellQuote(path.Join(stagePath, rel))
		lines = append(lines,
			fmt.Sprintf("actual=$(/usr/bin/shasum -a 256 %s) || exit 1", fileQ),
			fmt.Sprintf("actual=${actual%%%% *}; [ \"$actual\" = %s ] || exit 1", shellQuote(sum)),
		)
	}
	lines = append(lines,
		"if [ -e "+dstQ+" ]; then /bin/mv "+dstQ+" "+bakQ+"; backed=1; fi",
		"installed=1",
		"/bin/mv "+stageQ+" "+dstQ,
		"trap - EXIT HUP INT TERM",
		"/usr/bin/touch "+dstQ+" || true",
		"/bin/rm -rf "+bakQ,
		"if [ -x "+helperQ+" ]; then "+privilege.ShellQuoteCommand(helperPath, []string{"install"})+" >/dev/null 2>&1 || true; fi",
	)
	return strings.Join(lines, "\n")
}

var macVerifiedBundleBinaries = []string{
	"Contents/MacOS/tunnelkit",
	"Contents/MacOS/tunnelkit-helper",
}

// buildMacRelaunchShellCmd 生成等待旧进程及 LaunchServices 注销后重新打开 .app 的后台脚本。
// 位置参数：$1=旧进程 PID，$2=.app 路径，可选 $3=open 路径、$4=lsappinfo 路径、$5=排空等待秒数。
func buildMacRelaunchShellCmd() string {
	return strings.Join([]string{
		"set -eu",
		`old_pid="$1"`,
		`bundle_path="$2"`,
		`open_bin="${3:-/usr/bin/open}"`,
		`lsappinfo_bin="${4:-/usr/bin/lsappinfo}"`,
		`settle_delay="${5:-0.15}"`,
		`sleep_bin="/bin/sleep"`,
		`if [ ! -x "$sleep_bin" ]; then sleep_bin="sleep"; fi`,
		`case "$old_pid" in ''|*[!0-9]*) exit 1 ;; esac`,
		`if [ "$old_pid" -le 1 ] || [ -z "$bundle_path" ]; then exit 1; fi`,
		`i=0`,
		`while kill -0 "$old_pid" 2>/dev/null; do`,
		`  i=$((i + 1))`,
		`  if [ "$i" -ge 1200 ]; then exit 0; fi`,
		`  "$sleep_bin" 0.05`,
		`done`,
		`if [ -x "$lsappinfo_bin" ]; then`,
		`  j=0`,
		`  while [ -n "$("$lsappinfo_bin" find pid="$old_pid" 2>/dev/null || true)" ]; do`,
		`    j=$((j + 1))`,
		`    if [ "$j" -ge 100 ]; then break; fi`,
		`    "$sleep_bin" 0.05`,
		`  done`,
		`fi`,
		`"$sleep_bin" "$settle_delay"`,
		`exec "$open_bin" "$bundle_path"`,
	}, "\n")
}
