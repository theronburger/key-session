#!/bin/sh
set -eu

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(dirname -- "$script_directory")
destination=${1:-"$repository_root/dist/Key Session.app"}
prebuilt_binary=${2:-}
release_version=$(tr -d '[:space:]' < "$repository_root/VERSION")

if [ -e "$destination" ]; then
	echo "Destination already exists: $destination" >&2
	exit 1
fi

contents="$destination/Contents"
mkdir -p "$contents/MacOS" "$contents/Resources" "$contents/Frameworks"
cp "$repository_root/packaging/Info.plist" "$contents/Info.plist"
cp "$repository_root/assets/KeySession.icns" "$contents/Resources/KeySession.icns"
cp "$repository_root/assets/key-session-key.png" "$contents/Resources/KeySessionKey.png"
mkdir -p "$contents/Resources/skills"
cp -R "$repository_root/skills/using-keys" "$contents/Resources/skills/using-keys"
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $release_version" "$contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $release_version" "$contents/Info.plist"

swift_architecture_arguments=""
if [ -n "$prebuilt_binary" ]; then
	swift_architecture_arguments="--arch arm64 --arch x86_64"
fi
# shellcheck disable=SC2086
swift build --package-path "$repository_root/app" -c release --product KeySessionApp $swift_architecture_arguments
# shellcheck disable=SC2086
swift_binary_directory=$(swift build --package-path "$repository_root/app" -c release --show-bin-path $swift_architecture_arguments)
cp "$swift_binary_directory/KeySessionApp" "$contents/MacOS/KeySessionApp"
chmod 0755 "$contents/MacOS/KeySessionApp"
ditto "$swift_binary_directory/Sparkle.framework" "$contents/Frameworks/Sparkle.framework"
install_name_tool -add_rpath "@executable_path/../Frameworks" "$contents/MacOS/KeySessionApp"

if [ -n "$prebuilt_binary" ]; then
	cp "$prebuilt_binary" "$contents/MacOS/key-session"
else
	"$script_directory/build-binary.sh" "$contents/MacOS/key-session"
fi
chmod 0755 "$contents/MacOS/key-session"

helper="$contents/Resources/Key Session Helper.app"
mkdir -p "$helper/Contents/MacOS" "$helper/Contents/Resources"
cp "$repository_root/packaging/Helper-Info.plist" "$helper/Contents/Info.plist"
cp "$repository_root/assets/KeySession.icns" "$helper/Contents/Resources/KeySession.icns"
mkdir -p "$helper/Contents/Resources/skills"
cp -R "$repository_root/skills/using-keys" "$helper/Contents/Resources/skills/using-keys"
cp "$contents/MacOS/key-session" "$helper/Contents/MacOS/KeySessionDaemon"
chmod 0755 "$helper/Contents/MacOS/KeySessionDaemon"
/usr/libexec/PlistBuddy -c "Add :CFBundleShortVersionString string $release_version" "$helper/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Add :CFBundleVersion string $release_version" "$helper/Contents/Info.plist"

signing_identity=${KEY_SESSION_SIGNING_IDENTITY:--}
if [ "$signing_identity" = "-" ]; then
	codesign --force --sign - --preserve-metadata=identifier,entitlements,flags "$contents/Frameworks/Sparkle.framework/Versions/B/XPCServices/Downloader.xpc"
	codesign --force --sign - --preserve-metadata=identifier,entitlements,flags "$contents/Frameworks/Sparkle.framework/Versions/B/XPCServices/Installer.xpc"
	codesign --force --sign - --preserve-metadata=identifier,entitlements,flags "$contents/Frameworks/Sparkle.framework/Versions/B/Updater.app"
	codesign --force --sign - "$contents/Frameworks/Sparkle.framework/Versions/B/Autoupdate"
	codesign --force --sign - --preserve-metadata=identifier,entitlements,flags "$contents/Frameworks/Sparkle.framework"
	codesign --force --sign - "$helper/Contents/MacOS/KeySessionDaemon"
	codesign --force --sign - "$helper"
	codesign --force --sign - "$contents/MacOS/key-session"
	codesign --force --sign - --entitlements "$repository_root/packaging/KeySession.entitlements" "$destination"
else
	codesign --force --options runtime --timestamp --sign "$signing_identity" --preserve-metadata=identifier,entitlements,flags "$contents/Frameworks/Sparkle.framework/Versions/B/XPCServices/Downloader.xpc"
	codesign --force --options runtime --timestamp --sign "$signing_identity" --preserve-metadata=identifier,entitlements,flags "$contents/Frameworks/Sparkle.framework/Versions/B/XPCServices/Installer.xpc"
	codesign --force --options runtime --timestamp --sign "$signing_identity" --preserve-metadata=identifier,entitlements,flags "$contents/Frameworks/Sparkle.framework/Versions/B/Updater.app"
	codesign --force --options runtime --timestamp --sign "$signing_identity" "$contents/Frameworks/Sparkle.framework/Versions/B/Autoupdate"
	codesign --force --options runtime --timestamp --sign "$signing_identity" --preserve-metadata=identifier,entitlements,flags "$contents/Frameworks/Sparkle.framework"
	codesign --force --options runtime --timestamp --sign "$signing_identity" "$helper/Contents/MacOS/KeySessionDaemon"
	codesign --force --options runtime --timestamp --sign "$signing_identity" "$helper"
	codesign --force --options runtime --timestamp --sign "$signing_identity" "$contents/MacOS/key-session"
	codesign --force --options runtime --timestamp --sign "$signing_identity" --entitlements "$repository_root/packaging/KeySession.entitlements" "$destination"
fi
echo "$destination"
