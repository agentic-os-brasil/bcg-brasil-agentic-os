#!/usr/bin/env bash
# Maestro release factory — builds one platform-specific Maestro ZIP.
#
# Usage:
#   installers/zip/build-release.sh <version> [macos|windows-powershell]
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

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
  echo "usage: build-release.sh <version> [macos|windows-powershell]" >&2
  exit 2
fi

VERSION="$1"
PLATFORM="${2:-macos}"

if ! printf '%s' "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "version must match X.Y.Z: got '$VERSION'" >&2
  exit 2
fi
case "$PLATFORM" in
  macos|windows-powershell) ;;
  *) echo "platform must be macos or windows-powershell: got '$PLATFORM'" >&2; exit 2 ;;
esac

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

# Each distributable has one authoritative settings.json.  The alternative is
# a factory input only, never something an owner must choose or rename.
if [ "$PLATFORM" = "windows-powershell" ]; then
  cp "$MAESTRO_DIR/.claude/settings.windows-powershell.json" "$MAESTRO_DIR/.claude/settings.json"
fi
rm -f "$MAESTRO_DIR/.claude/settings.windows-powershell.json"

# Windows PowerShell 5.1 decodes a script without a BOM using the legacy system
# code page.  Add a UTF-8 BOM to the staged Windows scripts so Portuguese text
# and paths survive on both 5.1 and PowerShell 7.  Source files remain normal
# UTF-8; this conversion affects only the release artifact.
if [ "$PLATFORM" = "windows-powershell" ]; then
  find "$MAESTRO_DIR/.claude/hooks" -type f -name '*.ps1' -print0 | while IFS= read -r -d '' ps_file; do
    bom_tmp="${ps_file}.bom"
    printf '\357\273\277' > "$bom_tmp"
    sed 's/\r$//; s/$/\r/' "$ps_file" >> "$bom_tmp"
    mv "$bom_tmp" "$ps_file"
  done
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

# data/ contract — release ZIPs must NEVER ship any data/. The workspace is
# always created on first run by .claude/hooks/first-run-scaffold.sh, and
# README-INSTALL.md promises "sua data/ nunca é tocada pelo ZIP". If a data/
# directory ever leaks into the template or gets copied in during staging
# (dev workspace pollution, backup restore, careless test scaffold), strip
# it here so the release stays clean. Defensive: exit non-zero if strip
# fails to keep leakage visible.
if [ -e "$MAESTRO_DIR/data" ]; then
  echo "==> Stripping data/ from staged release (must never ship)"
  rm -rf "$MAESTRO_DIR/data"
  if [ -e "$MAESTRO_DIR/data" ]; then
    echo "FATAL: could not strip $MAESTRO_DIR/data — aborting release" >&2
    exit 1
  fi
fi

# Belt-and-suspenders: ensure every hook is executable before zipping.
# macOS `zip` preserves Unix mode bits, but a source file that lost its +x
# in git would silently ship non-executable and hooks would never fire.
echo "==> Ensuring hooks are executable"
if [ -d "$MAESTRO_DIR/.claude/hooks" ]; then
  chmod +x "$MAESTRO_DIR/.claude/hooks"/*.sh 2>/dev/null || true
fi

STRIP_MANIFEST="$STAGE_DIR/go-strip-manifest.txt"
find "$MAESTRO_DIR/bundles" -type f \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) -print > "$STRIP_MANIFEST" 2>/dev/null || true
STRIP_COUNT=$(wc -l < "$STRIP_MANIFEST" | tr -d ' ')
echo "==> Stripping $STRIP_COUNT Go source file(s) from bundles (manifest: $STRIP_MANIFEST)"
find "$MAESTRO_DIR/bundles" -type f \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) -delete 2>/dev/null || true

mkdir -p "$DIST_DIR"
if [ "$PLATFORM" = "windows-powershell" ]; then
  ZIP_NAME="Maestro-v${VERSION}-windows-powershell.zip"
  SHA_NAME="Maestro-v${VERSION}-windows-powershell.sha256"
else
  ZIP_NAME="Maestro-v${VERSION}-macos.zip"
  SHA_NAME="Maestro-v${VERSION}-macos.sha256"
fi
ZIP_PATH="$DIST_DIR/$ZIP_NAME"
rm -f "$ZIP_PATH"

echo "==> Creating $ZIP_NAME"
if command -v zip >/dev/null 2>&1; then
  ( cd "$STAGE_DIR" && zip -qr "$ZIP_PATH" Maestro )
elif command -v powershell.exe >/dev/null 2>&1 && command -v cygpath >/dev/null 2>&1; then
  MAESTRO_ARCHIVE_SOURCE="$(cygpath -w "$MAESTRO_DIR")" \
    MAESTRO_ARCHIVE_DEST="$(cygpath -w "$ZIP_PATH")" \
    powershell.exe -NoLogo -NoProfile -NonInteractive -Command \
      '$ErrorActionPreference = "Stop"; Compress-Archive -LiteralPath $env:MAESTRO_ARCHIVE_SOURCE -DestinationPath $env:MAESTRO_ARCHIVE_DEST -Force'
else
  echo "FATAL: zip is unavailable and no native PowerShell archive fallback was found" >&2
  exit 1
fi

if command -v shasum >/dev/null 2>&1; then
  SHA256=$(shasum -a 256 "$ZIP_PATH" | awk '{print $1}')
elif command -v sha256sum >/dev/null 2>&1; then
  SHA256=$(sha256sum "$ZIP_PATH" | awk '{print $1}')
elif command -v powershell.exe >/dev/null 2>&1 && command -v cygpath >/dev/null 2>&1; then
  SHA256=$(MAESTRO_HASH_PATH="$(cygpath -w "$ZIP_PATH")" \
    powershell.exe -NoLogo -NoProfile -NonInteractive -Command \
      '(Get-FileHash -Algorithm SHA256 -LiteralPath $env:MAESTRO_HASH_PATH).Hash.ToLowerInvariant()' | tr -d '\r')
else
  echo "FATAL: no SHA-256 implementation is available" >&2
  exit 1
fi
echo "$SHA256  $ZIP_NAME" > "$DIST_DIR/$SHA_NAME"

echo ""
echo "Release pronto:"
echo "  ZIP:      $ZIP_PATH"
echo "  SHA256:   $SHA256"
echo ""
echo "Próximo passo:"
echo "  Envie $ZIP_NAME por email para o batch beta."
