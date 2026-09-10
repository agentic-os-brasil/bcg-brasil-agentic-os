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
  fail "brain/ pre-shipped in ZIP (must be created by hook, not shipped)"
else
  pass "brain/ correctly absent from ZIP"
fi

# Slash commands must reference paths that exist in the shipped layout.
# A command body is read verbatim by the runtime, so a path that resolves to
# nothing turns the command into a dead end. /maestro-onboarding is forced on
# the first session, so a wrong path here is the first thing an owner hits.
COMMANDS_DIR="$MAESTRO_DIR/.claude/commands"
if [ -d "$COMMANDS_DIR" ]; then
  CMD_BAD=0
  CMD_CHECKED=0
  for cmd in "$COMMANDS_DIR"/*.md; do
    [ -f "$cmd" ] || continue
    # Pull every backtick-quoted path ending in SKILL.md out of the command body.
    for ref in $(grep -oE '`[^`]*SKILL\.md`' "$cmd" 2>/dev/null | tr -d '`'); do
      CMD_CHECKED=$((CMD_CHECKED+1))
      if [ ! -f "$MAESTRO_DIR/$ref" ]; then
        CMD_BAD=$((CMD_BAD+1))
        fail "$(basename "$cmd") references a path absent from the ZIP: $ref"
      fi
    done
  done
  if [ "$CMD_CHECKED" -eq 0 ]; then
    fail "no SKILL.md references found in .claude/commands/ (check is not exercising anything)"
  elif [ "$CMD_BAD" -eq 0 ]; then
    pass "all $CMD_CHECKED slash-command skill path(s) resolve in the ZIP"
  fi
else
  fail ".claude/commands/ missing from ZIP"
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

for sub in accounts craft daily development learnings memory owner people tasks; do
  if [ -d "$MAESTRO_DIR/brain/$sub" ]; then
    pass "brain/$sub created"
  else
    fail "brain/$sub NOT created"
  fi
done

if [ -f "$MAESTRO_DIR/brain/.initialized" ] && [ -s "$MAESTRO_DIR/brain/.initialized" ]; then
  pass "brain/.initialized marker exists + non-empty"
else
  fail "brain/.initialized marker missing or empty"
fi

if [ -f "$MAESTRO_DIR/brain/.scaffold.log" ] && grep -q "DONE  marker written" "$MAESTRO_DIR/brain/.scaffold.log"; then
  pass "brain/.scaffold.log has DONE line"
else
  fail "brain/.scaffold.log missing DONE line"
fi

if [ -f "$MAESTRO_DIR/brain/README.md" ] && grep -q "workspaces" "$MAESTRO_DIR/brain/README.md"; then
  pass "brain/README.md present + mentions workspaces"
else
  fail "brain/README.md missing or malformed"
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

LOG_BEFORE=$(wc -l < "$MAESTRO_DIR/brain/.scaffold.log" | tr -d ' ')
if CLAUDE_PROJECT_DIR="$MAESTRO_DIR" bash "$HOOK" >/dev/null 2>&1; then
  pass "hook exit 0 on second run"
else
  fail "hook non-zero on second run"
fi
LOG_AFTER=$(wc -l < "$MAESTRO_DIR/brain/.scaffold.log" | tr -d ' ')
if [ "$LOG_BEFORE" = "$LOG_AFTER" ]; then
  pass "second run is a no-op ($LOG_BEFORE lines before/after)"
else
  fail "second run re-scaffolded: log grew $LOG_BEFORE → $LOG_AFTER lines"
fi

# --------------------------------------------------------------------------
phase "Phase 6 — First-run scaffold, failure modes"
# --------------------------------------------------------------------------

# Scenario B: brain/ pre-exists and is unwritable. Parent writable → breadcrumb should surface.
FAIL_SCRATCH=$(mktemp -d -t maestro-eval-failB-XXXXXX)
unzip -q "$ZIP_PATH" -d "$FAIL_SCRATCH"
FAIL_MAESTRO="$FAIL_SCRATCH/Maestro"
mkdir "$FAIL_MAESTRO/data"
chmod 555 "$FAIL_MAESTRO/data"

if CLAUDE_PROJECT_DIR="$FAIL_MAESTRO" bash "$FAIL_MAESTRO/.claude/hooks/first-run-scaffold.sh" >/dev/null 2>&1; then
  pass "hook fail-open (exit 0) with unwritable brain/ (Scenario B)"
else
  fail "hook non-zero when brain/ unwritable"
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

if grep -qE "confirmar.*brain.*maestro-doctor|maestro-doctor.*brain" "$README"; then
  pass "README-INSTALL step 7 gates deletion on brain/ present + doctor green"
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

# The PreToolUse matcher is a regex. Unanchored, "Write" also matches
# TodoWrite, which then reaches the isolation guard with no file target; and a
# matcher naming only Edit and Write never delivers MultiEdit or NotebookEdit
# to the guard at all. Both shapes were live bypasses.
if grep -q '"\^(Edit|MultiEdit|Write|NotebookEdit)\$"' "$SETTINGS"; then
  pass "PreToolUse matcher is anchored and names every file-writing tool"
else
  fail "PreToolUse matcher is not the anchored four-tool form (TodoWrite leaks in, MultiEdit/NotebookEdit never arrive)"
fi

# --------------------------------------------------------------------------
phase "Phase 11 — Workspace creation smoke test"
# --------------------------------------------------------------------------

# Simulate user starting a project. The hook created workspaces/; verify a project
# subdir can be created and marker files land where expected.
if mkdir -p "$MAESTRO_DIR/brain/workspaces/demo-project" \
   && echo "demo" > "$MAESTRO_DIR/brain/workspaces/demo-project/README.md" \
   && [ -f "$MAESTRO_DIR/brain/workspaces/demo-project/README.md" ]; then
  pass "workspace dir writable + accepts project subdir"
else
  fail "workspace dir not writable"
fi

if mkdir -p "$MAESTRO_DIR/brain/memory/notes" \
   && echo "test" > "$MAESTRO_DIR/brain/memory/notes/first.md" \
   && [ -f "$MAESTRO_DIR/brain/memory/notes/first.md" ]; then
  pass "memory dir writable + accepts note file"
else
  fail "memory dir not writable"
fi

# --------------------------------------------------------------------------
phase "Phase 12 — Memory scaffold + dreaming hook wiring"
# --------------------------------------------------------------------------

# 12a: Memory sub-tiers exist after first-run scaffold (Phase 4 already ran the hook)
for tier in recent weekly medium-term lifetime policies; do
  if [ -d "$MAESTRO_DIR/brain/memory/$tier" ]; then
    pass "brain/memory/$tier created by scaffold"
  else
    fail "brain/memory/$tier NOT created by scaffold"
  fi
done

# 12a2: Lifetime eligibility policy exists after first-run scaffold.
# dream-memory/SKILL.md step 5 refuses lifetime promotion without a named
# eligibility policy at brain/memory/policies/lifetime.json ("lifetime
# activation must fail closed"). Spec 006 states the base distribution ships
# that named policy, so an install without it can never activate the permanent
# memory tier.
LIFETIME_POLICY="$MAESTRO_DIR/brain/memory/policies/lifetime.json"
if [ -f "$LIFETIME_POLICY" ]; then
  pass "brain/memory/policies/lifetime.json created by scaffold"
  if python3 -c "import json;json.load(open(r'$(cygpath -w "$LIFETIME_POLICY" 2>/dev/null || echo "$LIFETIME_POLICY")'))" 2>/dev/null; then
    pass "lifetime.json is valid JSON"
  else
    fail "lifetime.json invalid JSON"
  fi
  if grep -q 'deterministic-l3-continuity-v1' "$LIFETIME_POLICY"; then
    pass "lifetime.json names the spec-006 eligibility policy"
  else
    fail "lifetime.json does NOT name the spec-006 eligibility policy"
  fi
else
  fail "brain/memory/policies/lifetime.json NOT created by scaffold (lifetime tier fails closed forever)"
fi

# 12b: Profile placeholder files exist
for pfile in identity.json; do
  if [ -f "$MAESTRO_DIR/brain/owner/$pfile" ]; then
    pass "brain/owner/$pfile placeholder created by scaffold"
  else
    fail "brain/owner/$pfile NOT created by scaffold"
  fi
done

# 12b2: Owner self directory and 10 SELF facet placeholder files (spec 013)
if [ -d "$MAESTRO_DIR/brain/owner/self" ]; then
  pass "brain/owner/self/ directory created by scaffold"
else
  fail "brain/owner/self/ directory NOT created by scaffold"
fi
SELF_FACET_COUNT=0
for facet in owner-identity personal-context professional-role communication-style \
             voice preferences motivations quality-bar decision-rules working-boundaries; do
  if [ -f "$MAESTRO_DIR/brain/owner/self/$facet.md" ]; then
    SELF_FACET_COUNT=$((SELF_FACET_COUNT + 1))
  else
    fail "brain/owner/self/$facet.md NOT created by scaffold"
  fi
done
if [ "$SELF_FACET_COUNT" -eq 10 ]; then
  pass "all 10 SELF facet placeholder files created by scaffold"
fi
# Verify placeholder content has ## Current section (spec 013 format)
if grep -q "## Current" "$MAESTRO_DIR/brain/owner/self/owner-identity.md" 2>/dev/null; then
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
mkdir -p "$DREAM_MAESTRO/brain/memory"
if CLAUDE_PROJECT_DIR="$DREAM_MAESTRO" bash "$DREAM_MAESTRO/.claude/hooks/session-stop-dream.sh" >/dev/null 2>&1; then
  pass "session-stop-dream.sh exits 0"
else
  fail "session-stop-dream.sh non-zero exit"
fi
if [ -f "$DREAM_MAESTRO/brain/memory/.dream-requested" ] && [ -s "$DREAM_MAESTRO/brain/memory/.dream-requested" ]; then
  pass ".dream-requested marker written with timestamp"
else
  fail ".dream-requested marker missing or empty after session-stop-dream.sh"
fi
chmod -R u+w "$DREAM_SCRATCH"
rm -rf "$DREAM_SCRATCH"

# 12g: session-start-memory-inject.sh is fail-open when brain/ absent
INJECT_SCRATCH=$(mktemp -d -t maestro-eval-inject-XXXXXX)
unzip -q "$ZIP_PATH" -d "$INJECT_SCRATCH"
INJECT_MAESTRO="$INJECT_SCRATCH/Maestro"
# Do NOT create brain/ — simulate first session where scaffold hasn't run yet
if CLAUDE_PROJECT_DIR="$INJECT_MAESTRO" bash "$INJECT_MAESTRO/.claude/hooks/session-start-memory-inject.sh" >/dev/null 2>&1; then
  pass "session-start-memory-inject.sh exits 0 when brain/ absent (fail-open)"
else
  fail "session-start-memory-inject.sh blocks when brain/ absent"
fi
chmod -R u+w "$INJECT_SCRATCH"
rm -rf "$INJECT_SCRATCH"

# 12h: memory inject outputs session context markers when memory exists
INJECT2_SCRATCH=$(mktemp -d -t maestro-eval-inject2-XXXXXX)
unzip -q "$ZIP_PATH" -d "$INJECT2_SCRATCH"
INJECT2_MAESTRO="$INJECT2_SCRATCH/Maestro"
mkdir -p "$INJECT2_MAESTRO/brain/memory/recent" "$INJECT2_MAESTRO/brain/memory/lifetime" \
         "$INJECT2_MAESTRO/brain/owner" "$INJECT2_MAESTRO/brain/owner/self"
printf '{"schema_version":1,"display_name":"Test User","role":"test","context":"","initialized":true}\n' \
  > "$INJECT2_MAESTRO/brain/owner/identity.json"
printf '# Test memory entry\nThis is a recent memory.\n' \
  > "$INJECT2_MAESTRO/brain/memory/recent/2024-01-01.md"
printf '# professional-role\n\n## Current\n\nSenior AI Scientist.\n' \
  > "$INJECT2_MAESTRO/brain/owner/self/professional-role.md"
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
  pass "session-start-memory-inject.sh includes facet content from brain/owner/self/"
else
  fail "session-start-memory-inject.sh does NOT include facet content from brain/owner/self/"
fi
chmod -R u+w "$INJECT2_SCRATCH"
rm -rf "$INJECT2_SCRATCH"

# 12h2: dream auto-trigger — session-start-memory-inject.sh emits mandatory dreaming block when
# .dream-requested marker is present; block is suppressed when marker is absent.
INJECT3_SCRATCH=$(mktemp -d -t maestro-inject3-XXXXXX)
INJECT3_MAESTRO="$INJECT3_SCRATCH/Maestro"
mkdir -p "$INJECT3_MAESTRO/brain/memory" "$INJECT3_MAESTRO/brain/owner" "$INJECT3_MAESTRO/brain/owner/self"
INJECT3_HOOK="$INJECT3_MAESTRO/.claude/hooks/session-start-memory-inject.sh"
mkdir -p "$(dirname "$INJECT3_HOOK")"
cp "$MAESTRO_DIR/.claude/hooks/session-start-memory-inject.sh" "$INJECT3_HOOK"
chmod +x "$INJECT3_HOOK"

# Sub-check A: marker present → dream-trigger block emitted
printf '%s\n' "2099-01-01T00:00:00Z" > "$INJECT3_MAESTRO/brain/memory/.dream-requested"
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
rm -f "$INJECT3_MAESTRO/brain/memory/.dream-requested"
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
  if grep -q "brain/owner" "$DOCTOR_SKILL"; then
    pass "maestro-doctor checks for brain/owner/ in workspace health"
  else
    fail "maestro-doctor does NOT check for brain/owner/ in workspace health"
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
phase "Phase 13 — Owner atlas tree"
# --------------------------------------------------------------------------

# start-day, eod, craft-update, feedback-capture, learnings-bridge and
# upward-feedback all operate on brain/owner/. Their reads carry
# continuity between sessions, so a missing tree silently degrades the daily
# ritual rather than failing loudly. Phase 4 already ran the scaffold.
for atlas_dir in \
  accounts \
  daily \
  craft/methods \
  craft/style \
  learnings \
  people \
  development/cdc \
  development/project-feedback \
  development/upward-feedback \
  development/retros
do
  if [ -d "$MAESTRO_DIR/brain/$atlas_dir" ]; then
    pass "brain/$atlas_dir created by scaffold"
  else
    fail "brain/$atlas_dir NOT created by scaffold"
  fi
done

for atlas_file in \
  craft/craft.md \
  learnings/learnings.md \
  development/objectives.md
do
  if [ -f "$MAESTRO_DIR/brain/$atlas_file" ]; then
    pass "brain/$atlas_file created by scaffold"
  else
    fail "brain/$atlas_file NOT created by scaffold"
  fi
done

# objectives.md must ship the headings feedback-capture writes into.
# `append-entry` never creates a heading and refuses one that appears more than
# once on a page, so a missing or duplicated heading means the write is
# declined rather than misfiled — silently, on every fresh install.
OBJ="$MAESTRO_DIR/brain/development/objectives.md"
if [ -f "$OBJ" ]; then
  for heading in "## Aposentados" "#### Evidência — objetivo 1"; do
    # grep -c prints its count and still exits 1 on no match, so the count is
    # taken on its own and the exit status deliberately ignored.
    COUNT=$(grep -cFx "$heading" "$OBJ" 2>/dev/null)
    COUNT=${COUNT:-0}
    if [ "$COUNT" = "1" ]; then
      pass "objectives.md carries exactly one '$heading'"
    elif [ "$COUNT" = "0" ]; then
      fail "objectives.md is missing '$heading' — feedback-capture's write is declined"
    else
      fail "objectives.md repeats '$heading' $COUNT times — append-entry refuses an ambiguous heading"
    fi
  done
else
  fail "brain/development/objectives.md NOT created by scaffold"
fi

# The daily page is authored by start-day, never pre-seeded: a placeholder
# there would be read back as a real entry.
if [ -n "$(ls -A "$MAESTRO_DIR/brain/daily" 2>/dev/null)" ]; then
  fail "brain/daily/ is pre-seeded (must be authored by start-day)"
else
  pass "brain/daily/ correctly left empty for start-day"
fi

# --------------------------------------------------------------------------
phase "Phase 14 — Cross-case guard is platform-independent"
# --------------------------------------------------------------------------

# The guard that stops one client's material being written into another
# client's case folder must behave identically whichever path shape the runtime
# hands it. On Windows the runtime supplies drive-letter paths; treating those
# as relative silently disables the guard while macOS still passes.
#
# End-to-end verdict assertions require the hook to read brain/accounts/.active at
# the same path shape it was handed. On a real Windows host,
# "C:/foo/brain/accounts/.active" is a readable file and the hook's block verdict
# is observable. On a POSIX eval host the synthetic drive-letter tree has no
# filesystem counterpart, so the hook fails-open (by design) and the verdict
# cannot be exercised without pretending the check ran when it did not.
#
# The property the Windows fix actually enforces is that PROJECT_DIR and the
# target path, whichever shape the runtime hands them in, both flow through
# the same canonicalization before the prefix comparison — so a drive-letter
# target is recognized as being inside the cases dir instead of being
# classified as a relative path outside it. That property is testable on any
# platform, and it is what broke on Windows before the fix.
XC_HOOK="$MAESTRO_DIR/.claude/hooks/block-cross-case-writes.sh"
if [ -f "$XC_HOOK" ]; then
  XC_ROOT=$(mktemp -d -t maestro-eval-xcase-XXXXXX)
  mkdir -p "$XC_ROOT/brain/accounts/alfa/cases/tmo" "$XC_ROOT/brain/accounts/beta/cases/tmo"
  printf 'alfa/tmo\n' > "$XC_ROOT/brain/accounts/.active"

  # verdict PROJECT_DIR TARGET -> "block" | "allow"
  # Backslashes must be escaped or the payload is not valid JSON and the hook
  # would exit on a parse failure rather than on its path logic — which would
  # make this check pass for the wrong reason.
  xc_verdict() {
    local esc=${2//\\/\\\\}
    printf '{"tool_name":"Write","tool_input":{"file_path":"%s"}}' "$esc" \
      | CLAUDE_PROJECT_DIR="$1" bash "$XC_HOOK" >/dev/null 2>&1
    [ "$?" -eq 2 ] && printf 'block' || printf 'allow'
  }

  # Kept in sync with canon_path() in
  # installers/zip/user-template/.claude/hooks/block-cross-case-writes.sh.
  # Any change to the hook's canonicalization rule must be mirrored here so
  # this eval reflects the same classification the hook performs on Windows.
  xc_canon() {
    local p="$1" root="" out="" seg oldIFS
    p="${p//\\//}"
    case "$p" in
      /[A-Za-z]/*)
        root="$(printf '%s' "${p#/}" | cut -c1):"
        p="${p#/?}"
        ;;
      [A-Za-z]:/*)
        root="$(printf '%s' "$p" | cut -c1):"
        p="${p#??}"
        ;;
    esac
    oldIFS="$IFS"
    IFS='/'
    set -f
    set -- $p
    set +f
    IFS="$oldIFS"
    for seg in "$@"; do
      case "$seg" in
        ''|.) ;;
        ..)   out="${out%/*}" ;;
        *)    out="$out/$seg" ;;
      esac
    done
    printf '%s%s' "$root" "$out" | tr '[:upper:]' '[:lower:]'
  }

  xc_classifies_inside_cases() {
    local proj="$1" target="$2"
    local cases_abs target_abs
    cases_abs=$(xc_canon "$proj/brain/accounts")
    target_abs=$(xc_canon "$target")
    case "$target_abs" in
      "$cases_abs"/*) return 0 ;;
      *)              return 1 ;;
    esac
  }

  xc_extracted_case_id() {
    local proj="$1" target="$2"
    local cases_abs target_abs rel
    cases_abs=$(xc_canon "$proj/brain/accounts")
    target_abs=$(xc_canon "$target")
    rel="${target_abs#"$cases_abs/"}"
    # <conta>/cases/<caso>/... -> <conta>/<caso>: a identidade guardada e o par,
    # nao o caso sozinho. Dois clientes podem nomear um caso igual.
    local acct rest
    acct="${rel%%/*}"
    rest="${rel#*/}"
    case "$rest" in
      cases/*) rest="${rest#cases/}"; printf '%s/%s' "$acct" "${rest%%/*}" ;;
      *)       printf '%s' "$acct" ;;
    esac
  }

  # verdict for a named tool and payload key, so the tools that reach the hook
  # through the settings matcher are all exercised, not just Write.
  xc_verdict_tool() {
    local proj="$1" tool="$2" key="$3" esc=${4//\\/\\\\}
    printf '{"tool_name":"%s","tool_input":{"%s":"%s"}}' "$tool" "$key" "$esc" \
      | CLAUDE_PROJECT_DIR="$proj" bash "$XC_HOOK" >/dev/null 2>&1
    [ "$?" -eq 2 ] && printf 'block' || printf 'allow'
  }

  # verdict with a raw payload, for shapes that are not one tool plus one path.
  xc_verdict_raw() {
    printf '%s' "$2" | CLAUDE_PROJECT_DIR="$1" bash "$XC_HOOK" >/dev/null 2>&1
    [ "$?" -eq 2 ] && printf 'block' || printf 'allow'
  }

  # verdict when the parse yields nothing — the state a machine without python3
  # is in. Driven with a stub interpreter that prints nothing rather than by
  # emptying PATH: the hook also needs cat, tr and cut, and a stripped PATH
  # would exercise a broken shell instead of a missing interpreter. Both routes
  # converge on the same branch, since an absent python3 leaves PARSED empty
  # exactly as a silent one does.
  xc_verdict_noparse() {
    local proj="$1" payload="$2" bindir rc
    bindir=$(mktemp -d -t maestro-eval-nopy-XXXXXX)
    printf '#!/bin/sh\nexit 0\n' > "$bindir/python3"
    chmod +x "$bindir/python3"
    printf '%s' "$payload" \
      | PATH="$bindir:$PATH" CLAUDE_PROJECT_DIR="$proj" bash "$XC_HOOK" >/dev/null 2>&1
    rc=$?
    rm -rf "$bindir"
    [ "$rc" -eq 2 ] && printf 'block' || printf 'allow'
  }

  # Guard against this check silently degrading into a JSON-parse test: the
  # hook must still block when handed a well-formed POSIX payload.
  if [ "$(xc_verdict "$XC_ROOT" "$XC_ROOT/brain/accounts/beta/cases/tmo/probe.md")" = "block" ]; then
    pass "cross-case harness drives the hook's path logic, not a parse failure"
  else
    fail "cross-case harness is not exercising the hook (payload rejected before path logic)"
  fi

  # POSIX shape — the shape macOS always produced, and the only one previously covered.
  XC_R="$XC_ROOT"
  [ "$(xc_verdict "$XC_R" "$XC_R/brain/accounts/beta/cases/tmo/x.md")" = "block" ] \
    && pass "cross-case write blocked (POSIX paths)" \
    || fail "cross-case write NOT blocked (POSIX paths)"
  [ "$(xc_verdict "$XC_R" "$XC_R/brain/accounts/alfa/cases/tmo/x.md")" = "allow" ] \
    && pass "same-case write allowed (POSIX paths)" \
    || fail "same-case write wrongly blocked (POSIX paths)"

  # Windows shapes. These are asserted at the canonicalization layer because
  # on a POSIX eval host the synthetic drive-letter tree has no filesystem
  # counterpart, so the hook cannot read .active and the verdict is
  # unobservable. What is observable on every host is whether both PROJECT_DIR
  # and the target canonicalize to a form where the target is a prefix-match
  # inside the cases dir and the case-id is extracted correctly. If that
  # classification fails, the guard is inactive on Windows in exactly the way
  # the pre-fix code was broken. On a real Windows runtime, the block verdict
  # is additionally asserted end-to-end at the bottom of this phase.
  if command -v cygpath >/dev/null 2>&1; then
    XC_W=$(cygpath -m "$XC_ROOT")            # C:/Users/...
  else
    XC_W="C:${XC_ROOT}"                      # synthetic drive-letter equivalent
  fi
  if xc_classifies_inside_cases "$XC_W" "$XC_W/brain/accounts/beta/cases/tmo/x.md" \
     && [ "$(xc_extracted_case_id "$XC_W" "$XC_W/brain/accounts/beta/cases/tmo/x.md")" = "beta/tmo" ]; then
    pass "cross-case target classified inside cases dir (drive-letter paths)"
  else
    fail "cross-case target NOT classified inside cases dir (drive-letter paths) — guard inactive on Windows"
  fi

  # Backslash separators, the literal shape the Windows runtime sends.
  XC_B=$(printf '%s' "$XC_W" | tr '/' '\\')
  if xc_classifies_inside_cases "$XC_B" "$XC_B\\brain\\accounts\\beta\\cases\\tmo\\x.md" \
     && [ "$(xc_extracted_case_id "$XC_B" "$XC_B\\brain\\accounts\\beta\\cases\\tmo\\x.md")" = "beta/tmo" ]; then
    pass "cross-case target classified inside cases dir (backslash paths)"
  else
    fail "cross-case target NOT classified inside cases dir (backslash paths) — guard inactive on Windows"
  fi

  # Mixed shapes: the runtime may describe the same location two ways — a
  # POSIX-style MSYS project dir with a drive-letter target. Both must
  # canonicalize to a form where the target is inside the cases dir.
  # Derived by re-spelling the drive-letter form as its MSYS equivalent
  # ("C:/x" -> "/c/x"), which is the same location written two ways. Asking
  # cygpath -u instead would return a mount alias (/tmp for C:/Users/../Temp),
  # a different location that no string normalizer can or should reconcile.
  # Lowercase via tr (portable) — BSD sed on macOS does not honor GNU \L.
  _xc_drive=$(printf '%s' "$XC_W" | cut -c1 | tr '[:upper:]' '[:lower:]')
  XC_U="/${_xc_drive}${XC_W#?:}"
  if xc_classifies_inside_cases "$XC_U" "$XC_W/brain/accounts/beta/cases/tmo/x.md" \
     && [ "$(xc_extracted_case_id "$XC_U" "$XC_W/brain/accounts/beta/cases/tmo/x.md")" = "beta/tmo" ]; then
    pass "cross-case target classified inside cases dir (mixed MSYS dir + drive-letter target)"
  else
    fail "cross-case target NOT classified inside cases dir (mixed MSYS dir + drive-letter target)"
  fi

  # Path-shape bypasses. Each of these reached the active case on its leading
  # segment (or lost the case id entirely) while the OS resolved the path
  # somewhere else. All are observable on any host: the verdict comes from
  # string canonicalization, not from the filesystem.
  [ "$(xc_verdict "$XC_R" "$XC_R/brain/accounts/alfa/cases/tmo/../../../beta/cases/tmo/leak.md")" = "block" ] \
    && pass "cross-case write blocked (.. traversal out of the active case)" \
    || fail "cross-case write NOT blocked (.. traversal out of the active case)"
  [ "$(xc_verdict "$XC_R" "$XC_R/brain/accounts/alfa/cases/tmo/sub/../../../../beta/cases/tmo/leak.md")" = "block" ] \
    && pass "cross-case write blocked (multi-level .. traversal)" \
    || fail "cross-case write NOT blocked (multi-level .. traversal)"
  [ "$(xc_verdict "$XC_R" "$XC_R/brain/accounts//beta/cases/tmo/x.md")" = "block" ] \
    && pass "cross-case write blocked (double separator)" \
    || fail "cross-case write NOT blocked (double separator)"
  [ "$(xc_verdict "$XC_R" "$XC_R/brain/accounts/./beta/cases/tmo/x.md")" = "block" ] \
    && pass "cross-case write blocked (/./ segment)" \
    || fail "cross-case write NOT blocked (/./ segment)"

  # Every tool the settings matcher admits must be guarded, not just Write.
  # MultiEdit and NotebookEdit previously reached the hook and were waved
  # through by a case statement that named only Edit and Write.
  [ "$(xc_verdict_tool "$XC_R" MultiEdit file_path "$XC_R/brain/accounts/beta/cases/tmo/x.md")" = "block" ] \
    && pass "cross-case write blocked (MultiEdit)" \
    || fail "cross-case write NOT blocked (MultiEdit)"
  [ "$(xc_verdict_tool "$XC_R" NotebookEdit notebook_path "$XC_R/brain/accounts/beta/cases/tmo/n.ipynb")" = "block" ] \
    && pass "cross-case write blocked (NotebookEdit via notebook_path)" \
    || fail "cross-case write NOT blocked (NotebookEdit via notebook_path)"
  [ "$(xc_verdict_raw "$XC_R" "$(printf '{"tool_name":"NotebookEdit","tool_input":{"file_path":"%s","notebook_path":"%s"}}' "$XC_R/brain/accounts/alfa/cases/tmo/ok.md" "$XC_R/brain/accounts/beta/cases/tmo/n.ipynb")")" = "block" ] \
    && pass "cross-case write blocked (NotebookEdit with a decoy file_path on the active case)" \
    || fail "cross-case write NOT blocked (NotebookEdit decoy file_path)"

  # A tool that writes no file has no target to verify and must never be
  # refused. The settings matcher is an unanchored regex in installs predating
  # this change, so TodoWrite does arrive here, and in a Portuguese-language
  # workspace its list mentions the cases tree routinely.
  [ "$(xc_verdict_raw "$XC_R" '{"tool_name":"TodoWrite","tool_input":{"todos":[{"content":"revisar brain/accounts/beta/cases/tmo"}]}}')" = "allow" ] \
    && pass "TodoWrite naming the cases tree is allowed" \
    || fail "TodoWrite naming the cases tree was refused"

  # Fail-closed without python3. This is the branch that silently disabled
  # client isolation on every machine without an interpreter: the parse
  # returned empty, the tool name matched nothing, and the hook exited 0 for
  # every write.
  [ "$(xc_verdict_noparse "$XC_R" "$(printf '{"tool_name":"Write","tool_input":{"file_path":"%s"}}' "$XC_R/brain/accounts/beta/cases/tmo/x.md")")" = "block" ] \
    && pass "write into the cases tree refused when the payload cannot be parsed" \
    || fail "write into the cases tree ALLOWED when the payload cannot be parsed — isolation inactive"
  [ "$(xc_verdict_noparse "$XC_R" '{"tool_name":"TodoWrite","tool_input":{"todos":[{"content":"revisar brain/accounts/beta/cases/tmo"}]}}')" = "allow" ] \
    && pass "TodoWrite still allowed when the payload cannot be parsed" \
    || fail "TodoWrite refused when the payload cannot be parsed"
  [ "$(xc_verdict_noparse "$XC_R" "$(printf '{"tool_name":"Write","tool_input":{"file_path":"%s"}}' "$XC_R/brain/memory/x.md")")" = "allow" ] \
    && pass "write outside the cases tree still allowed when the payload cannot be parsed" \
    || fail "write outside the cases tree refused when the payload cannot be parsed"

  # An unreadable active marker means the target cannot be shown to be the
  # right case. Previously both shapes exited 0 and allowed the write.
  mv "$XC_ROOT/brain/accounts/.active" "$XC_ROOT/brain/accounts/.active.evalbak"
  [ "$(xc_verdict "$XC_R" "$XC_R/brain/accounts/beta/cases/tmo/x.md")" = "block" ] \
    && pass "write into a case refused while .active is missing" \
    || fail "write into a case ALLOWED while .active is missing"
  [ "$(xc_verdict "$XC_R" "$XC_R/brain/memory/x.md")" = "allow" ] \
    && pass "write outside the cases tree still allowed while .active is missing" \
    || fail "write outside the cases tree refused while .active is missing"
  printf '   \n' > "$XC_ROOT/brain/accounts/.active"
  [ "$(xc_verdict "$XC_R" "$XC_R/brain/accounts/beta/cases/tmo/x.md")" = "block" ] \
    && pass "write into a case refused while .active is empty" \
    || fail "write into a case ALLOWED while .active is empty"
  mv "$XC_ROOT/brain/accounts/.active.evalbak" "$XC_ROOT/brain/accounts/.active"

  # The runtime reads stdout only on exit 0, so a reason printed there on the
  # block path never reaches the model. It must be on stderr.
  XC_MSG=$(printf '{"tool_name":"Write","tool_input":{"file_path":"%s"}}' "$XC_R/brain/accounts/beta/cases/tmo/x.md" \
    | CLAUDE_PROJECT_DIR="$XC_R" bash "$XC_HOOK" 2>&1 >/dev/null)
  case "$XC_MSG" in
    *"alfa/tmo"*|*"beta/tmo"*) pass "block reason reaches stderr, where the runtime reads it on exit 2" ;;
    *) fail "block reason is not on stderr (got: ${XC_MSG:-<empty>})" ;;
  esac

  # If a real Windows runtime is available (cygpath present and the drive
  # letter maps to a readable filesystem location), promote the drive-letter
  # shape to an end-to-end verdict assertion. This runs on Windows CI and
  # skips silently on POSIX eval hosts, so both platforms exercise the guard
  # at the strongest available fidelity without producing false failures.
  if command -v cygpath >/dev/null 2>&1 && [ -f "$XC_W/brain/accounts/.active" ]; then
    [ "$(xc_verdict "$XC_W" "$XC_W/brain/accounts/beta/cases/tmo/x.md")" = "block" ] \
      && pass "cross-case write blocked end-to-end (drive-letter paths, native Windows)" \
      || fail "cross-case write NOT blocked end-to-end (drive-letter paths, native Windows)"
  fi
else
  fail "block-cross-case-writes.sh missing from ZIP"
fi

# --------------------------------------------------------------------------
phase "Phase 15 — User-visible hook messages carry no internal syntax"
# --------------------------------------------------------------------------

# The `$skill` prefix is a development-repo convention: it expands only through
# a UserPromptExpansion hook that exists in the source repo's .claude/settings.json.
# The shipped settings.json has no such hook, so `$skill` is inert for an owner.
# Any hook `reason` string is rendered verbatim to the user, so it must never
# instruct them to type something that cannot work.
CROSS_CASE_HOOK="$MAESTRO_DIR/.claude/hooks/block-cross-case-writes.sh"
if [ -f "$CROSS_CASE_HOOK" ]; then
  pass "block-cross-case-writes.sh present in ZIP"

  # Drive the hook into its block branch and inspect the emitted reason string.
  XC_ROOT=$(mktemp -d -t maestro-eval-crosscase-XXXXXX)
  mkdir -p "$XC_ROOT/brain/accounts/alfa/cases/tmo" "$XC_ROOT/brain/accounts/beta/cases/tmo" "$XC_ROOT/.claude/hooks"
  cp "$CROSS_CASE_HOOK" "$XC_ROOT/.claude/hooks/"
  printf 'alfa/tmo\n' > "$XC_ROOT/brain/accounts/.active"
  # The reason travels on stderr, not stdout: the runtime reads a decision
  # object on stdout only when the hook exits 0, so a reason printed there on
  # the exit-2 path is discarded before anyone sees it.
  XC_ERR="$XC_ROOT/block.err"
  printf '{"tool_name":"Write","tool_input":{"file_path":"%s"}}' \
    "$XC_ROOT/brain/accounts/beta/cases/tmo/notes.md" \
    | CLAUDE_PROJECT_DIR="$XC_ROOT" bash "$XC_ROOT/.claude/hooks/block-cross-case-writes.sh" \
      >/dev/null 2>"$XC_ERR"
  XC_RC=$?
  XC_OUT=$(cat "$XC_ERR" 2>/dev/null)

  if [ "$XC_RC" -eq 2 ] && [ -n "$XC_OUT" ]; then
    pass "cross-case write is blocked, with a reason on stderr"

    if printf '%s' "$XC_OUT" | grep -q '\$'; then
      fail "block message leaks internal \$skill syntax to the user: $XC_OUT"
    else
      pass "block message contains no internal \$skill syntax"
    fi
  else
    fail "cross-case write was NOT blocked (rc=$XC_RC, stderr: ${XC_OUT:-<empty>})"
  fi
else
  fail "block-cross-case-writes.sh missing from ZIP"
fi

# --------------------------------------------------------------------------
phase "Phase 16 — Prompt-time context pointers resolve"
# --------------------------------------------------------------------------

# context-inject-userprompt.sh emits pointers into the model's context on every
# turn. A pointer naming a file the scaffold never creates costs a failed read
# on the first turn of every session and teaches the runtime that memory paths
# are unreliable. Every absolute path it emits must exist on a fresh install.
CTX_HOOK="$MAESTRO_DIR/.claude/hooks/context-inject-userprompt.sh"
if [ -f "$CTX_HOOK" ]; then
  pass "context-inject-userprompt.sh present in ZIP"

  # Fresh state dir so the "first fire" (richest) branch is the one exercised.
  CTX_HOME=$(mktemp -d -t maestro-eval-ctxhome-XXXXXX)
  CTX_OUT=$(HOME="$CTX_HOME" CLAUDE_PROJECT_DIR="$MAESTRO_DIR" bash "$CTX_HOOK" 2>/dev/null)

  if [ -n "$CTX_OUT" ]; then
    pass "context-inject emits a first-fire bundle"
  else
    fail "context-inject emitted nothing on first fire"
  fi

  # Every emitted path under the Maestro dir must resolve.
  CTX_DANGLING=0
  for p in $(printf '%s' "$CTX_OUT" | grep -oE "$MAESTRO_DIR[^ )]*" | sed 's/[.,]$//' | sort -u); do
    if [ ! -e "$p" ]; then
      CTX_DANGLING=$((CTX_DANGLING+1))
      fail "context-inject points at a path that does not exist: ${p#"$MAESTRO_DIR/"}"
    fi
  done
  [ "$CTX_DANGLING" -eq 0 ] && pass "every context-inject pointer resolves on a fresh install"
else
  fail "context-inject-userprompt.sh missing from ZIP"
fi

# --------------------------------------------------------------------------
phase "Phase 17 — Suggested skill ids resolve"
# --------------------------------------------------------------------------

# Skills suggest follow-ups to the owner as `/skill-id`. An id that matches no
# shipped skill is a dead end the owner cannot act on. maestro-onboarding is
# forced on the first session, so a bad id there reaches every user.
SKILLS_ROOT="$MAESTRO_DIR/bundles/base/skills"
if [ -d "$SKILLS_ROOT" ]; then
  # Slash-prefixed ids that are runtime commands rather than skills.
  SLASH_ALLOWLIST=" clear help "
  SLASH_BAD=0
  SLASH_CHECKED=0
  for skillmd in "$SKILLS_ROOT"/*/SKILL.md; do
    [ -f "$skillmd" ] || continue
    for id in $(grep -oE '`/[a-z0-9][a-z0-9-]*`' "$skillmd" 2>/dev/null | tr -d '`/' | sort -u); do
      case "$SLASH_ALLOWLIST" in *" $id "*) continue ;; esac
      SLASH_CHECKED=$((SLASH_CHECKED+1))
      if [ ! -d "$SKILLS_ROOT/$id" ]; then
        SLASH_BAD=$((SLASH_BAD+1))
        fail "$(basename "$(dirname "$skillmd")") suggests /$id, which is not a shipped skill"
      fi
    done
  done
  if [ "$SLASH_CHECKED" -eq 0 ]; then
    fail "no /skill-id suggestions found (check is not exercising anything)"
  elif [ "$SLASH_BAD" -eq 0 ]; then
    pass "all $SLASH_CHECKED suggested /skill-id reference(s) resolve to a shipped skill"
  fi
