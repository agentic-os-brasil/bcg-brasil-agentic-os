#!/usr/bin/env bash
# Maestro first-run scaffold — creates data/ workspace on first session.
# Idempotent. Fail-open. Never blocks Claude Code.

set +e

# CLAUDE_PROJECT_DIR is injected by Claude Code CLI. In non-standard paths
# (/tmp, paths with spaces, external drives) it may be missing. Fallback to
# `.` — but only if we can *verify* we are inside a Maestro project, by
# checking for a VERSION file next to this script's parent tree. Otherwise
# exit fail-open with a stderr note so the user is not left with a silently
# broken scaffold. See CLAUDE.md → "Runtime dependencies" and
# bundles/base/known-issues.md → claude-project-dir-nonstandard-path.
PROJECT_DIR="${CLAUDE_PROJECT_DIR:-}"
if [ -z "$PROJECT_DIR" ]; then
  # Try to locate VERSION relative to this hook's location (canonical: <root>/.claude/hooks/<script>).
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd)"
  CANDIDATE="$SCRIPT_DIR/../.."
  if [ -n "$SCRIPT_DIR" ] && [ -f "$CANDIDATE/VERSION" ]; then
    PROJECT_DIR="$(cd "$CANDIDATE" && pwd)"
  elif [ -f "./VERSION" ]; then
    PROJECT_DIR="."
  else
    printf 'maestro first-run-scaffold: CLAUDE_PROJECT_DIR unset and no VERSION found nearby — skipping scaffold (fail-open).\n' >&2
    exit 0
  fi
fi
DATA_DIR="$PROJECT_DIR/data"
MARKER="$DATA_DIR/.initialized"
LOG="$DATA_DIR/.scaffold.log"
TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)

log_line() {
  ( printf '%s  %s\n' "$TS" "$1" >> "$LOG" ) 2>/dev/null
}

