//go:build !windows

package sweeper

import (
	"os"
	"syscall"
)

// listDriveRoots returns distinct mounted filesystem roots on Unix systems.
func listDriveRoots() []string {
	candidates := []string{"/", "/home", "/var", "/data", "/mnt"}
	roots := make([]string, 0, len(candidates))
	seenDevs := make(map[uint64]struct{}, len(candidates))

	for _, c := range candidates {
		info, err := os.Stat(c)
		if err != nil || !info.IsDir() {
			continue
		}
		if st, ok := info.Sys().(*syscall.Stat_t); ok {
			dev := uint64(st.Dev)
			if _, exists := seenDevs[dev]; exists {
				continue
			}
			seenDevs[dev] = struct{}{}
		}
		roots = append(roots, c)
	}
	if len(roots) == 0 {
		return []string{"/"}
	}
	return roots
}

// formatDriveName returns the Unix mountpoint unchanged.
func formatDriveName(root string) string {
	return root
}

// driveSpace returns total and free bytes for a Unix mountpoint via statfs.
func driveSpace(root string) (int64, int64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(root, &st); err != nil {
		return 0, 0, false
	}
	total := int64(uint64(st.Blocks) * uint64(st.Bsize))
	free := int64(uint64(st.Bavail) * uint64(st.Bsize))
	if total < 0 || free < 0 {
		return 0, 0, false
	}
	return total, free, true
}
