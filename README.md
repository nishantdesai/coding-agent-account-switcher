# coding-agent-account-switcher

`ags` is a Go CLI to save and switch auth profiles for coding-agent tools.

## V1 command model

- `ags save <tool> <label>` (also supports `--label` / `-l`)
- `ags use <tool> <label>` (also supports `--label` / `-l`)
- `ags delete <tool> <label>` (also supports `--label` / `-l`)
- `ags list [tool] [--verbose]`
- `ags active [tool] [--verbose]`
- `ags version`
- `ags help [command]`

Supported tools in v1:

- `codex`
- `pi`

Codex support is available in two places:
- Tool mode: `ags save codex ...` / `ags use codex ...`
- Pi provider mode: `ags save pi ... --provider codex` / `ags use pi ... --provider codex`

## What each command does

- `save`: copies the current tool auth file into AGS-managed storage under a label.
  - For `pi`, you can optionally save only one provider with `--provider` (for example `codex` or `anthropic`).
- `use`: writes the saved labeled snapshot back into that tool's runtime auth file.
  - For `pi`, AGS now merges provider keys from the saved snapshot into the existing runtime file, so unrelated providers are preserved.
  - For `pi`, you can optionally apply only one provider from a saved snapshot with `--provider`.
- `delete`: removes a saved labeled snapshot and its state metadata for that tool.
- `list`: shows saved labels grouped by tool with compact human-readable status lines.
  - Use `--verbose` for account/timestamp/snapshot/detail lines.
  - Use `--plain` for script-friendly tab-separated rows (`--no-headers` optional).
- `active`: shows which saved label currently matches each tool runtime auth file.
- `version`: prints CLI version.

## Refresh signal behavior

`ags use ...` prints whether the saved snapshot is:

- `first use`
- `unchanged since last use`
- `changed since last use (likely refreshed)`

This is based on snapshot hash differences between uses.

## Default runtime auth paths

- `codex`: `~/.codex/auth.json`
- `pi`: `~/.pi/agent/auth.json`

You can override paths:
- `ags save codex work --source /path/to/auth.json`
- `ags use codex work --target /path/to/auth.json`
- `ags save pi work --source /path/to/auth.json`
- `ags use pi work --target /path/to/auth.json`

Pi provider-scoped examples:
- `ags save pi codex-work --source /path/to/pi-auth.json --provider codex`
- `ags save pi anthropic-work --source /path/to/pi-auth.json --provider anthropic`
- `ags use pi codex-work --provider codex`
- `ags use pi anthropic-work --provider anthropic`

Script-friendly list output (inspired by tools like `jira-cli`):
- `ags list --plain`
- `ags list codex --plain --no-headers`

## Data storage

AGS stores data under `~/.config/ags`:

- `state.json` metadata
- `snapshots/<tool>/<label>.json` auth snapshots

## Build

```bash
go build -o ags ./cmd/ags
```

## Release Readiness (GitHub + Homebrew)

Current gaps to close before a clean public release:

1. CI pipeline
- Add GitHub Actions for `go test ./...` and optional lint/format checks on PRs.

2. Versioned release process
- Define tag flow (`v0.1.0`, etc.) and changelog process.
- Ensure `ags version` is set from build metadata or release automation.

3. Homebrew packaging automation
- Add `.goreleaser.yaml` for cross-platform archives + checksums.
- Create/maintain a Homebrew tap with a formula that consumes release artifacts.

4. Install docs
- Add README install section with:
  - `brew tap ...`
  - `brew install ...`
  - direct binary download fallback.

5. Repo hygiene
- Add `LICENSE` (if not already planned) and contribution guidance (`CONTRIBUTING.md`) for OSS onboarding.

These are mostly packaging/automation tasks; core CLI behavior is already in place.

## Security notes

- Snapshot and state files are written with `0600`.
- This repo stores real auth snapshots on disk; keep your machine and backups encrypted.
- For a future version, move secret payloads to macOS Keychain and keep only references in `state.json`.

## Learning guide

See `CONCEPTS.md` for concepts used in the implementation and what to study next.
