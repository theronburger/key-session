#!/bin/sh
set -eu

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(dirname -- "$script_directory")
output_directory=${1:-"$repository_root/dist"}
release_version=$(tr -d '[:space:]' < "$repository_root/VERSION")
release_stem="key-session_${release_version}_macos_universal"

mkdir -p "$output_directory"
output_directory=$(CDPATH= cd -- "$output_directory" && pwd)
staging_directory=$(mktemp -d "$output_directory/.key-session-release.XXXXXX")
trap 'rm -rf "$staging_directory"' EXIT HUP INT TERM

application_path="$staging_directory/Key Session.app"
archive_path="$staging_directory/$release_stem.zip"
sbom_path="$staging_directory/$release_stem.sbom.cdx.json"
checksums_path="$staging_directory/checksums.txt"

mkdir -p "$staging_directory/bin"
GOOS=darwin GOARCH=arm64 "$script_directory/build-binary.sh" "$staging_directory/bin/key-session-arm64"
GOOS=darwin GOARCH=amd64 "$script_directory/build-binary.sh" "$staging_directory/bin/key-session-amd64"
lipo -create \
	"$staging_directory/bin/key-session-arm64" \
	"$staging_directory/bin/key-session-amd64" \
	-output "$staging_directory/bin/key-session-universal"

"$script_directory/build-app.sh" "$application_path" "$staging_directory/bin/key-session-universal"
ditto -c -k --sequesterRsrc --keepParent "$application_path" "$archive_path"

if command -v cyclonedx-gomod >/dev/null 2>&1; then
	(
		cd "$repository_root"
		cyclonedx-gomod app -json -output "$sbom_path" -main ./cmd/key-session
	)
	(
		cd "$staging_directory"
		shasum -a 256 "$(basename -- "$archive_path")" "$(basename -- "$sbom_path")" > "$(basename -- "$checksums_path")"
	)
else
	(
		cd "$staging_directory"
		shasum -a 256 "$(basename -- "$archive_path")" > "$(basename -- "$checksums_path")"
	)
fi

file "$staging_directory/bin/key-session-universal"
codesign --verify --deep --strict --verbose=2 "$application_path"
"$staging_directory/bin/key-session-universal" version --json

rm -rf "$output_directory/Key Session.app"
rm -f "$output_directory/$release_stem.zip" "$output_directory/$release_stem.sbom.cdx.json" "$output_directory/checksums.txt"
mv "$staging_directory/Key Session.app" "$output_directory/Key Session.app"
mkdir -p "$output_directory/bin"
for binary in key-session-arm64 key-session-amd64 key-session-universal; do
	mv -f "$staging_directory/bin/$binary" "$output_directory/bin/$binary"
done
mv "$staging_directory/$release_stem.zip" "$output_directory/$release_stem.zip"
if [ -f "$staging_directory/$release_stem.sbom.cdx.json" ]; then
	mv "$staging_directory/$release_stem.sbom.cdx.json" "$output_directory/$release_stem.sbom.cdx.json"
fi
mv "$staging_directory/checksums.txt" "$output_directory/checksums.txt"

echo "$output_directory/$release_stem.zip"
