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
