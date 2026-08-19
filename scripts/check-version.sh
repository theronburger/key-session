#!/bin/sh
set -eu

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(dirname -- "$script_directory")
release_version=$(tr -d '[:space:]' < "$repository_root/VERSION")
source_version=$(awk '$1 == "Version" && $2 == "=" {gsub(/"/, "", $3); print $3}' "$repository_root/internal/buildinfo/info.go")
short_bundle_version=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$repository_root/packaging/Info.plist")
bundle_version=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$repository_root/packaging/Info.plist")

if ! printf '%s\n' "$release_version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
	echo "VERSION is not semantic: $release_version" >&2
	exit 1
fi
if [ "$source_version" != "$release_version" ]; then
	echo "internal/buildinfo version $source_version does not match VERSION $release_version" >&2
	exit 1
fi
if [ "$short_bundle_version" != "$release_version" ] || [ "$bundle_version" != "$release_version" ]; then
	echo "Info.plist versions do not match VERSION $release_version" >&2
	exit 1
fi
if ! grep -Fq "## [v$release_version]" "$repository_root/CHANGELOG.md"; then
	echo "CHANGELOG.md has no v$release_version release section" >&2
	exit 1
fi
