#!/usr/bin/env bash
# Validate the user-facing update wrapper without mutating an installed Maestro.
#
# Usage:
#   installers/zip/eval-update-package.sh --zip dist/Maestro-Update-v0.1.12.zip \
#     --from-version 0.1.11 --to-version 0.1.12

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
UPDATE_ZIP=""
FROM_VERSION=""
TO_VERSION=""

while [ $# -gt 0 ]; do
  case "$1" in
    --zip) UPDATE_ZIP="${2:-}"; shift 2 ;;
    --from-version) FROM_VERSION="${2:-}"; shift 2 ;;
    --to-version) TO_VERSION="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,8p' "$0"; exit 0 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

if [ -z "$TO_VERSION" ] || [ -z "$FROM_VERSION" ]; then
  echo "--from-version and --to-version are required" >&2
  exit 2
fi
if ! printf '%s\n%s\n' "$FROM_VERSION" "$TO_VERSION" \
  | grep -qEv '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  :
else
  echo "versions must match X.Y.Z" >&2
  exit 2
fi

if [ -z "$UPDATE_ZIP" ]; then
  UPDATE_ZIP="$REPO_ROOT/dist/Maestro-Update-v${TO_VERSION}.zip"
fi
if [ ! -f "$UPDATE_ZIP" ]; then
  echo "update package not found: $UPDATE_ZIP" >&2
  exit 2
fi

PASS=0
FAIL=0
pass() { PASS=$((PASS + 1)); printf 'PASS  %s\n' "$1"; }
fail() { FAIL=$((FAIL + 1)); printf 'FAIL  %s\n' "$1"; }

SCRATCH=$(mktemp -d -t maestro-update-eval-XXXXXX)
trap 'chmod -R u+w "$SCRATCH" 2>/dev/null || true; rm -rf "$SCRATCH"' EXIT

ROOT_NAME="Maestro-Update-v${TO_VERSION}"
ROOT="$SCRATCH/$ROOT_NAME"

if unzip -q "$UPDATE_ZIP" -d "$SCRATCH"; then
  pass "wrapper extracts"
else
  fail "wrapper does not extract"
fi

if [ -d "$ROOT" ]; then
  pass "single versioned top-level directory exists"
else
  fail "missing $ROOT_NAME top-level directory"
fi

EXPECTED=(
  "LEIA-ME-PRIMEIRO.md"
  "PROMPT-1-PREPARAR.txt"
  "PROMPT-2-VERIFICAR.txt"
  "CANARIO-MAC-WINDOWS.md"
  "Maestro-v${TO_VERSION}.zip"
  "Maestro-v${TO_VERSION}.sha256"
)
for rel in "${EXPECTED[@]}"; do
  if [ -f "$ROOT/$rel" ]; then
    pass "contains $rel"
  else
    fail "missing $rel"
  fi
done

TOP_LEVEL_COUNT=$(find "$SCRATCH" -mindepth 1 -maxdepth 1 | wc -l | tr -d ' ')
[ "$TOP_LEVEL_COUNT" = 1 ] \
  && pass "wrapper has exactly one top-level entry" \
  || fail "wrapper has $TOP_LEVEL_COUNT top-level entries"

if [ -f "$ROOT/Maestro-v${TO_VERSION}.zip" ] && [ -f "$ROOT/Maestro-v${TO_VERSION}.sha256" ]; then
  EXPECTED_SHA=$(awk '{print $1}' "$ROOT/Maestro-v${TO_VERSION}.sha256")
  ACTUAL_SHA=$(shasum -a 256 "$ROOT/Maestro-v${TO_VERSION}.zip" | awk '{print $1}')
  [ "$EXPECTED_SHA" = "$ACTUAL_SHA" ] \
    && pass "embedded release matches its SHA-256" \
    || fail "embedded release checksum mismatch"

  INNER_ROOT="$SCRATCH/inner"
  mkdir -p "$INNER_ROOT"
  unzip -q "$ROOT/Maestro-v${TO_VERSION}.zip" -d "$INNER_ROOT"
  [ "$(cat "$INNER_ROOT/Maestro/VERSION" 2>/dev/null)" = "$TO_VERSION" ] \
    && pass "embedded release VERSION is $TO_VERSION" \
    || fail "embedded release VERSION is not $TO_VERSION"
  [ ! -e "$INNER_ROOT/Maestro/data" ] \
    && pass "embedded release does not ship data/" \
    || fail "embedded release ships data/"
fi

if find "$ROOT" -path '*/data' -o -path '*/data/*' | grep -q .; then
  fail "update wrapper contains a data/ payload"
