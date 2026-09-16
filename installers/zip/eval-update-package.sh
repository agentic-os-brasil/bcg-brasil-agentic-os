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
  "TESTE-MAC-WINDOWS.md"
  "DIAGNOSTICO-HOOKS.md"
  "Maestro-v${TO_VERSION}-macos.zip"
  "Maestro-v${TO_VERSION}-macos.sha256"
  "Maestro-v${TO_VERSION}-windows-powershell.zip"
  "Maestro-v${TO_VERSION}-windows-powershell.sha256"
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

for platform in macos windows-powershell; do
  release="$ROOT/Maestro-v${TO_VERSION}-${platform}.zip"
  sidecar="$ROOT/Maestro-v${TO_VERSION}-${platform}.sha256"
  if [ -f "$release" ] && [ -f "$sidecar" ]; then
    EXPECTED_SHA=$(awk '{print $1}' "$sidecar")
    ACTUAL_SHA=$(shasum -a 256 "$release" | awk '{print $1}')
    [ "$EXPECTED_SHA" = "$ACTUAL_SHA" ] \
      && pass "$platform release matches its SHA-256" \
      || fail "$platform release checksum mismatch"

    INNER_ROOT="$SCRATCH/inner-$platform"
    mkdir -p "$INNER_ROOT"
    unzip -q "$release" -d "$INNER_ROOT"
    [ "$(cat "$INNER_ROOT/Maestro/VERSION" 2>/dev/null)" = "$TO_VERSION" ] \
      && pass "$platform VERSION is $TO_VERSION" \
      || fail "$platform VERSION is not $TO_VERSION"
    [ ! -e "$INNER_ROOT/Maestro/data" ] \
      && pass "$platform release does not ship data/" \
      || fail "$platform release ships data/"
  fi
done

WIN_SETTINGS="$SCRATCH/inner-windows-powershell/Maestro/.claude/settings.json"
MAC_SETTINGS="$SCRATCH/inner-macos/Maestro/.claude/settings.json"
if [ -f "$WIN_SETTINGS" ]; then
  [ "$(grep -c '"shell": "powershell"' "$WIN_SETTINGS")" = 6 ] \
    && pass "Windows settings explicitly select PowerShell for all six hooks" \
    || fail "Windows settings do not select PowerShell for all six hooks"
  if grep -qiE 'bash|\.sh(["[:space:]]|$)' "$WIN_SETTINGS"; then
    fail "Windows settings still invoke Bash"
  else
    pass "Windows settings contain no Bash hook command"
  fi
