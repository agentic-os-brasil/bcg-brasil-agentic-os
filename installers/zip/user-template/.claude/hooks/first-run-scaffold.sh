#!/usr/bin/env bash
# Maestro first-run scaffold — creates brain/ workspace on first session.
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

# Resolvido a partir da pasta deste arquivo, nao do CLAUDE_PROJECT_DIR.
#
# Ate aqui o source vivia DENTRO do bloco de migracao, que so roda quando ha
# um `data/` para migrar — ou seja, nunca numa instalacao nova. Toda funcao
# deste hook que precisa de `maestro_py` saia em silencio na primeira sessao
# de um dono novo, que e exatamente a sessao em que ela mais importa.
# shellcheck source=lib/python.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/python.sh" 2>/dev/null || true
BRAIN_DIR="$PROJECT_DIR/brain"
MARKER="$BRAIN_DIR/.initialized"
LOG="$BRAIN_DIR/.scaffold.log"
TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)

log_line() {
  ( printf '%s  %s\n' "$TS" "$1" >> "$LOG" ) 2>/dev/null
}

# ---------------------------------------------------------------------------
# Lifetime eligibility policy — required by dream-memory before it may promote
# anything into the permanent memory tier. dream-memory/SKILL.md step 5 stops
# outright when this file is absent ("lifetime activation must fail closed"),
# so without it the permanent tier can never activate on any install.
#
# Spec 006 states the base distribution ships a *named* eligibility policy and
# names the deterministic rule: lifetime promotes only once the rollup carries
# two weekly L3 generations. That rule is implemented by
# DeterministicLifetimeEligibility in internal/memory/deep_synthesizer.go under
# the id `deterministic-l3-continuity-v1`. This file is the workspace-local
# declaration of that same policy, so the shipped skill and the engine agree.
#
# Writing it does not make promotion automatic or unconditional: the policy is
# conservative by construction (nothing is promoted on a first weekly pass) and
# every other lifetime invariant — provenance, version history, no in-place
# overwrite — still applies.
# ---------------------------------------------------------------------------
write_lifetime_policy() {
  local target="$1"
  local origin="$2"
  cat > "$target" 2>/dev/null <<EOF
{
  "schema_version": 1,
  "policy_id": "deterministic-l3-continuity-v1",
  "description": "Promove memória permanente somente quando a consolidação semanal já carrega duas gerações de L3, preservando continuidade antes de tornar algo permanente.",
  "min_l3_generations": 2,
  "promotion": "weekly_deep_dream",
  "automatic": true,
  "versioned_updates": true,
  "direct_overwrite": false,
  "provenance_required": true,
  "initialized_by": "$origin"
}
EOF
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
# O contexto do caso ativo NAO e emitido aqui.
#
# Ele vive em session-start-memory-inject.sh, que este commit traz e que roda
# no mesmo SessionStart. Com os dois emitindo, o brief, as decisoes e a lista
# de tarefas do caso entravam duas vezes no mesmo contexto — e os dois
# procuravam o brief em lugares diferentes, porque este seguia um layout com
# `projects/` que o canonico nao tem. Uma fonte, um leitor.
# ---------------------------------------------------------------------------
# MarkItDown — deteccao deterministica.
#
# O passo 3 do CLAUDE.md mandava eu rodar `markitdown --version` e gravar
# brain/owner/markitdown.json. O arquivo nunca existiu: o check nunca rodou em
# sessao nenhuma. Instrucao que depende de alguem lembrar falha exatamente
# quando o trabalho aperta — o mesmo raciocinio que tirou "ao criar pasta,
# atualize o indice" das maos e passou para o compilador.
#
# Cadencia: checa quando nao ha registro, e recheca depois de 30 dias quando o
# ultimo resultado foi negativo (pode ter sido instalado desde entao). Resultado
# positivo nao e rechecado.
# ---------------------------------------------------------------------------
detect_markitdown() {
  local out="$BRAIN_DIR/owner/markitdown.json"
  [ -d "$BRAIN_DIR/owner" ] || return 0
  maestro_python >/dev/null 2>&1 || return 0

  local due
  due=$(PYTHONIOENCODING=utf-8 maestro_py - "$out" <<'PY' 2>/dev/null
import json, sys, datetime
try:
    with open(sys.argv[1], encoding="utf-8") as f:
        d = json.load(f)
except Exception:
    print("yes"); sys.exit(0)           # sem registro: checa
if d.get("available"):
    print("no"); sys.exit(0)            # ja achou: nao recheca
try:
    last = datetime.datetime.fromisoformat(d["checked_at"].replace("Z", "+00:00"))
    age = (datetime.datetime.now(datetime.timezone.utc) - last).days
except Exception:
    print("yes"); sys.exit(0)
print("yes" if age >= 30 else "no")
PY
)
  [ "$due" = "yes" ] || return 0

  local ver="" avail="false"
  if command -v markitdown >/dev/null 2>&1; then
    ver=$(markitdown --version 2>/dev/null | head -1 | tr -d '"\')
    [ -n "$ver" ] && avail="true"
  fi
  local ts
  ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  if [ "$avail" = "true" ]; then
    printf '{\n  "available": true,\n  "version": "%s",\n  "checked_at": "%s"\n}\n' \
      "$ver" "$ts" > "$out" 2>/dev/null
    log_line "MARKITDOWN disponivel ($ver)"
    # Uma linha, so quando muda de estado: ingestao de documento passou a existir.
    printf '\n## Ingestão de documentos habilitada\n'
    printf 'MarkItDown detectado (%s). PDF, Office e páginas salvas podem ser ingeridos com `ingest-content`.\n' "$ver"
  else
    printf '{\n  "available": false,\n  "checked_at": "%s"\n}\n' "$ts" > "$out" 2>/dev/null
    log_line "MARKITDOWN ausente (recheca em 30 dias)"
    # Silencio de proposito: ausencia nao e problema do dono nem pedido de setup.
  fi
}

# ---------------------------------------------------------------------------
# O contexto do caso ativo NAO e emitido aqui — ver session-start-memory-inject.sh.
# ---------------------------------------------------------------------------

# Recovery detection: brain/ exists with real content but marker is missing.
# Means either: (a) user restored brain/ from a backup, or (b) marker was clobbered
# during an update. Drop a breadcrumb rather than silently re-scaffolding.
if [ -d "$BRAIN_DIR" ] && [ ! -f "$MARKER" ]; then
  if [ -d "$BRAIN_DIR/memory" ] && [ -n "$(ls -A "$BRAIN_DIR/memory" 2>/dev/null)" ]; then
    printf '%s\n' "$TS" > "$BRAIN_DIR/.recovered-$TS" 2>/dev/null
    log_line "RECOVERY  brain/memory has content, marker missing — breadcrumb written"
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
# Ensure brain/memory/ itself exists before backfilling tiers. Covers workspaces
# where brain/.initialized was written outside the scaffold (backup restore,
# manual copy, dev pre-population) and brain/memory/ never got created.
if [ -d "$BRAIN_DIR" ] && [ ! -d "$BRAIN_DIR/memory" ]; then
  mkdir -p "$BRAIN_DIR/memory" 2>/dev/null && \
    log_line "BACKFILL  brain/memory/ (root mkdir — missing from pre-existing workspace)"
fi

if [ -d "$BRAIN_DIR/memory" ]; then
  # Memory tier sub-dirs — required by dream-memory + session-start-memory-inject.
  # Idempotent. Runs even when brain/.initialized already exists (workspaces
  # restored from backup, copied manually, or pre-populated in dev), where the
  # first-run branch never executed. Without this, emit_latest_file/emit_all_files
  # find nothing and dream-memory refuses to write because the tier target is
  # missing.
  for tier in recent weekly medium-term lifetime policies; do
    if [ ! -d "$BRAIN_DIR/memory/$tier" ]; then
      mkdir -p "$BRAIN_DIR/memory/$tier" 2>/dev/null && \
        log_line "BACKFILL  brain/memory/$tier (tier mkdir)"
    fi
  done

  MEMORY_GITIGNORE="$BRAIN_DIR/memory/.gitignore"
  if [ ! -f "$MEMORY_GITIGNORE" ]; then
    printf '.dream-requested\n' > "$MEMORY_GITIGNORE" 2>/dev/null && \
      log_line "BACKFILL  brain/memory/.gitignore (ignores .dream-requested)"
  fi

  LIFETIME_POLICY="$BRAIN_DIR/memory/policies/lifetime.json"
  if [ ! -f "$LIFETIME_POLICY" ]; then
    write_lifetime_policy "$LIFETIME_POLICY" "first-run-scaffold.sh (backfill)" && \
      log_line "BACKFILL  brain/memory/policies/lifetime.json (deterministic-l3-continuity-v1)"
  fi

  MEMORY_SCHEMA_MARKER="$BRAIN_DIR/memory/.schema-version"
  if [ ! -f "$MEMORY_SCHEMA_MARKER" ]; then
    cat > "$MEMORY_SCHEMA_MARKER" 2>/dev/null <<'EOF'
{
  "schema_version": 1,
  "layers": ["recent", "weekly", "medium-term", "lifetime", "policies"],
  "policy_source": "bundles/base/memory/policy.json",
  "initialized_by": "first-run-scaffold.sh (backfill)"
}
EOF
    log_line "BACKFILL  brain/memory/.schema-version (v1)"
  fi
fi

if [ -f "$MARKER" ]; then
  # ---------------------------------------------------------------------------
  # GAP-C — Incremental upgrade detection.
  # Compare the running bundle VERSION against the marker previously written
  # into brain/.maestro-version. If they differ, emit an upgrade breadcrumb so
  # `maestro-setup-update` can pick it up. Never mutate brain/; only surface
  # signal. Fail-open on missing files.
  # ---------------------------------------------------------------------------
  RUNNING_VERSION="$(cat "$PROJECT_DIR/VERSION" 2>/dev/null | tr -d '[:space:]')"
  INSTALLED_MARKER="$BRAIN_DIR/.maestro-version"
  INSTALLED_VERSION="$(cat "$INSTALLED_MARKER" 2>/dev/null | tr -d '[:space:]')"

  if [ -n "$RUNNING_VERSION" ] && [ -z "$INSTALLED_VERSION" ]; then
    printf '%s\n' "$RUNNING_VERSION" > "$INSTALLED_MARKER" 2>/dev/null
    log_line "WRITE OK  brain/.maestro-version=$RUNNING_VERSION (backfilled)"
  elif [ -n "$RUNNING_VERSION" ] && [ -n "$INSTALLED_VERSION" ] && [ "$RUNNING_VERSION" != "$INSTALLED_VERSION" ]; then
    UPGRADE_MARKER="$BRAIN_DIR/.upgrade-pending"
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
  detect_markitdown 2>/dev/null
  exit 0
fi

# ---------------------------------------------------------------------------
# Migration data/ -> brain/ — runs BEFORE anything is scaffolded.
#
# The rename happened in the product, not in the update ritual. README-INSTALL.md
# tells the owner to copy their data folder from the old install into the new
# one, so after the rename it lands as data/ inside a product that only looks at
# brain/. Without this, someone who updates opens a session to an empty
# workspace with their entire history sitting invisibly beside it.
#
# Before the scaffold on purpose: the migrated identity.json has to be in place
# by the time the scaffold's [ ! -f ] tests decide whether to write a
# placeholder. The other way round, the owner gets a blank identity written over
# their own.
#
# The tool copies (never moves), never overwrites, and refuses when both trees
# hold content. Fail-open on any error.
#
# No interpreter, no migration — and in that case the scaffold must NOT build a
# fresh brain/ next to an unmigrated data/. An empty workspace beside an intact
# one reads as total memory loss to the owner, and it is the state hardest to
# tell apart from a real one. Better to stop, leave both trees untouched and say
# why: nothing is lost, and the next session with an interpreter completes it.
# ---------------------------------------------------------------------------
MIGRATE_TOOL="$PROJECT_DIR/bundles/base/tools/migrate-data-to-brain.py"
if [ -d "$PROJECT_DIR/data" ] && [ ! -f "$MARKER" ]; then
  if [ -f "$MIGRATE_TOOL" ] && maestro_python >/dev/null 2>&1; then
    MIG_OUT=$(PYTHONIOENCODING=utf-8 maestro_py "$MIGRATE_TOOL" --project "$PROJECT_DIR" 2>/dev/null)
    [ -n "$MIG_OUT" ] && log_line "MIGRATE   $MIG_OUT"
  else
    log_line "MIGRATE SKIP  data/ present and no interpreter available — scaffold aborted to avoid an empty brain/ beside it"
    printf '<!-- maestro:migration-blocked -->
'
    printf 'A pasta de trabalho da versão anterior (`data/`) está aqui e ainda não foi trazida para o formato novo (`brain/`).
'
    printf 'Nada foi perdido e nada foi alterado — a `data/` está intacta. O Maestro não criou uma pasta nova vazia de propósito, para não parecer que sua memória sumiu.
'
    printf '
**Ação:** avise o time BCG Brasil AI. Falta uma peça nesta máquina para concluir a atualização; assim que ela estiver lá, a próxima sessão traz tudo sozinha.
'
    exit 0
  fi
fi


log_line "SCAFFOLD  project_dir=$PROJECT_DIR"

# `tasks` is the ninth trunk, not an eighth-plus-one. It holds a view of the
# case and owner checkboxes that the brain index compiles; the compiler and
# the SessionStart block that reads brain/tasks/tasks.md arrive with the
# indexing change, so the directory is empty until then. Created here anyway:
# the tree shape belongs to the scaffold, and adding it later would mean a
# second pass over this file for a directory name.
for sub in accounts craft daily development learnings memory owner people tasks; do
  if mkdir -p "$BRAIN_DIR/$sub" 2>/dev/null; then
    log_line "MKDIR OK  brain/$sub"
  else
    log_line "MKDIR FAIL  brain/$sub  (permissions or path issue)"
  fi
done

# Memory layer sub-tiers — required by dream-memory skill (L1/L2/L3 + policies)
for tier in recent weekly medium-term lifetime policies; do
  if mkdir -p "$BRAIN_DIR/memory/$tier" 2>/dev/null; then
    log_line "MKDIR OK  brain/memory/$tier"
  else
    log_line "MKDIR FAIL  brain/memory/$tier  (permissions or path issue)"
  fi
done

# Ensure the dreaming marker never gets committed if the user's workspace is a git repo.
MEMORY_GITIGNORE="$BRAIN_DIR/memory/.gitignore"
if [ ! -f "$MEMORY_GITIGNORE" ]; then
  printf '.dream-requested\n' > "$MEMORY_GITIGNORE" 2>/dev/null && \
    log_line "WRITE OK  brain/memory/.gitignore  (ignores .dream-requested)"
fi

# Lifetime eligibility policy — see write_lifetime_policy() above.
LIFETIME_POLICY="$BRAIN_DIR/memory/policies/lifetime.json"
if [ ! -f "$LIFETIME_POLICY" ]; then
  write_lifetime_policy "$LIFETIME_POLICY" "first-run-scaffold.sh" && \
    log_line "WRITE OK  brain/memory/policies/lifetime.json (deterministic-l3-continuity-v1)"
fi

# Memory schema version marker — GAP-D. Consumed by dream-memory to detect
# migrations. The current schema is v1 (L1/L2/L3 + lifetime + policies as
# described in bundles/base/memory/policy.json). When the schema evolves,
# dream-memory refuses to write until a migration bumps this marker.
MEMORY_SCHEMA_MARKER="$BRAIN_DIR/memory/.schema-version"
if [ ! -f "$MEMORY_SCHEMA_MARKER" ]; then
  cat > "$MEMORY_SCHEMA_MARKER" 2>/dev/null <<'EOF'
{
  "schema_version": 1,
  "layers": ["recent", "weekly", "medium-term", "lifetime", "policies"],
  "policy_source": "bundles/base/memory/policy.json",
  "initialized_by": "first-run-scaffold.sh"
}
EOF
  log_line "WRITE OK  brain/memory/.schema-version (v1)"
fi

# Owner self facets — ten individually-addressable markdown files (spec 013)
# Creates placeholder files only; content is filled in by /maestro-onboarding.
if mkdir -p "$BRAIN_DIR/owner/self" 2>/dev/null; then
  log_line "MKDIR OK  brain/owner/self"
  for facet in owner-identity personal-context professional-role communication-style \
               voice preferences motivations quality-bar decision-rules working-boundaries; do
    FACET_FILE="$BRAIN_DIR/owner/self/$facet.md"
    if [ ! -f "$FACET_FILE" ]; then
      printf '# %s\n\n## Current\n\n_Não preenchido. Use /maestro-onboarding para configurar._\n' \
        "$facet" > "$FACET_FILE" 2>/dev/null && \
        log_line "WRITE OK  brain/owner/self/$facet.md (placeholder)"
    fi
  done
else
  log_line "MKDIR FAIL  brain/owner/self  (permissions or path issue)"
fi

# Owner context tree — extended structure per spec 013
# registry.json — policy/pointer index for all owner sub-trees
if [ ! -f "$BRAIN_DIR/owner/registry.json" ]; then
  cat > "$BRAIN_DIR/owner/registry.json" 2>/dev/null <<'EOF'
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
  log_line "WRITE OK  brain/owner/registry.json (placeholder)"
fi

# owner/self/README.md — canonical index of the 10 SELF facets
if [ ! -f "$BRAIN_DIR/owner/self/README.md" ]; then
  cat > "$BRAIN_DIR/owner/self/README.md" 2>/dev/null <<'EOF'
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
  log_line "WRITE OK  brain/owner/self/README.md"
fi

# owner/operating/work-state.md — work continuity placeholder
if mkdir -p "$BRAIN_DIR/owner/operating" 2>/dev/null; then
  log_line "MKDIR OK  brain/owner/operating"
  if [ ! -f "$BRAIN_DIR/owner/operating/work-state.md" ]; then
    cat > "$BRAIN_DIR/owner/operating/work-state.md" 2>/dev/null <<'EOF'
# Work State

_Não inicializado. Atualizado automaticamente pelo Maestro ao final de cada sessão de trabalho._

## Last session
- date: —
- active_project: —
- last_decision: —

## Open threads
_Nenhum registrado._
EOF
    log_line "WRITE OK  brain/owner/operating/work-state.md (placeholder)"
  fi
else
  log_line "MKDIR FAIL  brain/owner/operating"
fi

# owner/observations/ — append-only observations log
if mkdir -p "$BRAIN_DIR/owner/observations" 2>/dev/null; then
  log_line "MKDIR OK  brain/owner/observations"
  if [ ! -f "$BRAIN_DIR/owner/observations/observations.jsonl" ]; then
    printf '' > "$BRAIN_DIR/owner/observations/observations.jsonl" 2>/dev/null && \
      log_line "WRITE OK  brain/owner/observations/observations.jsonl (empty)"
  fi
else
  log_line "MKDIR FAIL  brain/owner/observations"
fi

# The owner's own trees (spec 007 navigation layer). start-day, eod,
# craft-update, feedback-capture, learnings-bridge and upward-feedback all read
# and write here. Their reads are what rank the day and carry continuity
# between sessions; with no tree and no objectives file those reads return
# nothing and the daily ritual degrades to a briefing composed from the session
# alone.
#
# These sit at the top of brain/ rather than under owner/atlas/. Each has one
# clear owner and is addressed by its own name — brain/daily, brain/craft —
# everywhere else in the product, so nesting them under a navigation folder
# added a level nothing referenced.
#
# Only the directories and the two index pages are created. Daily pages, method
# and style pages, learnings and feedback captures are authored by the skills
# themselves — placeholders there would be mistaken for real content.
if mkdir -p "$BRAIN_DIR/craft" 2>/dev/null; then
  log_line "MKDIR OK  brain/craft"
  for owner_sub in daily craft/methods craft/style learnings people                    development/cdc development/project-feedback development/upward-feedback                    development/retros; do
    if mkdir -p "$BRAIN_DIR/$owner_sub" 2>/dev/null; then
      log_line "MKDIR OK  brain/$owner_sub"
    else
      log_line "MKDIR FAIL  brain/$owner_sub  (permissions or path issue)"
    fi
  done

  if [ ! -f "$BRAIN_DIR/craft/craft.md" ]; then
    cat > "$BRAIN_DIR/craft/craft.md" 2>/dev/null <<'EOF'
# Craft

Métodos e calibrações de estilo que se mantêm verdadeiros entre projetos.

- `methods/` — técnicas reutilizáveis, cada uma em sua própria página.
- `style/` — como você calibra o trabalho em uma situação específica.

_Ainda vazio. As páginas são criadas por `/craft-update` e `/learnings-bridge`._
EOF
    log_line "WRITE OK  brain/craft/craft.md"
  fi

  if [ ! -f "$BRAIN_DIR/learnings/learnings.md" ]; then
    cat > "$BRAIN_DIR/learnings/learnings.md" 2>/dev/null <<'EOF'
# Learnings

Aprendizados profissionais duráveis, corrigíveis e ligados às suas fontes
quando aplicável.

_Ainda vazio. As páginas são criadas por `/learnings-bridge`._
EOF
    log_line "WRITE OK  brain/learnings/learnings.md"
  fi

  # objectives.md is read by start-day, eod and feedback-capture to tie the day
  # to what the owner is actually working toward. It is authored by the owner,
  # so it ships as an empty structure rather than invented content.
  if [ ! -f "$BRAIN_DIR/development/objectives.md" ]; then
    # The heading set is load-bearing, not decoration. feedback-capture files
    # evidence and retirements with `append-entry`, which never creates a
    # heading and refuses one that appears more than once on a page. So the
    # page must ship with a uniquely-numbered evidence heading per objective
    # and a single retirement heading, or those writes are declined outright.
    cat > "$BRAIN_DIR/development/objectives.md" 2>/dev/null <<'EOF'
# Objetivos de desenvolvimento

O que você está tentando desenvolver neste período, e o que conta como
progresso. Lido no início e no fim do dia para conectar o trabalho ao objetivo,
e atualizado quando chega feedback formal.

_Não preenchido. Diga "quero definir meus objetivos" para preencher._

## Objetivos ativos

### 1. <objetivo>
- **Status:** ativo | em risco | atingido
- **Como saber que avançou:**
- **Última confirmação:** YYYY-MM-DD
- **Próxima revisão:** YYYY-MM-DD

#### Evidência — objetivo 1

<!-- append-only: uma linha datada por evidência, citando a origem -->

## Aposentados

<!-- append-only: uma linha datada por objetivo aposentado, nomeando a review que o aposentou -->
EOF
    log_line "WRITE OK  brain/development/objectives.md (placeholder)"
  fi
else
  log_line "MKDIR FAIL  brain/owner  (permissions or path issue)"
fi

# owner/interview/ — interview confirmations + drafts
if mkdir -p "$BRAIN_DIR/owner/interview/drafts" 2>/dev/null; then
  log_line "MKDIR OK  brain/owner/interview/drafts"
  if [ ! -f "$BRAIN_DIR/owner/interview/confirmations.json" ]; then
    cat > "$BRAIN_DIR/owner/interview/confirmations.json" 2>/dev/null <<'EOF'
{
  "schema_version": 1,
  "completed_tracks": [],
  "last_updated": null
}
EOF
    log_line "WRITE OK  brain/owner/interview/confirmations.json (placeholder)"
  fi
else
  log_line "MKDIR FAIL  brain/owner/interview/drafts"
fi

if [ ! -d "$BRAIN_DIR/memory" ]; then
  log_line "ABORT  brain/memory was not created — hook exiting fail-open"
  BREADCRUMB_BODY="Maestro first-run scaffold failed at $TS.

O hook tentou criar $BRAIN_DIR/memory mas não conseguiu (permissões, path bloqueado
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

cat > "$BRAIN_DIR/README.md" 2>/dev/null <<'EOF'
# brain/ — sua workspace do Maestro

Tudo dentro de `brain/` é seu. Atualizações do Maestro nunca sobrescrevem este diretório.

- `memory/`      — memória consolidada, escrita pelo motor de dreaming (recente, semanal, médio prazo, permanente)
- `owner/`       — quem você é: identidade, estilo, facetas SELF, estado de trabalho, observações
- `daily/`       — a página de cada dia de trabalho
- `learnings/`   — aprendizados profissionais duráveis
- `craft/`       — métodos e calibrações de estilo que se mantêm entre projetos
- `people/`      — perfis de colegas com quem você trabalhou
- `development/` — objetivos, retrospectivas, feedback recebido e a dar
- `accounts/`    — clientes, cada um com `cases/<projeto>/`: o brief em `<projeto>.md` na raiz do caso, e decisions/,
  tasks/, deliverables/, sources/, canon/ em subpastas
- `tasks/`       — visão de tarefas derivada dos casos

Há ainda um `.maestro/`, que é área de máquina: índices, backlinks, diagnóstico e
log, todos regeneráveis a partir das suas páginas. Não precisa de backup e não deve
ser editado à mão.

O índice de tudo é `brain_index.md`, na raiz — sempre regenerado, nunca editado à
mão. Cada conta e cada caso têm o seu, na própria pasta.

O caso ativo é indicado por `accounts/.active`, que contém `<cliente>/<projeto>`.
O Maestro injeta o contexto do caso ativo a cada sessão.

Se quiser fazer backup, basta copiar `brain/` inteiro. Nenhum arquivo aqui depende de código externo.
EOF

# Profile placeholders — created only once; user fills them in via /maestro-onboarding
if [ ! -f "$BRAIN_DIR/owner/identity.json" ]; then
  cat > "$BRAIN_DIR/owner/identity.json" 2>/dev/null <<'EOF'
{
  "schema_version": 1,
  "display_name": "",
  "role": "",
  "context": "",
  "initialized": false
}
EOF
  log_line "WRITE OK  brain/owner/identity.json (placeholder)"
fi

printf '%s\n' "$TS" > "$MARKER" 2>/dev/null
log_line "DONE  marker written"

# GAP-C — record installed bundle version so future runs can detect upgrades.
FIRST_RUN_VERSION="$(cat "$PROJECT_DIR/VERSION" 2>/dev/null | tr -d '[:space:]')"
if [ -n "$FIRST_RUN_VERSION" ]; then
  printf '%s\n' "$FIRST_RUN_VERSION" > "$BRAIN_DIR/.maestro-version" 2>/dev/null
  log_line "WRITE OK  brain/.maestro-version=$FIRST_RUN_VERSION"
fi

emit_skills_rollup
detect_markitdown 2>/dev/null

exit 0
