#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VERSION=${1:-dev}
APP_NAME="DJI 4G Connect"
EXECUTABLE_NAME=DJI4GConnect
PACKAGE_STEM=DJI-4G-Connect
DIST_DIR="${ROOT_DIR}/dist"
ARCHIVE="${DIST_DIR}/${PACKAGE_STEM}-macOS-arm64-${VERSION}.zip"
LIBUSB_VERSION=1.0.30
LIBUSB_SHA256=fea36f34f9156400209595e300840767ab1a385ede1dc7ee893015aea9c6dbaf
LIBUSB_URL="https://github.com/libusb/libusb/releases/download/v${LIBUSB_VERSION}/libusb-${LIBUSB_VERSION}.tar.bz2"
BUILD_ROOT="${TMPDIR:-/tmp}/dji-4g-connect-build-arm64"
STAGE_DIR="${BUILD_ROOT}/stage"
APP_DIR="${STAGE_DIR}/${APP_NAME}.app"
VERIFY_DIR="${BUILD_ROOT}/verify"
ICON_SOURCE="${ROOT_DIR}/assets/AppIcon-1024-glass-logo.png"
ICONSET_DIR="${BUILD_ROOT}/AppIcon.iconset"
LIBUSB_ARCHIVE="${BUILD_ROOT}/libusb-${LIBUSB_VERSION}.tar.bz2"
LIBUSB_SOURCE="${BUILD_ROOT}/libusb-source"
LIBUSB_OBJECTS="${BUILD_ROOT}/libusb-objects"
LIBUSB_PREFIX="${BUILD_ROOT}/libusb-prefix"

if [ "$(uname -m)" != "arm64" ]; then
  echo "This build currently requires an Apple Silicon Mac." >&2
  exit 1
fi
for tool in go curl pkg-config clang codesign; do
  command -v "${tool}" >/dev/null 2>&1 || {
    echo "Missing build tool: ${tool}" >&2
    exit 1
  }
done
for tool in sips iconutil; do
  command -v "${tool}" >/dev/null 2>&1 || {
    echo "Missing macOS icon tool: ${tool}" >&2
    exit 1
  }
done
if [ ! -f "${ICON_SOURCE}" ]; then
  echo "Missing icon source: ${ICON_SOURCE}" >&2
  exit 1
fi

mkdir -p "${BUILD_ROOT}" "${DIST_DIR}"
if [ ! -f "${LIBUSB_ARCHIVE}" ]; then
  curl -fL "${LIBUSB_URL}" -o "${LIBUSB_ARCHIVE}"
fi
ACTUAL_SHA256=$(shasum -a 256 "${LIBUSB_ARCHIVE}" | awk '{print $1}')
if [ "${ACTUAL_SHA256}" != "${LIBUSB_SHA256}" ]; then
  echo "libusb source checksum mismatch." >&2
  exit 1
fi

rm -rf "${LIBUSB_SOURCE}" "${LIBUSB_OBJECTS}" "${LIBUSB_PREFIX}" "${STAGE_DIR}" "${VERIFY_DIR}" "${ICONSET_DIR}" "${DIST_DIR}/${APP_NAME}.app"
mkdir -p "${LIBUSB_SOURCE}" "${LIBUSB_OBJECTS}" "${LIBUSB_PREFIX}/lib" "${LIBUSB_PREFIX}/include/libusb-1.0"
mkdir -p "${APP_DIR}/Contents/MacOS" "${APP_DIR}/Contents/Frameworks" "${APP_DIR}/Contents/Resources"
mkdir -p "${ICONSET_DIR}"
tar -xjf "${LIBUSB_ARCHIVE}" -C "${LIBUSB_SOURCE}" --strip-components=1

(
  cd "${LIBUSB_SOURCE}"
  MACOSX_DEPLOYMENT_TARGET=13.0 ./configure \
    --prefix="${LIBUSB_PREFIX}" \
    --disable-static --enable-shared --disable-dependency-tracking >/dev/null
  sed -i '' 's/#define HAVE_PIPE2 1/\/\* #undef HAVE_PIPE2 \*\//' config.h
)

for source in \
  libusb/core.c \
  libusb/descriptor.c \
  libusb/hotplug.c \
  libusb/io.c \
  libusb/strerror.c \
  libusb/sync.c \
  libusb/os/events_posix.c \
  libusb/os/threads_posix.c \
  libusb/os/darwin_usb.c
do
  object="${LIBUSB_OBJECTS}/$(basename "${source}" .c).o"
  clang -arch arm64 -mmacosx-version-min=13.0 -DHAVE_CONFIG_H \
    -I"${LIBUSB_SOURCE}" -I"${LIBUSB_SOURCE}/libusb" -fPIC \
    -c "${LIBUSB_SOURCE}/${source}" -o "${object}"
done