# ---------------------------------------------------------------------------
# Skills rollup — emitted to stdout every session as additionalContext.
# Compact index only (name + description first sentence). Full SKILL.md is
# loaded on demand by the Skill tool. Fail-open: on any error, print nothing.
# ---------------------------------------------------------------------------
_emit_skills_rollup_bundle() {
  local skills_dir="$1"
  local heading="$2"
  local blurb="$3"
  [ -d "$skills_dir" ] || return 0

  local rollup
  rollup=$(awk '
    FNR == 1 {
      in_fm = 0; fm_count = 0; name = ""; desc = ""
    }
    /^---[[:space:]]*$/ {
      fm_count++
      if (fm_count == 1) { in_fm = 1; next }
      if (fm_count == 2) {
        in_fm = 0
        if (name != "" && desc != "") {
          sub(/\. .*$/, ".", desc)
          if (length(desc) > 140) desc = substr(desc, 1, 137) "..."
          printf "- **%s** — %s\n", name, desc
        }
        nextfile
      }
    }
    in_fm && /^name:[[:space:]]/ {
      sub(/^name:[[:space:]]*/, "")
      gsub(/^["\x27]|["\x27]$/, "")
      name = $0
    }
    in_fm && /^description:[[:space:]]/ {
      sub(/^description:[[:space:]]*/, "")
      gsub(/^["\x27]|["\x27]$/, "")
      desc = $0
    }
  ' "$skills_dir"/*/SKILL.md 2>/dev/null | sort)

  [ -z "$rollup" ] && return 0

  printf '## %s\n\n' "$heading"
  printf '%s\n\n' "$blurb"
  printf '%s\n' "$rollup"
  printf '\n'
}

emit_skills_rollup() {
  _emit_skills_rollup_bundle \
    "$PROJECT_DIR/bundles/base/skills" \
    "Maestro skills disponíveis" \
    "Índice compacto. A skill completa é carregada sob demanda quando o pedido do dono a aciona."

  # tech-core — engineering skills bundle. Invisible until now (§3.1 diagnostic).
  # Enumerated only when the directory exists; fail-open otherwise.
  _emit_skills_rollup_bundle \
    "$PROJECT_DIR/bundles/tech-core/skills" \
    "Skills técnicas (tech-core)" \
    "Skills de engenharia — testes, revisão, pipelines de dados, entrega por spec. Carregadas sob demanda."
}

# ---------------------------------------------------------------------------
# Active case context — emitted every session when a case is active.
# Reads data/cases/.active for the case ID, then emits a compact summary:
# project brief (first 25 lines), last 5 decision headings, open task count.
# Fail-open: any error silently returns without output.
# ---------------------------------------------------------------------------
emit_active_case_context() {
  local cases_dir="$DATA_DIR/cases"
  local active_file="$cases_dir/.active"

  [ -f "$active_file" ] || return 0

  local case_id
  case_id=$(tr -d '[:space:]' < "$active_file" 2>/dev/null)
  [ -z "$case_id" ] && return 0

  local case_dir="$cases_dir/$case_id"
  [ -d "$case_dir" ] || return 0

  printf '## Caso ativo: %s\n\n' "$case_id"

  # Project brief — first .md in brain/projects/, first 25 lines
  local brief_file=""
  for f in "$case_dir/brain/projects/"*.md; do
    [ -f "$f" ] && brief_file="$f" && break
  done
  if [ -n "$brief_file" ]; then
    printf '### Brief\n\n'
    head -25 "$brief_file" 2>/dev/null
    printf '\n'
  fi

  # Last 5 decision headings
  local decision_log="$case_dir/brain/decisions/decision-log.md"
  if [ -f "$decision_log" ]; then
    printf '### Últimas decisões\n\n'
    grep -E "^## D-[0-9]+" "$decision_log" 2>/dev/null | tail -5 | sed 's/^## /- /'
    printf '\n'
  fi

  # Open tasks count + names (max 10)
  if [ -d "$case_dir/brain/tasks" ]; then
    local task_count
    task_count=$(find "$case_dir/brain/tasks" -name "*.md" 2>/dev/null | wc -l | tr -d ' ')
    if [ "${task_count:-0}" -gt 0 ] 2>/dev/null; then
      printf '### Tarefas abertas (%s)\n\n' "$task_count"
      find "$case_dir/brain/tasks" -name "*.md" 2>/dev/null | head -10 | while read -r tf; do
        printf '- %s\n' "$(basename "$tf" .md)"
      done
      printf '\n'
    fi
  fi
}

# Recovery detection: data/ exists with real content but marker is missing.
# Means either: (a) user restored data/ from a backup, or (b) marker was clobbered
# during an update. Drop a breadcrumb rather than silently re-scaffolding.
if [ -d "$DATA_DIR" ] && [ ! -f "$MARKER" ]; then
  if [ -d "$DATA_DIR/agents" ] && [ -n "$(ls -A "$DATA_DIR/agents" 2>/dev/null)" ]; then
    printf '%s\n' "$TS" > "$DATA_DIR/.recovered-$TS" 2>/dev/null
    log_line "RECOVERY  data/agents has content, marker missing — breadcrumb written"
  fi
fi

# ---------------------------------------------------------------------------
# Retroactive memory-layer backfills (GAP-D + gitignore).
# These writes are idempotent (guarded by `! -f`) and must run BEFORE the
# first-run branch check so existing installs from earlier bundles (which
# never wrote these files) get them populated on the next session start.
# Without this, dream-memory silently refuses to write against pre-existing
# workspaces because .schema-version is missing.
# ---------------------------------------------------------------------------
# Ensure data/memory/ itself exists before backfilling tiers. Covers workspaces
# where data/.initialized was written outside the scaffold (backup restore,
# manual copy, dev pre-population) and data/memory/ never got created.
if [ -d "$DATA_DIR" ] && [ ! -d "$DATA_DIR/memory" ]; then
  mkdir -p "$DATA_DIR/memory" 2>/dev/null && \
    log_line "BACKFILL  data/memory/ (root mkdir — missing from pre-existing workspace)"
fi

if [ -d "$DATA_DIR/memory" ]; then
  # Memory tier sub-dirs — required by dream-memory + session-start-memory-inject.
  # Idempotent. Runs even when data/.initialized already exists (workspaces
  # restored from backup, copied manually, or pre-populated in dev), where the
  # first-run branch never executed. Without this, emit_latest_file/emit_all_files
  # find nothing and dream-memory refuses to write because the tier target is
  # missing.
  for tier in recent weekly medium-term lifetime policies; do
    if [ ! -d "$DATA_DIR/memory/$tier" ]; then
      mkdir -p "$DATA_DIR/memory/$tier" 2>/dev/null && \
        log_line "BACKFILL  data/memory/$tier (tier mkdir)"
    fi
  done

  MEMORY_GITIGNORE="$DATA_DIR/memory/.gitignore"
  if [ ! -f "$MEMORY_GITIGNORE" ]; then
    printf '.dream-requested\n' > "$MEMORY_GITIGNORE" 2>/dev/null && \
      log_line "BACKFILL  data/memory/.gitignore (ignores .dream-requested)"
  fi

  MEMORY_SCHEMA_MARKER="$DATA_DIR/memory/.schema-version"
  if [ ! -f "$MEMORY_SCHEMA_MARKER" ]; then
    cat > "$MEMORY_SCHEMA_MARKER" 2>/dev/null <<'EOF'
{
  "schema_version": 1,
  "layers": ["recent", "weekly", "medium-term", "lifetime", "policies"],
  "policy_source": "bundles/base/memory/policy.json",
  "initialized_by": "first-run-scaffold.sh (backfill)"
}
EOF
    log_line "BACKFILL  data/memory/.schema-version (v1)"
  fi
fi

if [ -f "$MARKER" ]; then
  # ---------------------------------------------------------------------------
  # GAP-C — Incremental upgrade detection.
  # Compare the running bundle VERSION against the marker previously written
  # into data/.maestro-version. If they differ, emit an upgrade breadcrumb so
  # `maestro-setup-update` can pick it up. Never mutate data/; only surface
  # signal. Fail-open on missing files.
  # ---------------------------------------------------------------------------
  RUNNING_VERSION="$(cat "$PROJECT_DIR/VERSION" 2>/dev/null | tr -d '[:space:]')"
  INSTALLED_MARKER="$DATA_DIR/.maestro-version"
  INSTALLED_VERSION="$(cat "$INSTALLED_MARKER" 2>/dev/null | tr -d '[:space:]')"

  if [ -n "$RUNNING_VERSION" ] && [ -z "$INSTALLED_VERSION" ]; then
    printf '%s\n' "$RUNNING_VERSION" > "$INSTALLED_MARKER" 2>/dev/null
    log_line "WRITE OK  data/.maestro-version=$RUNNING_VERSION (backfilled)"
  elif [ -n "$RUNNING_VERSION" ] && [ -n "$INSTALLED_VERSION" ] && [ "$RUNNING_VERSION" != "$INSTALLED_VERSION" ]; then
    UPGRADE_MARKER="$DATA_DIR/.upgrade-pending"
    cat > "$UPGRADE_MARKER" 2>/dev/null <<EOF
{
  "from_version": "$INSTALLED_VERSION",
  "to_version": "$RUNNING_VERSION",
  "detected_at": "$TS",
  "action": "run /maestro-setup-update to complete the migration"
}
EOF
    log_line "UPGRADE DETECTED  $INSTALLED_VERSION -> $RUNNING_VERSION (marker: .upgrade-pending)"
  fi

  emit_skills_rollup 2>/dev/null
  emit_active_case_context 2>/dev/null
  exit 0
fi

log_line "SCAFFOLD  project_dir=$PROJECT_DIR"

for sub in agents canary cases memory owner profile workspaces; do
  if mkdir -p "$DATA_DIR/$sub" 2>/dev/null; then
    log_line "MKDIR OK  data/$sub"
  else
    log_line "MKDIR FAIL  data/$sub  (permissions or path issue)"
  fi
done

# Memory layer sub-tiers — required by dream-memory skill (L1/L2/L3 + policies)
for tier in recent weekly medium-term lifetime policies; do
  if mkdir -p "$DATA_DIR/memory/$tier" 2>/dev/null; then
    log_line "MKDIR OK  data/memory/$tier"
  else
    log_line "MKDIR FAIL  data/memory/$tier  (permissions or path issue)"
  fi
done

# Ensure the dreaming marker never gets committed if the user's workspace is a git repo.
MEMORY_GITIGNORE="$DATA_DIR/memory/.gitignore"
if [ ! -f "$MEMORY_GITIGNORE" ]; then
  printf '.dream-requested\n' > "$MEMORY_GITIGNORE" 2>/dev/null && \
    log_line "WRITE OK  data/memory/.gitignore  (ignores .dream-requested)"
fi

# Memory schema version marker — GAP-D. Consumed by dream-memory to detect
# migrations. The current schema is v1 (L1/L2/L3 + lifetime + policies as
# described in bundles/base/memory/policy.json). When the schema evolves,
# dream-memory refuses to write until a migration bumps this marker.
MEMORY_SCHEMA_MARKER="$DATA_DIR/memory/.schema-version"
if [ ! -f "$MEMORY_SCHEMA_MARKER" ]; then
  cat > "$MEMORY_SCHEMA_MARKER" 2>/dev/null <<'EOF'
{
  "schema_version": 1,
  "layers": ["recent", "weekly", "medium-term", "lifetime", "policies"],
  "policy_source": "bundles/base/memory/policy.json",
  "initialized_by": "first-run-scaffold.sh"
}
EOF
  log_line "WRITE OK  data/memory/.schema-version (v1)"
fi

# Owner self facets — ten individually-addressable markdown files (spec 013)
# Creates placeholder files only; content is filled in by /maestro-onboarding.
if mkdir -p "$DATA_DIR/owner/self" 2>/dev/null; then
  log_line "MKDIR OK  data/owner/self"
  for facet in owner-identity personal-context professional-role communication-style \
               voice preferences motivations quality-bar decision-rules working-boundaries; do
    FACET_FILE="$DATA_DIR/owner/self/$facet.md"
    if [ ! -f "$FACET_FILE" ]; then
      printf '# %s\n\n## Current\n\n_Não preenchido. Use /maestro-onboarding para configurar._\n' \
        "$facet" > "$FACET_FILE" 2>/dev/null && \
        log_line "WRITE OK  data/owner/self/$facet.md (placeholder)"
    fi
  done
else
  log_line "MKDIR FAIL  data/owner/self  (permissions or path issue)"
fi

# Owner context tree — extended structure per spec 013
# registry.json — policy/pointer index for all owner sub-trees
if [ ! -f "$DATA_DIR/owner/registry.json" ]; then
  cat > "$DATA_DIR/owner/registry.json" 2>/dev/null <<'EOF'
{
  "schema_version": 1,
  "trees": {
    "self":         "owner/self/",
    "operating":    "owner/operating/",
    "observations": "owner/observations/",
    "interview":    "owner/interview/"
  },
  "initialized": false,
  "owner_type": null,
  "personal_context": {
    "state": "not_asked",
    "state_timestamp": null,
    "source_file": "owner/self/personal-context.md"
  }
}
EOF
  log_line "WRITE OK  data/owner/registry.json (placeholder)"
fi

# owner/self/README.md — canonical index of the 10 SELF facets
if [ ! -f "$DATA_DIR/owner/self/README.md" ]; then
  cat > "$DATA_DIR/owner/self/README.md" 2>/dev/null <<'EOF'
# SELF — Owner Context Facets

Dez arquivos individualmente endereçáveis. Preenchidos por /maestro-onboarding.

| Facet | Arquivo |
|---|---|
| Identidade | owner-identity.md |
| Contexto pessoal | personal-context.md |
| Papel profissional | professional-role.md |
| Estilo de comunicação | communication-style.md |
| Voz | voice.md |
| Preferências | preferences.md |
| Motivações | motivations.md |
| Barra de qualidade | quality-bar.md |
| Regras de decisão | decision-rules.md |
| Limites de trabalho | working-boundaries.md |
EOF
  log_line "WRITE OK  data/owner/self/README.md"
fi

# owner/operating/work-state.md — work continuity placeholder
if mkdir -p "$DATA_DIR/owner/operating" 2>/dev/null; then
  log_line "MKDIR OK  data/owner/operating"
  if [ ! -f "$DATA_DIR/owner/operating/work-state.md" ]; then
    cat > "$DATA_DIR/owner/operating/work-state.md" 2>/dev/null <<'EOF'
# Work State

_Não inicializado. Atualizado automaticamente pelo Maestro ao final de cada sessão de trabalho._

## Last session
- date: —
- active_project: —
- last_decision: —

## Open threads
_Nenhum registrado._
EOF
    log_line "WRITE OK  data/owner/operating/work-state.md (placeholder)"
  fi
else
  log_line "MKDIR FAIL  data/owner/operating"
fi

# owner/observations/ — append-only observations log
if mkdir -p "$DATA_DIR/owner/observations" 2>/dev/null; then
  log_line "MKDIR OK  data/owner/observations"
  if [ ! -f "$DATA_DIR/owner/observations/observations.jsonl" ]; then
    printf '' > "$DATA_DIR/owner/observations/observations.jsonl" 2>/dev/null && \
      log_line "WRITE OK  data/owner/observations/observations.jsonl (empty)"
  fi
else
  log_line "MKDIR FAIL  data/owner/observations"
fi

# owner/interview/ — interview confirmations + drafts
if mkdir -p "$DATA_DIR/owner/interview/drafts" 2>/dev/null; then
  log_line "MKDIR OK  data/owner/interview/drafts"
  if [ ! -f "$DATA_DIR/owner/interview/confirmations.json" ]; then
    cat > "$DATA_DIR/owner/interview/confirmations.json" 2>/dev/null <<'EOF'
{
  "schema_version": 1,
  "completed_tracks": [],
  "last_updated": null
}
EOF
    log_line "WRITE OK  data/owner/interview/confirmations.json (placeholder)"
  fi
else
  log_line "MKDIR FAIL  data/owner/interview/drafts"
fi

if [ ! -d "$DATA_DIR/agents" ]; then
  log_line "ABORT  data/agents was not created — hook exiting fail-open"
  BREADCRUMB_BODY="Maestro first-run scaffold failed at $TS.

O hook tentou criar $DATA_DIR/agents mas não conseguiu (permissões, path bloqueado
por OneDrive/MDM, ou disco cheio). O Maestro está rodando sem workspace persistente.

Próximo passo: peça \"/maestro-doctor\" na próxima mensagem para diagnóstico."

  if ! printf '%s\n' "$BREADCRUMB_BODY" > "$PROJECT_DIR/FIRST-RUN-FAILED.txt" 2>/dev/null; then
    FALLBACK_DIR="${TMPDIR:-/tmp}"
    FALLBACK_PATH="$FALLBACK_DIR/Maestro-FIRST-RUN-FAILED-$$.txt"
    if printf '%s\n\n(Escrito em fallback: diretório do projeto é read-only.)\n' "$BREADCRUMB_BODY" > "$FALLBACK_PATH" 2>/dev/null; then
      log_line "BREADCRUMB FALLBACK  wrote to $FALLBACK_PATH (project dir read-only)"
      printf 'Maestro: setup falhou. Breadcrumb: %s\n' "$FALLBACK_PATH" >&2
    else
      log_line "BREADCRUMB DOUBLE-FAIL  neither project dir nor TMPDIR writable"
    fi
  fi
  exit 0
fi

cat > "$DATA_DIR/README.md" 2>/dev/null <<'EOF'
# data/ — sua workspace do Maestro

Tudo dentro de `data/` é seu. Atualizações do Maestro nunca sobrescrevem este diretório.

- `agents/`   — estado de cada agente (memória de trabalho, decisões, contexto)
- `cases/`    — casos de cliente ativos; cada caso tem brain/ (projects/, decisions/, tasks/, deliverables/, sources/, canon/)
- `memory/`   — memória de longo prazo do Maestro sobre você
- `profile/`  — identidade e preferências
- `workspaces/` — projetos ativos

O caso ativo é indicado por `cases/.active` (contém o case-id). O Maestro injeta contexto do caso ativo a cada sessão.

Se quiser fazer backup, basta copiar `data/` inteiro. Nenhum arquivo aqui depende de código externo.
EOF

# Profile placeholders — created only once; user fills them in via /maestro-onboarding
if [ ! -f "$DATA_DIR/profile/identity.json" ]; then
  cat > "$DATA_DIR/profile/identity.json" 2>/dev/null <<'EOF'
{
  "schema_version": 1,
  "display_name": "",
  "role": "",
  "context": "",
  "initialized": false
}
EOF
  log_line "WRITE OK  data/profile/identity.json (placeholder)"
fi

printf '%s\n' "$TS" > "$MARKER" 2>/dev/null
log_line "DONE  marker written"

# GAP-C — record installed bundle version so future runs can detect upgrades.
FIRST_RUN_VERSION="$(cat "$PROJECT_DIR/VERSION" 2>/dev/null | tr -d '[:space:]')"
if [ -n "$FIRST_RUN_VERSION" ]; then
  printf '%s\n' "$FIRST_RUN_VERSION" > "$DATA_DIR/.maestro-version" 2>/dev/null
  log_line "WRITE OK  data/.maestro-version=$FIRST_RUN_VERSION"
fi

emit_skills_rollup
emit_active_case_context 2>/dev/null

exit 0
