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

# Resolved from this file's own directory, not from CLAUDE_PROJECT_DIR: the hook
# must find its library whatever the working directory is.
#
# `|| true` is load-bearing: `set -eu` is already active here and the ERR trap
# that guarantees the minimal-pointer fallback is not installed until further
# down. Without it, an unreadable library would kill the hook before its own
# fail-open contract could apply.
# shellcheck source=lib/python.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/python.sh" 2>/dev/null || true

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
BRAIN_DIR="$PROJECT_DIR/brain"
OWNER_DIR="$BRAIN_DIR/owner"
MEMORY_DIR="$BRAIN_DIR/memory"

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
  # Absolute fallback — used on any read error. Points at the memory root,
  # which the scaffold always creates. Naming a specific index file here would
  # assert a path that may not exist.
  printf '<!-- maestro:context-inject:minimal -->\nMemory: %s/brain/memory/ (load on demand).\n' "$PROJECT_DIR"
}

# Trap any unexpected error -> emit minimal, exit 0.
trap 'emit_minimal; exit 0' ERR

# ---------------------------------------------------------------------------
# Agent routing — runs on EVERY prompt, deliberately above the marker branch.
#
# The marker below silences this hook after the first fire of a session, which
# is right for context pointers and wrong for this: a request that needs a
# spoke can arrive at any turn. So the routing sits before that gate.
#
# It is conservative by construction. The router stays silent on the
# overwhelming majority of messages, because dispatching a spoke costs a whole
# model call. When it does match, it carries the closed packet the agent
# requires — a truncated packet is worse than none, since the spoke then
# answers about something else, which is why this budget is larger than the
# pointer budgets above.
#
# Every step is `|| true` or guarded: `set -eu` and the ERR trap are both live
# here, and a router that cannot run must cost the owner nothing.
# ---------------------------------------------------------------------------
AGENT_ROUTER="$PROJECT_DIR/bundles/base/tools/agent-route.py"
AGENT_BUDGET=1600

if [ -f "$AGENT_ROUTER" ] && maestro_python >/dev/null 2>&1; then
  AGENT_HOOK_INPUT=$(cat 2>/dev/null || true)
  if [ -n "${AGENT_HOOK_INPUT:-}" ]; then
    # Quoted heredoc: unquoted, bash reinterprets the body and a stray
    # backtick or $ in the payload becomes shell.
    AGENT_PROMPT=$(printf '%s' "$AGENT_HOOK_INPUT"       | PYTHONIOENCODING=utf-8 maestro_py - <<'AGENT_PY' 2>/dev/null || true
import sys, json
try:
    print(json.load(sys.stdin).get("prompt", "") or "")
except Exception:
    print("")
AGENT_PY
    )
    if [ -n "${AGENT_PROMPT:-}" ]; then
      AGENTS_OUT=$( (cd "$PROJECT_DIR" && printf '%s' "$AGENT_PROMPT"         | PYTHONIOENCODING=utf-8 maestro_py "$AGENT_ROUTER" --max 2 2>/dev/null)         | truncate_stdout "$AGENT_BUDGET" || true)
      if [ -n "${AGENTS_OUT:-}" ]; then
        printf '%s
' "$AGENTS_OUT"
      fi
    fi
  fi
fi

if [ -f "$MARKER" ]; then
  # -------- subsequent fires: stub only --------
  {
    printf '<!-- maestro:context-inject:stub -->\n'
    printf 'Memory: %s/brain/memory/ · Load specific tiers on demand.\n' "$PROJECT_DIR"
  } | truncate_stdout "$NEXT_BUDGET"
  exit 0
fi

# -------- first fire: richer (but still bounded) bundle --------
: > "$MARKER" 2>/dev/null || true

{
  printf '<!-- maestro:context-inject:first -->\n'
  printf '# Context pointers\n'

  # Profile identity headline (name / role / track) — best-effort.
  IDENTITY_FILE="$OWNER_DIR/identity.json"
  if [ -f "$IDENTITY_FILE" ] && maestro_python >/dev/null 2>&1; then
    HEADLINE=$(maestro_py - "$IDENTITY_FILE" <<'PY' 2>/dev/null || true
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

  # Pointers only — no file contents. Each is emitted only when the target
  # actually exists: neither MEMORY.md nor decisions/decision-log.md is created
  # by the scaffold, and naming a file that is not there invites a failed read
  # on the first turn of every session.
  printf 'Memory: %s/brain/memory/\n' "$PROJECT_DIR"
  if [ -f "$MEMORY_DIR/MEMORY.md" ]; then
    printf 'Memory index: %s/MEMORY.md\n' "$MEMORY_DIR"
  fi
  if [ -f "$MEMORY_DIR/decisions/decision-log.md" ]; then
    printf 'Decision log: %s/decisions/decision-log.md\n' "$MEMORY_DIR"
  fi
  if [ -f "${IDENTITY_FILE:-}" ]; then
    printf 'Profile: %s\n' "$IDENTITY_FILE"
  fi
  printf 'Reminder: full memory tiers are loaded at SessionStart. Read specific files on demand — do not request a re-dump each turn.\n'
} | truncate_stdout "$FIRST_BUDGET"

exit 0
