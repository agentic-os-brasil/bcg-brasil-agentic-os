#!/usr/bin/env bash
# Maestro SessionStart memory injection — reads tiered memory layers and
# profile context and emits structured context for the session.
# Fail-open: any missing layer is skipped with a diagnostic; never blocks Claude.
#
# Injection order follows the memory policy context_injection order:
#   lifetime → medium-term → weekly → recent → profile

set +e

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
DATA_DIR="$PROJECT_DIR/data"
MEMORY_DIR="$DATA_DIR/memory"
PROFILE_DIR="$DATA_DIR/profile"

# If data/ does not exist yet (first-run-scaffold.sh had not run or failed),
# exit silently — no context to inject.
[ -d "$DATA_DIR" ] || exit 0

# Helper: emit a layer section, reading all *.md files in a directory.
# Silently skips if directory is empty or absent.
emit_layer() {
  local label="$1"
  local dir="$2"
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

# Helper: emit a JSON profile file as a fenced block.
emit_profile_json() {
  local label="$1"
  local file="$2"
  [ -f "$file" ] || return
  # Skip placeholder (uninitialized) profiles to avoid injecting empty noise.
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

# --- Output block begins ---
# Wrapped in a comment fence recognized by the Maestro runtime as injected
# session context. Raw content between the markers is appended to the system
# context budget for this session.
printf '<!-- maestro:session-context:start -->\n'
printf '# Maestro — Contexto da sessão\n'
printf '_Injetado automaticamente pelo hook de início de sessão._\n'

# Profile: identity and preferences (highest routing priority)
emit_profile_json "Identidade do usuário" "$PROFILE_DIR/identity.json"
emit_profile_json "Preferências" "$PROFILE_DIR/preferences.json"

# Memory layers (lifetime → medium-term → weekly → recent)
emit_layer "Memória permanente"     "$MEMORY_DIR/lifetime"
emit_layer "Memória de médio prazo" "$MEMORY_DIR/medium-term"
emit_layer "Memória semanal"        "$MEMORY_DIR/weekly"
emit_layer "Memória recente"        "$MEMORY_DIR/recent"

printf '\n<!-- maestro:session-context:end -->\n'

exit 0
