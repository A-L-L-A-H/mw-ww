# mw-ww — MoonWitness Walter White

> **"I am the one who sweeps."** ∗ *Clean code. Clean disk. Clean conscience.*

A native Windows disk-sweeping utility for the MoonWitness workspace. Walter White is no longer cooking — he is **the one who sweeps**. His laboratory is your hard drive; his product is clean space. Polite, absurdly serious, never destructive without consent.

## Why this exists

`mw-ww` replaces the old Disk Doctor feature in `mw-echo` (cut-over, not a second copy). It keeps the useful scanner semantics and safety tiers while adding what the old tool lacked:

- A real native `.exe` (Go + Wails, WebView2 — no Chromium payload).
- A UI built entirely from **`@moonwitness/ui`** design tokens and components.
- **Process guard**: refuse to sweep folders owned by a running app.
- **Known-target presets**: Antigravity worktrees/brain/conversations, updater orphans, Chrome/Edge caches, npm/pnpm caches, Playwright browsers.
- **Lab Notes**: a plain CSV audit log (lighter than the old SQLite + AI insight stack).
- Windows Cleanup (`cleanmgr`) entry point, with system paths (WinSxS/pagefile) shown as protected information only.

## Safety model (R0–R4)

| Tier | Meaning | UI behavior |
|---|---|---|
| R0 | Read-only scan and drive info | Always available |
| R1 (`safe`) | Regenerable cache / build artifacts | Selectable, dry-run + confirm |
| R2 (`caution`) | App data / worktrees | Selectable, warning + process guard |
| R3 | Blocked while owner app is running | Guard message, cannot select |
| R4 (`protected`) | Windows, Program Files, pagefile, drive roots | Never selectable |

Deletion removes the **contents** of a target folder, never the folder itself. Dry-run is mandatory before any live clean. Locked files are skipped and counted.

## Build

Prerequisites: Go 1.27+, Bun, Windows 10/11 with WebView2 runtime (present on Windows 11).

```bash
cd frontend && bun install && bun run build
cd ..
go build -tags "production" -ldflags "-H windowsgui" -o mw-ww.exe ./cmd/mw-ww
```

Run `mw-ww.exe`. Lab notes are written to `%APPDATA%\mw-ww\lab-notes.csv`.

## Character registry

Walter White (`walter-white`) is registered in `@moonwitness/ui`'s asset registry with four expressions: `calm`, `sweeping`, `gloating`, `victorious`. Portraits resolve to `illustrations/riot-universe/characters/walter-white-expression-*.webp` (with `.png` fallback). The character is decorative unless a consumer gives it an accessible name via `CharacterDisplay`.
