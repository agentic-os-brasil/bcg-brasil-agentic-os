#!/usr/bin/env bash
# Offline factory contracts only. Native execution is checked by platform CI.
# Usage: eval-update-package.sh --zip PATH --from-version X.Y.Z --to-version X.Y.Z [--platform both|macos|windows-powershell]
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
UPDATE_ZIP=''
FROM_VERSION=''
TO_VERSION=''
PLATFORM='both'
while [ "$#" -gt 0 ]; do
  case "$1" in
    --zip|--from-version|--to-version|--platform)
      [ "$#" -ge 2 ] || { echo "Missing value: $1" >&2; exit 2; }
      case "$1" in
        --zip) UPDATE_ZIP="$2" ;;
        --from-version) FROM_VERSION="$2" ;;
        --to-version) TO_VERSION="$2" ;;
        --platform) PLATFORM="$2" ;;
      esac
      shift 2 ;;
    -h|--help) sed -n '2,3p' "$0"; exit 0 ;;
    *) echo "Unknown flag: $1" >&2; exit 2 ;;
  esac
done
for version in "$FROM_VERSION" "$TO_VERSION"; do
  printf '%s' "$version" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo 'Required versions must match X.Y.Z' >&2; exit 2; }
done
case "$PLATFORM" in
  both) SUFFIX='' ;;
  macos|windows-powershell) SUFFIX="-$PLATFORM" ;;
  *) echo 'Invalid platform' >&2; exit 2 ;;
esac
[ -n "$UPDATE_ZIP" ] || UPDATE_ZIP="$REPO_ROOT/dist/Maestro-Update-v$TO_VERSION$SUFFIX.zip"
python3 "$REPO_ROOT/acceptance/zip-update/update_package_contract.py" kit \
  --zip "$UPDATE_ZIP" --from-version "$FROM_VERSION" --to-version "$TO_VERSION" --platform "$PLATFORM"
printf '\nSummary: 1 package-contract pass, 0 fail; native execution not evaluated.\n'
