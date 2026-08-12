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

# Recovery detection: data/ exists with real content but marker is missing.
# Means either: (a) user restored data/ from a backup, or (b) marker was clobbered
# during an update. Drop a breadcrumb rather than silently re-scaffolding.
if [ -d "$DATA_DIR" ] && [ ! -f "$MARKER" ]; then
  if [ -d "$DATA_DIR/agents" ] && [ -n "$(ls -A "$DATA_DIR/agents" 2>/dev/null)" ]; then
    printf '%s\n' "$TS" > "$DATA_DIR/.recovered-$TS" 2>/dev/null
    log_line "RECOVERY  data/agents has content, marker missing — breadcrumb written"
  fi
fi

[ -f "$MARKER" ] && exit 0

log_line "SCAFFOLD  project_dir=$PROJECT_DIR"

for sub in agents memory profile workspaces; do
  if mkdir -p "$DATA_DIR/$sub" 2>/dev/null; then
    log_line "MKDIR OK  data/$sub"
  else
    log_line "MKDIR FAIL  data/$sub  (permissions or path issue)"
  fi
done

# Memory layer sub-tiers — required by dream-memory skill
for tier in recent weekly medium-term lifetime policies; do
  if mkdir -p "$DATA_DIR/memory/$tier" 2>/dev/null; then
    log_line "MKDIR OK  data/memory/$tier"
  else
    log_line "MKDIR FAIL  data/memory/$tier  (permissions or path issue)"
  fi
done

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

exit 0
