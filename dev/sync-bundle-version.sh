#!/usr/bin/env bash
# sync-bundle-version.sh — read root VERSION and propagate into every
# bundle manifest's `bundle_version` field.
#
# Fixes §7.1 of the 2026-08-12 Darwin diagnostic: the OKF-1 manifest
# `bundle_version` drifted from the shipping `VERSION` (0.0.0 vs 0.1.1),
# so any tool reading the manifest to detect version reported wrong.
#
# Idempotent. Prints one line per manifest touched. Non-zero exit only
# if VERSION is missing or a manifest exists but is unwritable.
#
# Intended to be called by the release pipeline (see dev/release/*).

set -eu

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
VERSION_FILE="$ROOT_DIR/VERSION"

if [ ! -f "$VERSION_FILE" ]; then
  echo "ERROR: $VERSION_FILE not found" >&2
  exit 1
fi

VERSION="$(tr -d '[:space:]' < "$VERSION_FILE")"

if [ -z "$VERSION" ]; then
  echo "ERROR: $VERSION_FILE is empty" >&2
  exit 1
fi

updated=0
for manifest in "$ROOT_DIR"/bundles/*/manifest.json; do
  [ -f "$manifest" ] || continue
  # Use python for a safe JSON edit (macOS sed lacks -i portably).
  python3 - "$manifest" "$VERSION" <<'PY'
import json, sys, pathlib
path = pathlib.Path(sys.argv[1])
version = sys.argv[2]
data = json.loads(path.read_text())
if data.get("bundle_version") == version:
    print(f"unchanged  {path.relative_to(pathlib.Path.cwd()) if path.is_absolute() else path}  {version}")
    sys.exit(0)
data["bundle_version"] = version
path.write_text(json.dumps(data, indent=2) + "\n")
print(f"updated    {path}  -> {version}")
PY
  updated=$((updated + 1))
done

if [ "$updated" -eq 0 ]; then
  echo "no manifests found under $ROOT_DIR/bundles/*/manifest.json"
fi
