# Release runbook

## One-time GitHub setup

Create a protected GitHub environment named `release`, require Theron as a reviewer, and create these **environment secrets** rather than repository-wide secrets:

- `SPARKLE_PRIVATE_KEY`: the private Ed25519 key exported by Sparkle's `generate_keys` tool.
- `HOMEBREW_TAP_DEPLOY_KEY`: a dedicated SSH deploy private key whose public half has write access only to `theronburger/homebrew-tap`.
- `KEY_SESSION_CERTIFICATE_BASE64`: the persistent self-signed code-signing identity exported as PKCS#12 and base64 encoded.
- `KEY_SESSION_CERTIFICATE_PASSWORD`: the PKCS#12 export password.
- `KEY_SESSION_SIGNING_IDENTITY`: the certificate common name (`Theron Burger Apps Release`).

The matching public key is committed as `SUPublicEDKey` in `packaging/Info.plist`. Keep the private key outside the repository.

The Homebrew deploy key is intentionally disposable and scoped to the tap repository; never substitute a broad personal token. The protected environment is the mandatory human gate before any publishing secret becomes available to the job.

The project currently has no Apple Developer identity. Release bundles use one persistent self-signed certificate and cannot be notarized. Homebrew removed its `--no-quarantine` option, so users explicitly remove quarantine from the installed Key Session app after Homebrew verifies and installs the pinned archive. The persistent certificate supplies a stable executable identity for Keychain ACLs across releases; Sparkle separately authenticates update archives and the update feed with Ed25519 before extraction.

## Publishing-key ownership

GitHub Actions secrets are deployment copies, not backups. The recoverable sources of truth are the private items in Theron's personal Keychains:

- **Publisher identity:** `Theron Burger Apps Release`, valid through 2036-08-16, SHA-256 fingerprint `C972D3B2E8DEA42A078BA464ADFC43C95B18A0F6EEEF022A7588B9C954D11F08`. Keep the identity and private key in at least two independently protected personal Keychains.
- **Sparkle update key:** Keychain account `com.theronburger.key-session`, service `https://sparkle-project.org`, public key `QpoIZkdB6VODiziVnNe2wR2EY5RZdRJTdlm6h72BOY4=`. Keep the private seed in the same independently protected locations.

For an offline recovery copy, export the publisher identity—not merely its certificate—from Keychain Access under **My Certificates** as an encrypted `.p12`. Export the Sparkle seed with the matching Sparkle tool:

```bash
generate_keys --account com.theronburger.key-session -x key-session-sparkle-private-key
```

Put both exports and the `.p12` password in an encrypted password manager or offline encrypted volume, then remove the plaintext Sparkle export. Never commit either private key or place it in a release artifact.

The publisher identity is intentionally personal and reusable across Theron's self-signed apps. A future app may also reuse one organization-level Sparkle key, as Sparkle supports one key for multiple apps, or use a separate account/key to reduce compromise blast radius. Whichever policy is chosen, record the Keychain account and public key in that app's release runbook.

If the publisher private key is lost, replacement releases have a new signing identity and may trigger fresh macOS/Keychain approval. If the Sparkle private key is lost, existing installs reject newly signed updates. Rotate a Sparkle key only by first shipping an update signed by the old key that embeds the new public key.

The publisher certificate expires on 2036-08-16. Release signatures are timestamped, but rotate well before expiry: ship an old-identity build that updates any identity-sensitive migration state, retain the old identity through the transition, then validate profile access and Sparkle installation with the replacement identity on a clean Mac.

Version 0.5.0 is the first published binary release, so there is no public Developer ID-to-self-signed migration. Do not change the publisher identity after the first release without an explicit migration plan and release note.

## Cut a release

1. Update `VERSION`, the default in `internal/buildinfo`, and both version values in `packaging/Info.plist`.
2. Move entries from `Unreleased` into a dated `## [vX.Y.Z]` section in `CHANGELOG.md`.
3. Run `make check` and `make release-dry-run` on macOS.
4. Merge the release change to `main`.
5. Create an annotated tag from the reviewed `main` commit: `git tag -a vX.Y.Z -m "Key Session X.Y.Z" && git push origin vX.Y.Z`.
6. Inspect the pending deployment and approve the protected `release` environment only when the tag, workflow diff, and expected version match.

The tag workflow validates version alignment, reruns checks, builds Intel and Apple Silicon binaries, creates a universal app, signs every nested component with the persistent self-signed release identity, produces an SBOM and checksums, generates and signs the Sparkle appcast, attests the artifacts, publishes the GitHub release, and updates the Homebrew Cask.

## Verify a published release

```bash
shasum -a 256 -c checksums.txt
codesign --verify --deep --strict --verbose=2 "Key Session.app"
gh attestation verify key-session_X.Y.Z_macos_universal.zip --repo theronburger/key-session
brew tap theronburger/tap
brew trust --cask theronburger/tap/key-session
brew install --cask key-session
xattr -dr com.apple.quarantine "/Applications/Key Session.app"
```
