# Concepts Used In This Build

This file maps implementation choices to concepts you can study further.

## CLI architecture

- Subcommands (`save`, `use`, `list`) using Go `flag.FlagSet`
- Command dispatcher pattern in `internal/ags/cli.go`
- UX contracts: explicit required flags (`--label`) and predictable command grammar

## Data modeling and serialization

- `struct` modeling for persistent metadata (`State`, `StateEntry`)
- JSON encode/decode with `encoding/json`
- State file versioning (`Version` field) for future migrations

## Filesystem safety

- Atomic writes (`temp file + rename`) in `internal/ags/files.go`
- Permission hardening (`0600` for secrets, `0700` directories)
- Path expansion and normalization (`~` handling)

## Integrity and change detection

- SHA-256 snapshot hashing for change detection
- Refresh signal logic based on hash transitions:
  - first use
  - unchanged since last use
  - changed since last use

## Token/expiry inspection

- JWT payload parsing by decoding Base64URL token segment
- Reading `exp` claims for Codex access-token expiry
- Provider-specific expiry parsing for Pi (`expires` epoch milliseconds)
- Multi-provider worst-status aggregation (`valid`, `expiring_soon`, `expired`)

## Extensibility patterns

- Tool adapters via a `Tool` enum + per-tool default paths
- Pluggable path candidates and `--source`/`--target` overrides
- Separation between:
  - command parsing (`cli.go`)
  - state and storage (`manager.go`)
  - inspection logic (`inspect.go`)

## Concepts To Read Next

- Go project layout for CLIs (`cmd/`, `internal/`, package boundaries)
- `spf13/cobra` and `charmbracelet` ecosystem for richer CLI UX
- Keychain integration for Go (`go-keyring`)
- JSON schema validation strategies in Go
- Release automation with `goreleaser`
- Homebrew tap + formula + bottles
