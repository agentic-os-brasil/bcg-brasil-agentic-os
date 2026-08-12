#!/usr/bin/env bash
# Maestro semantic eval — Dimension C (LLM-as-judge).
#
# Usage:
#   installers/zip/eval-semantic.sh --calibrate
#   installers/zip/eval-semantic.sh --score [--zip PATH]
#   installers/zip/eval-semantic.sh --release-mode [--zip PATH]
#
# Purpose: address 6 semantic failure modes that eval-quality.sh (static proxy)
# does not catch. See eval/judge-prompt.md for the rubric and eval/golden-broken/
# for planted-failure calibration set.
#
# Design constraints:
#   - Dimension C is a SIGNAL not a gate. Beta-signoff artifact is the gate.
#   - Isolated API call per check (no anchoring bias across C2..C6).
#   - C1 (behavioral) = NÃO MEDIDO nesta versão (Wave 3).
#   - Calibration mandatory: all 6 planted failures must score <70 on target
#     check before Dimension C scores are considered trustworthy.
#   - --release-mode runs N=3, spread >15pt on any check → check excluded.
#   - judge-prompt.md version pinned; mismatch = fail-loud.
#
# Requires: ANTHROPIC_API_KEY, curl, jq.

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EVAL_DIR="$REPO_ROOT/installers/zip/eval"
JUDGE_PROMPT="$EVAL_DIR/judge-prompt.md"
GOLDEN_BROKEN="$EVAL_DIR/golden-broken/planted-failures.md"
TEMPLATE_DIR="$REPO_ROOT/installers/zip/user-template"
DIST_DIR="$REPO_ROOT/dist"

JUDGE_PROMPT_VERSION="c-v1-2026-08-11"
MODEL="claude-haiku-4-5-20251001"

MODE=""
ZIP_PATH=""
VERBOSE=0

while [ $# -gt 0 ]; do
  case "$1" in
    --calibrate)    MODE="calibrate"; shift ;;
    --score)        MODE="score"; shift ;;
    --release-mode) MODE="release"; shift ;;
    --zip)          ZIP_PATH="$2"; shift 2 ;;
    --verbose|-v)   VERBOSE=1; shift ;;
    -h|--help)      sed -n '2,25p' "$0"; exit 0 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

if [ -z "$MODE" ]; then
  echo "must specify one of: --calibrate | --score | --release-mode" >&2
  exit 2
fi

