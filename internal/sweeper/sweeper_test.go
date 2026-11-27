package sweeper

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClassifyPathSafety(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{`C:\Windows\System32`, SafetyProtected},
		{`C:\Users\dev\.gemini\antigravity\worktrees`, SafetyCaution},
		{`C:\Users\dev\AppData\Local\antigravity-updater`, SafetySafe},
		{`C:\Users\dev\AppData\Local\Google\Chrome\User Data\Default\Cache`, SafetySafe},
	}
	for _, tc := range cases {
		_, got := ClassifyPath(tc.path)
		if got != tc.want {
			t.Errorf("ClassifyPath(%q) = %s, want %s", tc.path, got, tc.want)
		}
	}
}

func TestProtectedPathBlocksDriveRoot(t *testing.T) {
	if blocked, reason := ProtectedPath(`C:\`); !blocked {
		t.Errorf("expected drive root blocked, got reason %q", reason)
	}
	if blocked, reason := ProtectedPath(`C:\Windows`); !blocked {
		t.Errorf("expected Windows blocked, got reason %q", reason)
	}
}

func TestCleanPathsDryRunAndLive(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "cache-test")
	if err := os.MkdirAll(filepath.Join(target, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "nested", "junk.txt"), []byte("1234567890"), 0o644); err != nil {
		t.Fatal(err)
	}

	dry, err := CleanPaths(context.Background(), []string{target}, true)
	if err != nil {
		t.Fatal(err)
	}
	if dry.FreedBytes != 10 || dry.DeletedFiles != 1 {
		t.Fatalf("dry run: got %d bytes / %d files, want 10 / 1", dry.FreedBytes, dry.DeletedFiles)
	}
	if _, err := os.Stat(filepath.Join(target, "nested", "junk.txt")); err != nil {
		t.Fatal("dry run must not delete files")
	}

	live, err := CleanPaths(context.Background(), []string{target}, false)
	if err != nil {
		t.Fatal(err)
	}
	if live.FreedBytes != 10 || live.DeletedFiles != 1 {
		t.Fatalf("live: got %d bytes / %d files, want 10 / 1", live.FreedBytes, live.DeletedFiles)
	}
	if _, err := os.Stat(filepath.Join(target, "nested")); !os.IsNotExist(err) {
		t.Fatalf("live clean must remove nested contents: %v", err)
	}
}

func TestClassifyPresetsAndUnixProtected(t *testing.T) {
	cases := []struct {
		path     string
		wantCat  string
		wantSafe string
	}{
		{"/etc/nginx", "System / Protected", SafetyProtected},
		{"/home/dev/.cargo/registry/cache/index", "Rust Package Cache", SafetySafe},
		{"/var/lib/docker/buildx/activity", "Docker Build Cache", SafetySafe},
		{"/home/dev/.rocksoul/worktrees/wt-1", "Rocksoul Worktrees", SafetyCaution},
		{`C:\Users\dev\AppData\Local\go-build`, "Go Build Cache", SafetySafe},
	}
	for _, tc := range cases {
		cat, safe := ClassifyPath(tc.path)
		if cat != tc.wantCat || safe != tc.wantSafe {
			t.Errorf("ClassifyPath(%q) = (%q, %q), want (%q, %q)", tc.path, cat, safe, tc.wantCat, tc.wantSafe)
		}
	}
}

func TestProtectedPathGitAndUnixRoot(t *testing.T) {
	if blocked, reason := ProtectedPath("/repo/.git/objects"); !blocked {
		t.Errorf("expected .git blocked, got reason %q", reason)
	}
	if filepath.Separator == '/' {
		if blocked, reason := ProtectedPath("/"); !blocked {
			t.Errorf("expected / blocked on unix, got reason %q", reason)
		}
	}
}

func TestLabNotesAppendFlushesAndReads(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "lab-notes.csv")
	f, err := os.Create(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	w := csv.NewWriter(f)
	if err := w.Write([]string{"when", "kind", "root", "paths", "dryRun", "freedBytes", "deletedFiles", "skippedBusy", "recordId"}); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	_ = f.Close()

	notes := &LabNotes{path: csvPath}
	err = notes.Append(HistoryRecord{
		When:     time.Now().UTC().Format(time.RFC3339),
		Kind:     "clean",
		Paths:    []string{"/tmp/cache"},
		DryRun:   true,
		Freed:    1024,
		Deleted:  3,
		RecordID: "rec-1",
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	rows, err := notes.Read(10)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(rows) != 1 || rows[0].RecordID != "rec-1" || rows[0].Freed != 1024 || rows[0].Deleted != 3 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}
