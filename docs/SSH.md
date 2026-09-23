# SSH signing with approved leases

Use this procedure to configure another Mac. The existing secret-profile workflow remains unchanged. SSH profiles instead provide signing through the standard OpenSSH agent protocol, with access controlled by Key Session leases.

## Create and enroll an identity

Install the published app using the [release runbook](RELEASING.md), open it once so launchd installs its helper, and run `key-session doctor`. The app's New Profile sheet offers an SSH signing identity; the equivalent CLI command is:

```sh
key-session ssh setup infrastructure --duration 1h
key-session ssh public-key infrastructure
key-session ssh socket
```

Creation generates an Ed25519 private key inside the daemon and stores it in Keychain. Only the public key is printed. There is no private-key file, export command or editable private-key field. Existing profiles cannot be overwritten with this command. For another Mac, generate another identity and enroll its public key; do not copy Keychain state or private keys.

With the owner's authorization, use an existing trusted administrative session to add the public key to the intended server accounts' `authorized_keys`. Preserve any required restricted-key options on forwarding gateways. Keep recovery access open until the new path passes. Do not replace entire authorized-key files or remove unrelated users' keys.

Save the public key on the client:

```sh
key-session ssh public-key infrastructure > ~/.ssh/infrastructure.pub
chmod 600 ~/.ssh/infrastructure.pub
```

Back up and merge the following into the relevant host blocks, substituting the actual socket path printed above. A quoted absolute path accommodates the space in `Application Support`:

```sshconfig
Host my-server my-gateway
  IdentityFile ~/.ssh/infrastructure.pub
  IdentityAgent "/Users/YOUR_ACCOUNT/Library/Application Support/key-session/runtime/ssh-agent.sock"
  IdentitiesOnly yes
  AddKeysToAgent no
  UseKeychain no
  PreferredAuthentications publickey
  PasswordAuthentication no
  KbdInteractiveAuthentication no
  ForwardAgent no
  StrictHostKeyChecking yes
  ControlMaster no
  ControlPath none
```

Keep independently verified host-key records. `UseKeychain` is an Apple OpenSSH option. A ProxyJump or nested ProxyCommand needs the leased identity configured on **both** its gateway and target blocks. Remove conflicting earlier IdentityAgent/IdentityFile settings; IdentityFile can accumulate across matching blocks. Verify effective settings with `ssh -G`. Do not globally change GitHub, Colima or other identities.

## Approve and use

In the native app, open the SSH profile, choose a duration, then **Approve SSH Access**. Touch ID approves a personal session shown in the normal consumer/lease view. Revoke there when finished.

A coding agent requests a lease using the existing consumer workflow:

```sh
key-session grant infrastructure \
  --consumer "Codex: infrastructure maintenance" \
  --reason "Inspect the approved home servers" \
  --duration 1h
ssh my-server
```

Retain the returned consumer capability only in that task, using `KEY_SESSION_CONSUMER_TOKEN` for subsequent consumer-scoped status/revocation. Do not write it into SSH config, shell history, files or permanent agent configuration. SSH itself does not need the token. `key-session exec` deliberately refuses SSH profiles.

## Scope and lifetime

- Any program running under your macOS account can use the identity while **any** approved lease for it is active. This is not per-process isolation.
- Expiry or revocation of the last lease stops new authentication, including on an already opened agent socket. Restarting the daemon loses all leases.
- Existing shells, tunnels and multiplexed transports can remain usable. These are not terminated by signing expiry. The sample disables multiplexing for predictable new-login behavior.
- Screen lock does not revoke access. Short leases and explicit revocation limit the approved window.
- The signer accepts ordinary and OpenSSH host-bound authentication payloads, but does not enforce its own destination allowlist or agent session bindings. Enrollment and server policy determine where the identity is accepted.

## Verify the migration

1. Before approval, an ordinary SSH attempt with only the new public identity must fail authentication without a passphrase fallback.
2. Approve with Touch ID; verify a real command reaches each expected server with strict host-key checking, including any gateway path.
3. Revoke the lease. With no other lease for that identity, a new connection must fail. Repeat with a one-minute lease and let it expire.
4. Verify the old automatically unlocked identities cannot still reach the protected accounts. Only after successful new-key tests, remove the specifically identified old public-key entries within the approved scope. A separate GitHub key can remain in Keychain because the infrastructure servers no longer authorize it.
5. Test the intended network/VPN setup and local/remote paths. Document untested cases and any deliberately retained recovery identity.

Do not interpret a network timeout as proof of authentication denial. Do not claim lease-only protection if a cached alternative identity still works. Keep any backup of old authorized-key contents out of the live authorized-key path.

## Recovery and rotation

An expired lease needs a fresh human approval. Missing/unresponsive sockets need the installed app's Connection Doctor/repair flow; do not start a second standalone signer or fall back to an unprotected key. Socket paths can change with the macOS account name.

A lost Keychain identity cannot be exported from this interface for recovery. Use a separately agreed recovery path, such as the device console or administrative UI, to enroll a new profile's public key. Verify that path before removing old access. Rotate by creating a new profile, enrolling and testing it, removing the old public keys from servers, then removing the old profile with Touch ID.

## Release acceptance

Changes to this feature require `make check` and `make release-dry-run`, normal PR/Release Please checks, and the protected publishing workflow. Test the signed installed release with real Touch ID before migrating live accounts. Never replace the installed signed helper with a development binary just to bypass that release path.
