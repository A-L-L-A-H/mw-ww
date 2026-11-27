package sweeper

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// LabNotes is the CSV audit log of every scan and clean ("lab notes").
type LabNotes struct {
	mu   sync.Mutex
	path string
}

type HistoryRecord struct {
	When      string   `json:"when"`
	Kind      string   `json:"kind"` // scan | clean
	Root      string   `json:"root"`
	Paths     []string `json:"paths,omitempty"`
	DryRun    bool     `json:"dryRun"`
	Freed     int64    `json:"freed"`
	Deleted   int64    `json:"deleted"`
	Skipped   int64    `json:"skipped"`
	RecordID  string   `json:"recordId"`
}

var _ = sync.Mutex{} // keep sync import stable

// OpenLabNotes creates (or opens) the CSV log under the app data dir.
func OpenLabNotes() (*LabNotes, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir, _ = os.UserHomeDir()
	}
	appDir := filepath.Join(dir, "mw-ww")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return nil, err
	}
	notes := &LabNotes{path: filepath.Join(appDir, "lab-notes.csv")}
	if _, err := os.Stat(notes.path); os.IsNotExist(err) {
		f, ferr := os.Create(notes.path)
		if ferr != nil {
			return nil, ferr
		}
		defer f.Close()
		w := csv.NewWriter(f)
		_ = w.Write([]string{"when", "kind", "root", "paths", "dryRun", "freedBytes", "deletedFiles", "skippedBusy", "recordId"})
		w.Flush()
	}
	return notes, nil
}

// Append writes one record row.
func (n *LabNotes) Append(rec HistoryRecord) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	f, err := os.OpenFile(n.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write([]string{
		rec.When, rec.Kind, rec.Root,
		joinPaths(rec.Paths),
		strconv.FormatBool(rec.DryRun),
		strconv.FormatInt(rec.Freed, 10),
		strconv.FormatInt(rec.Deleted, 10),
		strconv.FormatInt(rec.Skipped, 10),
		rec.RecordID,
	}); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

// Read returns recent rows, newest last.
func (n *LabNotes) Read(limit int) ([]HistoryRecord, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	f, err := os.Open(n.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	out := []HistoryRecord{}
	for _, row := range rows[1:] { // skip header
		if len(row) < 9 {
			continue
		}
		freed, _ := strconv.ParseInt(row[5], 10, 64)
		deleted, _ := strconv.ParseInt(row[6], 10, 64)
		skipped, _ := strconv.ParseInt(row[7], 10, 64)
		out = append(out, HistoryRecord{
			When: row[0], Kind: row[1], Root: row[2],
			Paths: splitPaths(row[3]),
			DryRun: row[4] == "true",
			Freed: freed, Deleted: deleted, Skipped: skipped,
			RecordID: row[8],
		})
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// Path returns the CSV location for display.
func (n *LabNotes) Path() string { return n.path }

func joinPaths(ps []string) string {
	if len(ps) == 0 {
		return ""
	}
	out := ps[0]
	for _, p := range ps[1:] {
		out += "||" + p
	}
	return out
}

func splitPaths(s string) []string {
	if s == "" {
		return nil
	}
	out := []string{}
	for _, p := range filepath.SplitList(s) {
		_ = p
	}
	// SplitList uses ';'; our separator is "||" — split manually.
	parts := splitDouble(s)
	return append(out, parts...)
}

func splitDouble(s string) []string {
	var out []string
	for _, p := range splitAll(s, "||") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitAll(s, sep string) []string {
	var out []string
	for {
		i := indexOf(s, sep)
		if i < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:i])
		s = s[i+len(sep):]
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// RecordHistory persists a CleanResult to the lab notes and returns the record id.
func RecordHistory(r CleanResult) string {
	notes, err := OpenLabNotes()
	if err != nil {
		return ""
	}
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	_ = notes.Append(HistoryRecord{
		When: time.Now().Format(time.RFC3339), Kind: "clean",
		Root: "", Paths: r.Paths, DryRun: r.DryRun,
		Freed: r.FreedBytes, Deleted: r.DeletedFiles, Skipped: r.SkippedBusy,
		RecordID: id,
	})
	return id
}

// RecordScan persists a scan summary.
func RecordScan(root string, bytes, files int64, durationMs int64) {
	notes, err := OpenLabNotes()
	if err != nil {
		return
	}
	_ = notes.Append(HistoryRecord{
		When: time.Now().Format(time.RFC3339), Kind: "scan",
		Root: root, Freed: bytes, Deleted: files,
		RecordID: fmt.Sprintf("%d", time.Now().UnixNano()),
	})
	_ = durationMs
}
