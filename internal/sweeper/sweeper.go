// Package sweeper is the disk scanning and cleaning core of mw-ww.
// "I am the one who sweeps."
package sweeper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Safety tiers, mirroring the workspace R0-R4 convention.
const (
	SafetySafe      = "safe"      // R1: regenerable cache / build artifact
	SafetyCaution   = "caution"   // R2: app data, review before delete
	SafetyProtected = "protected" // R4: system paths, never deletable
)

const (
	defaultMaxDepth   = 4
	defaultMaxEntries = 80000
)

// DirUsage is one directory row for the evidence board.
type DirUsage struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Bytes     int64  `json:"bytes"`
	FileCount int64  `json:"fileCount"`
	Category  string `json:"category"`
	Safety    string `json:"safety"`
	Depth     int    `json:"depth"`
}

// DriveInfo is a summary row for the lab overview.
type DriveInfo struct {
	Drive   string `json:"drive"`
	Label   string `json:"label"`
	Total   int64  `json:"total"`
	Free    int64  `json:"free"`
	UsedPct int    `json:"usedPct"`
	Status  string `json:"status"` // ok | warn | critical
}

// ScanReport is the result of one sweep of a root path.
type ScanReport struct {
	RootPath       string     `json:"rootPath"`
	ScannedBytes   int64      `json:"scannedBytes"`
	ScannedFiles   int64      `json:"scannedFiles"`
	DurationMs     int64      `json:"durationMs"`
	TopDirectories []DirUsage `json:"topDirectories"`
	Cleanable      []DirUsage `json:"cleanable"`
}

// CleanResult summarizes a dry-run or live clean.
type CleanResult struct {
	Paths        []string `json:"paths"`
	DryRun       bool     `json:"dryRun"`
	FreedBytes   int64    `json:"freedBytes"`
	DeletedFiles int64    `json:"deletedFiles"`
	SkippedBusy  int64    `json:"skippedBusy"`
	Errors       []string `json:"errors,omitempty"`
	RecordID     string   `json:"recordId,omitempty"`
}

// ClassifyPath maps a path to category and safety tier.
// Ported from mw-echo internal/diskdoctor with Antigravity additions.
func ClassifyPath(target string) (string, string) {
	cleaned := filepath.Clean(target)
	norm := strings.ToLower(filepath.ToSlash(cleaned))
	base := strings.ToLower(filepath.Base(cleaned))

	// R4 protected: never touch.
	for _, seg := range []string{"windows", "program files", "program files (x86)", "programdata", "mw-echo-data"} {
		if hasSegment(norm, seg) {
			return "System / Protected", SafetyProtected
		}
	}
	for _, prefix := range []string{"/etc", "/usr", "/bin", "/sbin", "/boot", "/lib", "/proc", "/sys", "/dev"} {
		if norm == prefix || strings.HasPrefix(norm, prefix+"/") {
			return "System / Protected", SafetyProtected
		}
	}
	if hasSegment(norm, "$recycle.bin") || hasSegment(norm, "system volume information") {
		return "System / Protected", SafetyProtected
	}

	// AI IDE worktrees and conversation stores.
	if hasSegment(norm, ".gemini/antigravity/worktrees") {
		return "Antigravity Worktrees", SafetyCaution
	}
	if hasSegment(norm, ".gemini/antigravity/brain") || hasSegment(norm, ".gemini/antigravity/conversations") {
		return "Antigravity Memory", SafetyCaution
	}
	if strings.Contains(norm, ".rocksoul/worktrees") || hasSegment(norm, "rocksoul-sandbox") {
		return "Rocksoul Worktrees", SafetyCaution
	}
	if base == "worktrees" || hasSegment(norm, "worktrees") {
		return "AI Agent Worktrees", SafetyCaution
	}

	// Updater leftovers from removed desktop apps.
	if strings.Contains(norm, "-updater") && strings.Contains(norm, "/appdata/local/") {
		return "Updater Orphan", SafetySafe
	}

	// Playwright browsers.
	if hasSegment(norm, "ms-playwright") || hasSegment(norm, "ms-playwright-mcp") {
		return "Playwright Browsers", SafetyCaution
	}

	if hasSegment(norm, "node_modules") {
		return "Node Dependencies", SafetyCaution
	}
	if base == "target" && isRustTarget(cleaned) {
		return "Rust Build Artifacts", SafetySafe
	}
	if strings.Contains(norm, ".cargo/registry/cache") {
		return "Rust Package Cache", SafetySafe
	}
	if strings.Contains(norm, "docker/buildx") || hasSegment(norm, "buildkit") {
		return "Docker Build Cache", SafetySafe
	}
	switch base {
	case ".next", ".turbo", ".vite", "dist", "build":
		return "Frontend Build Cache", SafetySafe
	case "temp", "tmp":
		return "Temp Files", SafetySafe
	}
	if strings.Contains(norm, "npm-cache") || strings.Contains(norm, ".pnpm-store") ||
		strings.Contains(norm, ".bun/install/cache") || strings.Contains(norm, ".yarn/cache") {
		return "JS Package Cache", SafetySafe
	}
	if strings.Contains(norm, "go-build") || strings.Contains(norm, "go/pkg/mod/cache") {
		return "Go Build Cache", SafetySafe
	}
	if strings.Contains(norm, "pip/cache") || strings.Contains(norm, ".cache/uv") || strings.Contains(norm, ".cache/huggingface") {
		return "Python / ML Cache", SafetyCaution
	}
	if (strings.Contains(norm, "chrome/user data") || strings.Contains(norm, "edge/user data")) &&
		hasSegment(norm, "cache") {
		return "Browser Cache", SafetySafe
	}
	return "General Files", SafetyCaution
}

