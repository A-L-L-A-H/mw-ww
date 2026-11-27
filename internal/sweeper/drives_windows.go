//go:build windows

package sweeper

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpW = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// listDriveRoots returns all ready Windows drive roots (e.g. "C:\").
func listDriveRoots() []string {
	roots := make([]string, 0, 8)
	for c := 'A'; c <= 'Z'; c++ {
		root := string(c) + `:\`
		if _, err := os.Stat(root); err == nil {
			roots = append(roots, root)
		}
	}
	return roots
}

// formatDriveName strips trailing backslash for display ("C:" from "C:\").
func formatDriveName(root string) string {
	return strings.TrimSuffix(root, `\`)
}

// driveSpace returns total and free bytes for a drive root.
func driveSpace(root string) (int64, int64, bool) {
	lpRootPath, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return 0, 0, false
	}
	var freeBytes, totalBytes, totalFree int64
	r1, _, _ := procGetDiskFreeSpW.Call(
		uintptr(unsafe.Pointer(lpRootPath)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if r1 == 0 {
		return 0, 0, false
	}
	return totalBytes, freeBytes, true
}
