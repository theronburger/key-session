#!/bin/sh
set -eu

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(dirname -- "$script_directory")
runner_directory="$repository_root/.github-runner"
runner_version="2.336.0"
runner_archive="actions-runner-osx-arm64-$runner_version.tar.gz"
runner_url="https://github.com/actions/runner/releases/download/v$runner_version/$runner_archive"
runner_sha256="8e8839c49b7060b6b2154f4931f815df330c27f167d53ef2239ee3dfce28b079"
runner_label="key-session-local"
safe_path="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

usage() {
	printf 'usage: %s <setup|start|stop|status|foreground>\n' "$0" >&2
	exit 2
}

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		printf 'required command is unavailable: %s\n' "$1" >&2
		exit 1
	fi
}

runner_status() {
	if [ ! -x "$runner_directory/svc.sh" ] || [ ! -f "$runner_directory/.service" ]; then
		return 1
	fi
	(
		cd "$runner_directory"
		./svc.sh status
	)
}

download_runner() {
	require_command curl
	require_command shasum
	archive_path=$(mktemp "${TMPDIR:-/tmp}/key-session-runner.XXXXXX")
	trap 'rm -f "$archive_path"' EXIT HUP INT TERM
	mkdir -p "$runner_directory"
	curl --fail --location --silent --show-error "$runner_url" --output "$archive_path"
	printf '%s  %s\n' "$runner_sha256" "$archive_path" | shasum -a 256 --check
	tar -xzf "$archive_path" -C "$runner_directory"
}

setup_runner() {
	require_command gh
	require_command git
	require_command jq

	if [ ! -x "$runner_directory/bin/Runner.Listener" ]; then
		download_runner
	fi

	repository=$(gh repo view --json nameWithOwner,url,isPrivate)
	repository_name=$(printf '%s' "$repository" | jq -r '.nameWithOwner')
	repository_url=$(printf '%s' "$repository" | jq -r '.url')
	repository_private=$(printf '%s' "$repository" | jq -r '.isPrivate')
	if [ "$repository_private" != "true" ]; then
		printf 'refusing to register a persistent self-hosted runner for a public repository\n' >&2
		exit 1
	fi

	if [ ! -f "$runner_directory/.runner" ]; then
		registration_token=$(gh api --method POST "repos/$repository_name/actions/runners/registration-token" --jq '.token')
		host_name=$(scutil --get LocalHostName 2>/dev/null || hostname -s)
		(
			cd "$runner_directory"
			PATH="$safe_path" ./config.sh \
				--unattended \
				--url "$repository_url" \
				--token "$registration_token" \
				--name "key-session-$host_name" \
				--labels "$runner_label" \
				--work _work \
				--replace
		)
		registration_token=""
	fi

	if [ ! -f "$runner_directory/.service" ]; then
		(
			cd "$runner_directory"
			./svc.sh install
		)
	fi

	git -C "$repository_root" config core.hooksPath .githooks
	(
		cd "$runner_directory"
		./svc.sh start
	)
	runner_status
}

case "${1:-}" in
	setup)
		setup_runner
		;;
	start)
		if [ ! -x "$runner_directory/svc.sh" ] || [ ! -f "$runner_directory/.service" ]; then
			printf 'runner is not configured; run scripts/local-runner.sh setup first\n' >&2
			exit 1
		fi
		(cd "$runner_directory" && ./svc.sh start)
		;;
	stop)
		if [ ! -x "$runner_directory/svc.sh" ] || [ ! -f "$runner_directory/.service" ]; then
			printf 'runner is not configured\n' >&2
			exit 1
		fi
		(cd "$runner_directory" && ./svc.sh stop)
		;;
	status)
		runner_status
		;;
	foreground)
		if [ ! -x "$runner_directory/run.sh" ] || [ ! -f "$runner_directory/.runner" ]; then
			printf 'runner is not configured; run scripts/local-runner.sh setup first\n' >&2
			exit 1
		fi
		(cd "$runner_directory" && exec ./run.sh)
		;;
	*)
		usage
		;;
esac
