## What changed

<!-- Describe the user-visible behavior and why it belongs in key-session. -->

## Security impact

<!-- Note any change to secret handling, Keychain access, process execution, IPC, permissions, networking, signing, or updates. Write "None" when not applicable. -->

## Verification

- [ ] PR title uses `type: summary` or `type(scope): summary`
- [ ] `make check`
- [ ] App bundle built with `make app` when packaging changed
- [ ] No secret values, credentials, or connection strings appear in code, tests, fixtures, logs, or screenshots
- [ ] User-facing behavior is described clearly; Release Please will update `CHANGELOG.md`
