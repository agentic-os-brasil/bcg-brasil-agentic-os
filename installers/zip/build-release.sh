#!/usr/bin/env bash
# Maestro release factory — builds Maestro-vX.Y.Z.zip.
#
# Usage:
#   installers/zip/build-release.sh <version>
#
# Example:
#   installers/zip/build-release.sh 0.1.0
#
# Output (in dist/):
#   Maestro-v<version>.zip
#   Maestro-v<version>.sha256

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DIST_DIR="$REPO_ROOT/dist"
TEMPLATE_DIR="$REPO_ROOT/installers/zip/user-template"
BUNDLES_DIR="$REPO_ROOT/bundles"

if [ $# -lt 1 ]; then
  echo "usage: build-release.sh <version>" >&2
  exit 2
fi

VERSION="$1"

if ! printf '%s' "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "version must match X.Y.Z: got '$VERSION'" >&2
  exit 2
fi

STAGE_DIR=$(mktemp -d)
trap 'rm -rf "$STAGE_DIR"' EXIT

MAESTRO_DIR="$STAGE_DIR/Maestro"
mkdir -p "$MAESTRO_DIR"

echo "==> Staging core files"
cp -R "$TEMPLATE_DIR/." "$MAESTRO_DIR/"
cp -R "$BUNDLES_DIR" "$MAESTRO_DIR/bundles"
if [ -d "$REPO_ROOT/schemas" ]; then
  cp -R "$REPO_ROOT/schemas" "$MAESTRO_DIR/schemas"
fi
if [ ! -f "$MAESTRO_DIR/CLAUDE.md" ]; then
  echo "FATAL: CLAUDE.md missing at $TEMPLATE_DIR/CLAUDE.md — required for session bootstrap" >&2
  exit 1
fi

echo "==> Writing VERSION"
printf '%s\n' "$VERSION" > "$MAESTRO_DIR/VERSION"

echo "==> Stripping dev artifacts"
find "$MAESTRO_DIR" -name '.DS_Store' -delete 2>/dev/null || true
find "$MAESTRO_DIR" -name '__pycache__' -type d -exec rm -rf {} + 2>/dev/null || true
find "$MAESTRO_DIR" -name '*.pyc' -delete 2>/dev/null || true

STRIP_MANIFEST="$STAGE_DIR/go-strip-manifest.txt"
find "$MAESTRO_DIR/bundles" -type f \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) -print > "$STRIP_MANIFEST" 2>/dev/null || true
STRIP_COUNT=$(wc -l < "$STRIP_MANIFEST" | tr -d ' ')
echo "==> Stripping $STRIP_COUNT Go source file(s) from bundles (manifest: $STRIP_MANIFEST)"
find "$MAESTRO_DIR/bundles" -type f \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) -delete 2>/dev/null || true

mkdir -p "$DIST_DIR"
ZIP_NAME="Maestro-v${VERSION}.zip"
ZIP_PATH="$DIST_DIR/$ZIP_NAME"
rm -f "$ZIP_PATH"

echo "==> Creating $ZIP_NAME"
( cd "$STAGE_DIR" && zip -qr "$ZIP_PATH" Maestro )

SHA256=$(shasum -a 256 "$ZIP_PATH" | awk '{print $1}')
echo "$SHA256  $ZIP_NAME" > "$DIST_DIR/Maestro-v${VERSION}.sha256"

echo ""
echo "Release pronto:"
echo "  ZIP:      $ZIP_PATH"
echo "  SHA256:   $SHA256"
echo ""
echo "Próximo passo:"
echo "  Envie $ZIP_NAME por email para o batch beta."
