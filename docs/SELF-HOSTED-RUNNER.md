# Self-hosted GitHub Actions runner

Key Session intentionally runs every GitHub Actions job on one repository-scoped Apple Silicon Mac. GitHub is the queue and status surface; it supplies no build compute. This preserves the Swift 6.2-or-newer toolchain used to develop the app instead of weakening the package for an older hosted image.

## Install and start

The setup script downloads the pinned official macOS ARM64 runner into the ignored `.github-runner/` directory, verifies its SHA-256 digest, registers it only with the current private repository, installs its launchd service, starts it, and enables the repository's versioned Git hooks:

```bash
scripts/local-runner.sh setup
```

Manage it with:

```bash
scripts/local-runner.sh status
scripts/local-runner.sh start
scripts/local-runner.sh stop
```

The runner application updates itself through GitHub. Update the pinned bootstrap version and checksum in `scripts/local-runner.sh` when rebuilding the local installation from scratch.

## Trust boundary

The workflows never use `pull_request`. CI runs on same-repository branch pushes, `main` pushes, manual dispatches, schedules, and release tags. A fork cannot enqueue its code on this Mac. Do not add a `pull_request` or `pull_request_target` trigger while the workflows target this persistent machine.

Keep the repository private while this runner is registered. A self-hosted runner is not an ephemeral sandbox: jobs can access the user account's files and network privileges. The runner must not hold Key Session consumer capabilities or plaintext credentials in its service environment. Release credentials remain GitHub environment secrets and are imported only by the gated tag workflow.

The runner checks out jobs under `.github-runner/_work`, separate from the developer checkout. The pre-commit hook is intentionally non-blocking and warns when the launchd service is offline; a commit remains possible while disconnected, and GitHub queues its workflow until the runner returns.