else
  pass "update wrapper contains no data/ payload"
fi

README="$ROOT/LEIA-ME-PRIMEIRO.md"
if [ -f "$README" ]; then
  grep -q "$FROM_VERSION" "$README" && grep -q "$TO_VERSION" "$README" \
    && pass "instructions bind the exact from/to versions" \
    || fail "instructions do not bind both versions"
  grep -qi 'não extraia.*por cima\|nao extraia.*por cima' "$README" \
    && pass "instructions forbid extract-over" \
    || fail "instructions do not forbid extract-over"
  grep -q 'Maestro-old' "$README" && grep -q 'data/' "$README" \
    && pass "instructions require rollback copy and data/ migration" \
    || fail "instructions omit Maestro-old or data/ migration"
  grep -qi 'feche.*Claude' "$README" \
    && pass "instructions close Claude before folder replacement" \
    || fail "instructions do not close Claude before replacement"
fi

PRE="$ROOT/PROMPT-1-PREPARAR.txt"
POST="$ROOT/PROMPT-2-VERIFICAR.txt"
if [ -f "$PRE" ]; then
  grep -q 'README-INSTALL.md' "$PRE" && grep -q 'maestro-setup-update' "$PRE" \
    && pass "pre-update prompt delegates to the shipped update contract" \
    || fail "pre-update prompt bypasses the shipped update contract"
  grep -qi 'não mova\|nao mova' "$PRE" \
    && pass "pre-update prompt does not self-replace the running folder" \
    || fail "pre-update prompt lacks the self-replacement boundary"
fi
if [ -f "$POST" ]; then
  grep -q "$TO_VERSION" "$POST" && grep -q 'data/' "$POST" \
    && pass "post-update prompt verifies version and data/" \
    || fail "post-update prompt omits version or data/ verification"
  grep -Fq "diff -qr -- data ../Maestro-old-${FROM_VERSION}/data" "$POST" \
    && pass "post-update prompt compares new data/ directly with the old copy" \
    || fail "post-update prompt lacks a durable old/new data comparison"
  grep -qi 'Yoda' "$POST" && grep -qi 'Agent' "$POST" \
    && pass "post-update prompt requires a live Yoda Agent canary" \
    || fail "post-update prompt omits the live Agent canary"
fi

# Execute the published rename -> extract -> copy-only-data ritual against a
# disposable 0.1.11-shaped installation. This proves the kit cannot erase or
# replace owner data when the update is followed literally.
REHEARSAL="$SCRATCH/rehearsal"
mkdir -p "$REHEARSAL/Maestro/data/agents" "$REHEARSAL/Maestro/data/workspaces"
printf '%s\n' "$FROM_VERSION" > "$REHEARSAL/Maestro/VERSION"
printf 'agent-owner-sentinel\r\n' > "$REHEARSAL/Maestro/data/agents/owner.txt"
printf 'workspace-sentinel\n' > "$REHEARSAL/Maestro/data/workspaces/case.txt"
BEFORE_DATA=$(shasum -a 256 \
  "$REHEARSAL/Maestro/data/agents/owner.txt" \
  "$REHEARSAL/Maestro/data/workspaces/case.txt" \
  | awk '{print $1}' | shasum -a 256 | awk '{print $1}')
mv "$REHEARSAL/Maestro" "$REHEARSAL/Maestro-old-${FROM_VERSION}"
unzip -q "$ROOT/Maestro-v${TO_VERSION}.zip" -d "$REHEARSAL"
cp -R "$REHEARSAL/Maestro-old-${FROM_VERSION}/data" "$REHEARSAL/Maestro/"
AFTER_DATA=$(shasum -a 256 \
  "$REHEARSAL/Maestro/data/agents/owner.txt" \
  "$REHEARSAL/Maestro/data/workspaces/case.txt" \
  | awk '{print $1}' | shasum -a 256 | awk '{print $1}')

[ "$BEFORE_DATA" = "$AFTER_DATA" ] \
  && pass "published update ritual preserves data/ byte-for-byte" \
  || fail "published update ritual changed data/"
[ "$(cat "$REHEARSAL/Maestro/VERSION")" = "$TO_VERSION" ] \
  && [ "$(cat "$REHEARSAL/Maestro-old-${FROM_VERSION}/VERSION")" = "$FROM_VERSION" ] \
  && pass "published ritual keeps the old core and activates only $TO_VERSION" \
  || fail "published ritual does not preserve old/new core identities"

printf '\nSummary: %d pass, %d fail\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
