# Key Session contributor contract

The parent `AGENTS.md` contains the general coding rules. These rules apply specifically to this repository.

## Pull requests

- Use Conventional Commit titles: `type: summary` or `type(scope): summary`.
- Allowed types are `feat`, `fix`, `docs`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, and `revert`.
- Add `!` before the colon for a breaking change, for example `feat!: replace the daemon contract`.
- Use `feat` for user-visible capabilities, `fix` for bug fixes, and the narrowest non-release type for maintenance work.
- Keep the title accurate because squash merges make it the commit Release Please uses to calculate the next version and changelog.
- Do not manually bump versions or add a release changelog section in ordinary feature PRs. Release Please owns those changes in its release PR.

## Releases

- Merges to `main` update an automated Release Please PR; they do not publish immediately.
- `feat` increments the minor version, `fix` increments the patch version, and a title with `!` increments the major version.
- The release PR must keep `VERSION`, `internal/buildinfo/info.go`, `packaging/Info.plist`, `.release-please-manifest.json`, and `CHANGELOG.md` aligned.
- GitHub creates native PR checks for Release Please's workflow-created PR, but holds them for maintainer approval as a security boundary. Approve those checks from the PR before merging; do not replace them with `workflow_dispatch` runs because dispatched checks do not satisfy PR rulesets.
- Merge the release PR only when its version and notes describe everything intended for the release.
- The merge creates a draft GitHub release and invokes the signed publishing workflow. The protected `release` environment remains the human approval gate.
- Before approving publication, verify the tag, version diff, changelog, and workflow changes. The publishing workflow runs the full checks, signs the app and Sparkle feed, publishes the GitHub release, and updates Homebrew.
- Use manual tags only for recovery according to `docs/RELEASING.md`.

## Verification

- Run `make check` for every change.
- Run `make release-dry-run` after changing Swift UI, packaging, version metadata, daemon contracts, signing, updates, or native Keychain code.
