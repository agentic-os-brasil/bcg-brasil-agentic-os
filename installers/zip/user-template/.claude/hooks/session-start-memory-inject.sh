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
  # Pass the path as argv, never interpolated into the Python source: a Windows
  # path such as C:\Users\... makes \U an invalid unicode escape inside a string
  # literal, the interpreter raises SyntaxError, 2>/dev/null swallows it, and the
  # guard fails open — injecting the empty placeholder identity into every
  # session. context-inject-userprompt.sh already reads it this way.
  if command -v python3 >/dev/null 2>&1; then
    local initialized
    initialized=$(python3 - "$file" <<'PY' 2>/dev/null
import json, sys
try:
    with open(sys.argv[1], encoding="utf-8") as f:
        print(json.load(f).get("initialized", True))
except Exception:
    print(True)
PY
)
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

# Operational method pointer (spec 050) — always first, before any task routing.
# SessionStart carries the pointer only; skill body is loaded on demand.
printf '\n## Método operacional\n'
printf '<!-- maestro:pointer: maestro-operator · reason: deterministic_operational_method -->\n'
printf 'Skill: %s/bundles/base/skills/maestro-operator/SKILL.md\n' "${PROJECT_DIR}"
printf 'Instrução: carregar este skill antes de escolher, interpretar ou recuperar qualquer operação de controle do Maestro.\n'

# Tech-core pointer (§3.1 diagnostic) — engineering skills bundle. Emitted only
# when the directory exists; fail-open otherwise. Loaded on demand.
TECH_CORE_DIR="$PROJECT_DIR/bundles/tech-core"
if [ -d "$TECH_CORE_DIR/skills" ]; then
  printf '\n## Skills técnicas (tech-core)\n'
  printf '<!-- maestro:pointer: tech-core · reason: engineering_skills_bundle -->\n'
  printf 'Bundle path: %s\n' "$TECH_CORE_DIR"
  if [ -f "$TECH_CORE_DIR/INDEX.md" ]; then
    printf 'Índice: %s/INDEX.md\n' "$TECH_CORE_DIR"
  fi
  if [ -f "$TECH_CORE_DIR/catalog.json" ]; then
    printf 'Catálogo: %s/catalog.json\n' "$TECH_CORE_DIR"
  fi
  printf 'Instrução: skills de engenharia (testes, revisão, pipelines, entrega por spec) — carregar sob demanda quando a tarefa exigir.\n'
fi

# Dreaming auto-trigger — check for .dream-requested marker written by session-stop-dream.sh.
# When present, emit a mandatory action block so Claude runs dream-memory before any other task.
DREAM_MARKER="$MEMORY_DIR/.dream-requested"
if [ -f "$DREAM_MARKER" ]; then
  printf '\n## ⚠️ Dreaming pendente — executar antes de qualquer outra tarefa\n'
  printf '<!-- maestro:dream-trigger: marker=%s -->\n' "$DREAM_MARKER"
  printf 'O marcador `.dream-requested` foi detectado (escrito pelo hook da sessão anterior).\n'
  printf '\n**Ação obrigatória:** leia `bundles/base/skills/dream-memory/SKILL.md` e execute o ciclo diário como primeira ação desta sessão, antes de responder ao usuário ou executar qualquer tarefa.\n'
  # The marker is consumed by dream-memory itself, which deletes it both at the
  # start of the cycle and again on completion. Deleting it here instead meant a
  # skipped cycle destroyed the request: the consolidation was never retried and
  # nothing recorded that it had been dropped. A repeated prompt is recoverable;
  # silently losing a day of memory is not.
fi

# GAP-C — Upgrade-pending auto-trigger. Marker written by first-run-scaffold.sh
# when a session boots against a bundle whose VERSION differs from
# data/.maestro-version. Surfaces a mandatory action block so Claude routes to
# /maestro-setup-update before any other work.
UPGRADE_MARKER="$DATA_DIR/.upgrade-pending"
if [ -f "$UPGRADE_MARKER" ]; then
  printf '\n## ⚠️ Upgrade Maestro pendente — verificar antes de qualquer outra tarefa\n'
  printf '<!-- maestro:upgrade-trigger: marker=%s -->\n' "$UPGRADE_MARKER"
  printf 'O marcador `.upgrade-pending` foi detectado — o VERSION do bundle mudou desde a última sessão.\n\n'
  printf 'Conteúdo do marcador:\n\n```json\n'
  cat "$UPGRADE_MARKER" 2>/dev/null
  printf '\n```\n\n'
  printf '**Ação obrigatória:** invoque `/maestro-setup-update` (fluxo de atualização) para validar a migração antes de responder ao usuário. Apagar o marcador só após o ciclo de update concluir sem erro.\n'
fi

# Profile (highest routing priority — who the user is and how they prefer to work)
emit_profile_json "Identidade do usuário" "$PROFILE_DIR/identity.json"
emit_profile_json "Preferências e estilo" "$PROFILE_DIR/style.json"

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
