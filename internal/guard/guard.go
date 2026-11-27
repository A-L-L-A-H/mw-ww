// Package guard detects whether a directory is in use by a running process,
// so mw-ww never deletes the workspace of an active application unless explicitly confirmed.
package guard

import (
	"fmt"
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

// GuardConfirmation carries opt-in user confirmation to proceed past a process lock.
type GuardConfirmation struct {
	OptInOverride     bool     `json:"optInOverride"`
	ConfirmedProcesses []string `json:"confirmedProcesses"`
	ConfirmationToken string   `json:"confirmationToken"`
}

// GuardDecision represents the evaluated process-guard result for a target path.
type GuardDecision struct {
	Path                string          `json:"path"`
	Allowed             bool            `json:"allowed"`
	RequiresConfirmation bool            `json:"requiresConfirmation"`
	BlockingProcesses   []ProcessReport `json:"blockingProcesses"`
	Reason              string          `json:"reason,omitempty"`
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
	{".rocksoul", "rocksoul", true},
}

// CheckPath inspects running processes against a target directory.
func CheckPath(target string) ([]ProcessReport, error) {
	return checkPathWithProcs(target, runningProcesses)
}

func checkPathWithProcs(target string, lister func() ([]runningProc, error)) ([]ProcessReport, error) {
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
	running, err := lister()
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

// EvaluateConfirmation checks whether a target path can be cleaned given live process reports
// and an optional user opt-in confirmation adapter.
func EvaluateConfirmation(target string, reports []ProcessReport, confirmation *GuardConfirmation) GuardDecision {
	blocking := make([]ProcessReport, 0)
	for _, r := range reports {
		if r.Blocking && r.PID != "" {
			blocking = append(blocking, r)
		}
	}
	if len(blocking) == 0 {
		return GuardDecision{
			Path:                target,
			Allowed:             true,
			RequiresConfirmation: false,
			BlockingProcesses:   []ProcessReport{},
		}
	}

	if confirmation != nil && confirmation.OptInOverride && strings.TrimSpace(confirmation.ConfirmationToken) != "" {
		confirmedSet := make(map[string]struct{}, len(confirmation.ConfirmedProcesses))
		for _, name := range confirmation.ConfirmedProcesses {
			confirmedSet[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
		}
		allConfirmed := true
		for _, b := range blocking {
			if _, ok := confirmedSet[strings.ToLower(b.Name)]; !ok {
				if _, okWildcard := confirmedSet["*"]; !okWildcard {
					allConfirmed = false
					break
				}
			}
		}
		if allConfirmed {
			return GuardDecision{
				Path:                target,
				Allowed:             true,
				RequiresConfirmation: false,
				BlockingProcesses:   blocking,
				Reason:              "opt-in process-guard override confirmed",
			}
		}
	}

	first := blocking[0]
	return GuardDecision{
		Path:                target,
		Allowed:             false,
		RequiresConfirmation: true,
		BlockingProcesses:   blocking,
		Reason:              fmt.Sprintf("%s: close %s (PID %s) first or confirm opt-in override", target, first.Name, first.PID),
	}
}

type runningProc struct {
	Name string
	PID  string
}
