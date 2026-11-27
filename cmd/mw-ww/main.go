package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	wwassets "github.com/A-L-L-A-H/mw-ww"
	"github.com/A-L-L-A-H/mw-ww/internal/guard"
	"github.com/A-L-L-A-H/mw-ww/internal/sweeper"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// App is the bridge between the frontend and the sweeper core.
type App struct {
	ctx   context.Context
	notes *sweeper.LabNotes
}

// NewApp creates the App.
func NewApp() *App { return &App{} }

// startup captures the Wails context and opens the lab notes CSV.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	notes, err := sweeper.OpenLabNotes()
	if err != nil {
		// Lab notes are an audit nicety; never block startup.
		notes = nil
	}
	a.notes = notes
}

// ListDrives returns drive summaries for the Lab Overview.
func (a *App) ListDrives() []sweeper.DriveInfo { return sweeper.ListDrives() }

// ScanPath sweeps one directory and returns the evidence board.
func (a *App) ScanPath(path string, maxDepth int) (sweeper.ScanReport, error) {
	report, err := sweeper.ScanDirectory(a.ctx, path, maxDepth, 0)
	if err == nil && a.notes != nil {
		sweeper.RecordScan(path, report.ScannedBytes, report.ScannedFiles, report.DurationMs)
	}
	return report, err
}

// QuickAudit probes known cache hotspots only.
func (a *App) QuickAudit() []sweeper.DirUsage { return sweeper.KnownTargets() }

// GuardPath checks whether a running process owns the target directory.
func (a *App) GuardPath(path string) ([]guard.ProcessReport, error) {
	return guard.CheckPath(path)
}

// DryRun simulates deletion of the given paths and returns totals only.
func (a *App) DryRun(paths []string) (sweeper.CleanResult, error) {
	return sweeper.CleanPaths(a.ctx, paths, true)
}

// Clean deletes the CONTENTS of the given paths after safety validation.
func (a *App) Clean(paths []string) (sweeper.CleanResult, error) {
	for _, p := range paths {
		if blocked, reason := sweeper.ProtectedPath(p); blocked {
			return sweeper.CleanResult{}, fmt.Errorf("%s: %s", p, reason)
		}
		// Backend guard: never clean a path owned by a live blocking app.
		hits, err := guard.CheckPath(p)
		if err != nil {
			return sweeper.CleanResult{}, fmt.Errorf("%s: guard unavailable: %w", p, err)
		}
		for _, hit := range hits {
			if hit.Blocking && hit.PID != "" {
				return sweeper.CleanResult{}, fmt.Errorf("%s: close %s (PID %s) first", p, hit.Name, hit.PID)
			}
		}
	}
	return sweeper.CleanPaths(a.ctx, paths, false)
}

// History returns recent lab-notes rows.
func (a *App) History(limit int) ([]sweeper.HistoryRecord, error) {
	if a.notes == nil {
		return []sweeper.HistoryRecord{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	return a.notes.Read(limit)
}

// LabNotesPath returns the CSV location.
func (a *App) LabNotesPath() string {
	if a.notes == nil {
		return ""
	}
	return a.notes.Path()
}

// OpenCleanMgr launches the built-in Windows Disk Cleanup tool.
func (a *App) OpenCleanMgr() error {
	return exec.Command("cleanmgr", "/d", "C:").Start()
}

// HomeDir resolves the user profile for the scan input default.
func (a *App) HomeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

// ResolvePath normalizes a user-entered path.
func (a *App) ResolvePath(p string) string {
	abs, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return filepath.Clean(p)
	}
	return abs
}

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:  "mw-ww — I am the one who sweeps",
		Width:  1180,
		Height: 780,
		AssetServer: &assetserver.Options{
			Assets: wwassets.Frontend,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mw-ww:", err)
		os.Exit(1)
	}
}