else
  fail "bundles/base/skills missing from ZIP"
fi

# --------------------------------------------------------------------------
phase "Phase 18 — Update ritual has one source of truth"
# --------------------------------------------------------------------------

# README-INSTALL.md is the declared single source of the install/update ritual,
# and it mandates rename + copy brain/ across. WELCOME.md tells the owner not to
# extract over the folder. A shipped skill that tells them to "extraia por cima"
# contradicts both, from inside the same folder.
#
# Matches the instruction, not the word: a line that forbids extract-over, or
# that restores a personal backup over brain/, is fine.
RITUAL_BAD=0
for skillmd in "$MAESTRO_DIR/bundles/base/skills"/*/SKILL.md; do
  [ -f "$skillmd" ] || continue
  HITS=$(grep -nE '(extraia|extrair|reextraia|reextrair|extração)[^.]{0,40}por cima' "$skillmd" 2>/dev/null \
         | grep -viE 'nunca|não orient|jamais' \
         | grep -viE 'backup|time machine|nuvem pessoal' || true)
  if [ -n "$HITS" ]; then
    RITUAL_BAD=$((RITUAL_BAD+1))
    fail "$(basename "$(dirname "$skillmd")") instructs extract-over, contradicting README-INSTALL.md: $(printf '%s' "$HITS" | head -1 | cut -c1-90)"
  fi
done
[ "$RITUAL_BAD" -eq 0 ] && pass "no shipped skill instructs extract-over"

if grep -q 'README-INSTALL' "$MAESTRO_DIR/bundles/base/skills/maestro-setup-update/SKILL.md" 2>/dev/null; then
  pass "maestro-setup-update points at README-INSTALL.md"
else
  fail "maestro-setup-update never names README-INSTALL.md (the declared source of truth)"
fi

if grep -q 'README-INSTALL' "$MAESTRO_DIR/bundles/base/skills/maestro-doctor/SKILL.md" 2>/dev/null; then
  pass "maestro-doctor points at README-INSTALL.md"
else
  fail "maestro-doctor never names README-INSTALL.md (the declared source of truth)"
fi

# --------------------------------------------------------------------------
# Summary
# --------------------------------------------------------------------------
# --------------------------------------------------------------------------
phase "Phase 19 — Hook interpreter resolution"
# --------------------------------------------------------------------------

# The hooks parse JSON with Python. Testing `command -v python3` and nothing
# else made a Windows box that has Python indistinguishable from one that does
# not: the python.org installer ships python.exe and the py launcher and
# creates no python3, so every hook needing an interpreter exited 0 in silence.
PY_LIB="$MAESTRO_DIR/.claude/hooks/lib/python.sh"
if [ -f "$PY_LIB" ]; then
  pass "hooks/lib/python.sh present in ZIP"

  py_stub_dir() {
    local name="$1" body="$2" d
    d=$(mktemp -d -t maestro-eval-py-XXXXXX)
    printf '%s\n' "$body" > "$d/$name"
    chmod +x "$d/$name"
    printf '%s' "$d"
  }
  PY_REAL=$(command -v python3 2>/dev/null || command -v python 2>/dev/null || true)

  if [ -n "$PY_REAL" ]; then
    PY_ONLY_PYTHON=$(py_stub_dir python "#!/bin/sh
exec \"$PY_REAL\" \"\$@\"")
    RESOLVED=$(env PATH="$PY_ONLY_PYTHON:/usr/bin:/bin" bash -c ". '$PY_LIB'; maestro_python" 2>/dev/null)
    [ "$RESOLVED" = "python" ] \
      && pass "resolver finds 'python' when 'python3' is absent" \
      || fail "resolver did not find 'python' when 'python3' is absent (got: ${RESOLVED:-<none>})"

    PY_ONLY_LAUNCHER=$(py_stub_dir py "#!/bin/sh
shift
exec \"$PY_REAL\" \"\$@\"")
    RESOLVED=$(env PATH="$PY_ONLY_LAUNCHER:/usr/bin:/bin" bash -c ". '$PY_LIB'; maestro_python" 2>/dev/null)
    [ "$RESOLVED" = "py -3" ] \
      && pass "resolver falls back to the 'py -3' launcher" \
      || fail "resolver did not fall back to 'py -3' (got: ${RESOLVED:-<none>})"

    rm -rf "$PY_ONLY_PYTHON" "$PY_ONLY_LAUNCHER"
  else
    skip "no interpreter on this host to build resolver stubs from"
  fi

  # A `python` that is Python 2 must be rejected rather than selected: the hook
  # scripts are Python 3, and the resulting error is swallowed by 2>/dev/null —
  # the silent failure this resolver exists to end.
  PY_TWO=$(py_stub_dir python '#!/bin/sh
exit 1')
  RESOLVED=$(env PATH="$PY_TWO:/usr/bin:/bin" bash -c ". '$PY_LIB'; maestro_python" 2>/dev/null)
  [ -z "$RESOLVED" ] \
    && pass "resolver rejects a 'python' that is not Python 3" \
    || fail "resolver selected a non-Python-3 interpreter (got: $RESOLVED)"
  rm -rf "$PY_TWO"

  # Regression guard: a hook that calls the interpreter by name again
  # reintroduces the bug for every owner whose Python is not called python3.
  HARDCODED=$(grep -rlE '(\||^|[[:space:]])python3[[:space:]]+(-c|-)' \
                "$MAESTRO_DIR/.claude/hooks" 2>/dev/null \
              | grep -v '/lib/python.sh$' || true)
  if [ -z "$HARDCODED" ]; then
    pass "no hook invokes python3 directly; all resolve through the library"
  else
    fail "hook(s) still invoke python3 directly: $(printf '%s' "$HARDCODED" | tr '\n' ' ')"
  fi

  # A provisioned interpreter is recorded by path, and on Windows that path
  # routinely contains a space ("C:\Users\Firstname Lastname\..."). It must
  # both resolve and RUN, which is why the library invokes through a function
  # instead of interpolating the interpreter at each call site.
  if [ -n "$PY_REAL" ]; then
    PY_REC_ROOT=$(mktemp -d -t maestro-eval-pyrec-XXXXXX)
    mkdir -p "$PY_REC_ROOT/with space" "$PY_REC_ROOT/brain"
    printf '#!/bin/sh\nexec "%s" "$@"\n' "$PY_REAL" > "$PY_REC_ROOT/with space/maestro-python"
    chmod +x "$PY_REC_ROOT/with space/maestro-python"
    printf '%s\n' "$PY_REC_ROOT/with space/maestro-python" > "$PY_REC_ROOT/brain/.maestro-python"

    RAN=$(env PATH="/usr/bin:/bin" CLAUDE_PROJECT_DIR="$PY_REC_ROOT" \
            bash -c ". '$PY_LIB'; maestro_py -c 'print(\"ran\")'" 2>/dev/null)
    [ "$RAN" = "ran" ] \
      && pass "a recorded interpreter resolves and runs, including a path with a space" \
      || fail "recorded interpreter did not run (got: ${RAN:-<none>})"

    # A record outlives the interpreter it names — uninstalled, moved, or
    # carried in a workspace copied to another machine. It must be skipped, not
    # trusted, or provisioning leaves behind a permanent false positive.
    printf '%s\n' "$PY_REC_ROOT/gone/maestro-python" > "$PY_REC_ROOT/brain/.maestro-python"
    RESOLVED=$(env PATH="/usr/bin:/bin" CLAUDE_PROJECT_DIR="$PY_REC_ROOT" \
                 bash -c ". '$PY_LIB'; maestro_python" 2>/dev/null)
    [ -z "$RESOLVED" ] \
      && pass "a stale interpreter record is rejected rather than trusted" \
      || fail "stale interpreter record was accepted (got: $RESOLVED)"

    # Absence must be visible. Every hook that needs the interpreter exits 0
    # without it, so silence here is the whole defect.
    MEM_HOOK="$MAESTRO_DIR/.claude/hooks/session-start-memory-inject.sh"
    if [ -f "$MEM_HOOK" ]; then
      mkdir -p "$PY_REC_ROOT/brain/memory/recent"
      rm -f "$PY_REC_ROOT/brain/.maestro-python"
      NUDGE=$(env PATH="/usr/bin:/bin" CLAUDE_PROJECT_DIR="$PY_REC_ROOT" bash "$MEM_HOOK" 2>/dev/null)
      case "$NUDGE" in
        *maestro:python-missing*) pass "SessionStart names a missing interpreter instead of degrading in silence" ;;
        *) fail "SessionStart emitted no signal for a missing interpreter" ;;
      esac
      QUIET=$(CLAUDE_PROJECT_DIR="$PY_REC_ROOT" bash "$MEM_HOOK" 2>/dev/null)
      case "$QUIET" in
        *maestro:python-missing*) fail "SessionStart warns about a missing interpreter while one is available" ;;
        *) pass "SessionStart stays quiet when an interpreter is available" ;;
      esac
    fi

    rm -rf "$PY_REC_ROOT"
  fi
else
  fail "hooks/lib/python.sh missing from ZIP"
fi



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
