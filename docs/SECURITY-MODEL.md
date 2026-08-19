# Security model

## Goal

key-session lets a local user approve short-lived secret access for one specific agent task and reason without exposing the secret to the requesting agent, shell history, process arguments, or disk.

## Trust boundaries

1. macOS login Keychain encrypts the stored secret, while LocalAuthentication gates human profile management with Touch ID.
2. Lease requests put the self-declared consumer label and reason directly in the native Touch ID sheet.
3. A persistent per-user daemon is the sole owner of profile metadata, Keychain operations, active secret memory, expiry, and child execution.
4. The Swift app, CLI, and MCP server use a schema-v2 loopback API discovered through an owner-only runtime descriptor and authenticated with a high-entropy daemon bearer token.
5. Agent-facing lease operations additionally require an ephemeral consumer capability. Execution also requires the exact lease ID owned by that consumer.
6. The daemon starts one requested child process with a minimal allowlist of ordinary process variables plus the selected lease's secret, then captures its output for redaction.

Lease and execution APIs never return secrets. Human profile management is the deliberate exception. Clicking a profile's pencil opens an editor with profile metadata but no secret. Clicking the eye invokes biometrics-only LocalAuthentication: Touch ID must succeed, and the sheet has no password fallback. The Keychain item trusts only Key Session's signed executable, so the approved daemon can read it without a second Keychain dialog. After that single system authorization, the daemon returns the profile's secret and an unguessable five-minute management capability to the native app. The secret may be hidden again, is never persisted by the app, and is cleared from view state when the editor closes. Save and delete consume the capability; cancel invalidates it.

LocalAuthentication and the subsequent file-based Keychain ACL read are separate application-enforced steps; the authentication context is not cryptographically bound to the Keychain item. Published builds use one persistent self-signed release certificate so the executable requirement remains stable across updates. This identity is not anchored by Apple and therefore does not provide Developer ID trust or notarization. Ad-hoc development builds do not provide stable identity. The CLI asks launchd to start only the installed helper, keeping one supervisor and one canonical signed identity for profile creation and reads; users must open Key Session.app once before using the CLI.

The self-signed identity has no Apple Team ID. The SwiftUI frontend therefore carries `com.apple.security.cs.disable-library-validation` so hardened runtime can load the bundled, independently re-signed Sparkle framework. The daemon, helper, and CLI do not carry that entitlement. This weakens same-user framework injection resistance for the frontend; it does not broaden daemon or Keychain authorization, and downloaded updates still require the embedded Sparkle Ed25519 signature before extraction.

This gate prevents unattended secret reveal and mutation because the editor cannot receive a secret or management capability without Touch ID. Opening the metadata-only editor does not authenticate. This is not cryptographic caller attestation: another process running as the same macOS user can reach the authenticated loopback API and trigger the same visible prompt. Users must deny unexpected management prompts. Strong proof that the caller is the signed app would require an audit-token-aware XPC boundary rather than the shared loopback transport.

## Consumer and lease scope

The first approved lease request for an agent task creates a random `ksc_…` consumer capability. The daemon returns it once, retains only its SHA-256 hash in memory, and never includes it in status, the native frontend, audit events, or files. The agent retains the capability in its task context and presents it to future grant, status, execution, and revocation operations.

A consumer defaults to a 24-hour absolute lifetime, configurable from one hour to seven days. It can own independent leases for multiple profiles. A lease is separately bounded from one minute to 24 hours and can never outlive its consumer. Re-approving the same profile replaces only that profile's lease inside the same consumer. Other consumers are unaffected.

Every execution requires both the consumer capability and the exact non-secret lease ID returned by approval. Consumer status returns only that consumer's metadata. A consumer may revoke one owned lease or end itself and all its leases; it cannot execute with or revoke another consumer's lease through agent APIs. Consumer expiry or daemon shutdown zeroes all owned secret buffers.

