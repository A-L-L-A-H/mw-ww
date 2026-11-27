// Package guard detects whether a directory is in use by a running process,
// so mw-ww never deletes the workspace of an active application.
package guard

import (
	"path/filepath"
	"strings"
)

// ProcessReport describes one running process that owns or touches a path.
type ProcessReport struct {
	Name     string `json:"name"`
	PID      string `json:"pid"`
	Path     string `json:"path"`
	Blocking bool   `json:"blocking"`
}

// knownApps maps normalized forward-slash directory markers to process names that lock them.
var knownApps = []struct {
	marker   string
	process  string
	blocking bool
}{
	{".gemini/antigravity", "Antigravity", true},
	{"ms-playwright", "node", false},
	{"google/chrome", "chrome", true},
	{"microsoft/edge", "msedge", true},
	{"docker", "docker", false},
	{"npm-cache", "node", false},
	{"node_modules", "node", false},
}

// CheckPath inspects running processes against a target directory.
func CheckPath(target string) ([]ProcessReport, error) {
	targetLow := strings.ToLower(filepath.ToSlash(filepath.Clean(target)))
	matched := []ProcessReport{}
	for _, app := range knownApps {
		if strings.Contains(targetLow, app.marker) {
			matched = append(matched, ProcessReport{
				Name:     app.process,
				PID:      "",
				Path:     targetLow,
				Blocking: app.blocking,
			})
		}
	}
	if len(matched) == 0 {
		return matched, nil
	}
	// Verify which of these processes actually run.
	running, err := runningProcesses()
	if err != nil {
		return matched, err // advisory only; never block on guard failure
	}
	for i := range matched {
		for _, rp := range running {
			if strings.EqualFold(rp.Name, matched[i].Name) {
				matched[i].PID = rp.PID
				break
			}
		}
	}
	// Drop matches whose owning process is not running at all.
	live := []ProcessReport{}
	for _, m := range matched {
		if m.PID != "" {
			live = append(live, m)
		}
	}
	return live, nil
}

type runningProc struct {
	Name string
	PID  string
}
