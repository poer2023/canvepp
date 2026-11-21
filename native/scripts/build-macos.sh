#!/bin/sh
set -eu
native_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
staging_dir=$(mktemp -d /tmp/canvepp-build.XXXXXX)
trap 'rm -rf "$staging_dir"' EXIT
cd "$native_dir"
CGO_ENABLED=1 go run github.com/egoist/mygo/cmd/mygo build -skip-dmg -o "$staging_dir"
mkdir -p "$staging_dir/darwin-arm64/Canvepp Native.app/Contents/Resources/Licenses"
cp "$native_dir/assets/lucide-LICENSE" "$staging_dir/darwin-arm64/Canvepp Native.app/Contents/Resources/Licenses/Lucide.txt"
cp "$native_dir/assets/go-pinyin-LICENSE" "$staging_dir/darwin-arm64/Canvepp Native.app/Contents/Resources/Licenses/go-pinyin.txt"
/usr/libexec/PlistBuddy -c "Add :NSAppleEventsUsageDescription string Canvepp reads the active Adobe document when you request a PNG snapshot." "$staging_dir/darwin-arm64/Canvepp Native.app/Contents/Info.plist"
python3 scripts/prepare-codex.py darwin-arm64
cp -R build/runtime/darwin-arm64 "$staging_dir/darwin-arm64/Canvepp Native.app/Contents/Resources/runtime"
cp scripts/licenses/Codex-*.txt "$staging_dir/darwin-arm64/Canvepp Native.app/Contents/Resources/Licenses/"
cp scripts/licenses/go-x-image-LICENSE.txt "$staging_dir/darwin-arm64/Canvepp Native.app/Contents/Resources/Licenses/"
codesign --force --deep --sign - "$staging_dir/darwin-arm64/Canvepp Native.app"
mkdir -p "$native_dir/build"
# Keep signed bundles outside iCloud-managed directories. Finder can attach
# metadata to a bare .app there after xattr removal; a zip stays immutable.
codesign --verify --deep --strict "$staging_dir/darwin-arm64/Canvepp Native.app"
ditto -c -k --norsrc --noextattr --keepParent "$staging_dir/darwin-arm64/Canvepp Native.app" "$native_dir/build/CanveppNative-macos-arm64.zip"
printf '%s\n' "$native_dir/build/CanveppNative-macos-arm64.zip"