fi
if [ -f "$MAC_SETTINGS" ]; then
  [ "$(grep -c '\.sh' "$MAC_SETTINGS")" = 6 ] \
    && pass "macOS settings retain all six Bash hooks" \
    || fail "macOS settings do not retain all six Bash hooks"
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
  grep -qi 'não contém\|nao contem' "$README" \
    && grep -qi 'apenas receitas' "$README" \
    && grep -qi 'núcleo completo\|nucleo completo' "$README" \
    && grep -q "Maestro-v${TO_VERSION}-macos.zip" "$README" \
    && grep -q "Maestro-v${TO_VERSION}-windows-powershell.zip" "$README" \
    && pass "instructions identify one kit with two complete platform cores" \
    || fail "instructions do not distinguish the universal kit from its complete platform cores"
  grep -qi 'não se instala sozinho\|nao se instala sozinho' "$README" \
    && grep -qi 'não procura este kit\|nao procura este kit' "$README" \
    && grep -qi 'atualizações do Maestro\|atualizacoes do Maestro' "$README" \
    && pass "instructions state that download and activation are not automatic" \
    || fail "instructions imply automatic download or activation"
  grep -qi 'não extraia.*por cima\|nao extraia.*por cima' "$README" \
    && pass "instructions forbid extract-over" \
    || fail "instructions do not forbid extract-over"
  grep -q 'Maestro-old' "$README" && grep -q 'data/' "$README" \
    && pass "instructions require rollback copy and data/ migration" \
    || fail "instructions omit Maestro-old or data/ migration"
  grep -qi 'feche.*Claude' "$README" \
    && pass "instructions close Claude before folder replacement" \
    || fail "instructions do not close Claude before replacement"
  grep -q 'claude --debug hooks' "$README" \
    && grep -q '/status' "$README" \
    && grep -q '/hooks' "$README" \
    && grep -q 'machine-readable' "$README" \
    && pass "instructions require hook discovery preflight from the Maestro root" \
    || fail "instructions omit the hook discovery preflight"
  grep -q 'UPDATE-RUNBOOK.md' "$README" \
    && grep -q '/goal' "$README" \
    && grep -q 'Auto mode' "$README" \
    && pass "instructions route long-running verification through the shipped contract" \
    || fail "instructions omit the long-running update contract"
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
  grep -q ".maestro-update-baseline-${FROM_VERSION}.json" "$POST" \
    && grep -q 'Get-FileHash' "$POST" \
    && grep -qi 'SHA-256' "$POST" \
    && pass "post-update prompt verifies every preexisting data file against the baseline" \
    || fail "post-update prompt lacks a durable preexisting-file baseline comparison"
  grep -q '^/goal ' "$POST" \
    && grep -qi 'Yoda' "$POST" \
    && grep -qi 'Agent' "$POST" \
    && ! grep -qi 'Darwin' "$POST" \
    && grep -q "data/canary/update-${TO_VERSION}.json" "$POST" \
    && grep -q 'status: pass' "$POST" \
    && grep -q 'checks\[\*\]\.state' "$POST" \
    && grep -q 'target_release_sha256\|SHA-256 do ZIP exato' "$POST" \
    && grep -q 'baseline.*SHA-256\|SHA-256 do.*baseline' "$POST" \
    && grep -q -- '-stale-<attempt_id>.json' "$POST" \
    && pass "post-update goal requires real Yoda and the closed durable receipt schema" \
    || fail "post-update goal omits Yoda, persistence or the closed receipt schema"
  grep -q '/status' "$POST" && grep -q '/hooks' "$POST" \
    && grep -q 'machine-readable' "$POST" \
    && grep -qi 'configurado.*carregado.*executado' "$POST" \
    && pass "post-update prompt separates configured, loaded and executed hooks" \
    || fail "post-update prompt does not prove hooks were loaded and executed"
fi

for platform in macos windows-powershell; do
  platform_root="$SCRATCH/inner-$platform/Maestro"
  RUNBOOK="$platform_root/UPDATE-RUNBOOK.md"
  if [ -f "$RUNBOOK" ] \
      && grep -q '^contract_id: maestro-update-long-run-v1$' "$RUNBOOK" \
      && grep -q '^model_family: opus$' "$RUNBOOK" \
      && grep -q '^preferred_effort: xhigh$' "$RUNBOOK" \
      && grep -q '^  - yoda$' "$RUNBOOK" \
      && ! grep -q '^  - darwin$' "$RUNBOOK"; then
    pass "$platform ships the Yoda-only long-running Agent contract"
  else
    fail "$platform omits or weakens the long-running Agent contract"
  fi
done

HOOK_DIAG="$ROOT/DIAGNOSTICO-HOOKS.md"
QUALIFICATION_TEST="$ROOT/TESTE-MAC-WINDOWS.md"
if [ -f "$HOOK_DIAG" ]; then
  grep -q 'claude --debug hooks' "$HOOK_DIAG" \
    && grep -q 'allowManagedHooksOnly' "$HOOK_DIAG" \
    && grep -q 'disableAllHooks' "$HOOK_DIAG" \
    && grep -q 'Maestro-hook-probe' "$HOOK_DIAG" \
    && grep -q 'Expand-Archive' "$HOOK_DIAG" \
    && grep -q 'ditto -x -k' "$HOOK_DIAG" \
    && pass "hook diagnosis distinguishes discovery, policy and runtime failures" \
    || fail "hook diagnosis omits a required failure branch"
  if grep -q 'ExecutionPolicy Bypass' "$HOOK_DIAG"; then
    fail "hook diagnosis tells users to bypass PowerShell policy"
  else
    pass "hook diagnosis preserves normal PowerShell policy"
  fi
  grep -qi 'confiança.*workspace\|workspace.*confiança' "$HOOK_DIAG" \
    && grep -qi 'não envie.*log bruto\|nao envie.*log bruto' "$HOOK_DIAG" \
    && pass "hook diagnosis checks workspace trust and sanitizes debug evidence" \
    || fail "hook diagnosis omits workspace trust or debug-log privacy"
