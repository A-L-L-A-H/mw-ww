package sweeper

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// TrayWatchdogConfig configures thresholds for the Windows tray disk watchdog.
type TrayWatchdogConfig struct {
	WarnFreeBytes     int64 `json:"warnFreeBytes"`
	CriticalFreeBytes int64 `json:"criticalFreeBytes"`
	WarnUsedPct       int   `json:"warnUsedPct"`
	CriticalUsedPct   int   `json:"criticalUsedPct"`
	AutoSweepOptIn    bool  `json:"autoSweepOptIn"`
}

// DefaultTrayWatchdogConfig returns balanced workstation thresholds (15 GB warn, 5 GB critical).
func DefaultTrayWatchdogConfig() TrayWatchdogConfig {
	return TrayWatchdogConfig{
		WarnFreeBytes:     15 << 30,
		CriticalFreeBytes: 5 << 30,
		WarnUsedPct:       85,
		CriticalUsedPct:   92,
		AutoSweepOptIn:    false,
	}
}

// TrayWatchdogAlert describes a low-disk condition surfaced to the system tray.
type TrayWatchdogAlert struct {
	Drive                 string `json:"drive"`
	Level                 string `json:"level"` // ok | warn | critical
	FreeBytes             int64  `json:"freeBytes"`
	UsedPct               int    `json:"usedPct"`
	Tooltip               string `json:"tooltip"`
	RequiresConfirmation  bool   `json:"requiresConfirmation"`
	RecommendEmpireSweep  bool   `json:"recommendEmpireSweep"`
	CheckedAt             string `json:"checkedAt"`
}

// EvaluateTrayWatchdog evaluates drive snapshots against watchdog thresholds.
func EvaluateTrayWatchdog(drives []DriveInfo, cfg TrayWatchdogConfig) []TrayWatchdogAlert {
	if cfg.WarnFreeBytes <= 0 {
		cfg = DefaultTrayWatchdogConfig()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	alerts := make([]TrayWatchdogAlert, 0, len(drives))
	for _, d := range drives {
		level := "ok"
		switch {
		case (d.Total > 0 && d.Free <= cfg.CriticalFreeBytes) || d.UsedPct >= cfg.CriticalUsedPct || d.Status == "critical":
			level = "critical"
		case (d.Total > 0 && d.Free <= cfg.WarnFreeBytes) || d.UsedPct >= cfg.WarnUsedPct || d.Status == "warn":
			level = "warn"
		}
		recommend := level == "warn" || level == "critical"
		tooltip := fmt.Sprintf("mw-ww Tray Watchdog [%s]: Drive %s %d%% used (%.2f GB free)",
			strings.ToUpper(level), d.Drive, d.UsedPct, float64(d.Free)/float64(1<<30))
		alerts = append(alerts, TrayWatchdogAlert{
			Drive:                d.Drive,
			Level:                level,
			FreeBytes:            d.Free,
			UsedPct:              d.UsedPct,
			Tooltip:              tooltip,
			RequiresConfirmation: recommend && !cfg.AutoSweepOptIn,
			RecommendEmpireSweep: recommend,
			CheckedAt:            now,
		})
	}
	return alerts
}

// EmpireDomain identifies one of the 5 canonical Empire cache domains (V53).
type EmpireDomain string

const (
	EmpireAntigravity EmpireDomain = "antigravity"
	EmpireDocker      EmpireDomain = "docker"
	EmpireGo          EmpireDomain = "go"
	EmpireCargo       EmpireDomain = "cargo"
	EmpireRocksoul    EmpireDomain = "rocksoul"
)

// EmpireCacheGroup aggregates cache directories and reclaimable bytes for one Empire domain.
type EmpireCacheGroup struct {
	Domain         EmpireDomain `json:"domain"`
	Label          string       `json:"label"`
	Safety         string       `json:"safety"`
	ReclaimBytes   int64        `json:"reclaimBytes"`
	FileCount      int64        `json:"fileCount"`
	Paths          []string     `json:"paths"`
	PresentTargets int          `json:"presentTargets"`
}

// EmpireSweepSummary is the backend payload for the Empire Sweep Panel component (V53).
type EmpireSweepSummary struct {
	Groups            []EmpireCacheGroup `json:"groups"`
	TotalReclaimBytes int64              `json:"totalReclaimBytes"`
	SafeReclaimBytes  int64              `json:"safeReclaimBytes"`
	GeneratedAt       string             `json:"generatedAt"`
}

// EmpireSweepCandidates returns the canonical directory list for each Empire domain rooted at homeDir.
func EmpireSweepCandidates(homeDir string) map[EmpireDomain][]string {
	if strings.TrimSpace(homeDir) == "" {
		homeDir, _ = os.UserHomeDir()
	}
	local := filepath.Join(homeDir, "AppData", "Local")
	candidates := map[EmpireDomain][]string{
		EmpireAntigravity: {
			filepath.Join(homeDir, ".gemini", "antigravity", "worktrees"),
			filepath.Join(homeDir, ".gemini", "antigravity", "brain"),
			filepath.Join(homeDir, ".gemini", "antigravity", "conversations"),
			filepath.Join(local, "antigravity-updater"),
		},
		EmpireDocker: {
			filepath.Join(homeDir, ".docker", "buildx"),
			filepath.Join(local, "Docker", "wsl", "data"),
			"/var/lib/docker/buildx",
		},
		EmpireGo: {
			filepath.Join(local, "go-build"),
			filepath.Join(homeDir, "go", "pkg", "mod", "cache"),
			filepath.Join(homeDir, ".cache", "go-build"),
		},
		EmpireCargo: {
			filepath.Join(homeDir, ".cargo", "registry", "cache"),
			filepath.Join(homeDir, ".cargo", "git", "db"),
		},
		EmpireRocksoul: {
			filepath.Join(homeDir, ".rocksoul", "worktrees"),
			filepath.Join(homeDir, ".rocksoul", "cache"),
			filepath.Join(homeDir, ".rocksoul", "sandbox"),
		},
	}
	if runtime.GOOS != "windows" {
		if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
			candidates[EmpireGo] = append(candidates[EmpireGo], filepath.Join(cacheDir, "go-build"))
		}
	}
	return candidates
}

