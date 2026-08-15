#!/usr/bin/env bash
# Maestro release eval — loop-able QA harness for the shippable ZIP.
#
# Usage:
#   installers/zip/eval-release.sh [--zip PATH] [--keep] [--verbose]
#
# Defaults to the highest-versioned dist/Maestro-v*.zip. Runs a fixed battery
# of checks and prints a PASS/FAIL summary. Exit code is non-zero on any FAIL,
# so you can chain it in dev loops:
#   while ! installers/zip/eval-release.sh; do vim ...; bash installers/zip/build-release.sh 0.1.0; done
#
# The eval is intentionally self-contained: no Go toolchain, no external deps
# beyond unzip / shasum / python3 (for JSON parsing).

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEMPLATE_DIR="$REPO_ROOT/installers/zip/user-template"
DIST_DIR="$REPO_ROOT/dist"

ZIP_PATH=""
KEEP_SCRATCH=0
VERBOSE=0

while [ $# -gt 0 ]; do
  case "$1" in
    --zip) ZIP_PATH="$2"; shift 2 ;;
    --keep) KEEP_SCRATCH=1; shift ;;
    --verbose|-v) VERBOSE=1; shift ;;
    -h|--help)
      sed -n '2,15p' "$0"
      exit 0
      ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

if [ -z "$ZIP_PATH" ]; then
  ZIP_PATH=$(ls -1 "$DIST_DIR"/Maestro-v*.zip 2>/dev/null | sort -V | tail -1)
fi

if [ ! -f "$ZIP_PATH" ]; then
  echo "no ZIP found (looked in $DIST_DIR)" >&2
  echo "run: bash installers/zip/build-release.sh <version>  first" >&2
  exit 2
fi

