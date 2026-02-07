# AGENTS.md

Project memory and contributor guide for `coding-agent-account-switcher`.

## What We Decided

- Build this project in Go (not Python/Swift) to balance learning, shipping speed, and binary distribution.
- Keep v1 intentionally simple:
  - `ags save <tool> <label>` (also supports `--label`)
  - `ags use <tool> <label>` (also supports `--label`)
  - `ags delete <tool> <label>` (also supports `--label`)
  - `ags list [tool] [--verbose]`
- v1 tools are exactly:
  - `codex`
  - `pi`
- Cross-tool linking/composition (for example, codex + pi -> unified profile) is deferred to a later release.

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

`--source` and `--target` can override file paths when needed.

## Important Constraints

- Auth files must be strict JSON (no comments).
- `.jsonc` files in this repo are sample templates only.
- Labels are restricted to: `[a-zA-Z0-9._-]+`
- Go is installed in this environment and local `go build` plus basic CLI smoke tests have been run.

## Git Commit Policy

- Commit as soon as a coherent, reviewable unit of work is complete.
- Prefer small commits over large batched commits.
- Do not mix unrelated changes in one commit.
- Commit docs/help updates separately when they are independent of code changes.
- Use clear commit messages that describe user-visible impact.
- If uncertain whether a change is "reasonable" to commit, default to committing.

## Security and Privacy Expectations

- Never commit real auth snapshots or tokens.
- Treat `~/.config/ags` contents as sensitive secrets.
- Prefer eventual migration to Keychain-backed secret storage in a future release.

## Near-Term Roadmap

1. Add tests for save/use/list flows and expiry parsing edge cases.
2. Add `link` command for profile composition (codex + pi -> unified profile).
4. Add Homebrew + Goreleaser packaging.
5. Add optional auto-refresh workflow where supported by tool auth mechanisms.

## Useful Commands

```bash
# Build
go build -o ags ./cmd/ags

# Save current auth into labeled snapshots
./ags save codex work
./ags save pi personal

# Switch to a saved snapshot
./ags use codex work
./ags use pi personal

# Delete a saved snapshot
./ags delete codex work

# Inspect inventory and status
./ags list
./ags list codex --verbose
```

## Naming Context

- Repo name: `coding-agent-account-switcher` (SEO-friendly)
- CLI binary name: `ags` (short, ergonomic)
