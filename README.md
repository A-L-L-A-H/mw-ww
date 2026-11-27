# mw-ww — Native Windows disk sweeper with strict R0–R4 safety tiers.

**Repository:** [`A-L-L-A-H/mw-ww`](https://github.com/A-L-L-A-H/mw-ww) · **Stack:** Go 1.27 · Wails/WebView2 · React/Vite

## What it does

Audits drives and regenerable caches, then removes safe targets only with an explicit dry-run and confirmation.

## Quick start

```bash
go run ./cmd/mw-ww-cli -audit
```

## Verify

```bash
go test ./...
(cd frontend && bun run typecheck)
```

## Configuration

- **Key configuration:** CSV lab notes in %APPDATA%\mw-ww

```bash
go build -tags "production" -ldflags "-H windowsgui" -o mw-ww.exe ./cmd/mw-ww
```

## Notes

The headless CLI is dry-run by default; -apply is required to delete. R0 is read-only, R4 is never selectable.
