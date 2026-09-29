#!/usr/bin/env bash
# Native, offline qualification of the exact macOS ZIP.
# Produces a sanitized JSON receipt; it does not call Claude or prove an Agent
# model invocation.

set -euo pipefail

usage() {
  echo "usage: native-smoke.sh --zip PATH --output RECEIPT.json" >&2
  exit 2
}

ZIP_PATH=""
OUTPUT=""
while [ $# -gt 0 ]; do
  case "$1" in
    --zip) ZIP_PATH="${2:-}"; shift 2 ;;
    --output) OUTPUT="${2:-}"; shift 2 ;;
    *) usage ;;
  esac
done
[ -n "$ZIP_PATH" ] && [ -f "$ZIP_PATH" ] && [ -n "$OUTPUT" ] || usage
[ ! -e "$OUTPUT" ] || { echo "receipt already exists: $OUTPUT" >&2; exit 1; }

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if ! git -C "$REPO_ROOT" diff --quiet \
  || ! git -C "$REPO_ROOT" diff --cached --quiet \
  || [ -n "$(git -C "$REPO_ROOT" ls-files --others --exclude-standard)" ]; then
  echo "source tree is dirty; commit and freeze the candidate before issuing a receipt" >&2
  exit 1
fi
# shellcheck source=../../installers/zip/user-template/.claude/hooks/lib/python.sh
. "$REPO_ROOT/installers/zip/user-template/.claude/hooks/lib/python.sh"
PYTHON_LABEL=$(maestro_python) || {
  echo "Python 3 is required (python3, python or py -3)." >&2
  exit 1
}

UNAME_S=$(uname -s)
case "$UNAME_S" in
  Darwin) PLATFORM="macos" ;;
  *)
    echo "this script qualifies macOS; Windows uses native-smoke.ps1. Got $UNAME_S" >&2
    exit 1
    ;;
esac

SCRATCH=$(mktemp -d -t maestro-native-smoke-XXXXXX)
trap 'rm -rf "$SCRATCH"' EXIT
LOG="$SCRATCH/eval.log"

set +e
bash "$REPO_ROOT/installers/zip/eval-release.sh" --zip "$ZIP_PATH" | tee "$LOG"
EVAL_RC=${PIPESTATUS[0]}
set -e

# This classifier permits only the exact foreign-platform deferred check.
# It verifies complete logs, counts and native Mac alias PASS records before
# issuing a macos-only result; Windows and global release gates stay closed.
CLASSIFICATION="$SCRATCH/macos-classification.json"
maestro_py "$REPO_ROOT/acceptance/zip-update/classify_macos_eval.py" \
  --log "$LOG" --eval-exit-code "$EVAL_RC" > "$CLASSIFICATION"

ZIP_SHA=$(shasum -a 256 "$ZIP_PATH" | awk '{print $1}')
VERSION=$(unzip -p "$ZIP_PATH" Maestro/VERSION | tr -d '\r\n')
BASH_VERSION_TEXT=$(bash --version | head -1)
CLAUDE_VERSION=$(claude --version 2>/dev/null || printf 'unavailable')
SOURCE_COMMIT=$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || printf 'unavailable')

maestro_py - "$OUTPUT" "$PLATFORM" "$UNAME_S" "$(uname -m)" \
  "$ZIP_SHA" "$VERSION" "$SOURCE_COMMIT" "$BASH_VERSION_TEXT" \
  "$PYTHON_LABEL" "$CLAUDE_VERSION" "$CLASSIFICATION" <<'PY'
import json, sys
(
    output, platform, os_name, architecture, zip_sha, version, commit,
    bash_version, python_resolver, claude_version, classification_path,
) = sys.argv[1:]
with open(classification_path, encoding="utf-8") as stream:
    classification = json.load(stream)
receipt = {
    "schema_version": 1,
    "evidence_kind": "maestro_zip_native_offline",
    "platform": platform,
    "os": os_name,
    "architecture": architecture,
    "release_version": version,
    "release_sha256": zip_sha,
    "source_commit": commit,
    "runtime": {
        "bash": bash_version,
        "python_resolver": python_resolver,
        "claude_code": claude_version,
    },
    **classification,
    "limits": [
        "offline receipt; does not prove a model-backed Agent invocation",
        "Windows qualification is deferred; the global release gate remains closed",
        "does not publish or sign the release",
    ],
}
with open(output, "x", encoding="utf-8", newline="\n") as f:
    json.dump(receipt, f, ensure_ascii=False, indent=2)
    f.write("\n")
PY

echo "macOS-only offline receipt (release not qualified): $OUTPUT"
