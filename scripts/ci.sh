#!/bin/sh
set -eu

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(dirname -- "$script_directory")
temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/key-session-ci.XXXXXX")
trap 'rm -rf "$temporary_directory"' EXIT HUP INT TERM

cd "$repository_root"
"$script_directory/check-version.sh"
"$script_directory/check-pr-title.sh" --self-test
unformatted_files=$(gofmt -l $(find cmd internal -name '*.go' -type f))
if [ -n "$unformatted_files" ]; then
	echo "gofmt required for:" >&2
	echo "$unformatted_files" >&2
	exit 1
fi

go mod tidy -diff
go vet ./...
go test -race ./...
swift test --package-path app
swift build --package-path app -c release --product KeySessionApp
"$script_directory/build-binary.sh" "$temporary_directory/key-session"
"$temporary_directory/key-session" version --json
