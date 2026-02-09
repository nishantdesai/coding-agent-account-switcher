# Security Policy

## Reporting

If you find a security issue, open a private security advisory on GitHub for this repository.

## Data sensitivity

AGS stores auth snapshots and metadata on local disk.

- `~/.config/ags/snapshots/...` contains secret auth payloads.
- `~/.config/ags/state.json` contains metadata and identity cache entries.

Current protections:

- files are written with mode `0600`
- directories are created with mode `0700`
- writes use temp file + atomic rename

## Current security limitations

- Secrets are disk-backed JSON, not OS keychain-backed.
- `--source` and `--target` intentionally allow explicit path override; misuse can overwrite arbitrary files owned by the current user.

## Hardening roadmap

- Add optional keychain-backed storage for secret payloads.
- Add optional `--dry-run` for `use`.
- Add optional confirmation/guardrails for default runtime-path writes.
