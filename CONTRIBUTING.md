# Contributing

key-session sits on a credential boundary. Keep changes small, explicit, and easy to audit.

## Prerequisites

- macOS 14 or newer
- Go version declared in `go.mod`
- Xcode command-line tools
- Swift 6.2 or newer

## Local checks

```bash
make check
make app
key-session doctor
```

`make check` is the core Go formatting, module, vet, race-test, Swift test, and build contract used by CI. CI additionally runs golangci-lint, actionlint, govulncheck, CodeQL, and a packaging dry run. Use `make release-dry-run` after changing packaging, icons, version metadata, Swift UI, daemon contracts, or native Keychain code.

GitHub Actions uses ephemeral GitHub-hosted macOS 26 runners. Pull requests and changes to `main` run the same checks as `make check`, plus vulnerability scanning, CodeQL, and a universal packaging dry run.

## Security boundaries

- Never put secrets or consumer capabilities in arguments, files, logs, fixtures, screenshots, consumer labels, or reasons.
- Never forward the daemon's complete launch environment to leased children. Preserve the small allowlist in `internal/execution` and add any new inherited variable deliberately.
- Preserve interactive approval before every Keychain read.
- Preserve consumer ownership checks on status, execution, and agent-facing revocation; require an exact lease ID for execution.
- Consumer capabilities remain memory-only and are stored by hash. Do not add compatibility paths that bypass them.
- Keep launchd as the sole daemon supervisor and the installed helper as its only identity. Do not add standalone, direct-spawn, or migration fallbacks.
- Treat changes to IPC, process execution, redaction, permissions, updates, signing, and release automation as security-sensitive.
- Add tests for every new validation rule and failure mode.
- Do not add background telemetry. Network behavior must be documented, bounded, and user-disableable.

## Pull requests

Explain user-visible behavior, security impact, and verification. CI must pass on all supported runners before merge.

PR titles use Conventional Commits because squash-merged titles drive release versioning and changelog generation. Use `feat: summary` for a user-visible capability, `fix: summary` for a bug fix, or another allowed type documented in `AGENTS.md`. Scopes are optional, as in `fix(menu-bar): preserve the last visible surface`. Add `!` before the colon for a breaking change.

Do not bump versions or edit the release changelog in an ordinary PR. Release Please maintains a separate release PR containing those changes.
