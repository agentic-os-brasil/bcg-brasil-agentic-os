#!/usr/bin/env bash
# Maestro SessionStart memory injection — reads tiered memory layers and
# profile context and emits structured context for the session.
#
# Injection order and depth (mirrors Kowalski OS rollup model):
#   - L3  long-term memory   → full content (compact, high-signal, always injected)
#   - L2  weekly resume      → latest file (most recent weekly synthesis)
#   - L1  daily log          → latest consolidated daily log
#   - profile identity + preferences
#
# Fail-open: any missing layer is skipped with a one-line diagnostic.
# Never blocks Claude from starting a session.

set +e

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
DATA_DIR="$PROJECT_DIR/data"
MEMORY_DIR="$DATA_DIR/memory"
PROFILE_DIR="$DATA_DIR/profile"

# If data/ does not exist yet (first-run-scaffold.sh had not run or failed),
# exit silently — nothing to inject.
[ -d "$DATA_DIR" ] || exit 0

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# emit_latest_file LABEL DIR
# Emits the content of the most recently modified *.md file in DIR.
# Silently skips if DIR is empty or absent.
emit_latest_file() {
  local label="$1"
  local dir="$2"
  [ -d "$dir" ] || return
  local latest
  latest=$(find "$dir" -maxdepth 1 -name "*.md" -type f 2>/dev/null | sort | tail -1)
  [ -z "$latest" ] && return
  printf '\n## %s\n' "$label"
  printf '<!-- source: %s -->\n' "$(basename "$latest")"
  cat "$latest" 2>/dev/null
}

# emit_all_files LABEL DIR
# Emits ALL *.md files in DIR (for the long-term tier — compact, high-signal).
emit_all_files() {
  local label="$1"
  local dir="$2"
  [ -d "$dir" ] || return
  local files
  files=$(find "$dir" -maxdepth 1 -name "*.md" -type f 2>/dev/null | sort)
  [ -z "$files" ] && return
  printf '\n## %s\n' "$label"
  while IFS= read -r f; do
    [ -f "$f" ] || continue
    printf '\n<!-- source: %s -->\n' "$(basename "$f")"
    cat "$f" 2>/dev/null
  done <<< "$files"
}

# emit_profile_json LABEL FILE
# Emits a JSON profile file as a fenced block.
# Skips placeholder (uninitialized) profiles to avoid injecting empty noise.
emit_profile_json() {
  local label="$1"
  local file="$2"
  [ -f "$file" ] || return
  if command -v python3 >/dev/null 2>&1; then
    local initialized
    initialized=$(python3 -c "import json,sys; d=json.load(open('$file')); print(d.get('initialized', True))" 2>/dev/null)
    [ "$initialized" = "False" ] && return
  fi
  printf '\n## %s\n' "$label"
  printf '```json\n'
  cat "$file" 2>/dev/null
  printf '```\n'
}

# ---------------------------------------------------------------------------
# Output block
# ---------------------------------------------------------------------------
# The maestro:session-context markers are recognized by the Maestro runtime.
# Content between them is appended to the session context budget.

printf '<!-- maestro:session-context:start -->\n'
printf '# Maestro — Contexto da sessão\n'
printf '_Injetado automaticamente pelo hook de início de sessão._\n'

# Profile (highest routing priority — who the user is and how they prefer to work)
emit_profile_json "Identidade do usuário" "$PROFILE_DIR/identity.json"
emit_profile_json "Preferências" "$PROFILE_DIR/preferences.json"

# Owner SELF facets (spec 013) — ten individually-addressable markdown files
# Injected in full: each facet is small, high-routing-priority context.
emit_all_files "SELF do usuário" "$DATA_DIR/owner/self"

# L3 — long-term memory (injected in full: compact, high-signal, rarely changes)
emit_all_files "Memória de longo prazo (L3)" "$MEMORY_DIR/lifetime"

# L2 — weekly resume (latest weekly synthesis)
emit_latest_file "Resumo semanal (L2)" "$MEMORY_DIR/weekly"

# L1 — last consolidated daily log
emit_latest_file "Último log diário consolidado (L1)" "$MEMORY_DIR/recent"

printf '\n<!-- maestro:session-context:end -->\n'

exit 0
