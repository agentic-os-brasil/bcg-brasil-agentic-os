#!/usr/bin/env bash
# Maestro quality eval — scored assessment of ZIP for onboarding UX + response quality.
#
# Complements eval-release.sh:
#   - eval-release.sh   → pass/fail structural checks (must be green to ship)
#   - eval-quality.sh   → scored (0-200) assessment of user-facing quality
#
# Usage:
#   installers/zip/eval-quality.sh [--zip PATH] [--verbose]
#
# Exits 0 if score >= threshold (default 75%), 1 otherwise.

set -u

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
DIM='\033[2m'
NC='\033[0m'

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DIST_DIR="$REPO_ROOT/dist"
ZIP_PATH=""
THRESHOLD_PCT=75
VERBOSE=0

while [ $# -gt 0 ]; do
  case "$1" in
    --zip) ZIP_PATH="$2"; shift 2 ;;
    --threshold) THRESHOLD_PCT="$2"; shift 2 ;;
    --verbose|-v) VERBOSE=1; shift ;;
    -h|--help)
      cat <<EOF
Usage: eval-quality.sh [--zip PATH] [--threshold N] [--verbose]
Scored quality eval for Maestro ZIP. Two dimensions (100 pts each):
  A) Onboarding readiness
  B) Response quality signals
Exits 0 if score / 200 >= threshold percent (default 75).
EOF
      exit 0
      ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

if [ -z "$ZIP_PATH" ]; then
  ZIP_PATH=$(ls -t "$DIST_DIR"/Maestro-v*.zip 2>/dev/null | head -n1 || true)
fi
[ -z "$ZIP_PATH" ] && { echo "no ZIP found; run build-release.sh first or pass --zip" >&2; exit 2; }
[ ! -f "$ZIP_PATH" ] && { echo "ZIP does not exist: $ZIP_PATH" >&2; exit 2; }

SCRATCH=$(mktemp -d -t maestro-quality-XXXXXX)
trap 'rm -rf "$SCRATCH"' EXIT
unzip -q "$ZIP_PATH" -d "$SCRATCH" || { echo "unzip failed" >&2; exit 2; }
M="$SCRATCH/Maestro"

echo -e "${BLUE}Maestro quality eval — $(basename "$ZIP_PATH")${NC}"
echo -e "${DIM}Threshold: ${THRESHOLD_PCT}% (of 200 total points)${NC}"
echo ""

SCORE_A=0
SCORE_B=0
MAX_A=100
MAX_B=100

# Grade helpers: award pts (0..max) with a labeled line.
grade() {
  local pts="$1" ; local max="$2" ; local section="$3" ; local label="$4" ; local detail="${5:-}"
  local color="$RED"
  if [ "$pts" -eq "$max" ]; then color="$GREEN"
  elif [ "$pts" -ge $(( max / 2 )) ]; then color="$YELLOW"
  fi
  printf "  ${color}%3d/%2d${NC}  %s" "$pts" "$max" "$label"
  if [ -n "$detail" ] && [ "$VERBOSE" -eq 1 ]; then
    printf "  ${DIM}(%s)${NC}" "$detail"
  fi
  echo ""
  if [ "$section" = "A" ]; then
    SCORE_A=$(( SCORE_A + pts ))
  else
    SCORE_B=$(( SCORE_B + pts ))
  fi
}

# ─────────────────────────────────────────────────────────────
# A) Onboarding readiness (100 pts)
# ─────────────────────────────────────────────────────────────
echo -e "${YELLOW}A. Onboarding readiness (100 pts)${NC}"

# A1 — Essential skills present (15 pts)
essential=(maestro-onboarding maestro-doctor maestro-setup-update)
missing=0
for s in "${essential[@]}"; do
  [ ! -f "$M/bundles/base/skills/$s/SKILL.md" ] && missing=$(( missing + 1 ))
done
if [ "$missing" -eq 0 ]; then
  grade 15 15 A "3 essential skills present" "maestro-onboarding/doctor/setup-update"
else
  pts=$(( 15 - missing * 5 ))
  [ "$pts" -lt 0 ] && pts=0
  grade "$pts" 15 A "essential skills missing: $missing"
fi

# A2 — Onboarding captures profile (10 pts)
ONB="$M/bundles/base/skills/maestro-onboarding/SKILL.md"
pts=0
if [ -f "$ONB" ]; then
  grep -q 'data/profile/identity.json' "$ONB" && pts=$(( pts + 5 ))
  grep -q 'data/profile/style.json' "$ONB" && pts=$(( pts + 5 ))
