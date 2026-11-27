package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/A-L-L-A-H/mw-ww/internal/guard"
	"github.com/A-L-L-A-H/mw-ww/internal/sweeper"
)

var (
	Version = "1.0.0"
	Commit  = "dev"
)

type AuditReport struct {
	Drives         []sweeper.DriveInfo `json:"drives"`
	Targets        []sweeper.DirUsage  `json:"targets"`
	SafeBytes      int64               `json:"safeBytes"`
	CautionBytes   int64               `json:"cautionBytes"`
	CriticalDrives int                 `json:"criticalDrives"`
	GeneratedAt    string              `json:"generatedAt"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mw-ww-cli", flag.ContinueOnError)
	fs.SetOutput(stderr)

	showDrives := fs.Bool("drives", false, "list ready drives/mountpoints")
	showAudit := fs.Bool("audit", false, "audit drives and known cleanable targets")
	showEmpire := fs.Bool("empire", false, "summarize Empire Sweep Panel cache domains (Antigravity, Docker, Go, Cargo, Rocksoul)")
	showWatchdog := fs.Bool("watchdog", false, "evaluate Windows tray disk watchdog alerts")
	scanRoot := fs.String("scan", "", "scan directory path")
	scanDepth := fs.Int("depth", 4, "max scan directory depth")
	scanEntries := fs.Int("max-entries", 80000, "max filesystem entries to inspect during scan")
	cleanSafe := fs.Bool("clean-safe", false, "clean SafetySafe targets (dry-run by default unless --apply is set)")
	dryRunFlag := fs.Bool("dry-run", true, "simulate cleanup without deleting files")
	applyFlag := fs.Bool("apply", false, "actually delete safe cleanup targets")
	optInGuard := fs.Bool("confirm-guard", false, "opt-in confirmation to override active process-guard locks")
	customPath := fs.String("path", "", "explicit target directory path for --clean-safe")
	asJSON := fs.Bool("json", false, "emit JSON output")
	ntfyURL := fs.String("ntfy-url", strings.TrimSpace(os.Getenv("NTFY_TOPIC_URL")), "optional ntfy.sh topic URL for webhook summary")
	strictNotify := fs.Bool("strict-notify", false, "fail exit code if --ntfy-url delivery fails")
	showVersion := fs.Bool("version", false, "print CLI version")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showVersion {
		info := map[string]string{
			"name":    "mw-ww-cli",
			"version": Version,
			"commit":  Commit,
			"go":      runtime.Version(),
			"os":      runtime.GOOS,
			"arch":    runtime.GOARCH,
		}
		if *asJSON {
			_ = json.NewEncoder(stdout).Encode(info)
			return 0
		}
		fmt.Fprintf(stdout, "mw-ww-cli %s (%s, %s/%s)\n", Version, Commit, runtime.GOOS, runtime.GOARCH)
		return 0
	}

	modeCount := 0
	if *showDrives {
		modeCount++
	}
	if *showAudit {
		modeCount++
	}
	if *showEmpire {
		modeCount++
	}
	if *showWatchdog {
		modeCount++
	}
	if strings.TrimSpace(*scanRoot) != "" {
		modeCount++
	}
	if *cleanSafe {
		modeCount++
	}
	if modeCount == 0 {
		*showAudit = true
	} else if modeCount > 1 {
		fmt.Fprintln(stderr, "error: specify only one of --drives, --audit, --empire, --watchdog, --scan, or --clean-safe")
		return 2
	}

	var summary string

	switch {
	case *showDrives:
		drives := sweeper.ListDrives()
		if *asJSON {
			_ = json.NewEncoder(stdout).Encode(drives)
		} else {
			for _, d := range drives {
				fmt.Fprintf(stdout, "%s\tfree=%s\ttotal=%s\tused=%d%%\tstatus=%s\n",
					d.Drive, formatBytes(d.Free), formatBytes(d.Total), d.UsedPct, d.Status)
			}
		}
		summary = fmt.Sprintf("mw-ww drives check: %d drives inspected", len(drives))

	case *showEmpire:
		emp := sweeper.BuildEmpireSweepSummary("")
		if *asJSON {
			_ = json.NewEncoder(stdout).Encode(emp)
		} else {
			fmt.Fprintf(stdout, "mw-ww Empire Sweep (%s): total=%s safe=%s\n",
				emp.GeneratedAt, formatBytes(emp.TotalReclaimBytes), formatBytes(emp.SafeReclaimBytes))
			for _, g := range emp.Groups {
				fmt.Fprintf(stdout, "  [%s] %s (%s): %s across %d files (%d present)\n",
					strings.ToUpper(g.Safety), g.Label, g.Domain, formatBytes(g.ReclaimBytes), g.FileCount, g.PresentTargets)
			}
		}
		summary = fmt.Sprintf("mw-ww empire sweep: total=%s safe=%s",
			formatBytes(emp.TotalReclaimBytes), formatBytes(emp.SafeReclaimBytes))

	case *showWatchdog:
		cfg := sweeper.DefaultTrayWatchdogConfig()
		cfg.AutoSweepOptIn = *optInGuard
		alerts := sweeper.EvaluateTrayWatchdog(sweeper.ListDrives(), cfg)
		if *asJSON {
			_ = json.NewEncoder(stdout).Encode(alerts)
		} else {
			for _, a := range alerts {
				fmt.Fprintf(stdout, "%s\tlevel=%s\tused=%d%%\tfree=%s\tconfirm=%v\n",
					a.Drive, a.Level, a.UsedPct, formatBytes(a.FreeBytes), a.RequiresConfirmation)
			}
		}
		summary = fmt.Sprintf("mw-ww tray watchdog: %d drives evaluated", len(alerts))

	case *showAudit:
		report := buildAuditReport()
		if *asJSON {
			_ = json.NewEncoder(stdout).Encode(report)
		} else {
			fmt.Fprintf(stdout, "mw-ww Audit (%s): %d drives (%d critical), safe=%s, caution=%s\n",
				report.GeneratedAt, len(report.Drives), report.CriticalDrives,
				formatBytes(report.SafeBytes), formatBytes(report.CautionBytes))
			for _, t := range report.Targets {
				fmt.Fprintf(stdout, "  [%s] %s (%s) -> %s\n", strings.ToUpper(t.Safety), formatBytes(t.Bytes), t.Category, t.Path)
			}
		}
		summary = fmt.Sprintf("mw-ww audit: %d critical drives, safe reclaimable=%s, caution=%s",
			report.CriticalDrives, formatBytes(report.SafeBytes), formatBytes(report.CautionBytes))

	case strings.TrimSpace(*scanRoot) != "":
		report, err := sweeper.ScanDirectory(ctx, *scanRoot, *scanDepth, *scanEntries)
		if err != nil {
			fmt.Fprintf(stderr, "scan failed: %v\n", err)
			return 1
		}
		if *asJSON {
			_ = json.NewEncoder(stdout).Encode(report)
		} else {
			fmt.Fprintf(stdout, "mw-ww Scan %s: %s across %d files (%dms)\n",
				report.RootPath, formatBytes(report.ScannedBytes), report.ScannedFiles, report.DurationMs)
			for i, d := range report.TopDirectories {
				if i >= 15 {
					break
				}
				fmt.Fprintf(stdout, "  [%s] %s (%s) -> %s\n", strings.ToUpper(d.Safety), formatBytes(d.Bytes), d.Category, d.Path)
			}
		}
		summary = fmt.Sprintf("mw-ww scan %s: %s across %d files",
			report.RootPath, formatBytes(report.ScannedBytes), report.ScannedFiles)

	case *cleanSafe:
		dryRun := *dryRunFlag
		if *applyFlag {
			dryRun = false
		}
		targets, skippedGuard, err := resolveSafeTargets(strings.TrimSpace(*customPath))
		if err != nil {
			fmt.Fprintf(stderr, "resolve targets failed: %v\n", err)
			return 1
		}
		result, err := sweeper.CleanPaths(ctx, targets, dryRun)
		if err != nil {
			fmt.Fprintf(stderr, "clean failed: %v\n", err)
			return 1
		}
		if len(skippedGuard) > 0 {
			result.Errors = append(result.Errors, skippedGuard...)
		}
		if *asJSON {
			_ = json.NewEncoder(stdout).Encode(result)
		} else {
			mode := "DRY-RUN"
			if !result.DryRun {
				mode = "LIVE"
			}
			fmt.Fprintf(stdout, "mw-ww Clean [%s]: freed=%s deleted=%d skippedBusy=%d targets=%d\n",
				mode, formatBytes(result.FreedBytes), result.DeletedFiles, result.SkippedBusy, len(result.Paths))
		}
		summary = fmt.Sprintf("mw-ww clean (dryRun=%v): freed=%s, files=%d, busy=%d",
			result.DryRun, formatBytes(result.FreedBytes), result.DeletedFiles, result.SkippedBusy)
	}

	if targetNtfy := strings.TrimSpace(*ntfyURL); targetNtfy != "" && summary != "" {
		if err := postNtfy(ctx, targetNtfy, summary); err != nil {
			fmt.Fprintf(stderr, "warning: ntfy notification failed: %v\n", err)
			if *strictNotify {
				return 1
			}
		}
	}
	return 0
}

func buildAuditReport() AuditReport {
	drives := sweeper.ListDrives()
	targets := sweeper.KnownTargets()
	var safeBytes, cautionBytes int64
	critical := 0
	for _, d := range drives {
		if d.Status == "critical" {
			critical++
		}
	}
	for _, t := range targets {
		switch t.Safety {
		case sweeper.SafetySafe:
			safeBytes += t.Bytes
		case sweeper.SafetyCaution:
			cautionBytes += t.Bytes
		}
	}
	return AuditReport{
		Drives:         drives,
		Targets:        targets,
		SafeBytes:      safeBytes,
		CautionBytes:   cautionBytes,
		CriticalDrives: critical,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
	}
}

func resolveSafeTargets(customPath string) ([]string, []string, error) {
	if customPath != "" {
		if blocked, reason := sweeper.ProtectedPath(customPath); blocked {
			return nil, nil, fmt.Errorf("protected path %q: %s", customPath, reason)
		}
		if locks, _ := guard.CheckPath(customPath); hasBlockingLock(locks) {
			return nil, []string{fmt.Sprintf("%s: skipped by active process guard", customPath)}, nil
		}
		return []string{customPath}, nil, nil
	}

	targets := make([]string, 0)
	skipped := make([]string, 0)
	for _, t := range sweeper.KnownTargets() {
		if t.Safety != sweeper.SafetySafe || t.Bytes == 0 {
			continue
		}
		if locks, _ := guard.CheckPath(t.Path); hasBlockingLock(locks) {
			skipped = append(skipped, fmt.Sprintf("%s: skipped by active process guard", t.Path))
			continue
		}
		targets = append(targets, t.Path)
	}
	return targets, skipped, nil
}

func hasBlockingLock(locks []guard.ProcessReport) bool {
	for _, l := range locks {
		if l.Blocking {
			return true
		}
	}
	return false
}

func postNtfy(ctx context.Context, topicURL, message string) error {
	if !strings.HasPrefix(topicURL, "http://") && !strings.HasPrefix(topicURL, "https://") {
		return errors.New("ntfy URL must start with http:// or https://")
	}
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, topicURL, strings.NewReader(message))
	if err != nil {
		return err
	}
	req.Header.Set("Title", "mw-ww Disk Sweeper Report")
	req.Header.Set("Priority", "default")
	req.Header.Set("Tags", "broom")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy HTTP %d", resp.StatusCode)
	}
	return nil
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}