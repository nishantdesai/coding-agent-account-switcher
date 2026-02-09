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
4. Update `Formula/ags.rb` to the new version and release checksums.
5. Push the formula update to `main`.

## Homebrew install (simple mode)

```bash
brew tap nishantdesai/coding-agent-account-switcher https://github.com/nishantdesai/coding-agent-account-switcher
brew install nishantdesai/coding-agent-account-switcher/ags
```

## Optional future improvement

Automate formula version/checksum updates on each release.