RED=$'\033[31m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; DIM=$'\033[2m'; RESET=$'\033[0m'

pass()  { printf '  %sPASS%s  %s\n' "$GREEN" "$RESET" "$1"; }
fail()  { printf '  %sFAIL%s  %s\n' "$RED" "$RESET" "$1"; }
info()  { printf '  %s...%s   %s\n' "$DIM" "$RESET" "$1"; }
phase() { printf '\n%s%s%s\n' "$YELLOW" "$1" "$RESET"; }

# ---------------------------------------------------------------------------
# Pre-flight
# ---------------------------------------------------------------------------

if [ ! -f "$JUDGE_PROMPT" ]; then
  echo "FATAL: judge prompt missing: $JUDGE_PROMPT" >&2
  exit 1
fi

PINNED_VERSION=$(grep -m1 '^VERSION:' "$JUDGE_PROMPT" | awk '{print $2}')
if [ "$PINNED_VERSION" != "$JUDGE_PROMPT_VERSION" ]; then
  echo "FATAL: judge prompt version drift" >&2
  echo "  eval-semantic.sh expects: $JUDGE_PROMPT_VERSION" >&2
  echo "  judge-prompt.md declares: $PINNED_VERSION" >&2
  echo "  → bump one to match after re-calibration" >&2
  exit 1
fi

if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
  echo "FATAL: ANTHROPIC_API_KEY not set" >&2
  exit 1
fi

for bin in curl jq; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "FATAL: missing dependency: $bin" >&2
    exit 1
  fi
done

# ---------------------------------------------------------------------------
# Judge call — one isolated API request per invocation.
# Args: check_id, temperature, material_file
# Prints: raw JSON score object on stdout, sanitized (or empty on error).
# ---------------------------------------------------------------------------

judge_call() {
  local check_id="$1"
  local temperature="$2"
  local material_file="$3"

  if [ ! -f "$material_file" ]; then
    echo "" ; return 1
  fi

  local system_prompt
  system_prompt=$(cat "$JUDGE_PROMPT")

  local user_message
  user_message=$(cat <<EOF
Executando **$check_id**.

Material a auditar:

$(cat "$material_file")

Retorne o JSON conforme rúbrica. Nada além do JSON.
EOF
)

  local payload
  payload=$(jq -n \
    --arg model "$MODEL" \
    --argjson temp "$temperature" \
    --arg system "$system_prompt" \
    --arg user "$user_message" \
    '{
      model: $model,
      max_tokens: 1024,
      temperature: $temp,
      system: $system,
      messages: [{ role: "user", content: $user }]
    }')

  local response
  response=$(curl -sS https://api.anthropic.com/v1/messages \
    -H "x-api-key: $ANTHROPIC_API_KEY" \
    -H "anthropic-version: 2023-06-01" \
    -H "content-type: application/json" \
    --data "$payload")

  local raw
  raw=$(printf '%s' "$response" | jq -r '.content[0].text // empty' 2>/dev/null)

  if [ -z "$raw" ]; then
    if [ "$VERBOSE" = 1 ]; then
      printf '%s\n' "$response" >&2
    fi
    echo ""
    return 1
  fi

  # Strip markdown code fences if judge disobeyed
  raw=$(printf '%s' "$raw" | sed -e 's/^```json//' -e 's/^```//' -e 's/```$//')

  # Validate JSON
  if ! printf '%s' "$raw" | jq -e '.score' >/dev/null 2>&1; then
    if [ "$VERBOSE" = 1 ]; then
      printf 'malformed judge output for %s: %s\n' "$check_id" "$raw" >&2
    fi
    echo ""
    return 1
  fi

  printf '%s' "$raw"
}

# ---------------------------------------------------------------------------
# Material extraction from either template dir or unzipped ZIP.
# Writes per-check material files into a scratch dir and echoes the dir path.
# ---------------------------------------------------------------------------

extract_material() {
  local source_dir="$1"
  local out_dir="$2"

  mkdir -p "$out_dir"

  local skills_root="$source_dir/bundles/base/skills"
  local onboarding="$skills_root/maestro-onboarding/SKILL.md"
  local doctor="$skills_root/maestro-doctor/SKILL.md"
  local setup="$skills_root/maestro-setup-update/SKILL.md"
  local claude_md="$source_dir/CLAUDE.md"
  local welcome="$source_dir/WELCOME.md"

  # C2 — 3 skills side by side
  {
    echo "### maestro-onboarding"
    [ -f "$onboarding" ] && cat "$onboarding" || echo "(missing)"
    echo ""
    echo "### maestro-doctor"
    [ -f "$doctor" ] && cat "$doctor" || echo "(missing)"
    echo ""
    echo "### maestro-setup-update"
    [ -f "$setup" ] && cat "$setup" || echo "(missing)"
  } > "$out_dir/C2.md"

  # C3 — onboarding SKILL.md alone
  {
    [ -f "$onboarding" ] && cat "$onboarding" || echo "(missing)"
  } > "$out_dir/C3.md"

  # C4 — CLAUDE.md + WELCOME.md + first 30 lines of onboarding
  {
    echo "### CLAUDE.md"
    [ -f "$claude_md" ] && cat "$claude_md" || echo "(missing)"
    echo ""
    echo "### WELCOME.md"
    [ -f "$welcome" ] && cat "$welcome" || echo "(missing)"
    echo ""
    echo "### maestro-onboarding (first 30 lines)"
    [ -f "$onboarding" ] && sed -n '1,30p' "$onboarding" || echo "(missing)"
  } > "$out_dir/C4.md"

  # C5 — 3 skills body (same as C2 material, different judge)
  cp "$out_dir/C2.md" "$out_dir/C5.md"

  # C6 — CLAUDE.md alone
  {
    [ -f "$claude_md" ] && cat "$claude_md" || echo "(missing)"
  } > "$out_dir/C6.md"
}

# ---------------------------------------------------------------------------
# Extract planted-failure sections from golden-broken/planted-failures.md
# ---------------------------------------------------------------------------

extract_planted() {
  local out_dir="$1"
  mkdir -p "$out_dir"

  for check in C2 C3 C4 C5 C6; do
    awk -v check="$check" '
      $0 ~ "=== " check " PLANTED FAILURE ===" { p=1; next }
      $0 ~ "=== END " check " ===" { p=0 }
      p { print }
    ' "$GOLDEN_BROKEN" > "$out_dir/$check.md"
  done
}

# ---------------------------------------------------------------------------
# Score one check. Args: check_id, material_file, [n_samples].
# Prints one line: "C{n}\t{median_score}\t{spread}\t{rationale_first}"
# ---------------------------------------------------------------------------

temp_for_check() {
  case "$1" in
    C3|C4) echo "0.3" ;;
    *)     echo "0" ;;
  esac
}

