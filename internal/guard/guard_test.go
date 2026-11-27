package guard

import (
	"path/filepath"
	"testing"
)

func TestParseProcessJSONArrayAndSingleObject(t *testing.T) {
	arrJSON := `[{"ProcessName":"chrome","Id":1234},{"ProcessName":"node","Id":"5678"},{"ProcessName":"","Id":9},{"ProcessName":"skip","Id":null}]`
	procs, err := parseProcessJSON(arrJSON)
	if err != nil {
		t.Fatalf("parse array failed: %v", err)
	}
	if len(procs) != 2 || procs[0].Name != "chrome" || procs[0].PID != "1234" || procs[1].PID != "5678" {
		t.Fatalf("unexpected parsed processes: %+v", procs)
	}

	singleWrapped := `[{"ProcessName":"Antigravity","Id":42}]`
	singleProcs, err := parseProcessJSON(singleWrapped)
	if err != nil || len(singleProcs) != 1 || singleProcs[0].PID != "42" {
		t.Fatalf("parse single wrapped failed: procs=%+v err=%v", singleProcs, err)
	}

	if _, err := parseProcessJSON(`{not-json`); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestCheckPathSafeTempDirReturnsEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "isolated-cache")
	reports, err := CheckPath(dir)
	if err != nil {
		t.Fatalf("CheckPath failed: %v", err)
	}
	if len(reports) != 0 {
		t.Fatalf("expected 0 process locks on isolated path, got %+v", reports)
	}
}

func TestEvaluateConfirmationOptInProcessGuard(t *testing.T) {
	target := `C:\Users\dev\.gemini\antigravity\worktrees`
	reports := []ProcessReport{
		{Name: "Antigravity", PID: "4242", Path: target, Blocking: true},
	}

	// Default: blocked without confirmation
	denied := EvaluateConfirmation(target, reports, nil)
	if denied.Allowed || !denied.RequiresConfirmation || len(denied.BlockingProcesses) != 1 {
		t.Fatalf("expected blocked decision without confirmation, got %+v", denied)
	}

	// Missing token or mismatched process name still blocked
	badConfirm := EvaluateConfirmation(target, reports, &GuardConfirmation{
		OptInOverride:      true,
		ConfirmedProcesses: []string{"chrome"},
		ConfirmationToken:  "tok-1",
	})
	if badConfirm.Allowed || !badConfirm.RequiresConfirmation {
		t.Fatalf("expected blocked decision for unconfirmed process, got %+v", badConfirm)
	}

	// Explicit opt-in confirmation allows override
	allowed := EvaluateConfirmation(target, reports, &GuardConfirmation{
		OptInOverride:      true,
		ConfirmedProcesses: []string{"antigravity"},
		ConfirmationToken:  "tok-override-42",
	})
	if !allowed.Allowed || allowed.RequiresConfirmation {
		t.Fatalf("expected allowed decision with valid opt-in confirmation, got %+v", allowed)
	}

	// Mock process lister test via checkPathWithProcs
	hits, err := checkPathWithProcs(target, func() ([]runningProc, error) {
		return []runningProc{{Name: "Antigravity", PID: "9001"}}, nil
	})
	if err != nil || len(hits) != 1 || hits[0].PID != "9001" {
		t.Fatalf("expected live match from injected lister, got hits=%+v err=%v", hits, err)
	}
}
