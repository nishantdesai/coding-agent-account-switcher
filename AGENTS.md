# AGENTS.md

Project memory and contributor guide for `coding-agent-account-switcher`.

## What We Decided

- Build this project in Go (not Python/Swift) to balance learning, shipping speed, and binary distribution.
- Keep v1 intentionally simple:
  - `ags save <tool> --label <label>`
  - `ags use <tool> --label <label>`
  - `ags list [tool] [--verbose]`
- v1 tools are exactly:
  - `codex`
  - `claude`
  - `pi`
- Cross-tool linking/composition (for example, codex/claude -> pi) is deferred to a later release.

## Product Intent

- Manage multiple account snapshots (work/personal/etc.) for coding-agent CLIs.
- Make switching explicit and understandable from command names.
- Show account health signals:
  - token status (`valid`, `expiring_soon`, `expired`, `unknown`)
  - `needs_refresh` (`yes`, `no`, `unknown`)
  - change signal on use (`first use`, `unchanged`, `changed since last use`)

## Current Implementation Notes

- Entry point: `cmd/ags/main.go`
- Core logic: `internal/ags/`
  - `cli.go` command parsing and output
  - `manager.go` save/use/list and state management
  - `inspect.go` auth/expiry inspection logic
  - `files.go` filesystem helpers and atomic writes
  - `types.go` shared types/state structs
- Data root default: `~/.config/ags`
  - metadata: `~/.config/ags/state.json`
  - snapshots: `~/.config/ags/snapshots/<tool>/<label>.json`
- File security:
  - auth snapshots and state are written with `0600`
  - created directories use `0700`

## Runtime Auth Paths (v1 defaults)

- Codex: `~/.codex/auth.json`
- Pi: `~/.pi/agent/auth.json`
- Claude save candidates (in order):
  - `~/.claude.json`
  - `~/.claude/auth.json`
  - `~/.config/claude/auth.json`
  - `~/.claude.json.backup`

`--source` and `--target` can override file paths when needed.

## Important Constraints

- Auth files must be strict JSON (no comments).
- `.jsonc` files in this repo are sample templates only.
- Labels are restricted to: `[a-zA-Z0-9._-]+`
- This environment did not have Go installed at scaffold time, so build/test verification was not run here.

## Security and Privacy Expectations

- Never commit real auth snapshots or tokens.
- Treat `~/.config/ags` contents as sensitive secrets.
- Prefer eventual migration to Keychain-backed secret storage in a future release.

## Near-Term Roadmap

1. Add tests for save/use/list flows and expiry parsing edge cases.
2. Improve Claude adapter reliability (confirm canonical auth source).
3. Add `link` command for profile composition (codex + claude -> pi).
4. Add Homebrew + Goreleaser packaging.
5. Add optional auto-refresh workflow where supported by tool auth mechanisms.

## Useful Commands

```bash
# Build
go build -o ags ./cmd/ags

# Save current auth into labeled snapshots
./ags save codex --label work
./ags save claude --label personal
./ags save pi --label work

# Switch to a saved snapshot
./ags use codex --label work
./ags use pi --label work

# Inspect inventory and status
./ags list
./ags list codex --verbose
```

## Naming Context

- Repo name: `coding-agent-account-switcher` (SEO-friendly)
- CLI binary name: `ags` (short, ergonomic)