fi
grade "$pts" 10 A "onboarding persists identity + style"

# A3 — Onboarding introduces hub/delegation without jargon (10 pts)
pts=0
if [ -f "$ONB" ]; then
  grep -qi 'hub\|especialistas\|delegação\|delega' "$ONB" && pts=$(( pts + 5 ))
  # Penalize *unquoted* jargon exposed to user. Quoted mentions in
  # "don't use X" instructions are fine.
  jargon_unquoted=$(grep -oE '(subagent|hub-and-spoke)[^"]' "$ONB" 2>/dev/null | grep -vE '(subagent|hub-and-spoke)[",]' | wc -l | tr -d ' ')
  jargon_unquoted=${jargon_unquoted:-0}
  [ "$jargon_unquoted" -eq 0 ] && pts=$(( pts + 5 ))
fi
grade "$pts" 10 A "delegation explained w/o jargon"

# A4 — Portuguese primary language (10 pts)
pts=0
if [ -f "$ONB" ]; then
  pt_markers=$(grep -cE '\b(você|seu|sua|Nome|Preferir|Ex\.:|Persist|padrão)\b' "$ONB" || true)
  pt_markers=${pt_markers:-0}
  if [ "$pt_markers" -ge 3 ]; then
    pts=10
  elif [ "$pt_markers" -ge 1 ]; then
    pts=5
  fi
fi
grade "$pts" 10 A "PT-BR primary in onboarding" "markers=${pt_markers:-0}"

# A5 — Communication rules honored (10 pts)
pts=10
CLAUDE_MD="$M/CLAUDE.md"
if [ -f "$CLAUDE_MD" ]; then
  # em-dash penalty if used to teach (allowed sparingly in dev docs but the file goes to users)
  emdash_count=$(grep -c '—' "$CLAUDE_MD" || true)
  emdash_count=${emdash_count:-0}
  if [ "$emdash_count" -gt 5 ]; then
    pts=$(( pts - 5 ))
    detail="em-dashes=$emdash_count (>5)"
  else
    detail="em-dashes=$emdash_count OK"
  fi
else
  pts=0
  detail="CLAUDE.md missing"
fi
grade "$pts" 10 A "CLAUDE.md comms rules" "$detail"

# A6 — Doctor covers required checks (15 pts)
DOC="$M/bundles/base/skills/maestro-doctor/SKILL.md"
pts=0
if [ -f "$DOC" ]; then
  grep -q 'VERSION' "$DOC" && pts=$(( pts + 3 ))
  grep -q 'CLAUDE.md' "$DOC" && pts=$(( pts + 3 ))
  grep -q 'settings.json' "$DOC" && pts=$(( pts + 3 ))
  grep -q 'first-run-scaffold' "$DOC" && pts=$(( pts + 3 ))
  grep -q 'data/' "$DOC" && pts=$(( pts + 3 ))
fi
grade "$pts" 15 A "doctor covers core+workspace+hook+version"

# A7 — Setup-update covers 3 outcomes (15 pts)
SET="$M/bundles/base/skills/maestro-setup-update/SKILL.md"
pts=0
if [ -f "$SET" ]; then
  grep -qiE 'instalação|instalar|install' "$SET" && pts=$(( pts + 5 ))
  grep -qiE 'atualiz|update|upgrade' "$SET" && pts=$(( pts + 5 ))
  grep -qiE 'reparo|reparar|rollback' "$SET" && pts=$(( pts + 5 ))
fi
grade "$pts" 15 A "setup-update covers install/update/repair"

# A8 — Root CLAUDE.md instructs first-session behavior (15 pts)
pts=0
if [ -f "$CLAUDE_MD" ]; then
  grep -q 'data/.initialized\|FIRST-RUN-FAILED.txt\|maestro-onboarding\|maestro-doctor' "$CLAUDE_MD" && pts=$(( pts + 5 ))
  grep -q 'first-run-scaffold\|scaffold' "$CLAUDE_MD" && pts=$(( pts + 5 ))
  grep -qE 'terminal|shell|comandos shell' "$CLAUDE_MD" && pts=$(( pts + 5 ))
fi
grade "$pts" 15 A "CLAUDE.md steers first session"

# ─────────────────────────────────────────────────────────────
# B) Response quality signals (100 pts)
# ─────────────────────────────────────────────────────────────
echo ""
echo -e "${YELLOW}B. Response quality signals (100 pts)${NC}"

