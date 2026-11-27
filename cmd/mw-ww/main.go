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

// EmpireSweep returns the V53 Empire Sweep Panel summary for Antigravity, Docker, Go, Cargo, and Rocksoul caches.
func (a *App) EmpireSweep() sweeper.EmpireSweepSummary {
	return sweeper.BuildEmpireSweepSummary("")
}

// TrayWatchdog evaluates the V52 Windows tray disk watchdog status.
func (a *App) TrayWatchdog(autoSweepOptIn bool) []sweeper.TrayWatchdogAlert {
	cfg := sweeper.DefaultTrayWatchdogConfig()
	cfg.AutoSweepOptIn = autoSweepOptIn
	return sweeper.EvaluateTrayWatchdog(sweeper.ListDrives(), cfg)
}

// GuardPath checks whether a running process owns the target directory.
func (a *App) GuardPath(path string) ([]guard.ProcessReport, error) {
	return guard.CheckPath(path)
}

// ConfirmGuard evaluates whether a locked target path can proceed with an opt-in confirmation token.
func (a *App) ConfirmGuard(path string, confirmation guard.GuardConfirmation) (guard.GuardDecision, error) {
	hits, err := guard.CheckPath(path)
	if err != nil {
		return guard.GuardDecision{}, err
	}
	return guard.EvaluateConfirmation(path, hits, &confirmation), nil
}

// DryRun simulates deletion of the given paths and returns totals only.
func (a *App) DryRun(paths []string) (sweeper.CleanResult, error) {
	return sweeper.CleanPaths(a.ctx, paths, true)
}

// Clean deletes the CONTENTS of the given paths after safety validation.
func (a *App) Clean(paths []string) (sweeper.CleanResult, error) {
	return a.CleanWithConfirmation(paths, nil)
}

// CleanWithConfirmation deletes the contents of paths while honoring an optional opt-in process-guard confirmation.
func (a *App) CleanWithConfirmation(paths []string, confirmation *guard.GuardConfirmation) (sweeper.CleanResult, error) {
	for _, p := range paths {
		if blocked, reason := sweeper.ProtectedPath(p); blocked {
			return sweeper.CleanResult{}, fmt.Errorf("%s: %s", p, reason)
		}
		hits, err := guard.CheckPath(p)
		if err != nil {
			return sweeper.CleanResult{}, fmt.Errorf("%s: guard unavailable: %w", p, err)
		}
		decision := guard.EvaluateConfirmation(p, hits, confirmation)
		if !decision.Allowed {
			return sweeper.CleanResult{}, fmt.Errorf("%s", decision.Reason)
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
