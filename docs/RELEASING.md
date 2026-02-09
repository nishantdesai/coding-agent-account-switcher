# Releasing AGS

This project ships release archives with GoReleaser and publishes a Homebrew formula in a separate tap.

## One-time setup

1. Create the tap repo: `nishantdesai/homebrew-ags`.
2. In this repo, add secret `TAP_GITHUB_TOKEN`.
- Use a GitHub fine-grained token with write access to `nishantdesai/homebrew-ags` contents.
3. Ensure `LICENSE` exists (release archives include it).

## Release flow

1. Confirm main is green.
2. Tag and push:
```bash
git tag v0.1.0
git push origin v0.1.0
```
3. GitHub Actions `release` workflow will:
- run tests,
- publish release artifacts,
- update Homebrew formula in `nishantdesai/homebrew-ags`.

## Install commands after release

```bash
brew tap nishantdesai/ags
brew install ags
```

If tap name changes, update `.goreleaser.yaml` and README install instructions.
