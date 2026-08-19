#!/bin/sh
set -eu

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(dirname -- "$script_directory")

cd "$repository_root"
KEY_SESSION_KEYCHAIN_INTEGRATION=1 go test ./internal/keychain \
	-run '^TestProtectedKeychainRoundTripSetup$' \
	-count=1 \
	-v
