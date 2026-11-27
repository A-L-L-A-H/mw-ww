//go:build windows

package guard

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func runningProcesses() ([]runningProc, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-Process | Select-Object -Property ProcessName,Id | ConvertTo-Json -Compress`)
	cmd.Env = append(os.Environ(), "POWERSHELL_TELEMETRY_OPTOUT=1")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	return parseProcessJSON(normalizeProcessJSON(string(out)))
}

func normalizeProcessJSON(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "[]"
	}
	if !strings.HasPrefix(trimmed, "[") {
		return "[" + trimmed + "]"
	}
	return trimmed
}
