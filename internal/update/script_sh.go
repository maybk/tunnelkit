package update

import (
	"fmt"
	"path/filepath"
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
	for i := len(names) - 1; i >= 0; i-- {
		dst := filepath.Join(targetDir, names[i])
		lines = append(lines,
			fmt.Sprintf("if [ \"$installed_%d\" -eq 1 ]; then rm -f %s %s || status=1; fi", i, shellQuote(dst), shellQuote(dst+".new")),
			fmt.Sprintf("if [ \"$backed_%d\" -eq 1 ]; then mv -f %s %s || status=1; fi", i, shellQuote(dst+".old"), shellQuote(dst)),
		)
	}
	lines = append(lines, "fi", "exit \"$status\"", "}", "trap rollback EXIT", "trap 'exit 1' HUP INT TERM")
	for i, name := range names {
		dst := filepath.Join(targetDir, name)
		lines = append(lines,
			"rm -f "+shellQuote(dst+".old"),
			fmt.Sprintf("if [ -e %s ]; then mv %s %s; backed_%d=1; fi", shellQuote(dst), shellQuote(dst), shellQuote(dst+".old"), i),
		)
	}
	for i, name := range names {
		dst := filepath.Join(targetDir, name)
		lines = append(lines,
			fmt.Sprintf("installed_%d=1", i),
			shellQuote(installBin)+" -m 0755 "+shellQuote(filepath.Join(stageDir, name))+" "+shellQuote(dst+".new"),
			"mv -f "+shellQuote(dst+".new")+" "+shellQuote(dst),
		)
	}
	lines = append(lines, "trap - EXIT HUP INT TERM")
	if helperExe != "" {
		helperQ := shellQuote(helperExe)
		lines = append(lines, "if [ -x "+helperQ+" ]; then "+helperQ+" install >/dev/null 2>&1 || true; fi")
	}
	return strings.Join(lines, "\n")
}

// buildMacElevatedUpdateShellCmd 生成 macOS .app 应用包备份、ditto 替换、隔离属性清理与失败回滚脚本。
func buildMacElevatedUpdateShellCmd(srcAppPath, dstAppPath, backupPath string) string {
	srcQ := shellQuote(srcAppPath)
	dstQ := shellQuote(dstAppPath)
	bakQ := shellQuote(backupPath)
	helperQ := shellQuote(strings.TrimRight(dstAppPath, "/") + "/Contents/MacOS/tunnelkit-helper")
	return strings.Join([]string{
		"set -eu",
		"/bin/rm -rf " + bakQ,
		"/bin/mv " + dstQ + " " + bakQ,
		"if /usr/bin/ditto " + srcQ + " " + dstQ + "; then",
		"  /usr/bin/xattr -dr com.apple.quarantine " + dstQ + " || true;",
		"  /bin/rm -rf " + bakQ + ";",
		"  if [ -x " + helperQ + " ]; then " + helperQ + " install >/dev/null 2>&1 || true; fi;",
		"else",
		"  /bin/rm -rf " + dstQ + ";",
		"  /bin/mv " + bakQ + " " + dstQ + ";",
		"  exit 1;",
		"fi",
	}, "\n")
}
