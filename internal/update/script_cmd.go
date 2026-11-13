package update

import (
	"fmt"
	"strings"
)

func winJoin(dir, name string) string {
	trimmed := strings.TrimRight(strings.ReplaceAll(dir, "/", `\`), `\`)
	if trimmed == "" {
		return name
	}
	return trimmed + `\` + name
}

// cmdQuote 配合禁用延迟展开，保留 Windows 路径中的百分号和感叹号并剥离非法控制字符与引号。
func cmdQuote(s string) string {
	sanitized := strings.NewReplacer("\r", "", "\n", "", `"`, "").Replace(s)
	return `"` + strings.ReplaceAll(sanitized, "%", "%%") + `"`
}

func buildWindowsFileUpdate(stageDir, targetDir string, names []string, okFile, errFile string) string {
	return buildWindowsFileUpdateWithHelperSync(stageDir, targetDir, names, okFile, errFile, "")
}

func buildWindowsFileUpdateWithHelperSync(stageDir, targetDir string, names []string, okFile, errFile, helperExe string) string {
	lines := []string{"@echo off", "setlocal DisableDelayedExpansion"}
	for i := range names {
		lines = append(lines, fmt.Sprintf("set backed_%d=0", i), fmt.Sprintf("set installed_%d=0", i))
	}
	for i, name := range names {
		dst := winJoin(targetDir, name)
		lines = append(lines,
			"if exist "+cmdQuote(dst+".old")+" (", "del /f /q "+cmdQuote(dst+".old")+" || goto rollback", ")",
			"if exist "+cmdQuote(dst)+" (", "move /y "+cmdQuote(dst)+" "+cmdQuote(dst+".old")+" >nul || goto rollback",
			fmt.Sprintf("set backed_%d=1", i), ")",
		)
	}
	for i, name := range names {
		lines = append(lines,
			fmt.Sprintf("set installed_%d=1", i),
			"copy /y "+cmdQuote(winJoin(stageDir, name))+" "+cmdQuote(winJoin(targetDir, name))+" >nul || goto rollback",
		)
	}
	if helperExe != "" {
		lines = append(lines, cmdQuote(helperExe)+" install-service >nul 2>&1")
	}
	lines = append(lines, "echo ok > "+cmdQuote(okFile), "exit /b 0", ":rollback")
	for i := len(names) - 1; i >= 0; i-- {
		dst := winJoin(targetDir, names[i])
		lines = append(lines,
			fmt.Sprintf(`if "%%installed_%d%%"=="1" del /f /q %s >nul 2>&1`, i, cmdQuote(dst)),
			fmt.Sprintf(`if "%%backed_%d%%"=="1" move /y %s %s >nul`, i, cmdQuote(dst+".old"), cmdQuote(dst)),
		)
	}
	lines = append(lines, "echo fail > "+cmdQuote(errFile), "exit /b 1")
	return strings.Join(lines, "\r\n") + "\r\n"
}
