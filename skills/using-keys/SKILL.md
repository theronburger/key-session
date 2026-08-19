---
name: using-keys
description: Safely access credentials and other secrets through the local key-session CLI using consumer-scoped, expiring, user-approved leases. Use whenever a task mentions key-session, a named key profile, temporary database or API access, or requires a secret stored in macOS Keychain. Includes MongoDB and mongosh safeguards.
---

# Using Keys

Use `key-session` as the only path from macOS Keychain to a child process. Never reveal, print, persist, copy, or interpolate the Keychain secret itself.

Treat each agent task as a separate consumer. Retain its random consumer capability only in the current task context. Never put it in MCP configuration, files, source control, logs, commentary, or the final response.

## Workflow

1. Confirm the tool and inspect non-secret global metadata:

   ```sh
   command -v key-session
   key-session profiles
   key-session status
   ```

2. If this task does not yet have a consumer capability, ask the user to grant the needed profile unless they already explicitly requested it. Then run:

   ```sh
   key-session grant <profile> \
     --consumer "<agent and current task>" \
     --reason "<specific intended operation>" \
     --duration <lease duration> \
     --consumer-duration <task lifetime>
   ```

   Omit `--consumer-duration` for the 24-hour default. It accepts one hour through seven days. The Touch ID sheet shows the consumer, profile, lease duration, and reason. Capture the returned consumer capability and lease ID in task context without repeating either to the user. Never include secrets in labels or reasons.

   Do not create, replace, or remove profiles unless explicitly requested; those operations change Keychain or configuration state.

3. Before later use, inspect only this consumer by supplying the retained capability through `KEY_SESSION_CONSUMER_TOKEN`, not as a process argument:

   ```sh
   KEY_SESSION_CONSUMER_TOKEN='<consumer capability>' key-session status
   ```

4. Run exactly the required child process through the exact approved lease:

   ```sh
   KEY_SESSION_CONSUMER_TOKEN='<consumer capability>' \
     key-session exec --lease '<lease ID>' --timeout 2m -- <program> <arguments...>
   ```

5. For another profile in the same task, reuse the capability and omit `--consumer`:

   ```sh
   KEY_SESSION_CONSUMER_TOKEN='<consumer capability>' \
     key-session grant <profile> --reason "<specific intended operation>" --duration <duration>
   ```

6. Prefer read-only discovery before writes. Before a material mutation, verify the environment, database or account, and exact target IDs without exposing the credential. Use markers and idempotent filters for disposable test data.

7. Report the consumer label, profile used, target environment, and outcome—never the consumer capability, secret, lease ID, or a connection string containing it.

If the capability is lost or expires, do not attempt recovery from daemon internals or another task. Create a new consumer through another user-approved grant.

## Secret and capability handling

- Read Keychain secrets only from the environment injected into the daemon-launched child process.
- Expect the child to receive only a small process-basics allowlist plus the approved profile variable; pass any additional non-secret configuration explicitly.
- Never place a Keychain secret or consumer capability in command arguments, files, shell history, logs, or tool output.
- A consumer capability in a shell assignment becomes an environment variable, not a child-process argument. The task transcript may retain the tool call; this is the accepted local threat model.
- Lease and consumer IDs are selectors, not authorization secrets, but avoid unnecessary repetition.
- If inline code needs the Keychain value, copy it to a short-lived variable, delete the environment entry, connect, and discard the variable:

  ```sh
  KEY_SESSION_CONSUMER_TOKEN='<consumer capability>' \
    key-session exec --lease '<lease ID>' -- mongosh --nodb --quiet --eval '
      let uri = process.env.MONGODB_URI;
      delete process.env.MONGODB_URI;
      const connection = connect(uri);
      uri = undefined;
      printjson({ ping: connection.runCommand({ ping: 1 }).ok });
      quit(0);
    '
  ```

- Do not inspect Keychain files, daemon memory, runtime bearer tokens, process environments, shell history, another task, or credential stores to bypass key-session.
- Do not rely on output redaction as a containment boundary. Encoded, transformed, or fragmented secret values may not match the redactor.
- Use consumer-scoped `status` before long work. Set `--timeout` high enough for the child command but no broader than needed.

## Environment-variable mapping

Profiles inject the variable configured by `key-session profiles`, for example `MONGODB_URI`. A repository command may expect another name such as `MONGO_URI`.

Map it only inside the leased child process, without printing it:

```sh
KEY_SESSION_CONSUMER_TOKEN='<consumer capability>' \
  key-session exec --lease '<lease ID>' -- sh -c 'MONGO_URI="$MONGODB_URI" exec <program>'
```

Prefer direct program invocation when no mapping is needed. Avoid verbose shell modes such as `set -x`.

## MongoDB sharp edges

- Use an installed `mongosh`; do not download it through `npx` for every invocation.
- A MongoDB URI may default to an empty or unintended database. Explicitly select and verify the expected database after connecting.
- Validate the environment with a ping and harmless identifying or count query before mutation. Do not infer safety merely from the profile name.
- `key-session` controls credential release, not query safety. Use exact `_id` filters, assert discriminators, and inspect `matchedCount` and `modifiedCount`.
- Seed test data with a unique marker and provide cleanup. Cleanup of seeded documents does not reverse downstream effects such as role grants, emails, or webhooks.
- Prefer disposable test users and organizations for workflows with business side effects.

## Lease and consumer cleanup

Allow short leases to expire naturally. Revoke one lease when immediate teardown materially reduces risk:

```sh
KEY_SESSION_CONSUMER_TOKEN='<consumer capability>' key-session revoke --lease '<lease ID>'
```

When the agent task is complete and no child work remains, end its consumer and all owned leases:

```sh
KEY_SESSION_CONSUMER_TOKEN='<consumer capability>' key-session revoke
```

Do not revoke access owned by another consumer. The native app is the human administrative surface for inspecting or ending other consumers.
