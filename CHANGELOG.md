# Changelog

All notable changes are documented here. The project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.4.0] - 2026-08-19

### Added

- Ephemeral consumer capabilities scoped to one agent task, with a 24-hour default lifetime configurable from one hour to seven days.
- Independent per-consumer, per-profile leases and exact lease IDs for execution and revocation.
- Native consumer-session hierarchy showing every active consumer, lease, expiry, reason, and targeted revocation control.
- Capability-isolation tests proving one consumer cannot inspect, execute with, or revoke another consumer's lease through agent APIs.
- A versioned Codex skill for safe consumer creation, exact-lease execution, capability handling, and cleanup.

### Changed

- The first lease request lazily creates its consumer capability; later requests reuse it without adding another setup step.
- CLI and MCP execution now require a consumer capability plus the exact approved lease ID.
- The local daemon contract is schema v2. Schema v1 and the old single-lease broker are intentionally unsupported and removed.
- Leased children inherit only a small allowlist of ordinary process variables plus the approved profile variable.
- CLI clients use launchd to start only the installed Key Session helper, preserving one supervisor and one canonical Keychain ACL identity.
- Connection Doctor verifies runtime, descriptor, configuration, and audit-journal permissions from disk.
- Universal release builds stage transactionally and can be rerun safely against an existing output directory.
- CI, CodeQL, packaging, and release jobs run on ephemeral GitHub-hosted macOS 26 runners with the project Swift toolchain.
- Pull requests and changes to `main` automatically run the full production validation pipeline.

## [v0.3.0] - 2026-08-19

### Added

- Native SwiftUI command center and menu-bar UI for leases, profiles, activity, diagnostics, and settings.
- Persistent, authenticated per-user daemon with a versioned loopback JSON contract.
- Metadata-only local audit journal for grants, revocations, expiries, and profile changes.
- Agent-shaped MCP server with profile, status, grant, execute, and revoke tools.
- Branded background helper app and LaunchAgent installation/repair flow.

### Changed

- The CLI, MCP server, and Swift app are now thin daemon clients; the daemon exclusively owns Keychain operations, active secret memory, profile mutations, and child execution.
- The distributable app now contains a full native frontend while preserving the standalone `key-session` CLI.

## [v0.2.0] - 2026-08-19

### Added

- Contextual lease approval showing the requester, profile, duration, and reason before macOS asks for the login Keychain password.
- Branded Key Session icon for both the contextual approval and system Keychain dialogs.
- Request context in active lease status and JSON output.
- Structured build provenance through `key-session version --json`.
- Cached, privacy-conscious GitHub release checks and explicit `key-session update` command.
- Read-only installation diagnostics through `key-session doctor`.
- Universal macOS release packaging, Developer ID signing, notarization, checksums, SBOMs, and GitHub attestations.
- GitHub Actions CI, CodeQL analysis, vulnerability scanning, linting, Dependabot, and release automation.

### Changed

- `grant` now requires `--requester` and `--reason` so approval requests carry useful context.

## [v0.1.0] - 2026-08-06

### Added

- Named Keychain-backed secret profiles.
- Time-limited in-memory lease broker.
- Child-process execution with environment injection and output redaction.
- Profile status, listing, revocation, and removal commands.
