# coding-agent-account-switcher

`ags` is a Go CLI for managing multiple auth profiles across coding-agent tools.

It helps you:

- save labeled snapshots (`work`, `personal`, etc.),
- switch profiles explicitly,
- inspect token/account health,
- keep CLI usage simple and scriptable.

## Quick start

Install:

```bash
brew tap nishantdesai/coding-agent-account-switcher https://github.com/nishantdesai/coding-agent-account-switcher
brew install nishantdesai/coding-agent-account-switcher/ags
```

Use the tool:

```bash
# save current auth into labeled snapshots
ags save codex work
ags save pi personal

# switch to a saved snapshot
ags use codex work
ags use pi personal

# inspect saved profiles and active runtime match
ags list
ags active
```

## Supported tools

`ags` currently supports:

- `codex`
- `pi`

Codex can be managed in two ways:

- direct tool mode: `ags save codex ...` / `ags use codex ...`
- pi provider mode: `ags save pi ... --provider codex` / `ags use pi ... --provider codex`

## Install

Homebrew (single-repo formula):

```bash
brew tap nishantdesai/coding-agent-account-switcher https://github.com/nishantdesai/coding-agent-account-switcher
brew install nishantdesai/coding-agent-account-switcher/ags
```

Build from source:

```bash
go build -o ags ./cmd/ags
```

## Command reference

| Command | Purpose |
| --- | --- |
| `ags save <tool> <label>` | Save current runtime auth into a labeled snapshot |
| `ags use <tool> <label>` | Apply a saved snapshot to runtime auth |
| `ags delete <tool> <label>` | Remove a labeled snapshot and metadata |
| `ags list [tool] [--verbose]` | List saved profiles and token/account status |
| `ags active [tool] [--verbose]` | Show which label currently matches runtime auth |
| `ags version` | Print CLI version |
| `ags help [command]` | Show detailed help |

Label flags are also supported on `save`, `use`, and `delete`:

- `--label <name>`
- `-l <name>`

Labels must match: `[a-zA-Z0-9._-]+`

## Pi provider mode

For `pi`, you can save or apply only one provider from the auth file.

Examples:

```bash
ags save pi codex-work --source /path/to/pi-auth.json --provider codex
ags save pi anthropic-work --source /path/to/pi-auth.json --provider anthropic

ags use pi codex-work --provider codex
ags use pi anthropic-work --provider anthropic
```

`ags use pi ...` merges provider keys from the snapshot into the existing runtime file, so unrelated providers are preserved.

## Paths and storage

Default runtime auth paths:

- codex: `~/.codex/auth.json`
- pi: `~/.pi/agent/auth.json`

Path overrides:

- `ags save codex work --source /path/to/auth.json`
- `ags use codex work --target /path/to/auth.json`
- `ags save pi work --source /path/to/auth.json`
- `ags use pi work --target /path/to/auth.json`

Data storage root:

AGS stores data under `~/.config/ags`:

- `state.json` metadata
- `snapshots/<tool>/<label>.json` auth snapshots

Script-friendly list output:

- `ags list --plain`
- `ags list codex --plain --no-headers`

## Security

- Snapshot and state files are written with `0600`.
- This repo stores real auth snapshots on disk; keep your machine and backups encrypted.
- Manager-level validation now enforces tool and label constraints even for non-CLI callers.
- `ags use` now performs rollback of target auth writes if metadata/state persistence fails.
- For a future version, move secret payloads to macOS Keychain and keep only references in `state.json`.

Release publishing details are documented in `docs/RELEASING.md`.