The native app receives the complete metadata-only hierarchy and may administratively revoke any consumer or lease. Because the current loopback transport authenticates same-user clients with a shared daemon token, another same-user process could directly invoke that metadata-only administrative revocation and cause denial of service. It still cannot execute with a lease without the corresponding consumer capability. Strong attribution of native-app administration would require an audit-token-aware XPC boundary.

## Agent connection management

Connection Doctor can install the bundled `using-keys` skill and register Key Session's MCP server with detected Codex and Claude Code clients. The daemon performs these mutations through each agent's exact installed CLI, supplies an exact helper path and `mcp` argument, and never writes a consumer capability or Keychain secret into agent configuration. Configuration files must be owner-controlled regular files; symlinked, group-writable, world-writable, oversized, or structurally unsupported files are refused. Skill trees containing links or unsupported files are also refused.

This is an administrative same-user capability, not an agent-facing authorization boundary. Any process able to read the owner-only daemon descriptor can ask the loopback API to register or repair the MCP entry and replace the managed installed skill tree, including local edits inside it. It can also cause the daemon to execute a detected user-owned agent CLI for that exact operation. This cannot grant a lease or reveal a secret, but it can change agent configuration. An audit-token-aware XPC boundary would be required to prove that a repair originated in the native frontend.

Consumer capabilities deliberately identify tasks, not products. Two Codex tasks receive different capabilities, as do Codex and Claude. The capability must never be embedded in static MCP configuration. The agent host may retain MCP tool results in its task transcript; a different agent able to inspect that transcript while the capability remains valid can impersonate the consumer. That is an accepted limitation of this local, lightweight threat model.

## Protections

- Profile metadata, audit metadata, and runtime discovery files use owner-only permissions.
- Symlinked configuration and runtime directories are rejected.
- The daemon binds only to `127.0.0.1`, rejects browser-origin requests, requires bearer authentication, disables caching, and rejects redirects in clients.
- Consumer duration is bounded from one hour to seven days; lease duration is bounded from one minute to 24 hours.
- Consumer labels and reasons are mandatory when applicable, length-bounded, and reject control characters.
- Consumer capabilities are 256-bit random bearer values, stored only as hashes in daemon memory, and invalidated by daemon restart.
- Agent execution and revocation require consumer ownership; execution also requires an exact lease ID.
- Dangerous loader and interpreter environment variable names are rejected.
- Exact secrets and URI passwords are redacted from captured stdout and stderr. Encoded or otherwise transformed values are not guaranteed to match.
- Captured stdout and stderr are bounded to prevent a child process from exhausting daemon memory.
- Sparkle checks a signed feed, verifies the Ed25519 signature before extraction, and replaces the app only after an explicit update action.
- Agent connection repair uses exact CLI argument vectors, validates private configuration and skill paths, and leaves a concurrent configuration change untouched rather than overwriting it during rollback.
- The audit journal records request context and lifecycle metadata only; it never records secrets, child arguments, or child output.

## Explicit non-goals

- Consumer labels and reasons are context; the bearer capability, not the label, provides task isolation.
- A task or process that obtains another consumer's capability can impersonate it until expiry.
- A process running as the same macOS user may be able to inspect another process with sufficient local privileges.
- Output redaction is defense in depth, not a substitute for programs avoiding credential output.
- The self-signed release identity is not Apple-trusted; installation requires an explicit Gatekeeper bypass and cannot provide Developer ID or notarization assurance.
- key-session does not make unsafe database queries or destructive commands safe.
- A compromised user account, malicious administrator, kernel compromise, or malicious child executable is outside the protection boundary.

## Release integrity

Release automation builds a universal binary from a version tag, applies timestamped hardened-runtime signing with the persistent self-signed release certificate, signs the Sparkle archive and feed with a separate Ed25519 key, verifies the final nested signature and Sparkle linkage, publishes checksums and an SBOM, creates a GitHub build-provenance attestation, and updates the Homebrew Cask. The protected `release` environment requires human approval before its publishing secrets become available. The project cannot use Apple notarization until it obtains a Developer ID identity.
