#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
	echo "usage: $0 <output-path>" >&2
	exit 2
fi

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(dirname -- "$script_directory")
output_path=$1
release_version=$(tr -d '[:space:]' < "$repository_root/VERSION")
deployment_target=${MACOSX_DEPLOYMENT_TARGET:-14.0}
source_commit=$(git -C "$repository_root" rev-parse HEAD 2>/dev/null || echo unknown)
source_date=$(git -C "$repository_root" show -s --format=%cI HEAD 2>/dev/null || echo unknown)
source_dirty=false
if [ -n "$(git -C "$repository_root" status --porcelain 2>/dev/null || true)" ]; then
	source_dirty=true
fi
linker_flags="-s -w -X github.com/theronburger/key-session/internal/buildinfo.Version=$release_version -X github.com/theronburger/key-session/internal/buildinfo.Commit=$source_commit -X github.com/theronburger/key-session/internal/buildinfo.BuildDate=$source_date -X github.com/theronburger/key-session/internal/buildinfo.Dirty=$source_dirty"

mkdir -p "$(dirname -- "$output_path")"
(
	cd "$repository_root"
	CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET="$deployment_target" go build -trimpath -buildvcs=false -ldflags "$linker_flags" -o "$output_path" ./cmd/key-session
)
