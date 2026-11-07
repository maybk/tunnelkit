package update

import (
	"fmt"
	"path/filepath"
	"strings"
)

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

// buildPOSIXFileUpdate 与普通安装器保持同一事务：先备份全部组件，再替换，失败时按逆序恢复。
func buildPOSIXFileUpdate(stageDir, targetDir string, names []string, installBin string) string {
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
	return strings.Join(lines, "\n")
}

// cmdQuote 配合禁用延迟展开，保留 Windows 路径中的百分号和感叹号。
func cmdQuote(s string) string { return `"` + strings.ReplaceAll(s, "%", "%%") + `"` }

func buildWindowsFileUpdate(stageDir, targetDir string, names []string, okFile, errFile string) string {
	lines := []string{"@echo off", "setlocal DisableDelayedExpansion"}
	for i := range names {
		lines = append(lines, fmt.Sprintf("set backed_%d=0", i), fmt.Sprintf("set installed_%d=0", i))
	}
	for i, name := range names {
		dst := filepath.Join(targetDir, name)
		lines = append(lines,
			"if exist "+cmdQuote(dst+".old")+" (", "del /f /q "+cmdQuote(dst+".old")+" || goto rollback", ")",
			"if exist "+cmdQuote(dst)+" (", "move /y "+cmdQuote(dst)+" "+cmdQuote(dst+".old")+" >nul || goto rollback",
			fmt.Sprintf("set backed_%d=1", i), ")",
		)
	}
	for i, name := range names {
		lines = append(lines,
			fmt.Sprintf("set installed_%d=1", i),
			"copy /y "+cmdQuote(filepath.Join(stageDir, name))+" "+cmdQuote(filepath.Join(targetDir, name))+" >nul || goto rollback",
		)
	}
	lines = append(lines, "echo ok > "+cmdQuote(okFile), "exit /b 0", ":rollback")
	for i := len(names) - 1; i >= 0; i-- {
		dst := filepath.Join(targetDir, names[i])
		lines = append(lines,
			fmt.Sprintf(`if "%%installed_%d%%"=="1" del /f /q %s >nul 2>&1`, i, cmdQuote(dst)),
			fmt.Sprintf(`if "%%backed_%d%%"=="1" move /y %s %s >nul`, i, cmdQuote(dst+".old"), cmdQuote(dst)),
		)
	}
	lines = append(lines, "echo fail > "+cmdQuote(errFile), "exit /b 1")
	return strings.Join(lines, "\r\n") + "\r\n"
}
