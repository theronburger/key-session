# Key Session native design QA

- Final native capture: `design-qa/final-overview.png`
- Side-by-side comparison: `design-qa/overview-comparison.png`
- Consolidated Profiles capture: `design-qa/final-consolidated-profiles.png`
- Consolidation comparison: `design-qa/consolidated-profiles-comparison.png`
- Final locked editor capture: `design-qa-profile-editor.png`
- Capture viewport: 1180 × 760 points, current system appearance

## Visual review

- The sidebar now has clear top breathing room, a neutral grey selected state, and a lighter grey hover state.
- Teal action styling is gone; profile creation and management use neutral native controls while connection health is green and failure health is red.
- The meaningless `DEFAULT` label and all app-initiated `Request Access` actions are gone.
- Profile rows use the extracted Key Session gold key asset instead of the generic key symbol.
- Overview and gated Profiles management were inspected from the freshly built app using live daemon/profile metadata.
- The final navigation contains only Profiles, Activity, and Connection Doctor. The former Overview content now anchors Profiles, the redundant Manage link is gone, and every stored profile exposes a compact pencil action.
- Profile keys are rotated 90° clockwise, and edit actions use the native `square.and.pencil` SF Symbol.
- The profile editor now opens immediately with metadata and a fixed masked placeholder. Its eye is the only control that requests authentication; save and removal remain disabled until that succeeds.
- The duplicate `Secret` prompt is gone. The locked editor shows one label, one masked value, and one eye control.
- Activity rows use semantic native symbols with green/red/neutral status rather than an unexplained blue dot, and their relative times refresh every second.
- No clipped labels, overlapping controls, malformed cards, accidental scrollbars, or inconsistent corner treatments were observed at the target viewport.

## Interaction review

- Live daemon connection loaded four existing profiles through the authenticated contract.
- Profiles → Pencil → locked editor passed without presenting any authentication UI.
- The eye invokes one biometrics-only LocalAuthentication check. Profiles require Touch ID with no password fallback, then use a signed-app Keychain ACL for a silent read.
- Automation activated the eye only to display the human authentication boundary. It did not enter credentials, read or copy a secret, modify fields, save, or delete.
- The five-minute single-use capability is created only after successful authentication and remains required for save/delete.
- Connection Doctor passed daemon availability plus on-disk runtime, descriptor, configuration, and audit-journal permission checks.
- CLI metadata/status reads, Go tests, Swift tests, daemon helper self-update, code-signature validation, and the native release bundle passed independently.

Final result: passed

## v0.4 consumer-isolation follow-up

- The Profiles screen now renders every ephemeral consumer session, including sessions with no current lease, and nests each exact lease under its owning consumer.
- Consumer and lease expiries update live. Humans can revoke one lease or end an entire consumer from the native app without receiving the capability or secret.
- The menu bar summarizes multiple leases and also surfaces an idle consumer whose leases have expired.
- The installed schema-v2 app and helper completed a live two-consumer test with one Codex task and one Claude task. Both same-profile leases coexisted, each capability-scoped status returned only its own lease, and a cross-consumer execution attempt was rejected before child launch.
- Both demo consumers were ended after the test. Connection Doctor reported zero consumers, zero active leases, Touch ID-only approval, and a healthy daemon.
