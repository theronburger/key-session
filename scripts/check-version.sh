#!/bin/sh
set -eu

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(dirname -- "$script_directory")
release_version=$(tr -d '[:space:]' < "$repository_root/VERSION")
source_version=$(awk '$1 == "Version" && $2 == "=" {gsub(/"/, "", $3); print $3}' "$repository_root/internal/buildinfo/info.go")
short_bundle_version=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$repository_root/packaging/Info.plist")
bundle_version=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$repository_root/packaging/Info.plist")
manifest_version=$(awk -F'"' '$2 == "." {print $4}' "$repository_root/.release-please-manifest.json")

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
if [ "$manifest_version" != "$release_version" ]; then
	echo "Release Please manifest version $manifest_version does not match VERSION $release_version" >&2
	exit 1
fi
if [ "$(grep -c 'x-release-please-version' "$repository_root/internal/buildinfo/info.go")" -ne 1 ] ||
	[ "$(grep -c 'x-release-please-version' "$repository_root/packaging/Info.plist")" -ne 2 ]; then
	echo "Release Please version annotations are missing or duplicated" >&2
	exit 1
fi
if ! grep -Fq "## [v$release_version]" "$repository_root/CHANGELOG.md" &&
	! grep -Fq "## [$release_version](" "$repository_root/CHANGELOG.md"; then
	echo "CHANGELOG.md has no $release_version release section" >&2
	exit 1
fi