func isRustTarget(cleaned string) bool {
	parent := filepath.Dir(cleaned)
	if _, err := os.Stat(filepath.Join(parent, "Cargo.toml")); err == nil {
		return true
	}
	low := strings.ToLower(filepath.ToSlash(cleaned))
	return strings.Contains(low, "src-tauri") || strings.Contains(low, "rust")
}

func hasSegment(path, seg string) bool {
	for _, part := range strings.Split(path, "/") {
		if part == seg {
			return true
		}
	}
	return false
}

// ProtectedPath reports whether a path must never be cleaned.
func ProtectedPath(target string) (bool, string) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return true, "empty path"
	}
	cleaned, err := filepath.Abs(filepath.Clean(trimmed))
	if err != nil {
		return true, "unresolvable path"
	}
	vol := filepath.VolumeName(cleaned)
	if vol != "" && strings.EqualFold(cleaned, vol+`\`) {
		return true, "drive root is protected"
	}
	if filepath.ToSlash(cleaned) == "/" {
		return true, "filesystem root is protected"
	}
	if home, herr := os.UserHomeDir(); herr == nil && home != "" && strings.EqualFold(filepath.Clean(home), cleaned) {
		return true, "user home root is protected"
	}
	if hasSegment(strings.ToLower(filepath.ToSlash(cleaned)), ".git") {
		return true, ".git metadata is protected"
	}
	if cat, safety := ClassifyPath(cleaned); safety == SafetyProtected {
		return true, fmt.Sprintf("%s is protected", cat)
	}
	return false, ""
}

// ListDrives returns all ready fixed drives or mountpoints.
func ListDrives() []DriveInfo {
	roots := listDriveRoots()
	drives := make([]DriveInfo, 0, len(roots))
	for _, root := range roots {
		info := DriveInfo{Drive: formatDriveName(root)}
		if total, free, ok := driveSpace(root); ok {
			info.Total, info.Free = total, free
			if total > 0 {
				pct := int((total - free) * 100 / total)
				info.UsedPct = pct
				switch {
				case pct >= 90 || free < 2<<30:
					info.Status = "critical"
				case pct >= 80:
					info.Status = "warn"
				default:
					info.Status = "ok"
				}
			}
		}
		drives = append(drives, info)
	}
	return drives
}

// ScanDirectory walks rootPath up to maxDepth, skipping symlinks/junctions
// to avoid double-counting, and returns ranked directories.
func ScanDirectory(ctx context.Context, rootPath string, maxDepth, maxEntries int) (ScanReport, error) {
	if strings.TrimSpace(rootPath) == "" {
		return ScanReport{}, fmt.Errorf("rootPath is required")
	}
	if maxDepth <= 0 {
		maxDepth = defaultMaxDepth
	}
	if maxEntries <= 0 {
		maxEntries = defaultMaxEntries
	}
	root := filepath.Clean(rootPath)
	if _, err := os.Lstat(root); err != nil {
		return ScanReport{}, err
	}
	started := time.Now()
	report := ScanReport{RootPath: root}

	var usage []DirUsage
	visited := 0

	// Single-pass bottom-up walk: each directory is measured once, and its
	// subtree total bubbles up to the parent. No re-walking subtrees.
	var walk func(dir string, depth int) dirAgg
	walk = func(dir string, depth int) dirAgg {
		agg := dirAgg{}
		if ctx.Err() != nil || visited >= maxEntries {
			return agg
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return agg
		}
		for _, entry := range entries {
			visited++
			if visited >= maxEntries || ctx.Err() != nil {
				return agg
			}
			full := filepath.Join(dir, entry.Name())
			info, err := os.Lstat(full)
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
				continue // junction/symlink: never follow, never double-count
			}
			if entry.IsDir() {
				var child dirAgg
				if depth < maxDepth {
					child = walk(full, depth+1)
				} else {
					// Depth cap reached: measure this subtree once with the
					// bounded helper, without descending further for reporting.
					childVisited := 0
					sizeDir(full, &child, &childVisited, maxEntries)
				}
				agg.bytes += child.bytes
				agg.files += child.files
				u := DirUsage{
					Path: full, Name: entry.Name(),
					Bytes: child.bytes, FileCount: child.files,
					Depth: depth,
				}
				u.Category, u.Safety = ClassifyPath(full)
				usage = append(usage, u)
			} else {
				agg.bytes += info.Size()
				agg.files++
			}
		}
		return agg
	}
	total := walk(root, 1)
	report.ScannedBytes = total.bytes
	report.ScannedFiles = total.files
	report.TopDirectories = usage

	sort.SliceStable(report.TopDirectories, func(i, j int) bool {
		return report.TopDirectories[i].Bytes > report.TopDirectories[j].Bytes
	})
	for _, d := range report.TopDirectories {
		if d.Safety != SafetyProtected && d.Bytes > 50<<20 {
			report.Cleanable = append(report.Cleanable, d)
		}
	}
	if len(report.TopDirectories) > 60 {
		report.TopDirectories = report.TopDirectories[:60]
	}
	report.DurationMs = time.Since(started).Milliseconds()
	return report, nil
}

type dirAgg struct {
	bytes int64
	files int64
}

func sizeDir(dir string, agg *dirAgg, visited *int, maxEntries int) {
	if *visited >= maxEntries {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		*visited++
		if *visited >= maxEntries {
			return
		}
		full := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(full)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
			continue
		}
		if entry.IsDir() {
			sizeDir(full, agg, visited, maxEntries)
		} else {
			agg.bytes += info.Size()
			agg.files++
		}
	}
}

// KnownTargets are preset cleanup targets ("Say My Name").
func KnownTargets() []DirUsage {
	profile, _ := os.UserHomeDir()
	local := filepath.Join(profile, "AppData", "Local")
	roaming := filepath.Join(profile, "AppData", "Roaming")
	candidates := []string{
		filepath.Join(profile, ".gemini", "antigravity", "worktrees"),
		filepath.Join(profile, ".gemini", "antigravity", "brain"),
		filepath.Join(profile, ".gemini", "antigravity", "conversations"),
		filepath.Join(profile, ".bun", "install", "cache"),
		filepath.Join(profile, ".cargo", "registry", "cache"),
		filepath.Join(local, "@opencodedesktop-updater"),
		filepath.Join(local, "@genieworkbuddy-desktop-updater"),
		filepath.Join(local, "antigravity-updater"),
		filepath.Join(local, "Google", "Chrome", "User Data", "Default", "Cache"),
		filepath.Join(local, "Microsoft", "Edge", "User Data", "Default", "Cache"),
		filepath.Join(local, "npm-cache"),
		filepath.Join(local, "Temp"),
		filepath.Join(profile, "AppData", "Local", "pnpm"),
		filepath.Join(roaming, "npm-cache"),
		filepath.Join(local, "ms-playwright"),
		filepath.Join(local, "ms-playwright-mcp"),
	}
	if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
		candidates = append(candidates, filepath.Join(cacheDir, "go-build"))
	}
	if runtime.GOOS != "windows" {
		candidates = append(candidates,
			"/tmp",
			filepath.Join(profile, ".cache", "go-build"),
			filepath.Join(profile, ".npm"),
			filepath.Join(profile, ".cache", "pip"),
			filepath.Join(profile, ".cache", "uv"),
			filepath.Join(profile, ".cache", "ms-playwright"),
			"/var/cache/apt/archives",
		)
	}

	seen := make(map[string]struct{}, len(candidates))
	out := make([]DirUsage, 0, len(candidates))
	for _, p := range candidates {
		cleaned := filepath.Clean(p)
		key := strings.ToLower(cleaned)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		if fi, err := os.Lstat(cleaned); err != nil || !fi.IsDir() {
			continue
		}
		var agg dirAgg
		visited := 0
		sizeDir(cleaned, &agg, &visited, 200000)
		u := DirUsage{Path: cleaned, Name: filepath.Base(cleaned), Bytes: agg.bytes, FileCount: agg.files, Depth: 0}
		u.Category, u.Safety = ClassifyPath(cleaned)
		out = append(out, u)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	return out
}

// CleanPaths validates each target, then dry-runs or deletes its CONTENTS
// (never the directory itself). Busy files are skipped and counted.
func CleanPaths(ctx context.Context, paths []string, dryRun bool) (CleanResult, error) {
	result := CleanResult{Paths: paths, DryRun: dryRun, Errors: []string{}}
	for _, p := range paths {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if blocked, reason := ProtectedPath(p); blocked {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: blocked (%s)", p, reason))
			continue
		}
		cleaned, err := filepath.Abs(filepath.Clean(p))
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		if dryRun {
			var agg dirAgg
			visited := 0
			sizeDir(cleaned, &agg, &visited, 200000)
			result.FreedBytes += agg.bytes
			result.DeletedFiles += agg.files
			continue
		}
		freed, files, busy, errs := cleanDirContents(ctx, cleaned)
		result.FreedBytes += freed
		result.DeletedFiles += files
		result.SkippedBusy += busy
		result.Errors = append(result.Errors, errs...)
	}
	result.RecordID = RecordHistory(result)
	return result, nil
}

func cleanDirContents(ctx context.Context, dir string) (freed, files, busy int64, errs []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		errs = append(errs, fmt.Sprintf("%s: %v", dir, err))
		return
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		full := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			var agg dirAgg
			visited := 0
			sizeDir(full, &agg, &visited, 200000)
			if rmErr := os.RemoveAll(full); rmErr != nil {
				busy++
				errs = append(errs, fmt.Sprintf("%s: busy or locked", full))
			} else {
				freed += agg.bytes
				files += agg.files
			}
		} else {
			info, statErr := os.Lstat(full)
			if statErr != nil {
				continue
			}
			if rmErr := os.Remove(full); rmErr != nil {
				busy++
			} else {
				freed += info.Size()
				files++
			}
		}
	}
	return
}
