//go:build !windows

package guard

import (
	"fmt"
	"os/exec"
	"strings"
)

func runningProcesses() ([]runningProc, error) {
	cmd := exec.Command("ps", "-eo", "comm=,pid=")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	lines := strings.Split(string(out), "\n")
	procs := make([]runningProc, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		pid := fields[len(fields)-1]
		name := strings.Join(fields[:len(fields)-1], " ")
		procs = append(procs, runningProc{Name: name, PID: pid})
	}
	return procs, nil
}
