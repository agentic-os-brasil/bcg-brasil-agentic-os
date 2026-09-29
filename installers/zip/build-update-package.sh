#!/usr/bin/env bash
# Factory-only wrapper of existing verified candidate payloads; no installation.
# Usage: build-update-package.sh <from-version> <to-version> [both|macos|windows-powershell]
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DIST_DIR="$REPO_ROOT/dist"
TEMPLATE_DIR="$REPO_ROOT/installers/zip/update-template"
if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
  echo 'usage: build-update-package.sh <from-version> <to-version> [both|macos|windows-powershell]' >&2
  exit 2
fi
FROM_VERSION="$1"
TO_VERSION="$2"
PLATFORM="both"
[ "$#" -lt 3 ] || PLATFORM="$3"
for version in "$FROM_VERSION" "$TO_VERSION"; do
  printf '%s' "$version" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo 'Invalid version' >&2; exit 2; }
done
[ "$FROM_VERSION" != "$TO_VERSION" ] || { echo 'Versions must differ' >&2; exit 2; }
case "$PLATFORM" in
  both) PLATFORMS='macos windows-powershell'; SUFFIX='' ;;
  macos|windows-powershell) PLATFORMS="$PLATFORM"; SUFFIX="-$PLATFORM" ;;
  *) echo 'Invalid platform' >&2; exit 2 ;;
esac
command -v python3 >/dev/null || { echo 'Factory requires Python 3 for ZIP validation; recipients do not.' >&2; exit 1; }
VALIDATOR="$REPO_ROOT/acceptance/zip-update/update_package_contract.py"
# Validate every requested input before creating or replacing an output kit.
for platform in $PLATFORMS; do
  python3 "$VALIDATOR" payload --zip "$DIST_DIR/Maestro-v$TO_VERSION-$platform.zip" \
    --from-version "$FROM_VERSION" --to-version "$TO_VERSION" --platform "$platform"
done
STAGE=$(mktemp -d -t maestro-update-build-XXXXXX)
trap 'rm -rf "$STAGE"' EXIT
ROOT_NAME="Maestro-Update-v$TO_VERSION$SUFFIX"
KIT_ROOT="$STAGE/$ROOT_NAME"
mkdir -p "$KIT_ROOT"
printf '{"schema_version":1,"from_version":"%s","target_version":"%s","platform":"%s","qualification":"unqualified-candidate"}\n' "$FROM_VERSION" "$TO_VERSION" "$PLATFORM" > "$KIT_ROOT/UPDATE-KIT.json"
for name in LEIA-ME-PRIMEIRO.md PROMPT-1-PREPARAR.txt PROMPT-2-VERIFICAR.txt TESTE-MAC-WINDOWS.md DIAGNOSTICO-HOOKS.md; do
  sed -e "s/{{FROM_VERSION}}/$FROM_VERSION/g" -e "s/{{TO_VERSION}}/$TO_VERSION/g" \
    -e "s/{{PLATFORM}}/$PLATFORM/g" "$TEMPLATE_DIR/$name" > "$KIT_ROOT/$name"
done
for platform in $PLATFORMS; do
  payload="$DIST_DIR/Maestro-v$TO_VERSION-$platform.zip"
  cp "$payload" "$DIST_DIR/Maestro-v$TO_VERSION-$platform.sha256" "$KIT_ROOT/"
  unzip -p "$payload" Maestro/UPDATE-CONTRACT.json > "$STAGE/contract-$platform.json"
  if [ -f "$KIT_ROOT/UPDATE-CONTRACT.json" ]; then
    cmp "$KIT_ROOT/UPDATE-CONTRACT.json" "$STAGE/contract-$platform.json" || { echo 'Platform contracts differ' >&2; exit 1; }
  else
    cp "$STAGE/contract-$platform.json" "$KIT_ROOT/UPDATE-CONTRACT.json"
  fi
done
# Build and validate in staging. Existing candidate output survives failed validation.
(cd "$STAGE" && zip -qr "$ROOT_NAME.zip" "$ROOT_NAME")
DIGEST=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$STAGE/$ROOT_NAME.zip")
printf '%s  %s.zip\n' "$DIGEST" "$ROOT_NAME" > "$STAGE/$ROOT_NAME.sha256"
python3 "$VALIDATOR" kit --zip "$STAGE/$ROOT_NAME.zip" --from-version "$FROM_VERSION" --to-version "$TO_VERSION" --platform "$PLATFORM"
mv "$STAGE/$ROOT_NAME.zip" "$DIST_DIR/$ROOT_NAME.zip"
mv "$STAGE/$ROOT_NAME.sha256" "$DIST_DIR/$ROOT_NAME.sha256"
printf 'Draft update candidate: %s\nSHA256: %s\nNative qualification and distribution remain separate.\n' "$DIST_DIR/$ROOT_NAME.zip" "$DIGEST"
