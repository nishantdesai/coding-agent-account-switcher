# Contributing

## Prerequisites

- Go 1.22+
- No real auth data in tests or fixtures

## Local checks

```bash
go test ./...
go test ./... -race
go vet ./...
go build -o ags ./cmd/ags
```

## Rules for auth data

- Never commit real `~/.codex/auth.json` or `~/.pi/agent/auth.json` content.
- Use synthetic JSON auth payloads only.
- Keep any write-path verification pointed to `/tmp` or test temp dirs.

## Commit style

- Keep commits small and coherent.
- Separate docs-only changes from code changes when practical.
- Use clear commit messages focused on user-visible impact.

## Releasing

- See `docs/RELEASING.md`.
