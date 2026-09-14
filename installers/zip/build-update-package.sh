#!/usr/bin/env bash
# Build a self-contained, user-facing update kit around an already validated
# Maestro release ZIP. The kit never contains owner data and never edits an
# installed Maestro in place.
#
# Usage:
#   installers/zip/build-update-package.sh <from-version> <to-version>

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DIST_DIR="$REPO_ROOT/dist"
TEMPLATE_DIR="$REPO_ROOT/installers/zip/update-template"

if [ $# -ne 2 ]; then
  echo "usage: build-update-package.sh <from-version> <to-version>" >&2
  exit 2
fi

FROM_VERSION="$1"
TO_VERSION="$2"
for version in "$FROM_VERSION" "$TO_VERSION"; do
  if ! printf '%s' "$version" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "version must match X.Y.Z: got '$version'" >&2
    exit 2
  fi
done
if [ "$FROM_VERSION" = "$TO_VERSION" ]; then
  echo "from-version and to-version must differ" >&2
  exit 2
fi

RELEASE_ZIP="$DIST_DIR/Maestro-v${TO_VERSION}.zip"
RELEASE_SHA="$DIST_DIR/Maestro-v${TO_VERSION}.sha256"
if [ ! -f "$RELEASE_ZIP" ] || [ ! -f "$RELEASE_SHA" ]; then
  echo "missing release ZIP or sidecar for $TO_VERSION" >&2
  echo "run: installers/zip/build-release.sh $TO_VERSION" >&2
  exit 1
fi

EXPECTED_SHA=$(awk '{print $1}' "$RELEASE_SHA")
ACTUAL_SHA=$(shasum -a 256 "$RELEASE_ZIP" | awk '{print $1}')
if [ "$EXPECTED_SHA" != "$ACTUAL_SHA" ]; then
  echo "release checksum mismatch; refusing to wrap an unverified ZIP" >&2
  exit 1
fi

STAGE=$(mktemp -d -t maestro-update-build-XXXXXX)
trap 'rm -rf "$STAGE"' EXIT

ROOT_NAME="Maestro-Update-v${TO_VERSION}"
ROOT="$STAGE/$ROOT_NAME"
mkdir -p "$ROOT"

render() {
  sed \
    -e "s/{{FROM_VERSION}}/$FROM_VERSION/g" \
    -e "s/{{TO_VERSION}}/$TO_VERSION/g" \
    "$1" > "$2"
}

for name in LEIA-ME-PRIMEIRO.md PROMPT-1-PREPARAR.txt \
  PROMPT-2-VERIFICAR.txt CANARIO-MAC-WINDOWS.md; do
  render "$TEMPLATE_DIR/$name" "$ROOT/$name"
done

cp "$RELEASE_ZIP" "$ROOT/"
cp "$RELEASE_SHA" "$ROOT/"

UPDATE_ZIP="$DIST_DIR/${ROOT_NAME}.zip"
UPDATE_SHA="$DIST_DIR/${ROOT_NAME}.sha256"
rm -f "$UPDATE_ZIP" "$UPDATE_SHA"
(cd "$STAGE" && zip -qr "$UPDATE_ZIP" "$ROOT_NAME")

UPDATE_DIGEST=$(shasum -a 256 "$UPDATE_ZIP" | awk '{print $1}')
printf '%s  %s\n' "$UPDATE_DIGEST" "${ROOT_NAME}.zip" > "$UPDATE_SHA"

echo "Update kit pronto:"
echo "  ZIP:    $UPDATE_ZIP"
echo "  SHA256: $UPDATE_DIGEST"
