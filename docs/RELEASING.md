# Release runbook

## One-time GitHub setup

Create a protected `release` environment and add these secrets:

- `APPLE_CERTIFICATE_BASE64`: Developer ID Application certificate exported as PKCS#12 and base64 encoded.
- `APPLE_CERTIFICATE_PASSWORD`: password used for the PKCS#12 export.
- `APPLE_SIGNING_IDENTITY`: full `Developer ID Application: …` identity shown by `security find-identity -v -p codesigning`.
- `APPLE_ID`: Apple developer account used by the notary service.
- `APPLE_APP_PASSWORD`: app-specific password for that Apple ID.
- `APPLE_TEAM_ID`: Apple Developer team identifier.

Require approval for the environment if releases should have a human gate. Protect `main`, require CI and CodeQL, require reviewed pull requests, block force pushes, and enable private vulnerability reporting.

## Cut a release

1. Update `VERSION`, the defaults in `internal/buildinfo`, and both version values in `packaging/Info.plist`.
2. Move entries from `Unreleased` into a dated `## [vX.Y.Z]` section in `CHANGELOG.md`.
3. Run `make check` and `make release-dry-run` on macOS.
4. Merge the release change to `main`.
5. Create and push a signed tag: `git tag -s vX.Y.Z -m "Key Session X.Y.Z"`.

The tag workflow validates version alignment, reruns checks, builds Intel and Apple Silicon binaries, creates a universal app, signs with hardened runtime, notarizes and staples it, produces an SBOM and checksums, attests the artifacts, and publishes generated release notes.

## Verify a published release

```bash
shasum -a 256 -c checksums.txt
codesign --verify --deep --strict --verbose=2 "Key Session.app"
spctl --assess --type execute --verbose=4 "Key Session.app"
gh attestation verify key-session_X.Y.Z_macos_universal.zip --repo theronburger/key-session
```