RED=$'\033[31m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; DIM=$'\033[2m'; RESET=$'\033[0m'

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0
FAILURES=()

pass() { PASS_COUNT=$((PASS_COUNT+1)); printf '  %sPASS%s  %s\n' "$GREEN" "$RESET" "$1"; }
fail() { FAIL_COUNT=$((FAIL_COUNT+1)); FAILURES+=("$1"); printf '  %sFAIL%s  %s\n' "$RED" "$RESET" "$1"; }
skip() { SKIP_COUNT=$((SKIP_COUNT+1)); printf '  %sSKIP%s  %s\n' "$YELLOW" "$RESET" "$1"; }
info() { [ "$VERBOSE" = 1 ] && printf '  %s...%s   %s\n' "$DIM" "$RESET" "$1" || true; }
phase() { printf '\n%s%s%s\n' "$YELLOW" "$1" "$RESET"; }

SCRATCH_ROOT=$(mktemp -d -t maestro-eval-XXXXXX)
MAESTRO_DIR="$SCRATCH_ROOT/Maestro"
cleanup() {
  chmod -R u+w "$SCRATCH_ROOT" 2>/dev/null || true
  if [ "$KEEP_SCRATCH" = 1 ]; then
    echo ""
    echo "scratch kept: $SCRATCH_ROOT"
  else
    rm -rf "$SCRATCH_ROOT"
  fi
}
trap cleanup EXIT

printf '%sMaestro release eval%s\n' "$YELLOW" "$RESET"
printf '  ZIP:      %s\n' "$ZIP_PATH"
printf '  scratch:  %s\n' "$SCRATCH_ROOT"

# --------------------------------------------------------------------------
phase "Phase 1 — Integrity"
# --------------------------------------------------------------------------

SHA_FILE="${ZIP_PATH%.zip}.sha256"
if [ -f "$SHA_FILE" ]; then
  EXPECTED_SHA=$(awk '{print $1}' "$SHA_FILE")
  ACTUAL_SHA=$(shasum -a 256 "$ZIP_PATH" | awk '{print $1}')
  if [ "$EXPECTED_SHA" = "$ACTUAL_SHA" ]; then
    pass "sha256 matches sidecar ($ACTUAL_SHA)"
  else
    fail "sha256 mismatch: sidecar=$EXPECTED_SHA actual=$ACTUAL_SHA"
  fi
else
  skip "no .sha256 sidecar next to ZIP"
fi

if unzip -q "$ZIP_PATH" -d "$SCRATCH_ROOT"; then
  pass "ZIP extracted cleanly"
else
  fail "unzip failed — aborting further checks"
  exit 1
fi

if [ -d "$MAESTRO_DIR" ]; then
  pass "top-level Maestro/ present after extract"
else
  fail "no Maestro/ dir at top-level of ZIP"
  exit 1
fi

for pattern in '.DS_Store' '__pycache__' '*.pyc'; do
  if find "$MAESTRO_DIR" -name "$pattern" -print -quit | grep -q .; then
    fail "dev artifact leaked into ZIP: $pattern"
  else
    pass "no $pattern artifacts in ZIP"
  fi
done

# --------------------------------------------------------------------------
phase "Phase 2 — Structure (user-facing files)"
# --------------------------------------------------------------------------

REQUIRED_FILES=(
  "VERSION"
  "CLAUDE.md"
  "WELCOME.md"
  "README-INSTALL.md"
  ".claude/settings.json"
  ".claude/hooks/first-run-scaffold.sh"
  "bundles/base/skills/INDEX.md"
  "bundles/base/skills/catalog.json"
  "bundles/base/skills/agent-skill-policy.json"
  "bundles/base/distribution.json"
)
for f in "${REQUIRED_FILES[@]}"; do
  if [ -f "$MAESTRO_DIR/$f" ]; then
    pass "file present: $f"
  else
    fail "file missing: $f"
  fi
done

for d in "bundles/base/agents"; do
  if [ -d "$MAESTRO_DIR/$d" ] && [ -n "$(ls -A "$MAESTRO_DIR/$d" 2>/dev/null)" ]; then
    pass "dir present + non-empty: $d"
  else
    fail "dir missing or empty: $d"
  fi
done

VERSION_CONTENT=$(cat "$MAESTRO_DIR/VERSION" 2>/dev/null)
if [ -n "$VERSION_CONTENT" ] && echo "$VERSION_CONTENT" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  pass "VERSION file is semver (${VERSION_CONTENT})"
else
  fail "VERSION file malformed: '$VERSION_CONTENT'"
fi

if [ -x "$MAESTRO_DIR/.claude/hooks/first-run-scaffold.sh" ]; then
  pass "first-run-scaffold.sh is executable"
else
  fail "first-run-scaffold.sh is NOT executable"
fi

if [ -d "$MAESTRO_DIR/data" ]; then
  fail "data/ pre-shipped in ZIP (must be created by hook, not shipped)"
else
  pass "data/ correctly absent from ZIP"
fi

# --------------------------------------------------------------------------
phase "Phase 3 — No dev tree leakage"
# --------------------------------------------------------------------------

for forbidden in "cmd" "dev" "internal" ".github"; do
  if [ -e "$MAESTRO_DIR/$forbidden" ]; then
    fail "dev tree leaked: /$forbidden present in user ZIP"
  else
    pass "no /$forbidden in user ZIP"
  fi
done

for forbidden_file in "go.mod" "go.sum"; do
  if [ -e "$MAESTRO_DIR/$forbidden_file" ]; then
    fail "dev tree leaked: /$forbidden_file present in user ZIP"
  else
    pass "no /$forbidden_file in user ZIP"
  fi
done

GO_LEAK_COUNT=$(find "$MAESTRO_DIR" \( -name '*.go' -o -name '*_test.go' \) 2>/dev/null | wc -l | tr -d ' ')
if [ "$GO_LEAK_COUNT" = "0" ]; then
  pass "no .go source files in ZIP"
else
  fail "$GO_LEAK_COUNT .go source file(s) leaked into ZIP"
  if [ "$VERBOSE" = 1 ]; then
    find "$MAESTRO_DIR" \( -name '*.go' -o -name '*_test.go' \) | sed 's|^|      |'
  fi
fi

# --------------------------------------------------------------------------
phase "Phase 4 — First-run scaffold, happy path"
# --------------------------------------------------------------------------

HOOK="$MAESTRO_DIR/.claude/hooks/first-run-scaffold.sh"
info "running: CLAUDE_PROJECT_DIR=$MAESTRO_DIR $HOOK"
if CLAUDE_PROJECT_DIR="$MAESTRO_DIR" bash "$HOOK" >/dev/null 2>"$SCRATCH_ROOT/hook.stderr"; then
  pass "hook exit 0 on first run"
else
  fail "hook non-zero exit on first run"
fi

for sub in agents memory profile workspaces; do
  if [ -d "$MAESTRO_DIR/data/$sub" ]; then
    pass "data/$sub created"
  else
    fail "data/$sub NOT created"
  fi
done

if [ -f "$MAESTRO_DIR/data/.initialized" ] && [ -s "$MAESTRO_DIR/data/.initialized" ]; then
  pass "data/.initialized marker exists + non-empty"
else
  fail "data/.initialized marker missing or empty"
fi

if [ -f "$MAESTRO_DIR/data/.scaffold.log" ] && grep -q "DONE  marker written" "$MAESTRO_DIR/data/.scaffold.log"; then
  pass "data/.scaffold.log has DONE line"
else
  fail "data/.scaffold.log missing DONE line"
fi

if [ -f "$MAESTRO_DIR/data/README.md" ] && grep -q "workspaces" "$MAESTRO_DIR/data/README.md"; then
  pass "data/README.md present + mentions workspaces"
else
  fail "data/README.md missing or malformed"
fi

if [ -f "$MAESTRO_DIR/FIRST-RUN-FAILED.txt" ]; then
  fail "FIRST-RUN-FAILED.txt appeared on happy path (should not)"
else
  pass "no FIRST-RUN-FAILED.txt on happy path"
fi

STDERR_LINES=$(wc -l < "$SCRATCH_ROOT/hook.stderr" | tr -d ' ')
if [ "$STDERR_LINES" = "0" ]; then
  pass "hook silent on stderr (0 lines)"
else
  fail "hook noisy on happy path: $STDERR_LINES stderr line(s)"
  [ "$VERBOSE" = 1 ] && sed 's|^|      |' "$SCRATCH_ROOT/hook.stderr"
fi

# --------------------------------------------------------------------------
phase "Phase 5 — First-run scaffold, idempotency"
# --------------------------------------------------------------------------

LOG_BEFORE=$(wc -l < "$MAESTRO_DIR/data/.scaffold.log" | tr -d ' ')
if CLAUDE_PROJECT_DIR="$MAESTRO_DIR" bash "$HOOK" >/dev/null 2>&1; then
  pass "hook exit 0 on second run"
else
  fail "hook non-zero on second run"
fi
LOG_AFTER=$(wc -l < "$MAESTRO_DIR/data/.scaffold.log" | tr -d ' ')
if [ "$LOG_BEFORE" = "$LOG_AFTER" ]; then
  pass "second run is a no-op ($LOG_BEFORE lines before/after)"
else
  fail "second run re-scaffolded: log grew $LOG_BEFORE → $LOG_AFTER lines"
fi

# --------------------------------------------------------------------------
phase "Phase 6 — First-run scaffold, failure modes"
# --------------------------------------------------------------------------

# Scenario B: data/ pre-exists and is unwritable. Parent writable → breadcrumb should surface.
FAIL_SCRATCH=$(mktemp -d -t maestro-eval-failB-XXXXXX)
unzip -q "$ZIP_PATH" -d "$FAIL_SCRATCH"
FAIL_MAESTRO="$FAIL_SCRATCH/Maestro"
mkdir "$FAIL_MAESTRO/data"
chmod 555 "$FAIL_MAESTRO/data"

if CLAUDE_PROJECT_DIR="$FAIL_MAESTRO" bash "$FAIL_MAESTRO/.claude/hooks/first-run-scaffold.sh" >/dev/null 2>&1; then
  pass "hook fail-open (exit 0) with unwritable data/ (Scenario B)"
else
  fail "hook non-zero when data/ unwritable"
fi

if [ -f "$FAIL_MAESTRO/FIRST-RUN-FAILED.txt" ] && grep -q "maestro-doctor" "$FAIL_MAESTRO/FIRST-RUN-FAILED.txt"; then
  pass "FIRST-RUN-FAILED.txt breadcrumb visible + points at /maestro-doctor (Scenario B)"
else
  fail "FIRST-RUN-FAILED.txt breadcrumb missing under Scenario B (Yoda Fix 1)"
fi

chmod -R u+w "$FAIL_SCRATCH"
rm -rf "$FAIL_SCRATCH"

# Scenario A: whole project dir is read-only. Breadcrumb must land in $TMPDIR fallback.
FAIL_SCRATCH_A=$(mktemp -d -t maestro-eval-failA-XXXXXX)
unzip -q "$ZIP_PATH" -d "$FAIL_SCRATCH_A"
FAIL_MAESTRO_A="$FAIL_SCRATCH_A/Maestro"
chmod 555 "$FAIL_MAESTRO_A"

TMPDIR_A=$(mktemp -d -t maestro-eval-tmpA-XXXXXX)
SCEN_A_STDERR=$(mktemp -t maestro-eval-stderrA-XXXXXX)

if CLAUDE_PROJECT_DIR="$FAIL_MAESTRO_A" TMPDIR="$TMPDIR_A" bash "$FAIL_MAESTRO_A/.claude/hooks/first-run-scaffold.sh" >/dev/null 2>"$SCEN_A_STDERR"; then
  pass "hook fail-open (exit 0) with read-only project dir (Scenario A)"
else
  fail "hook non-zero when project dir read-only"
fi

FALLBACK_FOUND=$(find "$TMPDIR_A" -maxdepth 1 -name 'Maestro-FIRST-RUN-FAILED-*.txt' 2>/dev/null | head -1)
if [ -n "$FALLBACK_FOUND" ] && grep -q "maestro-doctor" "$FALLBACK_FOUND"; then
  pass "FIRST-RUN-FAILED breadcrumb fallback landed in \$TMPDIR (Scenario A, Yoda Wave 2 fix)"
else
  fail "Scenario A breadcrumb fallback missing — expected file in \$TMPDIR"
fi

if grep -q "Breadcrumb:" "$SCEN_A_STDERR"; then
  pass "hook stderr surfaces fallback path to operator (Scenario A)"
else
  fail "hook stderr does not surface fallback breadcrumb path"
fi

rm -f "$SCEN_A_STDERR"
rm -rf "$TMPDIR_A"
chmod -R u+w "$FAIL_SCRATCH_A"
rm -rf "$FAIL_SCRATCH_A"

# --------------------------------------------------------------------------
phase "Phase 7 — Skills catalog integrity"
# --------------------------------------------------------------------------

CATALOG="$MAESTRO_DIR/bundles/base/skills/catalog.json"
INDEX_MD="$MAESTRO_DIR/bundles/base/skills/INDEX.md"
POLICY="$MAESTRO_DIR/bundles/base/skills/agent-skill-policy.json"

if python3 -c "import json,sys; json.load(open('$CATALOG'))" 2>/dev/null; then
  pass "catalog.json is valid JSON"
else
  fail "catalog.json invalid JSON"
fi
if python3 -c "import json,sys; json.load(open('$POLICY'))" 2>/dev/null; then
  pass "agent-skill-policy.json is valid JSON"
else
  fail "agent-skill-policy.json invalid JSON"
fi

CATALOG_IDS=$(python3 -c "
import json
c=json.load(open('$CATALOG'))
ids=[]
def walk(x):
    if isinstance(x, dict):
        if 'id' in x and isinstance(x['id'], str):
            ids.append(x['id'])
        for v in x.values(): walk(v)
    elif isinstance(x, list):
        for v in x: walk(v)
walk(c)
print('\n'.join(sorted(set(ids))))
" 2>/dev/null)

CATALOG_COUNT=$(printf '%s\n' "$CATALOG_IDS" | grep -c . || true)
info "catalog.json lists $CATALOG_COUNT skill ids"

MISSING_SKILLS=0
while IFS= read -r skill_id; do
  [ -z "$skill_id" ] && continue
  if [ ! -f "$MAESTRO_DIR/bundles/base/skills/$skill_id/SKILL.md" ]; then
    MISSING_SKILLS=$((MISSING_SKILLS+1))
    [ "$VERBOSE" = 1 ] && echo "      missing SKILL.md for: $skill_id"
  fi
done <<< "$CATALOG_IDS"
if [ "$MISSING_SKILLS" = "0" ]; then
  pass "every catalog skill has a SKILL.md on disk ($CATALOG_COUNT skills)"
else
  fail "$MISSING_SKILLS catalog skill(s) missing SKILL.md"
fi

POLICY_SKILLS=$(python3 -c "
import json
p=json.load(open('$POLICY'))
out=[]
for r in p.get('direct', []):
    for s in r.get('skill_ids', []): out.append(s)
print('\n'.join(sorted(set(out))))
" 2>/dev/null)
POLICY_ORPHANS=0
while IFS= read -r sid; do
  [ -z "$sid" ] && continue
  if [ ! -f "$MAESTRO_DIR/bundles/base/skills/$sid/SKILL.md" ]; then
    POLICY_ORPHANS=$((POLICY_ORPHANS+1))
    [ "$VERBOSE" = 1 ] && echo "      policy references missing skill: $sid"
  fi
done <<< "$POLICY_SKILLS"
if [ "$POLICY_ORPHANS" = "0" ]; then
  pass "every agent-skill-policy skill_id resolves to a real skill dir"
else
  fail "$POLICY_ORPHANS policy skill_id(s) reference non-existent skills"
fi

INDEX_COUNT=$(grep -cE '^\| [A-Z]' "$INDEX_MD" 2>/dev/null)
INDEX_COUNT=${INDEX_COUNT:-0}
if [ "$INDEX_COUNT" -gt 0 ] 2>/dev/null; then
  pass "INDEX.md has $INDEX_COUNT skill row(s)"
else
  fail "INDEX.md has no skill rows (regex miss?)"
fi

# --------------------------------------------------------------------------
phase "Phase 8 — Distribution manifest coverage"
# --------------------------------------------------------------------------

DIST_JSON="$MAESTRO_DIR/bundles/base/distribution.json"
if python3 -c "import json; json.load(open('$DIST_JSON'))" 2>/dev/null; then
  pass "distribution.json is valid JSON"

  DIST_PATHS=$(python3 -c "
import json
d=json.load(open('$DIST_JSON'))
paths=[]
def walk(x):
    if isinstance(x, dict):
        for k,v in x.items():
            if isinstance(v, str) and ('/' in v or v.endswith('.json') or v.endswith('.md')):
                paths.append(v)
            walk(v)
    elif isinstance(x, list):
        for v in x: walk(v)
walk(d)
print('\n'.join(sorted(set(paths))))
")
  MISSING_ASSETS=0
  MISSING_LIST=""
  while IFS= read -r p; do
    [ -z "$p" ] && continue
    # asset paths in distribution.json are relative to the bundle root
    for candidate in "$MAESTRO_DIR/bundles/base/$p" "$MAESTRO_DIR/$p"; do
      if [ -e "$candidate" ]; then
        continue 2
      fi
    done
    MISSING_ASSETS=$((MISSING_ASSETS+1))
    MISSING_LIST="$MISSING_LIST $p"
  done <<< "$DIST_PATHS"
  if [ "$MISSING_ASSETS" = "0" ]; then
    pass "every distribution.json path exists in ZIP"
  else
    fail "$MISSING_ASSETS distribution.json path(s) missing from ZIP"
    [ "$VERBOSE" = 1 ] && printf '      %s\n' $MISSING_LIST
  fi
else
  fail "distribution.json invalid JSON"
fi

# --------------------------------------------------------------------------
phase "Phase 9 — Yoda fix regression checks"
# --------------------------------------------------------------------------

README="$MAESTRO_DIR/README-INSTALL.md"
if grep -q "Copiar" "$README" && grep -q "Ctrl+C" "$README" && grep -q "Option" "$README"; then
  pass "README-INSTALL step 4 uses Copiar + Ctrl+C + Option (Yoda Fix 2)"
else
  fail "README-INSTALL step 4 missing Copiar/Ctrl+C/Option — Yoda Fix 2 regressed"
fi

if grep -qE "confirmar.*data.*maestro-doctor|maestro-doctor.*data" "$README"; then
  pass "README-INSTALL step 7 gates deletion on data/ present + doctor green"
else
  fail "README-INSTALL step 7 missing dual-gate on deletion"
fi

CLAUDE_MD="$MAESTRO_DIR/CLAUDE.md"
if grep -q "FIRST-RUN-FAILED.txt" "$CLAUDE_MD" && grep -q "maestro-doctor" "$CLAUDE_MD"; then
  pass "CLAUDE.md references FIRST-RUN-FAILED.txt + /maestro-doctor (Yoda Fix 1)"
else
  fail "CLAUDE.md missing FIRST-RUN-FAILED.txt breadcrumb reference"
fi

# --------------------------------------------------------------------------
phase "Phase 10 — Settings + hook wiring"
# --------------------------------------------------------------------------

SETTINGS="$MAESTRO_DIR/.claude/settings.json"
if python3 -c "import json; json.load(open('$SETTINGS'))" 2>/dev/null; then
  pass "settings.json is valid JSON"
else
  fail "settings.json invalid JSON"
fi
if grep -q "first-run-scaffold.sh" "$SETTINGS" && grep -q "SessionStart" "$SETTINGS"; then
  pass "settings.json wires first-run-scaffold.sh to SessionStart"
else
  fail "settings.json does NOT wire scaffold to SessionStart"
fi

# --------------------------------------------------------------------------
phase "Phase 11 — Workspace creation smoke test"
# --------------------------------------------------------------------------

# Simulate user starting a project. The hook created workspaces/; verify a project
# subdir can be created and marker files land where expected.
if mkdir -p "$MAESTRO_DIR/data/workspaces/demo-project" \
   && echo "demo" > "$MAESTRO_DIR/data/workspaces/demo-project/README.md" \
   && [ -f "$MAESTRO_DIR/data/workspaces/demo-project/README.md" ]; then
  pass "workspace dir writable + accepts project subdir"
else
  fail "workspace dir not writable"
fi

if mkdir -p "$MAESTRO_DIR/data/memory/notes" \
   && echo "test" > "$MAESTRO_DIR/data/memory/notes/first.md" \
   && [ -f "$MAESTRO_DIR/data/memory/notes/first.md" ]; then
  pass "memory dir writable + accepts note file"
else
  fail "memory dir not writable"
fi

# --------------------------------------------------------------------------
phase "Phase 12 — Memory scaffold + dreaming hook wiring"
# --------------------------------------------------------------------------

# 12a: Memory sub-tiers exist after first-run scaffold (Phase 4 already ran the hook)
for tier in recent weekly medium-term lifetime policies; do
  if [ -d "$MAESTRO_DIR/data/memory/$tier" ]; then
    pass "data/memory/$tier created by scaffold"
  else
    fail "data/memory/$tier NOT created by scaffold"
  fi
done

# 12b: Profile placeholder files exist
for pfile in identity.json; do
  if [ -f "$MAESTRO_DIR/data/profile/$pfile" ]; then
    pass "data/profile/$pfile placeholder created by scaffold"
  else
    fail "data/profile/$pfile NOT created by scaffold"
  fi
done

# 12b2: Owner self directory and 10 SELF facet placeholder files (spec 013)
if [ -d "$MAESTRO_DIR/data/owner/self" ]; then
  pass "data/owner/self/ directory created by scaffold"
else
  fail "data/owner/self/ directory NOT created by scaffold"
fi
SELF_FACET_COUNT=0
for facet in owner-identity personal-context professional-role communication-style \
             voice preferences motivations quality-bar decision-rules working-boundaries; do
  if [ -f "$MAESTRO_DIR/data/owner/self/$facet.md" ]; then
    SELF_FACET_COUNT=$((SELF_FACET_COUNT + 1))
  else
    fail "data/owner/self/$facet.md NOT created by scaffold"
  fi
done
if [ "$SELF_FACET_COUNT" -eq 10 ]; then
  pass "all 10 SELF facet placeholder files created by scaffold"
fi
# Verify placeholder content has ## Current section (spec 013 format)
if grep -q "## Current" "$MAESTRO_DIR/data/owner/self/owner-identity.md" 2>/dev/null; then
  pass "SELF facet placeholder contains ## Current section (spec 013)"
else
  fail "SELF facet placeholder missing ## Current section"
fi

# 12c: Hook files exist and are non-empty in the ZIP
for hook in session-start-memory-inject.sh session-stop-dream.sh; do
  if [ -f "$MAESTRO_DIR/.claude/hooks/$hook" ] && [ -s "$MAESTRO_DIR/.claude/hooks/$hook" ]; then
    pass ".claude/hooks/$hook present + non-empty"
  else
    fail ".claude/hooks/$hook missing or empty"
  fi
done

# 12d: settings.json wires memory-inject to SessionStart
if grep -q "session-start-memory-inject.sh" "$SETTINGS" && grep -q "SessionStart" "$SETTINGS"; then
  pass "settings.json wires session-start-memory-inject.sh to SessionStart"
else
  fail "settings.json does NOT wire session-start-memory-inject.sh to SessionStart"
fi

# 12e: settings.json wires dream marker to Stop
if grep -q "session-stop-dream.sh" "$SETTINGS" && grep -q '"Stop"' "$SETTINGS"; then
  pass "settings.json wires session-stop-dream.sh to Stop"
else
  fail "settings.json does NOT wire session-stop-dream.sh to Stop"
fi

# 12f: session-stop-dream.sh produces .dream-requested marker
DREAM_SCRATCH=$(mktemp -d -t maestro-eval-dream-XXXXXX)
unzip -q "$ZIP_PATH" -d "$DREAM_SCRATCH"
DREAM_MAESTRO="$DREAM_SCRATCH/Maestro"
mkdir -p "$DREAM_MAESTRO/data/memory"
if CLAUDE_PROJECT_DIR="$DREAM_MAESTRO" bash "$DREAM_MAESTRO/.claude/hooks/session-stop-dream.sh" >/dev/null 2>&1; then
  pass "session-stop-dream.sh exits 0"
else
  fail "session-stop-dream.sh non-zero exit"
fi
if [ -f "$DREAM_MAESTRO/data/memory/.dream-requested" ] && [ -s "$DREAM_MAESTRO/data/memory/.dream-requested" ]; then
  pass ".dream-requested marker written with timestamp"
else
  fail ".dream-requested marker missing or empty after session-stop-dream.sh"
fi
chmod -R u+w "$DREAM_SCRATCH"
rm -rf "$DREAM_SCRATCH"

# 12g: session-start-memory-inject.sh is fail-open when data/ absent
INJECT_SCRATCH=$(mktemp -d -t maestro-eval-inject-XXXXXX)
unzip -q "$ZIP_PATH" -d "$INJECT_SCRATCH"
INJECT_MAESTRO="$INJECT_SCRATCH/Maestro"
# Do NOT create data/ — simulate first session where scaffold hasn't run yet
if CLAUDE_PROJECT_DIR="$INJECT_MAESTRO" bash "$INJECT_MAESTRO/.claude/hooks/session-start-memory-inject.sh" >/dev/null 2>&1; then
  pass "session-start-memory-inject.sh exits 0 when data/ absent (fail-open)"
else
  fail "session-start-memory-inject.sh blocks when data/ absent"
fi
chmod -R u+w "$INJECT_SCRATCH"
rm -rf "$INJECT_SCRATCH"

# 12h: memory inject outputs session context markers when memory exists
INJECT2_SCRATCH=$(mktemp -d -t maestro-eval-inject2-XXXXXX)
unzip -q "$ZIP_PATH" -d "$INJECT2_SCRATCH"
INJECT2_MAESTRO="$INJECT2_SCRATCH/Maestro"
mkdir -p "$INJECT2_MAESTRO/data/memory/recent" "$INJECT2_MAESTRO/data/memory/lifetime" \
         "$INJECT2_MAESTRO/data/profile" "$INJECT2_MAESTRO/data/owner/self"
printf '{"schema_version":1,"display_name":"Test User","role":"test","context":"","initialized":true}\n' \
  > "$INJECT2_MAESTRO/data/profile/identity.json"
printf '# Test memory entry\nThis is a recent memory.\n' \
  > "$INJECT2_MAESTRO/data/memory/recent/2024-01-01.md"
printf '# professional-role\n\n## Current\n\nSenior AI Scientist.\n' \
  > "$INJECT2_MAESTRO/data/owner/self/professional-role.md"
INJECT2_OUT=$(CLAUDE_PROJECT_DIR="$INJECT2_MAESTRO" bash "$INJECT2_MAESTRO/.claude/hooks/session-start-memory-inject.sh" 2>/dev/null)
if echo "$INJECT2_OUT" | grep -q "maestro:session-context:start"; then
  pass "session-start-memory-inject.sh emits session-context markers"
else
  fail "session-start-memory-inject.sh does NOT emit session-context markers"
fi
if echo "$INJECT2_OUT" | grep -q "Último log diário consolidado"; then
  pass "session-start-memory-inject.sh injects L1 daily log layer"
else
  fail "session-start-memory-inject.sh does NOT inject L1 daily log layer"
fi
if echo "$INJECT2_OUT" | grep -q "Identidade"; then
  pass "session-start-memory-inject.sh injects identity profile"
else
  fail "session-start-memory-inject.sh does NOT inject identity"
fi
if echo "$INJECT2_OUT" | grep -q "SELF do usuário"; then
  pass "session-start-memory-inject.sh injects owner SELF facets section"
else
  fail "session-start-memory-inject.sh does NOT inject owner SELF facets"
fi
if echo "$INJECT2_OUT" | grep -q "professional-role"; then
  pass "session-start-memory-inject.sh includes facet content from data/owner/self/"
else
  fail "session-start-memory-inject.sh does NOT include facet content from data/owner/self/"
fi
chmod -R u+w "$INJECT2_SCRATCH"
rm -rf "$INJECT2_SCRATCH"

# 12h2: dream auto-trigger — session-start-memory-inject.sh emits mandatory dreaming block when
# .dream-requested marker is present; block is suppressed when marker is absent.
INJECT3_SCRATCH=$(mktemp -d -t maestro-inject3-XXXXXX)
INJECT3_MAESTRO="$INJECT3_SCRATCH/Maestro"
mkdir -p "$INJECT3_MAESTRO/data/memory" "$INJECT3_MAESTRO/data/profile" "$INJECT3_MAESTRO/data/owner/self"
INJECT3_HOOK="$INJECT3_MAESTRO/.claude/hooks/session-start-memory-inject.sh"
mkdir -p "$(dirname "$INJECT3_HOOK")"
cp "$MAESTRO_DIR/.claude/hooks/session-start-memory-inject.sh" "$INJECT3_HOOK"
chmod +x "$INJECT3_HOOK"

# Sub-check A: marker present → dream-trigger block emitted
printf '%s\n' "2099-01-01T00:00:00Z" > "$INJECT3_MAESTRO/data/memory/.dream-requested"
INJECT3_OUT_WITH=$(CLAUDE_PROJECT_DIR="$INJECT3_MAESTRO" bash "$INJECT3_HOOK" 2>/dev/null)
if echo "$INJECT3_OUT_WITH" | grep -q "maestro:dream-trigger\|dream-requested"; then
  pass "session-start-memory-inject.sh emits dream-trigger block when .dream-requested present"
else
  fail "session-start-memory-inject.sh does NOT emit dream-trigger block when .dream-requested present"
fi
if echo "$INJECT3_OUT_WITH" | grep -qi "obrigatória\|mandatory\|dream-memory"; then
  pass "dream-trigger block contains mandatory action instruction"
else
  fail "dream-trigger block does NOT contain mandatory action instruction"
fi

# Sub-check B: marker absent → dream-trigger block NOT emitted
rm -f "$INJECT3_MAESTRO/data/memory/.dream-requested"
INJECT3_OUT_WITHOUT=$(CLAUDE_PROJECT_DIR="$INJECT3_MAESTRO" bash "$INJECT3_HOOK" 2>/dev/null)
if echo "$INJECT3_OUT_WITHOUT" | grep -q "maestro:dream-trigger"; then
  fail "session-start-memory-inject.sh emits dream-trigger block even without .dream-requested"
else
  pass "dream-trigger block correctly suppressed when .dream-requested absent"
fi
chmod -R u+w "$INJECT3_SCRATCH"
rm -rf "$INJECT3_SCRATCH"

# 12h3: dream-memory skill documents auto-trigger marker cleanup
DREAM_SKILL="$MAESTRO_DIR/bundles/base/skills/dream-memory/SKILL.md"
if [ -f "$DREAM_SKILL" ] && grep -q "dream-requested\|marker cleanup\|Marker cleanup" "$DREAM_SKILL"; then
  pass "dream-memory skill documents auto-trigger marker cleanup"
else
  fail "dream-memory skill does NOT document auto-trigger marker cleanup"
fi

# 12i: GAP-A — maestro-doctor references correct bundle names (tech-core, not data-practice)
DOCTOR_SKILL="$MAESTRO_DIR/bundles/base/skills/maestro-doctor/SKILL.md"
if [ -f "$DOCTOR_SKILL" ]; then
  if grep -q "bundles/data-practice" "$DOCTOR_SKILL" || grep -q "bundles/engineering-core" "$DOCTOR_SKILL"; then
    fail "maestro-doctor still references obsolete bundle names (data-practice/engineering-core)"
  else
    pass "maestro-doctor references correct bundle names (no data-practice/engineering-core)"
  fi
  if grep -q "bundles/tech-core" "$DOCTOR_SKILL"; then
    pass "maestro-doctor explicitly checks for bundles/tech-core"
  else
    fail "maestro-doctor does NOT check for bundles/tech-core"
  fi
  if grep -q "data/owner" "$DOCTOR_SKILL"; then
    pass "maestro-doctor checks for data/owner/ in workspace health"
  else
    fail "maestro-doctor does NOT check for data/owner/ in workspace health"
  fi
else
  fail "maestro-doctor/SKILL.md not found — cannot check bundle names"
fi

# 12j: GAP-B — maestro-operator skill exists and is registered in catalog
MAESTRO_OP_SKILL="$MAESTRO_DIR/bundles/base/skills/maestro-operator/SKILL.md"
CATALOG="$MAESTRO_DIR/bundles/base/skills/catalog.json"
if [ -f "$MAESTRO_OP_SKILL" ]; then
  pass "maestro-operator/SKILL.md exists"
else
  fail "maestro-operator/SKILL.md NOT found"
fi
if [ -f "$CATALOG" ] && grep -q '"maestro-operator"' "$CATALOG"; then
  pass "maestro-operator registered in catalog.json"
else
  fail "maestro-operator NOT registered in catalog.json"
fi

# 12k: GAP-B — session-start-memory-inject.sh emits maestro-operator pointer
INJECT_HOOK_BUILT="$MAESTRO_DIR/.claude/hooks/session-start-memory-inject.sh"
if [ -f "$INJECT_HOOK_BUILT" ] && grep -q "maestro-operator" "$INJECT_HOOK_BUILT"; then
  pass "session-start-memory-inject.sh emits maestro-operator pointer"
else
  fail "session-start-memory-inject.sh does NOT emit maestro-operator pointer"
fi

# 12k2: tech-core pointer resolves to the real index/catalog location.
# INDEX.md and catalog.json ship at bundles/tech-core/skills/, not at the
# bundle root. When the pointer looks at the root the lines are silently never
# emitted, so the session rollup names the bundle without ever telling the
# runtime where its index is. Asserted against real hook output, not source.
TC_OUT=$(CLAUDE_PROJECT_DIR="$MAESTRO_DIR" bash "$INJECT_HOOK_BUILT" 2>/dev/null)
if [ -f "$MAESTRO_DIR/bundles/tech-core/skills/INDEX.md" ]; then
  if printf '%s' "$TC_OUT" | grep -q "tech-core/skills/INDEX.md"; then
    pass "tech-core pointer emits the real INDEX.md path"
  else
    fail "tech-core INDEX.md exists but the pointer never emits it (wrong path)"
  fi
  if printf '%s' "$TC_OUT" | grep -q "tech-core/skills/catalog.json"; then
    pass "tech-core pointer emits the real catalog.json path"
  else
    fail "tech-core catalog.json exists but the pointer never emits it (wrong path)"
  fi
else
  skip "bundles/tech-core/skills/INDEX.md not in ZIP"
fi

# 12l: GAP-F — owner extended context tree scaffolded
SCAFFOLD_HOOK="$MAESTRO_DIR/.claude/hooks/first-run-scaffold.sh"
if [ -f "$SCAFFOLD_HOOK" ]; then
  if grep -q "owner/registry.json" "$SCAFFOLD_HOOK"; then
    pass "first-run-scaffold.sh creates owner/registry.json"
  else
    fail "first-run-scaffold.sh does NOT create owner/registry.json"
  fi
  if grep -q "owner/operating" "$SCAFFOLD_HOOK"; then
    pass "first-run-scaffold.sh creates owner/operating/"
  else
    fail "first-run-scaffold.sh does NOT create owner/operating/"
  fi
  if grep -q "owner/observations" "$SCAFFOLD_HOOK"; then
    pass "first-run-scaffold.sh creates owner/observations/"
  else
    fail "first-run-scaffold.sh does NOT create owner/observations/"
  fi
  if grep -q "owner/interview" "$SCAFFOLD_HOOK"; then
    pass "first-run-scaffold.sh creates owner/interview/"
  else
    fail "first-run-scaffold.sh does NOT create owner/interview/"
  fi
else
  fail "first-run-scaffold.sh not found — cannot check owner context tree"
fi

# --------------------------------------------------------------------------
# Summary
# --------------------------------------------------------------------------

echo ""
printf '%s──────────────────────────────────────────────%s\n' "$YELLOW" "$RESET"
printf 'Summary:  %s%d pass%s  %s%d fail%s  %s%d skip%s\n' \
  "$GREEN" "$PASS_COUNT" "$RESET" \
  "$RED" "$FAIL_COUNT" "$RESET" \
  "$YELLOW" "$SKIP_COUNT" "$RESET"

if [ "$FAIL_COUNT" -gt 0 ]; then
  echo ""
  echo "Failed checks:"
  for f in "${FAILURES[@]}"; do
    printf '  - %s\n' "$f"
  done
  exit 1
fi

echo ""
echo "All checks green. ZIP is shippable."
exit 0
