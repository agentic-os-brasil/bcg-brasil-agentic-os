#!/usr/bin/env bash
# Maestro UserPromptSubmit context-inject — LIGHTWEIGHT.
#
# Purpose:
#   Satisfy the `context_injection` capability contract
#   (Binding=UserPromptSubmit, Implementation=configured, state=operational_beta)
#   with a per-prompt payload that DOES NOT re-dump full memory each turn.
#
# Rationale:
#   The heavy tiered-memory dump belongs in SessionStart
#   (see session-start-memory-inject.sh). Firing that same payload on every
#   user prompt regresses token cost and induces context rot
#   (Du EMNLP 2025: -13.9% to -85% degradation with perfect retrieval).
#
# Behavior:
#   - First fire in a session: emit a compact bundle
#       - profile identity headline (name/role/track) if identity.json present
#       - pointers to MEMORY.md-style top-level indices (decision-log path,
#         profile identity path)
#       - 1-line reminder to load memory on demand
#     Budget: ~1600 chars (~400 tokens).
#   - Subsequent fires in the same session: 2-line pointer stub.
#     Budget: ~160 chars (~40 tokens).
#
# Fail-closed but silent:
#   Any read failure -> exit 0 with a minimal one-line MEMORY.md pointer.
#   Never blocks the prompt loop.

set -eu

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
DATA_DIR="$PROJECT_DIR/data"
PROFILE_DIR="$DATA_DIR/profile"
MEMORY_DIR="$DATA_DIR/memory"

FIRST_BUDGET=1600
NEXT_BUDGET=160

# Session marker directory. Prefer $HOME/.claude/state; fallback to /tmp.
STATE_DIR="${HOME:-/tmp}/.claude/state"
if ! mkdir -p "$STATE_DIR" 2>/dev/null; then
  STATE_DIR="/tmp"
fi

# Session id — use CLAUDE_SESSION_ID if present, else derive from PPID+date.
SESSION_ID="${CLAUDE_SESSION_ID:-${PPID:-noppid}-$(date +%Y%m%d 2>/dev/null || echo nodate)}"
MARKER="$STATE_DIR/context-inject-${SESSION_ID}.marker"

truncate_stdout() {
  # $1 = max chars
  local max="$1"
  awk -v max="$max" 'BEGIN{n=0} {
    line=$0
    if (n + length(line) + 1 > max) {
      remain = max - n
      if (remain > 0) { print substr(line,1,remain) }
      exit
    }
    print line
    n += length(line) + 1
  }'
}

emit_minimal() {
  # Absolute fallback — used on any read error.
  printf '<!-- maestro:context-inject:minimal -->\nMemory index: %s/data/memory/MEMORY.md (load on demand).\n' "$PROJECT_DIR"
}

# Trap any unexpected error -> emit minimal, exit 0.
trap 'emit_minimal; exit 0' ERR

if [ -f "$MARKER" ]; then
  # -------- subsequent fires: stub only --------
  {
    printf '<!-- maestro:context-inject:stub -->\n'
    printf 'Memory: %s/data/memory/ · Load MEMORY.md on demand.\n' "$PROJECT_DIR"
  } | truncate_stdout "$NEXT_BUDGET"
  exit 0
fi

# -------- first fire: richer (but still bounded) bundle --------
: > "$MARKER" 2>/dev/null || true

{
  printf '<!-- maestro:context-inject:first -->\n'
  printf '# Context pointers\n'

  # Profile identity headline (name / role / track) — best-effort.
  IDENTITY_FILE="$PROFILE_DIR/identity.json"
  if [ -f "$IDENTITY_FILE" ] && command -v python3 >/dev/null 2>&1; then
    HEADLINE=$(python3 - "$IDENTITY_FILE" <<'PY' 2>/dev/null || true
import json, sys
try:
    with open(sys.argv[1]) as f:
        d = json.load(f)
    if d.get("initialized") is False:
        sys.exit(0)
    parts = []
    for k in ("name", "role", "track"):
        v = d.get(k)
        if v:
            parts.append(f"{k}={v}")
    if parts:
        print(" · ".join(parts))
PY
)
    if [ -n "${HEADLINE:-}" ]; then
      printf 'Identity: %s\n' "$HEADLINE"
    fi
  fi

  # Pointers only — no file contents.
  printf 'Memory index: %s/data/memory/MEMORY.md\n' "$PROJECT_DIR"
  printf 'Decision log: %s/data/memory/decisions/decision-log.md\n' "$PROJECT_DIR"
  printf 'Profile: %s\n' "$IDENTITY_FILE"
  printf 'Reminder: full memory tiers are loaded at SessionStart. Read specific files on demand — do not request a re-dump each turn.\n'
} | truncate_stdout "$FIRST_BUDGET"

exit 0
