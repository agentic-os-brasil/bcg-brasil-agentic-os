#!/usr/bin/env bash
# Native, offline qualification of the exact ZIP on macOS or Windows Git Bash.
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
# shellcheck source=../../installers/zip/user-template/.claude/hooks/lib/python.sh
. "$REPO_ROOT/installers/zip/user-template/.claude/hooks/lib/python.sh"
PYTHON_LABEL=$(maestro_python) || {
  echo "Python 3 is required (python3, python or py -3)." >&2
  exit 1
}

UNAME_S=$(uname -s)
case "$UNAME_S" in
  Darwin) PLATFORM="macos"; NATIVE_ASSERTION="filesystem alias" ;;
  MINGW*|MSYS*|CYGWIN*)
    PLATFORM="windows-git-bash"
    NATIVE_ASSERTION="drive-letter paths, native Windows"
    command -v cygpath >/dev/null 2>&1 || {
      echo "Windows qualification must run inside Git Bash." >&2
      exit 1
    }
    ;;
  *)
    echo "native qualification supports macOS or Windows Git Bash; got $UNAME_S" >&2
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

SUMMARY=$(grep '^Summary:' "$LOG" | tail -1 | sed $'s/\033\\[[0-9;]*m//g' || true)
PASS_COUNT=$(printf '%s' "$SUMMARY" | sed -n 's/.*Summary:[^0-9]*\([0-9][0-9]*\) pass.*/\1/p')
FAIL_COUNT=$(printf '%s' "$SUMMARY" | sed -n 's/.*pass[^0-9]*\([0-9][0-9]*\) fail.*/\1/p')
SKIP_COUNT=$(printf '%s' "$SUMMARY" | sed -n 's/.*fail[^0-9]*\([0-9][0-9]*\) skip.*/\1/p')

[ "$EVAL_RC" -eq 0 ] || { echo "release eval failed" >&2; exit 1; }
[ "${FAIL_COUNT:-1}" = 0 ] || { echo "release eval contains failures" >&2; exit 1; }
[ "${SKIP_COUNT:-1}" = 0 ] || { echo "release eval contains skipped checks" >&2; exit 1; }
grep -F "$NATIVE_ASSERTION" "$LOG" >/dev/null || {
  echo "platform-native path assertion was not exercised" >&2
  exit 1
}

ZIP_SHA=$(shasum -a 256 "$ZIP_PATH" | awk '{print $1}')
VERSION=$(unzip -p "$ZIP_PATH" Maestro/VERSION | tr -d '\r\n')
BASH_VERSION_TEXT=$(bash --version | head -1)
CLAUDE_VERSION=$(claude --version 2>/dev/null || printf 'unavailable')
SOURCE_COMMIT=$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || printf 'unavailable')

maestro_py - "$OUTPUT" "$PLATFORM" "$UNAME_S" "$(uname -m)" \
  "$ZIP_SHA" "$VERSION" "$SOURCE_COMMIT" "$BASH_VERSION_TEXT" \
  "$PYTHON_LABEL" "$CLAUDE_VERSION" "$PASS_COUNT" "$FAIL_COUNT" "$SKIP_COUNT" <<'PY'
import json, sys
(
    output, platform, os_name, architecture, zip_sha, version, commit,
    bash_version, python_resolver, claude_version, passed, failed, skipped,
) = sys.argv[1:]
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
    "checks": {
        "passed": int(passed),
        "failed": int(failed),
        "skipped": int(skipped),
        "platform_native_path_exercised": True,
    },
    "verdict": "PASS",
    "limits": [
        "offline receipt; does not prove a model-backed Agent invocation",
        "does not publish or sign the release",
    ],
}
with open(output, "x", encoding="utf-8", newline="\n") as f:
    json.dump(receipt, f, ensure_ascii=False, indent=2)
    f.write("\n")
PY

echo "Native offline receipt: $OUTPUT"
