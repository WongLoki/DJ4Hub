#!/bin/sh
set -eu
ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
BACKEND_PACKAGE=${1:?Pass the existing portable backend package directory}
OUTPUT=${2:-"${ROOT_DIR}/dist/native/DJ 4G Hub.app"}
case "$OUTPUT" in /*) ;; *) echo 'Output must be an absolute .app path' >&2; exit 2;; esac
case "$OUTPUT" in *.app) ;; *) echo 'Output must end in .app' >&2; exit 2;; esac
if [ -e "$OUTPUT" ]; then echo 'Output already exists; choose another path to preserve it.' >&2; exit 1; fi
test -f "$BACKEND_PACKAGE/lib/libusb-1.0.0.dylib"
swift test --package-path "${ROOT_DIR}/apps/hub-macos"
swift build -c release --package-path "${ROOT_DIR}/apps/hub-macos"
mkdir -p "$(dirname -- "$OUTPUT")"
STAGE=$(mktemp -d "$(dirname -- "$OUTPUT")/.dj4hub-app.XXXXXX")
trap 'rmdir "$STAGE" 2>/dev/null || true' EXIT
APP="$STAGE/DJ 4G Hub.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources/backend/bin" "$APP/Contents/Resources/backend/lib"
cp "${ROOT_DIR}/apps/hub-macos/.build/release/DJ4Hub" "$APP/Contents/MacOS/DJ4Hub"
cp "${ROOT_DIR}/apps/hub-macos/Info.plist" "$APP/Contents/Info.plist"
cp "${ROOT_DIR}/docs/images/dj-4g-hub-icon.png" "$APP/Contents/Resources/AppIcon.png"
cp "$BACKEND_PACKAGE/lib/libusb-1.0.0.dylib" "$APP/Contents/Resources/backend/lib/"
cd "$ROOT_DIR"
go build -trimpath -o "$APP/Contents/Resources/backend/bin/dj4ghub-macos" ./cmd/dj4ghub-macos
cp LICENSE THIRD_PARTY_NOTICES.md "$APP/Contents/Resources/"
cp apps/hub-macos/README.md "$APP/Contents/Resources/Native-README.md"
if otool -L "$APP/Contents/Resources/backend/bin/dj4ghub-macos" | grep -q '/opt/homebrew\|/usr/local\|/Cellar/'; then
  echo 'Backend has a package-manager dependency. Use PKG_CONFIG_PATH from package-macos-arm64.sh.' >&2; exit 1
fi
codesign --force --sign - "$APP/Contents/Resources/backend/lib/libusb-1.0.0.dylib"
codesign --force --sign - "$APP/Contents/Resources/backend/bin/dj4ghub-macos"
codesign --force --sign - "$APP"
codesign --verify --deep --strict "$APP"
mv "$APP" "$OUTPUT"
echo "Native App: $OUTPUT"
