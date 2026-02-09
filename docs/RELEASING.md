# Releasing AGS

This project ships release archives with GoReleaser and keeps the Homebrew formula in this repo (`Formula/ags.rb`).

## Release flow

1. Confirm main is green.
2. Tag and push:
```bash
git tag v0.1.0
git push origin v0.1.0
```
3. GitHub Actions `release` workflow will:
- run tests,
- publish release artifacts.

## Homebrew install (simple mode)

```bash
brew install --HEAD https://raw.githubusercontent.com/nishantdesai/coding-agent-account-switcher/main/Formula/ags.rb
```

## Optional future improvement

If you want pinned version installs (`brew install ags` without `--HEAD`), add a dedicated tap repo and automated formula updates later.