// BuildEmpireSweepSummary scans the 5 Empire cache domains (Antigravity, Docker, Go, Cargo, Rocksoul).
func BuildEmpireSweepSummary(homeDir string) EmpireSweepSummary {
	candidates := EmpireSweepCandidates(homeDir)
	specOrder := []struct {
		domain EmpireDomain
		label  string
		safety string
	}{
		{EmpireAntigravity, "Antigravity IDE & Worktrees", SafetyCaution},
		{EmpireDocker, "Docker Buildx & Layer Cache", SafetySafe},
		{EmpireGo, "Go Build & Module Cache", SafetySafe},
		{EmpireCargo, "Cargo Registry & Rust Artifacts", SafetySafe},
		{EmpireRocksoul, "Rocksoul Worktrees & Sandbox", SafetyCaution},
	}

	groups := make([]EmpireCacheGroup, 0, len(specOrder))
	var totalBytes, safeBytes int64

	for _, spec := range specOrder {
		paths := candidates[spec.domain]
		seen := make(map[string]struct{}, len(paths))
		activePaths := make([]string, 0, len(paths))
		var domainBytes, domainFiles int64

		for _, rawPath := range paths {
			cleaned := filepath.Clean(rawPath)
			key := strings.ToLower(cleaned)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			activePaths = append(activePaths, cleaned)

			if fi, err := os.Lstat(cleaned); err == nil && fi.IsDir() {
				var agg dirAgg
				visited := 0
				sizeDir(cleaned, &agg, &visited, 100000)
				domainBytes += agg.bytes
				domainFiles += agg.files
			}
		}

		present := 0
		for _, p := range activePaths {
			if fi, err := os.Lstat(p); err == nil && fi.IsDir() {
				present++
			}
		}

		groups = append(groups, EmpireCacheGroup{
			Domain:         spec.domain,
			Label:          spec.label,
			Safety:         spec.safety,
			ReclaimBytes:   domainBytes,
			FileCount:      domainFiles,
			Paths:          activePaths,
			PresentTargets: present,
		})
		totalBytes += domainBytes
		if spec.safety == SafetySafe {
			safeBytes += domainBytes
		}
	}

	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].ReclaimBytes == groups[j].ReclaimBytes {
			return groups[i].Domain < groups[j].Domain
		}
		return groups[i].ReclaimBytes > groups[j].ReclaimBytes
	})

	return EmpireSweepSummary{
		Groups:            groups,
		TotalReclaimBytes: totalBytes,
		SafeReclaimBytes:  safeBytes,
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
	}
}