score_check() {
  local check_id="$1"
  local material="$2"
  local n="${3:-1}"

  local temp
  temp=$(temp_for_check "$check_id")

  local scores=()
  local rationale=""
  local improvement=""

  for i in $(seq 1 "$n"); do
    local out
    out=$(judge_call "$check_id" "$temp" "$material")
    if [ -z "$out" ]; then
      continue
    fi
    local s
    s=$(printf '%s' "$out" | jq -r '.score // empty')
    if [ -z "$rationale" ]; then
      rationale=$(printf '%s' "$out" | jq -r '.rationale // ""')
      improvement=$(printf '%s' "$out" | jq -r '.improvement // ""')
    fi
    if [ -n "$s" ]; then
      scores+=("$s")
    fi
  done

  if [ "${#scores[@]}" -eq 0 ]; then
    printf '%s\t\t\tno-valid-response\n' "$check_id"
    return 1
  fi

  # median + spread (min→max)
  local sorted
  sorted=$(printf '%s\n' "${scores[@]}" | sort -n)
  local count="${#scores[@]}"
  local mid=$(( count / 2 ))
  local median
  median=$(printf '%s\n' "$sorted" | sed -n "$((mid + 1))p")
  local min
  min=$(printf '%s\n' "$sorted" | head -1)
  local max
  max=$(printf '%s\n' "$sorted" | tail -1)
  local spread=$(( max - min ))

  printf '%s\t%s\t%s\t%s\n' "$check_id" "$median" "$spread" "$rationale"
}

# ---------------------------------------------------------------------------
# MODE: calibrate
# ---------------------------------------------------------------------------