clang -arch arm64 -mmacosx-version-min=13.0 -dynamiclib \
  -install_name "@executable_path/../Frameworks/libusb-1.0.0.dylib" \
  -compatibility_version 7.0.0 -current_version 7.0.0 \
  -o "${LIBUSB_PREFIX}/lib/libusb-1.0.0.dylib" \
  "${LIBUSB_OBJECTS}"/*.o \
  -framework IOKit -framework CoreFoundation -framework Security -lobjc
ln -s libusb-1.0.0.dylib "${LIBUSB_PREFIX}/lib/libusb-1.0.dylib"
cp "${LIBUSB_SOURCE}/libusb/libusb.h" "${LIBUSB_PREFIX}/include/libusb-1.0/libusb.h"
cp "${LIBUSB_PREFIX}/lib/libusb-1.0.0.dylib" "${APP_DIR}/Contents/Frameworks/libusb-1.0.0.dylib"

cd "${ROOT_DIR}"
PKG_CONFIG_PATH="${LIBUSB_SOURCE}" \
MACOSX_DEPLOYMENT_TARGET=13.0 CGO_CFLAGS="-mmacosx-version-min=13.0" CGO_LDFLAGS="-mmacosx-version-min=13.0" \
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build \
  -trimpath -buildvcs=false -ldflags="-s -w" \
  -o "${APP_DIR}/Contents/MacOS/${EXECUTABLE_NAME}" .

sed "s/__VERSION__/${VERSION}/g" "${ROOT_DIR}/scripts/Info.plist.in" >"${APP_DIR}/Contents/Info.plist"
cp "${ROOT_DIR}/README.md" "${APP_DIR}/Contents/Resources/README.md"
cp "${ROOT_DIR}/LICENSE" "${APP_DIR}/Contents/Resources/LICENSE"
cp "${ROOT_DIR}/THIRD_PARTY_NOTICES.md" "${APP_DIR}/Contents/Resources/THIRD_PARTY_NOTICES.md"
cp "${LIBUSB_SOURCE}/COPYING" "${APP_DIR}/Contents/Resources/libusb-COPYING"

sips -z 16 16 "${ICON_SOURCE}" --out "${ICONSET_DIR}/icon_16x16.png" >/dev/null
sips -z 32 32 "${ICON_SOURCE}" --out "${ICONSET_DIR}/icon_16x16@2x.png" >/dev/null
sips -z 32 32 "${ICON_SOURCE}" --out "${ICONSET_DIR}/icon_32x32.png" >/dev/null
sips -z 64 64 "${ICON_SOURCE}" --out "${ICONSET_DIR}/icon_32x32@2x.png" >/dev/null
sips -z 128 128 "${ICON_SOURCE}" --out "${ICONSET_DIR}/icon_128x128.png" >/dev/null
sips -z 256 256 "${ICON_SOURCE}" --out "${ICONSET_DIR}/icon_128x128@2x.png" >/dev/null
sips -z 256 256 "${ICON_SOURCE}" --out "${ICONSET_DIR}/icon_256x256.png" >/dev/null
sips -z 512 512 "${ICON_SOURCE}" --out "${ICONSET_DIR}/icon_256x256@2x.png" >/dev/null
sips -z 512 512 "${ICON_SOURCE}" --out "${ICONSET_DIR}/icon_512x512.png" >/dev/null
sips -z 1024 1024 "${ICON_SOURCE}" --out "${ICONSET_DIR}/icon_512x512@2x.png" >/dev/null
iconutil -c icns "${ICONSET_DIR}" -o "${APP_DIR}/Contents/Resources/AppIcon.icns"

chmod 755 "${APP_DIR}/Contents/MacOS/${EXECUTABLE_NAME}" "${APP_DIR}/Contents/Frameworks/libusb-1.0.0.dylib"
xattr -cr "${APP_DIR}"
codesign --force --sign - "${APP_DIR}/Contents/Frameworks/libusb-1.0.0.dylib"
codesign --force --deep --sign - "${APP_DIR}"
xattr -cr "${APP_DIR}"
codesign --verify --deep --strict "${APP_DIR}"

rm -f "${ARCHIVE}" "${ARCHIVE}.sha256"
ditto -c -k --keepParent --norsrc --noextattr --noqtn --noacl "${APP_DIR}" "${ARCHIVE}"
(
  cd "${DIST_DIR}"
  shasum -a 256 "$(basename -- "${ARCHIVE}")" >"$(basename -- "${ARCHIVE}.sha256")"
)
mkdir -p "${VERIFY_DIR}"
ditto -x -k "${ARCHIVE}" "${VERIFY_DIR}"
xattr -cr "${VERIFY_DIR}/${APP_NAME}.app"
codesign --verify --deep --strict "${VERIFY_DIR}/${APP_NAME}.app"

echo "Archive:  ${ARCHIVE}"
echo "Checksum: ${ARCHIVE}.sha256"