fi
if [ -f "$QUALIFICATION_TEST" ]; then
  grep -q '/status' "$QUALIFICATION_TEST" && grep -q '/hooks' "$QUALIFICATION_TEST" \
    && grep -q 'machine-readable' "$QUALIFICATION_TEST" \
    && grep -q 'DIAGNOSTICO-HOOKS.md' "$QUALIFICATION_TEST" \
    && pass "cross-platform qualification gates on effective hook loading" \
    || fail "cross-platform qualification does not gate on effective hook loading"
fi

# Execute rename -> extract -> copy-only-data -> first SessionStart against both
# platform payloads. Lifecycle metadata and new backfills may change, but every
# file that already belonged to the owner must remain byte-identical.
rehearse_platform() {
  platform="$1"
  rehearsal="$SCRATCH/rehearsal-$platform"
  mkdir -p "$rehearsal/Maestro/data/agents" "$rehearsal/Maestro/data/workspaces"
  printf '%s\n' "$FROM_VERSION" > "$rehearsal/Maestro/VERSION"
  printf 'agent-owner-sentinel\r\n' > "$rehearsal/Maestro/data/agents/owner.txt"
  printf 'workspace-sentinel\n' > "$rehearsal/Maestro/data/workspaces/case.txt"
  before_data=$(shasum -a 256 \
    "$rehearsal/Maestro/data/agents/owner.txt" \
    "$rehearsal/Maestro/data/workspaces/case.txt" \
    | awk '{print $1}' | shasum -a 256 | awk '{print $1}')
  mv "$rehearsal/Maestro" "$rehearsal/Maestro-old-${FROM_VERSION}"
  unzip -q "$ROOT/Maestro-v${TO_VERSION}-$platform.zip" -d "$rehearsal"
  cp -R "$rehearsal/Maestro-old-${FROM_VERSION}/data" "$rehearsal/Maestro/"

  if [ "$platform" = "macos" ]; then
    CLAUDE_PROJECT_DIR="$rehearsal/Maestro" \
      bash "$rehearsal/Maestro/.claude/hooks/first-run-scaffold.sh" >/dev/null 2>&1
  elif command -v pwsh >/dev/null 2>&1; then
    CLAUDE_PROJECT_DIR="$rehearsal/Maestro" \
      pwsh -NoLogo -NoProfile -File \
        "$rehearsal/Maestro/.claude/hooks/first-run-scaffold.ps1" >/dev/null 2>&1
  else
    fail "PowerShell is required to rehearse the Windows first-open update"
    return
  fi

  after_data=$(shasum -a 256 \
    "$rehearsal/Maestro/data/agents/owner.txt" \
    "$rehearsal/Maestro/data/workspaces/case.txt" \
    | awk '{print $1}' | shasum -a 256 | awk '{print $1}')
  [ "$before_data" = "$after_data" ] \
    && pass "$platform first open preserves every preexisting owner sentinel" \
    || fail "$platform first open changed preexisting owner content"
  [ "$(cat "$rehearsal/Maestro/VERSION")" = "$TO_VERSION" ] \
    && [ "$(cat "$rehearsal/Maestro-old-${FROM_VERSION}/VERSION")" = "$FROM_VERSION" ] \
    && pass "$platform ritual keeps the old core and activates only $TO_VERSION" \
    || fail "$platform ritual does not preserve old/new core identities"
}

rehearse_platform macos
rehearse_platform windows-powershell

printf '\nSummary: %d pass, %d fail\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