run_calibrate() {
  phase "Calibration — planted failures must score < 70 on target check"
  info "judge prompt version: $PINNED_VERSION"
  info "golden set:           $GOLDEN_BROKEN"

  local scratch
  scratch=$(mktemp -d)
  trap "rm -rf $scratch" EXIT

  extract_planted "$scratch"

  local calib_pass=0
  local calib_fail=0

  for check in C2 C3 C4 C5 C6; do
    local material="$scratch/$check.md"
    if [ ! -s "$material" ]; then
      fail "$check — planted failure block missing or empty"
      calib_fail=$((calib_fail+1))
      continue
    fi

    local line
    line=$(score_check "$check" "$material" 1)
    local score
    score=$(printf '%s' "$line" | cut -f2)
    local rationale
    rationale=$(printf '%s' "$line" | cut -f4)

    if [ -z "$score" ]; then
      fail "$check — judge returned no valid score"
      calib_fail=$((calib_fail+1))
    elif [ "$score" -lt 70 ]; then
      pass "$check — planted failure caught (score=$score) — $rationale"
      calib_pass=$((calib_pass+1))
    else
      fail "$check — planted failure NOT caught (score=$score) — $rationale"
      calib_fail=$((calib_fail+1))
    fi
  done

  echo ""
  if [ "$calib_fail" -eq 0 ]; then
    printf '%sCalibration OK%s  %d/%d planted failures caught. Dimension C is trustworthy.\n' \
      "$GREEN" "$RESET" "$calib_pass" "$((calib_pass+calib_fail))"
    exit 0
  else
    printf '%sCalibration FAILED%s  %d/%d planted failures caught. Do not trust Dimension C scores.\n' \
      "$RED" "$RESET" "$calib_pass" "$((calib_pass+calib_fail))"
    printf '  Fix the rubric (or planted failures) and re-calibrate before scoring real material.\n'
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# MODE: score / release
# ---------------------------------------------------------------------------

run_score() {
  local n_samples="$1"
  local mode_label="$2"

  phase "Semantic score ($mode_label, N=$n_samples per check)"
  info "judge prompt version: $PINNED_VERSION"

  local source_dir
  local scratch
  scratch=$(mktemp -d)
  trap "rm -rf $scratch" EXIT

  if [ -n "$ZIP_PATH" ]; then
    if [ ! -f "$ZIP_PATH" ]; then
      echo "no ZIP at $ZIP_PATH" >&2
      exit 2
    fi
    info "source: ZIP $ZIP_PATH"
    unzip -q "$ZIP_PATH" -d "$scratch/unzip"
    source_dir="$scratch/unzip/Maestro"
  else
    info "source: template dir $TEMPLATE_DIR"
    source_dir="$TEMPLATE_DIR"
  fi

  extract_material "$source_dir" "$scratch/material"

  local total=0
  local checks_scored=0
  local excluded=()

  # C1 is not measured in c-v1
  info "C1 — NÃO MEDIDO nesta versão (Wave 3 pendente)"

  for check in C2 C3 C4 C5 C6; do
    local line
    line=$(score_check "$check" "$scratch/material/$check.md" "$n_samples")
    local score
    score=$(printf '%s' "$line" | cut -f2)
    local spread
    spread=$(printf '%s' "$line" | cut -f3)
    local rationale
    rationale=$(printf '%s' "$line" | cut -f4)

    if [ -z "$score" ]; then
      fail "$check — no valid response"
      excluded+=("$check")
      continue
    fi

    if [ "$mode_label" = "release" ] && [ "$spread" -gt 15 ]; then
      fail "$check — spread=$spread pt (>15), check unreliable, excluding from gate signal"
      excluded+=("$check")
      continue
    fi

    printf '  %sC%s%s  %s (median=%s spread=%s) — %s\n' \
      "$DIM" "$RESET" "$check" "$check" "$score" "$spread" "$rationale"
    total=$((total + score))
    checks_scored=$((checks_scored + 1))
  done

  echo ""
  if [ "$checks_scored" -eq 0 ]; then
    fail "no checks produced a usable score — nothing to report"
    exit 1
  fi

  local avg=$(( total / checks_scored ))
  printf '%sDimension C score%s  avg=%d over %d/%d checks' \
    "$YELLOW" "$RESET" "$avg" "$checks_scored" 5
  if [ "${#excluded[@]}" -gt 0 ]; then
    printf ' (excluded: %s)' "${excluded[*]}"
  fi
  echo ""
  echo ""
  echo "Note: Dimension C is a SIGNAL, not a gate. The gate is beta signoff."
  exit 0
}

case "$MODE" in
  calibrate) run_calibrate ;;
  score)     run_score 1 "score" ;;
  release)   run_score 3 "release" ;;
  *)         echo "unknown mode: $MODE" >&2; exit 2 ;;
esac
