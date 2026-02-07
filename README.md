# coding-agent-account-switcher

`ags` is a Go CLI to save and switch auth profiles for coding-agent tools.

## V1 command model

- `ags save <tool> --label <name>`
- `ags use <tool> --label <name>`
- `ags list [tool] [--verbose]`

Supported tools in v1:

- `codex`
- `claude`
- `pi`

## What each command does

- `save`: copies the current tool auth file into AGS-managed storage under a label.
- `use`: writes the saved labeled snapshot back into that tool's runtime auth file.
- `list`: shows all saved labels with status, expiry, and refresh-needed signal.

## Refresh signal behavior

`ags use ...` prints whether the saved snapshot is:

- `first use`
- `unchanged since last use`
- `changed since last use (likely refreshed)`

This is based on snapshot hash differences between uses.

## Default runtime auth paths

- `codex`: `~/.codex/auth.json`
- `pi`: `~/.pi/agent/auth.json`
- `claude`: `~/.claude.json` (fallback candidates are checked during `save`)

You can override paths:

- `ags save claude --label work --source /path/to/auth.json`
- `ags use claude --label work --target /path/to/auth.json`

## Data storage

AGS stores data under `~/.config/ags`:

- `state.json` metadata
- `snapshots/<tool>/<label>.json` auth snapshots

## Build

```bash
go build -o ags ./cmd/ags
```

## Security notes

- Snapshot and state files are written with `0600`.
- This repo stores real auth snapshots on disk; keep your machine and backups encrypted.
- For a future version, move secret payloads to macOS Keychain and keep only references in `state.json`.

## Learning guide

See `CONCEPTS.md` for concepts used in the implementation and what to study next.
