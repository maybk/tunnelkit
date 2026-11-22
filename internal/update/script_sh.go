package update

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"tunnelkit/internal/privilege"
)

func shellQuote(s string) string { return privilege.ShellQuote(s) }

// buildPOSIXFileUpdate 与普通安装器保持同一事务：先备份全部组件，再替换，失败时按逆序恢复。
func buildPOSIXFileUpdate(stageDir, targetDir string, names []string, installBin string) string {
	return buildPOSIXFileUpdateWithHelperSync(stageDir, targetDir, names, installBin, "")
}

func buildPOSIXFileUpdateWithHelperSync(stageDir, targetDir string, names []string, installBin, helperExe string) string {
	lines := []string{"set -eu"}
	for i := range names {
		lines = append(lines, fmt.Sprintf("backed_%d=0; installed_%d=0", i, i))
	}
	lines = append(lines, "rollback() {", "status=$?", "trap - EXIT HUP INT TERM", "if [ \"$status\" -ne 0 ]; then")
	for i, name := range slices.Backward(names) {
		dst := path.Join(targetDir, name)
		lines = append(lines,
			fmt.Sprintf("if [ \"$installed_%d\" -eq 1 ]; then rm -f %s %s || status=1; fi", i, shellQuote(dst), shellQuote(dst+".new")),
			fmt.Sprintf("if [ \"$backed_%d\" -eq 1 ]; then mv -f %s %s || status=1; fi", i, shellQuote(dst+".old"), shellQuote(dst)),
		)
	}
	lines = append(lines, "fi", "exit \"$status\"", "}", "trap rollback EXIT", "trap 'exit 1' HUP INT TERM")
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
			shellQuote(installBin)+" -m 0755 "+shellQuote(path.Join(stageDir, name))+" "+shellQuote(dst+".new"),
			"mv -f "+shellQuote(dst+".new")+" "+shellQuote(dst),
		)
	}
	lines = append(lines, "trap - EXIT HUP INT TERM")
	if helperExe != "" {
		helperQ := shellQuote(helperExe)
		lines = append(lines, "if [ -x "+helperQ+" ]; then "+privilege.ShellQuoteCommand(helperExe, []string{"install"})+" >/dev/null 2>&1 || true; fi")
	}
	return strings.Join(lines, "\n")
}

// buildMacElevatedUpdateShellCmd 生成 macOS .app 提权暂存替换、隔离属性清理与失败回滚脚本。
func buildMacElevatedUpdateShellCmd(srcAppPath, dstAppPath, backupPath string) string {
	srcQ := shellQuote(srcAppPath)
	dstQ := shellQuote(dstAppPath)
	bakQ := shellQuote(backupPath)
	stageQ := shellQuote(dstAppPath + ".stage")
	helperPath := path.Join(dstAppPath, "Contents", "MacOS", "tunnelkit-helper")
	helperQ := shellQuote(helperPath)
	return strings.Join([]string{
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
		"if [ -e " + dstQ + " ]; then /bin/mv " + dstQ + " " + bakQ + "; backed=1; fi",
		"installed=1",
		"/bin/mv " + stageQ + " " + dstQ,
		"trap - EXIT HUP INT TERM",
		"/usr/bin/touch " + dstQ + " || true",
		"/bin/rm -rf " + bakQ,
		"if [ -x " + helperQ + " ]; then " + privilege.ShellQuoteCommand(helperPath, []string{"install"}) + " >/dev/null 2>&1 || true; fi",
	}, "\n")
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
