#!/usr/bin/env bash
# Maestro first-run scaffold — creates data/ workspace on first session.
# Idempotent. Fail-open. Never blocks Claude Code.

set +e

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
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
emit_skills_rollup() {
  local skills_dir="$PROJECT_DIR/bundles/base/skills"
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

  printf '## Maestro skills disponíveis\n\n'
  printf 'Índice compacto. A skill completa é carregada sob demanda quando o pedido do dono a aciona.\n\n'
  printf '%s\n' "$rollup"
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

if [ -f "$MARKER" ]; then
  emit_skills_rollup 2>/dev/null
  exit 0
fi

log_line "SCAFFOLD  project_dir=$PROJECT_DIR"

for sub in agents memory owner profile workspaces; do
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
  "initialized": false
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
- `memory/`   — memória de longo prazo do Maestro sobre você
- `profile/`  — identidade e preferências
- `workspaces/` — projetos ativos

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

if [ ! -f "$DATA_DIR/profile/preferences.json" ]; then
  cat > "$DATA_DIR/profile/preferences.json" 2>/dev/null <<'EOF'
{
  "schema_version": 1,
  "language": "pt-BR",
  "response_style": "direct",
  "interaction_profile": "standard",
  "initialized": false
}
EOF
  log_line "WRITE OK  data/profile/preferences.json (placeholder)"
fi

printf '%s\n' "$TS" > "$MARKER" 2>/dev/null
log_line "DONE  marker written"

emit_skills_rollup

exit 0
