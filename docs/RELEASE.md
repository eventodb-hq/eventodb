# Release Process

## Steps

1. Update `VERSION` file with new version number
2. Update `CHANGELOG.md` with release notes
3. Commit changes:
   ```bash
   git add VERSION CHANGELOG.md
   git commit -m "Chore: bump version to X.Y.Z"
   ```
4. Push to main:
   ```bash
   git push origin main
   ```
5. Create and push tag:
   ```bash
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```

## What Happens

Pushing the tag triggers `.github/workflows/release.yml` which:

1. Runs GoReleaser (`.goreleaser.yaml`)
2. Builds binaries for:
   - Linux (amd64, arm64)
   - macOS (amd64, arm64)
   - Windows (amd64, arm64)
3. Embeds version info via ldflags (`-X main.version=...`)
4. Creates GitHub release with artifacts and checksums

## Version Format

- Tag format: `vX.Y.Z` (e.g., `v0.5.1`)
- VERSION file: `X.Y.Z` (no `v` prefix)

## Verify Release

After release:
```bash
./eventodb --version -f
```

Should show version, commit hash, and build date.
