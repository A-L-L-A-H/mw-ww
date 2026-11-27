# mw-ww build notes

- Go uses the `production` build tag (required by Wails; plain `go build` produces a WebView runtime error dialog).
- The Wails CLI bindings generator is unavailable in this environment (`wailsbindings.exe` cannot execute), so the frontend calls bindings via `window.go?.main?.App` / `window.main?.App` directly. Both paths are checked.
- Frontend imports `@moonwitness/ui` through a workspace-local dependency (`file:../../mw-design/packages/ui`) rather than a published package. If this repo is cloned standalone, pin a published version instead.
- `bun run build` in `frontend/` must be run before `go build` so `frontend/dist` exists for `//go:embed`.
- Windows-only: `drives_windows.go` uses `GetDiskFreeSpaceExW`. Add a `drives_unix.go` with a `//go:build !windows` tag if cross-platform support is ever needed.