# B1 — No placeholder debris (15 pts)
placeholders=$(grep -RIn --include='*.md' -E '\b(TODO|FIXME|XXX|TBD|placeholder|lorem ipsum)\b' "$M/bundles" 2>/dev/null | wc -l | tr -d ' ')
placeholders=${placeholders:-0}
if [ "$placeholders" -eq 0 ]; then
  grade 15 15 B "zero placeholder debris in bundles"
elif [ "$placeholders" -le 3 ]; then
  grade 8 15 B "$placeholders placeholder marker(s) in bundles"
else
  grade 0 15 B "$placeholders placeholder markers in bundles"
fi

# B2 — Skills have substantial trigger + default_prompt (10 pts)
CAT="$M/bundles/base/skills/catalog.json"
thin_meta=0
total_skills=0
if [ -f "$CAT" ]; then
  read total_skills thin_meta < <(python3 -c "
import json
d=json.load(open('$CAT'))
skills=d.get('skills',[])
thin=0
for s in skills:
  trig=s.get('trigger','') or ''
  prompt=s.get('default_prompt','') or ''
  if len(trig)+len(prompt) < 40:
    thin+=1
print(len(skills), thin)
" 2>/dev/null || echo "0 0")
fi
total_skills=${total_skills:-0}
thin_meta=${thin_meta:-0}
if [ "$total_skills" -gt 0 ] && [ "$thin_meta" -eq 0 ]; then
  grade 10 10 B "all $total_skills catalog entries have substantial trigger+prompt"
elif [ "$total_skills" -gt 0 ]; then
  ratio=$(( (total_skills - thin_meta) * 10 / total_skills ))
  grade "$ratio" 10 B "$thin_meta/$total_skills entries with thin trigger+prompt"
else
  grade 0 10 B "catalog empty or unreadable"
fi

# B3 — Valid frontmatter on every SKILL.md (15 pts)
skills_dir="$M/bundles/base/skills"
total=0
bad=0
if [ -d "$skills_dir" ]; then
  while IFS= read -r f; do
    total=$(( total + 1 ))
    if ! head -1 "$f" 2>/dev/null | grep -q '^---$'; then bad=$(( bad + 1 )); continue; fi
    if ! head -5 "$f" 2>/dev/null | grep -q '^name:'; then bad=$(( bad + 1 )); continue; fi
    if ! head -8 "$f" 2>/dev/null | grep -q '^description:'; then bad=$(( bad + 1 )); continue; fi
  done < <(find "$skills_dir" -name SKILL.md 2>/dev/null)
fi
if [ "$total" -gt 0 ] && [ "$bad" -eq 0 ]; then
  grade 15 15 B "all $total SKILL.md files have valid frontmatter"
elif [ "$total" -gt 0 ]; then
  ratio=$(( (total - bad) * 15 / total ))
  grade "$ratio" 15 B "$bad/$total skills with malformed frontmatter"
else
  grade 0 15 B "no skills found"
fi

# B4 — Skills have substantive body (10 pts)
thin=0
if [ "$total" -gt 0 ]; then
  while IFS= read -r f; do
    lines=$(wc -l < "$f")
    [ "$lines" -lt 15 ] && thin=$(( thin + 1 ))
  done < <(find "$skills_dir" -name SKILL.md 2>/dev/null)
  if [ "$thin" -eq 0 ]; then
    grade 10 10 B "no thin skills (all >=15 lines)"
  else
    ratio=$(( (total - thin) * 10 / total ))
    grade "$ratio" 10 B "$thin/$total skills too thin (<15 lines)"
  fi
else
  grade 0 10 B "no skills to measure"
fi

# B5 — CLAUDE.md communication contract present (10 pts)
pts=0
if [ -f "$CLAUDE_MD" ]; then
  grep -qiE 'linguagem direta|diret[oa]|resultado.*próximo passo|português' "$CLAUDE_MD" && pts=$(( pts + 5 ))
  grep -qiE 'nunca.*terminal|não peça|nunca peça' "$CLAUDE_MD" && pts=$(( pts + 5 ))
fi
grade "$pts" 10 B "CLAUDE.md encodes communication contract"

# B6 — English contamination check in user-facing PT skills (10 pts)
en_heavy=0
pt_skills=(maestro-onboarding maestro-setup-update maestro-doctor)
for s in "${pt_skills[@]}"; do
  f="$skills_dir/$s/SKILL.md"
  [ ! -f "$f" ] && continue
  pt_count=$(grep -cE '\b(você|seu|sua|para|com|não|foi|será)\b' "$f" || true)
  en_count=$(grep -cE '\b(you|your|the|with|not|will|would)\b' "$f" || true)
  pt_count=${pt_count:-0}
  en_count=${en_count:-0}
  if [ "$en_count" -gt "$pt_count" ] && [ "$pt_count" -lt 5 ]; then
    en_heavy=$(( en_heavy + 1 ))
  fi
done
if [ "$en_heavy" -eq 0 ]; then
  grade 10 10 B "user-facing skills primarily PT-BR"
else
  pts=$(( 10 - en_heavy * 4 ))
  [ "$pts" -lt 0 ] && pts=0
  grade "$pts" 10 B "$en_heavy user-facing skill(s) English-heavy"
fi

# B7 — Feedback rules encoded (10 pts)
pts=0
if [ -f "$CLAUDE_MD" ]; then
  grep -q 'terminal' "$CLAUDE_MD" && pts=$(( pts + 3 ))
  grep -qiE 'jargão|jargao|técnic' "$CLAUDE_MD" && pts=$(( pts + 3 ))
  grep -qE 'diagnóstico|diagnostic|doctor' "$CLAUDE_MD" && pts=$(( pts + 4 ))
fi
grade "$pts" 10 B "safety envelope encoded"

# B8 — INDEX.md discoverable (10 pts)
IDX="$M/bundles/base/skills/INDEX.md"
if [ -f "$IDX" ]; then
  rows=$(grep -cE '^\| [A-Z]' "$IDX" 2>/dev/null || echo 0)
  rows=${rows:-0}
  if [ "$rows" -ge 20 ]; then
    grade 10 10 B "INDEX.md discoverable with $rows rows"
  elif [ "$rows" -ge 5 ]; then
    grade 5 10 B "INDEX.md sparse ($rows rows)"
  else
    grade 0 10 B "INDEX.md thin ($rows rows)"
  fi
else
  grade 0 10 B "INDEX.md missing"
fi

# B9 — Skill count matches catalog (10 pts)
if [ -f "$CAT" ]; then
  cat_count=$(python3 -c "import json; print(len(json.load(open('$CAT')).get('skills',[])))" 2>/dev/null || echo 0)
  disk_count=$(find "$skills_dir" -name SKILL.md -type f 2>/dev/null | wc -l | tr -d ' ')
  cat_count=${cat_count:-0}
  disk_count=${disk_count:-0}
  if [ "$cat_count" -eq "$disk_count" ]; then
    grade 10 10 B "catalog ($cat_count) matches disk ($disk_count)"
  else
    diff=$(( cat_count - disk_count ))
    [ "$diff" -lt 0 ] && diff=$(( -diff ))
    pts=$(( 10 - diff * 2 ))
    [ "$pts" -lt 0 ] && pts=0
    grade "$pts" 10 B "catalog=$cat_count disk=$disk_count"
  fi
else
  grade 0 10 B "catalog.json missing"
fi

# ─────────────────────────────────────────────────────────────
# Report
# ─────────────────────────────────────────────────────────────
TOTAL=$(( SCORE_A + SCORE_B ))
MAX=$(( MAX_A + MAX_B ))
PCT=$(( TOTAL * 100 / MAX ))
PCT_A=$(( SCORE_A * 100 / MAX_A ))
PCT_B=$(( SCORE_B * 100 / MAX_B ))

echo ""
echo -e "${YELLOW}──────────────────────────────────────────────${NC}"
printf "  Onboarding:     ${BLUE}%3d/100${NC} (%d%%)\n" "$SCORE_A" "$PCT_A"
printf "  Response qual:  ${BLUE}%3d/100${NC} (%d%%)\n" "$SCORE_B" "$PCT_B"
echo -e "${YELLOW}──────────────────────────────────────────────${NC}"
printf "  TOTAL:          ${BLUE}%3d/200${NC} (%d%%)\n" "$TOTAL" "$PCT"
echo ""

if [ "$PCT" -ge "$THRESHOLD_PCT" ]; then
  echo -e "${GREEN}Above threshold ($PCT% >= $THRESHOLD_PCT%). Shippable.${NC}"
  exit 0
else
  echo -e "${RED}Below threshold ($PCT% < $THRESHOLD_PCT%). Improve before ship.${NC}"
  exit 1
fi
