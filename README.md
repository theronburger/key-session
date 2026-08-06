# key-session

`key-session` gives local tools and coding agents time-limited access to secrets stored in macOS Keychain.

## Install

```bash
go build -o ~/.local/bin/key-session ./cmd/key-session
```

## Use

Store a secret once under a named profile. The terminal prompt is hidden:

```bash
key-session setup production-read-only --env MONGODB_URI --duration 1h
```

Approve a one-hour lease through the native macOS Keychain password prompt:

```bash
key-session grant production-read-only
```

Run a program with the secret injected into its configured environment variable:

```bash
key-session exec -- api-client fetch /resource
```

Inspect or end the lease:

```bash
key-session status
key-session revoke
```

Run `key-session help` or `key-session <command> --help` for complete examples. `status` and `profiles` support `--json` for agents.

The secret is encrypted in the macOS login Keychain at rest. Each grant requires the native Keychain password prompt. During a lease the secret exists in the memory of a per-user broker and is never placed in process arguments or written to disk. Exact secret values and URI passwords are redacted from captured child output as a last line of defense; callers should still avoid printing credentials.
