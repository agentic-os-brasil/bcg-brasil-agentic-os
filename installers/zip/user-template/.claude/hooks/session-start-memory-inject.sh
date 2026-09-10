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

# shellcheck source=lib/python.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/python.sh" 2>/dev/null || true

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"

# The maestro:session-context markers are recognized by the Maestro runtime.
# Content between them is appended to the session context budget.

printf '<!-- maestro:session-context:start -->\n'
printf '# Maestro — Contexto da sessão\n'
printf '_Injetado automaticamente pelo hook de início de sessão._\n'

# No interpreter, no automation. Every hook that parses JSON needs one, so
# without it memory injection, context routing and case isolation are all
# inert — and the owner has no way to tell, because each of those hooks exits
# 0 in silence. That silence is the defect; this block ends it.
#
# Emitted on every SessionStart where resolution fails — which includes
# `/clear` and `/compact`, not just the first start of the day. That is
# deliberate rather than noisy: those events drop the earlier notice out of
# context, so a model that has just lost it needs to be told again. A
# once-only marker would buy quiet at the cost of the model not knowing.
if ! maestro_python >/dev/null 2>&1; then
  printf '\n## ⚠️ Uma peça do Maestro não está instalada nesta máquina\n'
  printf '<!-- maestro:python-missing -->\n'
  printf 'Sem ela, o Maestro não lembra o contexto entre conversas, não sugere skills e não protege a separação entre clientes. Ele abre e funciona para conversar, mas essas rotinas estão desligadas.\n'
  printf '\n**Ação:** leia `bundles/base/skills/maestro-environment-setup/SKILL.md`, seção "Interpretador local". Se houver um interpretador nesta máquina fora do PATH, ela sabe apontar o Maestro para ele. Se não houver, avise o dono em uma linha e siga — instalar não está autorizado hoje.\n'
fi
BRAIN_DIR="$PROJECT_DIR/brain"
MEMORY_DIR="$BRAIN_DIR/memory"
PROFILE_DIR="$BRAIN_DIR/owner"

# ---------------------------------------------------------------------------
# Orcamento de contexto (bundles/base/memory/runtime.json).
#
# Ate agora esse arquivo declarava cinco tetos e nenhum era lido por linha
# nenhuma de codigo. O unico enforcement que existia era um 768 passado a mao
# na projecao de L3. Aqui os tetos passam a valer: cada bloco e medido, cortado
# em limite de linha quando estoura, e o medido vai para
# brain/.maestro/context-envelope.json — o context_envelope que o sensor de
# apodrecimento do Darwin pede e nunca recebia.
#
# Fail-open: sem interpretador resolvivel ou sem o arquivo, os defaults abaixo valem e nada e
# cortado alem deles.
# ---------------------------------------------------------------------------
RUNTIME_JSON="$PROJECT_DIR/bundles/base/memory/runtime.json"
CAP_self=14000; CAP_lifetime=3500; CAP_l3=768; CAP_l2=2600; CAP_l1=3600
CAP_learnings=2000; CAP_craft=1200; CAP_total=30000

if [ -f "$RUNTIME_JSON" ] && maestro_python >/dev/null 2>&1; then
  # shellcheck disable=SC1090
  eval "$(PYTHONIOENCODING=utf-8 maestro_py - "$RUNTIME_JSON" <<'PY' 2>/dev/null
import json, sys
KEYS = ("self", "lifetime", "l3", "l2", "l1", "learnings", "craft", "total")
try:
    with open(sys.argv[1], encoding="utf-8") as f:
        d = json.load(f)
except Exception:
    sys.exit(0)
for k in KEYS:
    v = d.get(f"session_context_{k}_max_bytes")
    if isinstance(v, int) and v > 0:
        print(f"CAP_{k}={v}")
PY
)"
fi

# O envelope se acumula em ARQUIVO, nao em variavel: cap_and_emit e usado do lado
# direito de um pipe e roda em subshell, onde toda atribuicao morre ao fechar.
ENVELOPE_FILE=""
MEM_EMITTER="$PROJECT_DIR/bundles/base/tools/session-memory-emit.py"
if [ -f "$MEM_EMITTER" ] && maestro_python >/dev/null 2>&1; then
  ENVELOPE_FILE=$(mktemp 2>/dev/null || printf '%s' "${TMPDIR:-/tmp}/maestro-envelope.$$")
  : > "$ENVELOPE_FILE" 2>/dev/null
else
  MEM_EMITTER=""
fi

# O corte por orcamento, a medicao e os quatro emissores de bloco vivem agora em
# `bundles/base/tools/session-memory-emit.py` — ver `emit_memory_blocks` abaixo.
# Em shell eles custavam ~350 spawns e 35 s por sessao.

# If brain/ does not exist yet (first-run-scaffold.sh had not run or failed),
# exit silently — nothing to inject.
[ -d "$BRAIN_DIR" ] || exit 0

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# Os emissores de bloco (emit_latest_file, emit_all_files, emit_titles,
# emit_l3_projection) sairam daqui para session-memory-emit.py — ver a nota
# em emit_memory_blocks. Em shell eram ~350 spawns por sessao.

emit_profile_json() {
  local label="$1"
  local file="$2"
  [ -f "$file" ] || return
  # Pass the path as argv, never interpolated into the Python source: a Windows
  # path such as C:\Users\... makes \U an invalid unicode escape inside a string
  # literal, the interpreter raises SyntaxError, 2>/dev/null swallows it, and the
  # guard fails open — injecting the empty placeholder identity into every
  # session. context-inject-userprompt.sh already reads it this way.
  if maestro_python >/dev/null 2>&1; then
    local initialized
    initialized=$(PYTHONIOENCODING=utf-8 maestro_py - "$file" <<'PY' 2>/dev/null
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
  # INDEX.md and catalog.json live beside the skills, at
  # bundles/tech-core/skills/, not at the bundle root.
  if [ -f "$TECH_CORE_DIR/skills/INDEX.md" ]; then
    printf 'Índice: %s/skills/INDEX.md\n' "$TECH_CORE_DIR"
  fi
  if [ -f "$TECH_CORE_DIR/skills/catalog.json" ]; then
    printf 'Catálogo: %s/skills/catalog.json\n' "$TECH_CORE_DIR"
  fi
  printf 'Instrução: skills de engenharia (testes, revisão, pipelines, entrega por spec) — carregar sob demanda quando a tarefa exigir.\n'
fi

# ---------------------------------------------------------------------------
# Agentes de instância — conta e caso. O que a sessão VIRA, não algo que chama.
#
# Estava no first-run-scaffold.sh, cujo trabalho é montar a brain na primeira
# execução. Os dois hooks rodam no mesmo evento, então nada quebrava — mas quem
# fosse procurar de onde vem o bloco de agentes procurava no lugar errado, e o
# hook que carrega a história ("marcador escrito e nunca lido... aconteceu com
# os agentes", mais abaixo neste arquivo) não era o que fazia o trabalho.
#
# A mudança que importa não é o endereço: é a fonte. As regras de escopo eram
# cinco `printf` fixos dentro do shell, sem relação nenhuma com as duas
# especificações que o catálogo declara. Editar `case-agent/AGENT.md` não movia
# uma vírgula do que a sessão obedecia. Agora o texto sai do bloco
# `maestro:session-scope` de cada spec, com {account} e {case} substituídos:
# uma fonte, um leitor.
#
# Fail-open em toda etapa: sem .active, sem agent.json, sem interpretador ou sem o
# bloco na spec, não emite nada e a sessão segue.
# ---------------------------------------------------------------------------
emit_active_case_context() {
  local accounts_dir="$BRAIN_DIR/accounts"
  local active_file="$accounts_dir/.active"

  [ -f "$active_file" ] || return 0
  maestro_python >/dev/null 2>&1 || return 0

  local active_id
  active_id=$(tr -d '[:space:]' < "$active_file" 2>/dev/null)
  [ -z "$active_id" ] && return 0

  # Divide "<conta>/<caso>". Um case-id solto (formato legado) resolve para um
  # diretório que não existe e cai na guarda abaixo — fail-open, sem saída.
  local account_id="${active_id%%/*}"
  local case_id="${active_id##*/}"
  [ -z "$account_id" ] && return 0
  [ -z "$case_id" ] && return 0

  local case_dir="$accounts_dir/$account_id/cases/$case_id"
  [ -d "$case_dir" ] || return 0

  PYTHONIOENCODING=utf-8 maestro_py - "$PROJECT_DIR" "$account_id" "$case_id" <<'PY' 2>/dev/null
import io, json, os, re, sys

project, account_id, case_id = sys.argv[1], sys.argv[2], sys.argv[3]
accounts = os.path.join(project, "brain", "accounts")
case_dir = os.path.join(accounts, account_id, "cases", case_id)

SCOPE_RE = re.compile(
    r"<!--\s*maestro:session-scope:start\s*-->(.*?)<!--\s*maestro:session-scope:end\s*-->",
    re.S)


def agent_field(path, key):
    try:
        v = json.load(io.open(path, encoding="utf-8")).get(key, "")
    except (OSError, ValueError, AttributeError):
        return ""
    return v if isinstance(v, str) else ""


def scope_lines(spec_rel):
    """As linhas de escopo declaradas pela especificação da camada."""
    try:
        raw = io.open(os.path.join(project, spec_rel), encoding="utf-8").read()
    except OSError:
        return []
    m = SCOPE_RE.search(raw)
    if not m:
        return []
    out = []
    for ln in m.group(1).splitlines():
        ln = ln.strip()
        if ln.startswith("- "):
            out.append(ln.replace("{account}", account_id).replace("{case}", case_id))
    return out


acct_json = os.path.join(accounts, account_id, "agent.json")
case_json = os.path.join(case_dir, "agent.json")
acct_name, acct_emoji = agent_field(acct_json, "name"), agent_field(acct_json, "emoji")
case_name, case_emoji = agent_field(case_json, "name"), agent_field(case_json, "emoji")

print(f"\n## Caso ativo: {account_id}/{case_id}\n")

if case_name or acct_name:
    print("### Agentes desta sessão\n")
    if acct_name:
        print(f"- **{acct_emoji} {acct_name}** — agente da conta `{account_id}` "
              f"(client_account_agent).")
    if case_name:
        print(f"- **{case_emoji} {case_name}** — agente do caso `{case_id}` "
              f"(case_agent). É o escopo ativo.")
    lines = (scope_lines("bundles/base/agents/case-agent/AGENT.md")
             + scope_lines("bundles/base/agents/client-account-agent/AGENT.md"))
    if lines:
        print("\nEnquanto este caso estiver ativo, o escopo de trabalho é o dele:")
        for ln in lines:
            print(ln)
    print()
PY

  # Brief do projeto — o unico .md na raiz do caso, primeiras 25 linhas.
  # Vivia em projects/<case-id>.md ate 2026-09-07; colapsado direto na raiz
  # do caso (o caso ja e o projeto) — ver case-agent-setup/SKILL.md. As 25
  # primeiras linhas ainda pegam so o brief: a secao gerada (canon/decisoes/
  # tarefas) fica colada bem mais abaixo, depois de um marcador.
  local brief_file=""
  for f in "$case_dir/"*.md; do
    [ -f "$f" ] && brief_file="$f" && break
  done
  if [ -n "$brief_file" ]; then
    printf '### Brief\n\n'
    head -25 "$brief_file" 2>/dev/null
    printf '\n'
  fi

  # Últimas 5 decisões
  local decision_log="$case_dir/decisions/decision-log.md"
  if [ -f "$decision_log" ]; then
    printf '### Últimas decisões\n\n'
    grep -E "^## D-[0-9]+" "$decision_log" 2>/dev/null | tail -5 | sed 's/^## /- /'
    printf '\n'
  fi

  # Tarefas abertas do caso ativo, lidas do compilador.
  #
  # Isto contava ARQUIVOS: `find tasks -name "*.md" | wc -l`, e imprimia o nome
  # de cada pagina como se fosse uma tarefa. O dono via "Tarefas abertas (4)"
  # com nomes de PAGINA enquanto o numero real de itens abertos era ZERO: todos
  # os checkboxes daquelas paginas ja estavam marcados. Um painel que mente para
  # cima e pior que painel nenhum. A fonte certa ja existia:
  # brain/.maestro/tasks.json, que o compilador produz lendo checkbox a checkbox.
  local tasks_json="$BRAIN_DIR/.maestro/tasks.json"
  if [ -f "$tasks_json" ]; then
    PYTHONIOENCODING=utf-8 maestro_py - "$tasks_json" "$account_id/$case_id" <<'PY' 2>/dev/null
import json, sys
try:
    tasks = json.load(open(sys.argv[1], encoding="utf-8"))
except Exception:
    sys.exit(0)
acct, case = sys.argv[2].split("/", 1)
scope = f"account/{acct}/case/{case}"
live = [t for t in tasks
        if t.get("scope") == scope and t.get("status") != "concluida"]
if not live:
    sys.exit(0)
live.sort(key=lambda t: ({"P0": 0, "P1": 1, "P2": 2}.get(t.get("priority"), 1),
                         -int(t.get("age_days") or 0)))
print(f"### Tarefas abertas ({len(live)})\n")
for t in live[:10]:
    txt = " ".join((t.get("text") or "").split())
    if len(txt) > 110:
        txt = txt[:107] + "..."
    marks = []
    if t.get("priority") == "P0":
        marks.append("P0")
    if t.get("status") == "em-andamento":
        marks.append("em andamento")
    if t.get("blocked_external"):
        marks.append("depende de terceiro")
    suffix = f" _({', '.join(marks)})_" if marks else ""
    print(f"- {txt}{suffix}")
if len(live) > 10:
    print(f"- _... e mais {len(live) - 10}. Visão completa em `brain/tasks/tasks.md`._")
print()
PY
  fi
}

emit_active_case_context 2>/dev/null

# Scaffold / onboarding status — antes CLAUDE.md mandava a sessão ler
# brain/.initialized e brain/owner/onboarding.json toda vez, para redescobrir
# um estado que este hook já resolveu (first-run-scaffold.sh roda antes deste
# e já sabe se deu certo). Emite bloco só quando há algo a fazer; silêncio
# quando scaffold e onboarding estão OK — mesma disciplina do resto deste
# arquivo (abertura de dia, saúde do brain).
FIRST_RUN_FAILED="$PROJECT_DIR/FIRST-RUN-FAILED.txt"
if [ -f "$FIRST_RUN_FAILED" ]; then
  printf '\n## ⚠️ Scaffold do Maestro falhou\n'
  printf '<!-- maestro:scaffold-failed -->\n'
  printf 'O arquivo `FIRST-RUN-FAILED.txt` existe na raiz do projeto — o hook de primeira execução não conseguiu montar `brain/`.\n'
  printf '\n**Ação obrigatória:** leia `bundles/base/skills/maestro-doctor/SKILL.md` e execute o diagnóstico antes de qualquer outra tarefa.\n'
fi

ONBOARDING_JSON="$PROFILE_DIR/onboarding.json"
if maestro_python >/dev/null 2>&1; then
  ONB_STATUS=$(PYTHONIOENCODING=utf-8 maestro_py - "$ONBOARDING_JSON" <<'PY' 2>/dev/null
import json, sys
try:
    d = json.load(open(sys.argv[1], encoding="utf-8"))
except Exception:
    print("missing"); sys.exit(0)
print(d.get("status") or "missing")
PY
)
  if [ -n "$ONB_STATUS" ] && [ "$ONB_STATUS" != "complete" ]; then
    printf '\n## Onboarding pendente\n'
    printf '<!-- maestro:onboarding-pending: status=%s -->\n' "$ONB_STATUS"
    printf 'Status atual: `%s`.\n' "$ONB_STATUS"
    printf '\n**Ação obrigatória:** leia `bundles/base/skills/maestro-onboarding/SKILL.md` e siga a partir de onde parou (do início, se `missing`), antes de responder ao pedido do dono.\n'
  fi

  # Rede de segurança: onboarding fechado como `complete` e nenhuma conta no
  # disco. Isso significa que a pergunta 9 não montou o espaço de trabalho —
  # por bug, por interrupção, ou porque a instalação veio de uma versão antiga
  # do onboarding, que não montava nada. Sem conta não há caso, sem caso não há
  # agentes, e a sessão inteira roda sem escopo: cada pedido de trabalho de
  # cliente cai no brain do dono em vez de cair no caso. Não é bloqueante — o
  # dono pode legitimamente estar entre projetos.
  if [ "$ONB_STATUS" = "complete" ] && [ ! -f "$PROFILE_DIR/.no-account-ack" ]; then
    ACC_DIR="$BRAIN_DIR/accounts"
    ACC_N=0
    if [ -d "$ACC_DIR" ]; then
      for _a in "$ACC_DIR"/*/; do
        [ -d "$_a" ] && ACC_N=$((ACC_N + 1))
      done
    fi
    if [ "$ACC_N" -eq 0 ]; then
      printf '\n## Sem conta nem caso montado (não bloqueante)\n'
      printf '<!-- maestro:no-account -->\n'
      printf 'O onboarding está `complete`, mas `brain/accounts/` está vazia: não há conta, caso nem agentes de caso.\n'
      printf '\n**Ação sugerida:** ofereça montar isso na primeira resposta, em uma linha, sem travar o pedido do dono — por exemplo: "reparei que você ainda não tem um caso montado aqui; quer que eu crie agora?". Se ele aceitar, leia `bundles/base/skills/account-case-setup/SKILL.md` e depois `bundles/base/skills/case-agent-setup/SKILL.md` e siga as duas.\n'
      printf 'Se ele recusar ou disser que está entre projetos, escreva o arquivo vazio `brain/owner/.no-account-ack` para este aviso parar de aparecer, e não insista de novo nesta sessão.\n'
    fi
  fi
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

# EOD auto-trigger — check for .eod-requested marker written by
# session-stop-eod-check.sh. Unlike the dream trigger this is never a
# mandatory pre-task block: closing a day always needs the owner's
# confirmation, so it cannot run silently. This is a visible, non-blocking
# reminder for Claude to raise once, early in the first reply.
#
# The marker used to carry only a trigger (its own existence); the message
# below was generic ("pelo menos um dia") and told Claude to go read
# eod/SKILL.md and reconstruct just to find out which day. The marker's
# content — and, since 2026-09-06, day-brief.json — already has that answer,
# computed once at the previous Stop. Emitting it here means the SKILL.md
# read (and the reconstruction it triggers) only happens after the owner
# actually says yes to closing.
EOD_MARKER="$BRAIN_DIR/owner/.eod-requested"
DAY_BRIEF="$BRAIN_DIR/.maestro/day-brief.json"
if [ -f "$EOD_MARKER" ]; then
  printf '\n## 📅 Fechamento de dia pendente (não bloqueante)\n'
  printf '<!-- maestro:eod-trigger: marker=%s -->\n' "$EOD_MARKER"
  EOD_MSG=""
  if [ -f "$DAY_BRIEF" ] && maestro_python >/dev/null 2>&1; then
    EOD_MSG=$(PYTHONIOENCODING=utf-8 maestro_py - "$DAY_BRIEF" <<'PY' 2>/dev/null
import json, sys
try:
    d = json.load(open(sys.argv[1], encoding="utf-8"))
except Exception:
    sys.exit(0)
dates = d.get("eod_open_dates") or []
if not dates:
    sys.exit(0)
if len(dates) == 1:
    print(f"O dia `{dates[0]}` não tem entrada de fechamento na página do dono.")
else:
    print(f"{len(dates)} dias sem fechamento: {', '.join('`'+x+'`' for x in dates)}.")
PY
)
  fi
  if [ -z "$EOD_MSG" ]; then
    EOD_RAW=$(cat "$EOD_MARKER" 2>/dev/null | tr -d '[:space:]')
    EOD_MSG="Ao menos um dia de trabalho não tem entrada de fechamento (marcador: \`${EOD_RAW:-desconhecido}\`)."
  fi
  printf '%s\n' "$EOD_MSG"
  printf '\n**Ação sugerida:** ofereça fechar o(s) dia(s) na primeira resposta desta sessão — sem travar o pedido do dono. Só leia `bundles/base/skills/eod/SKILL.md` (e rode a reconstrução que ela descreve) depois que ele confirmar; se o bloco "Day brief pré-computado" abaixo já cobre os mesmos dias, não releia as páginas diárias só para redescobrir o que já está ali. Fechar sempre exige confirmação dele; nunca grave sem isso. Se ele adiar, deixe o marcador como está e não insista de novo nesta mesma sessão.\n'
fi

# Day brief pré-computado — calculado uma vez no Stop anterior
# (session-stop-eod-check.sh), não a cada sessão. Cobre o que start-day e eod
# mais releem sem necessidade: as duas diárias anteriores e os objetivos
# ativos. Emitido sempre que o arquivo existir, não só quando há fechamento
# pendente — start-day usa isto em re-entrada mesmo sem marcador nenhum.
if [ -f "$DAY_BRIEF" ] && maestro_python >/dev/null 2>&1; then
  PYTHONIOENCODING=utf-8 maestro_py - "$DAY_BRIEF" <<'PY' 2>/dev/null
import json, sys
try:
    d = json.load(open(sys.argv[1], encoding="utf-8"))
except Exception:
    sys.exit(0)
pages = d.get("last_daily_pages") or []
objectives = d.get("objectives_active") or []
if not (pages or objectives):
    sys.exit(0)
print("\n## Day brief pré-computado (Stop da sessão anterior)")
print(f"<!-- maestro:day-brief: generated_at={d.get('generated_at', '')} -->")
print("Use isto em vez de reler as mesmas páginas diárias em `start-day` ou `eod`; "
      "releia o arquivo inteiro só se precisar de algo que não está aqui "
      "(ex.: uma linha específica de plano de projeto).\n")
for p in pages:
    print(f"- **{p['date']}** (`{p['path']}`)")
    if p.get("briefing_last"):
        print(f"  - último briefing: {p['briefing_last'][:200]}")
    if p.get("fechamento_last"):
        print(f"  - último fechamento: {p['fechamento_last'][:200]}")
if objectives:
    print(f"- Objetivos ativos: {', '.join(objectives)}")
PY
fi

# ---------------------------------------------------------------------------
# Rotinas do dia e da semana — checagem direta, sem marcador.
#
# start-day e retro nao ganham marcador de proposito. Marcador existe para
# carregar estado que so o Stop conhece: "o dia de hoje fechou?" so da para
# responder no fim dele. Ja "o dia abriu?" e "a semana teve retro?" o
# SessionStart le direto do disco. Marcador ali seria estado duplicado, com um
# lado livre para ficar velho — e estado velho e de onde vem o bug que nao
# reclama.
#
# Nao bloqueante nas duas. Mencionar uma vez, cedo, e nao insistir.
# ---------------------------------------------------------------------------
if maestro_python >/dev/null 2>&1; then
  PYTHONIOENCODING=utf-8 maestro_py - "$BRAIN_DIR" <<'PY' 2>/dev/null
import datetime, glob, io, os, re, sys

brain = sys.argv[1]
today = datetime.date.today()

# --- o dia abriu? ---------------------------------------------------------
page = os.path.join(brain, "daily", f"{today.isoformat()}.md")
opened = False
if os.path.exists(page):
    try:
        opened = bool(re.search(r"(?m)^### ", io.open(page, encoding="utf-8").read()))
    except OSError:
        opened = True                     # ilegivel: nao cutuca por causa de erro proprio
if not opened:
    print("\n## Abertura de dia pendente (não bloqueante)")
    print("<!-- maestro:startday-check -->")
    print(f"Não há entrada de abertura na página de hoje (`brain/daily/{today.isoformat()}.md`).")
    print("\n**Ação sugerida:** ofereça abrir o dia com `bundles/base/skills/start-day/SKILL.md` "
          "na primeira resposta, sem travar o pedido do dono. Se ele seguir direto para o "
          "trabalho, não insista de novo nesta sessão.")

# --- a semana teve retro? -------------------------------------------------
# So a partir de quinta: antes disso a semana ainda esta acontecendo e a retro
# seria sobre o que ainda vai mudar.
if today.weekday() >= 3:
    newest = None
    for f in glob.glob(os.path.join(brain, "development", "retros", "*.md")):
        m = re.search(r"(\d{4})-(\d{2})-(\d{2})", os.path.basename(f))
        if not m:
            continue
        try:
            d = datetime.date(*map(int, m.groups()))
        except ValueError:
            continue
        if newest is None or d > newest:
            newest = d
    days = (today - newest).days if newest else None
    if days is None or days > 7:
        since = f"A última foi em {newest.isoformat()} ({days} dias)." if newest else \
                "Não há nenhuma retro registrada ainda."
        print("\n## Retrospectiva da semana pendente (não bloqueante)")
        print("<!-- maestro:retro-check -->")
        print(since)
        print("\n**Ação sugerida:** ofereça fechar a semana com "
              "`bundles/base/skills/retro/SKILL.md`. Retro sempre exige conversa com o dono; "
              "nunca escreva a página sem isso. Mencione uma vez e siga.")
PY
fi
# GAP-C — Upgrade-pending auto-trigger. Marker written by first-run-scaffold.sh
# when a session boots against a bundle whose VERSION differs from
# brain/.maestro-version. Surfaces a mandatory action block so Claude routes to
# /maestro-setup-update before any other work.
UPGRADE_MARKER="$BRAIN_DIR/.upgrade-pending"
if [ -f "$UPGRADE_MARKER" ]; then
  printf '\n## ⚠️ Upgrade Maestro pendente — verificar antes de qualquer outra tarefa\n'
  printf '<!-- maestro:upgrade-trigger: marker=%s -->\n' "$UPGRADE_MARKER"
  printf 'O marcador `.upgrade-pending` foi detectado — o VERSION do bundle mudou desde a última sessão.\n\n'
  printf 'Conteúdo do marcador:\n\n```json\n'
  cat "$UPGRADE_MARKER" 2>/dev/null
  printf '\n```\n\n'
  printf '**Ação obrigatória:** invoque `/maestro-setup-update` (fluxo de atualização) para validar a migração antes de responder ao usuário. Apagar o marcador só após o ciclo de update concluir sem erro.\n'
fi

# Saude do brain. O indice ja foi recompilado pelo hook de Stop da sessao
# anterior — isto aqui surfaca apenas o que precisa de decisao do dono:
# link quebrado e lacuna de contrato sao erro e aparecem sempre; orfa e
# trabalho parado sao curadoria e aparecem no maximo uma vez por semana.
# Compilacao do indice falhou no ultimo Stop. Vem ANTES do bloco de saude de
# proposito: enquanto o compilador nao roda, o diagnostico em disco esta vencido
# e nao ha saude a reportar — dizer "tudo certo" a partir dele seria a falha
# silenciosa que o halt no hook do Stop existe para impedir.
#
# Marcador escrito e nunca lido e o padrao mais caro deste sistema: aconteceu com
# os agentes, com os tetos de contexto e com o check de MarkItDown. Este nasceu
# hoje e quase repetiu o padrao — produtor sem consumidor.
# Migração da workspace da versão anterior. Vem antes da saúde porque enquadra
# o que a saúde vai reportar: páginas antigas não carregam o frontmatter que o
# índice exige, e sem contexto o dono atualiza o Maestro e a primeira coisa que
# lê é uma ordem de corrigir centenas de páginas que ele nunca quebrou.
MIGRATION_NOTICE="$BRAIN_DIR/.maestro/.migration-notice"
if [ -f "$MIGRATION_NOTICE" ]; then
  printf '\n## Sua workspace foi migrada para o formato novo\n'
  printf 'Encontrei a pasta `data/` da versão anterior e trouxe o conteúdo para `brain/`.\n'
  printf 'A pasta antiga **não foi tocada** — continua inteira no disco.\n'
  printf 'O que mudou de lugar está em `brain/.maestro/migration-report.md`.\n\n'
  printf 'As páginas que vieram são de antes do formato atual, então o índice ainda\n'
  printf 'não consegue listar todas. Não é erro nem perda: é só formato. Peça "ajusta\n'
  printf 'o formato das páginas antigas" e eu faço numa passada — este aviso some\n'
  printf 'sozinho quando terminar.\n'
fi

INDEX_FAILED="$BRAIN_DIR/.maestro/.index-failed"
if [ -f "$INDEX_FAILED" ]; then
  printf '\n## ⚠️ Índice do brain parou de compilar\n'
  printf 'A última tentativa falhou em %s. Enquanto isso, a navegação, os backlinks,\n' "$(cat "$INDEX_FAILED" 2>/dev/null | head -1)"
  printf 'a visão de tarefas e o roteamento por página estão congelados no último estado válido.\n'
  printf 'Causa mais provável: uma página com caractere que não é UTF-8 válido.\n'
  printf 'Para diagnosticar e recompilar, carregue a skill `brain-index` — ela roda a\n'
  printf 'compilação e traduz o erro. Não peça ao dono para abrir terminal.\n'
fi

HEALTH_MARKER="$BRAIN_DIR/.maestro/.health-requested"
if [ -f "$HEALTH_MARKER" ] && maestro_python >/dev/null 2>&1; then
  PYTHONIOENCODING=utf-8 maestro_py - "$HEALTH_MARKER" <<'PY' 2>/dev/null
import json, sys
try:
    d = json.load(open(sys.argv[1], encoding="utf-8"))
except Exception:
    sys.exit(0)
err = d.get("broken_links", 0) + d.get("contract_gaps", 0)
if d.get("reason") == "erro":
    print("\n## Saúde do brain — corrigir")
    print(f"<!-- maestro:brain-health: reason=erro -->")
    if d.get("broken_links"):
        print(f"- **{d['broken_links']} link(s) quebrado(s)** — apontam para arquivo que não existe.")
    if d.get("contract_gaps"):
        print(f"- **{d['contract_gaps']} página(s) sem frontmatter completo** — não aparecem em índice nenhum.")
    if d.get("dead_paths"):
        print(f"- **{d['dead_paths']} caminho(s) morto(s)** — skill ou hook aponta para pasta que não existe. "
              f"A skill `brain-index` lista quais e explica cada um.")
    print("\nDetalhe em `brain/.maestro/diagnostics.json`. Ofereça corrigir na primeira resposta.")
else:
    print("\n## Saúde do brain — revisão de rotina (não bloqueante)")
    print("<!-- maestro:brain-health: reason=revisao -->")
    parts = []
    if d.get("orphans"):
        parts.append(f"{d['orphans']} página(s) órfã(s) — nenhuma outra página aponta para elas")
    if d.get("stale_open_work"):
        parts.append(f"{d['stale_open_work']} item(ns) aberto(s) sem toque há mais de 60 dias")
    for p in parts:
        print(f"- {p}")
    print("\nNão é erro: pode ser material que ninguém teceu ainda, ou escolha consciente.")
    print("Mencione uma vez, sem insistir. Detalhe em `brain/.maestro/diagnostics.json`.")
PY
  # Marca como visto: reinicia a cadencia semanal da revisao de curadoria.
  : > "$BRAIN_DIR/.maestro/.health-last-seen" 2>/dev/null
fi

# Agentes de sistema vencidos — armados pelo session-stop-agent-check.sh a partir
# dos blocos `auto` de bundles/base/agents/activation-policy.json.
#
# Emite e APAGA o marcador. Nao e descuido: quem controla a insistencia e a
# carencia declarada na politica, nao a permanencia do arquivo. Um marcador que
# sobrevive a emissao vira aviso permanente — o defeito que o
# session-stop-dream.sh documenta e que o dono ja viu uma vez. Se ele ignorar o
# pedido, ele volta no proximo vencimento; nunca na proxima sessao.
AGENT_POLICY="$PROJECT_DIR/bundles/base/agents/activation-policy.json"
if [ -f "$AGENT_POLICY" ] && maestro_python >/dev/null 2>&1; then
  PYTHONIOENCODING=utf-8 maestro_py - "$PROJECT_DIR" "$AGENT_POLICY" <<'PY' 2>/dev/null
import io, json, os, sys

project, policy_p = sys.argv[1], sys.argv[2]
try:
    policy = json.load(io.open(policy_p, encoding="utf-8"))
except (OSError, ValueError):
    sys.exit(0)

for agent in policy.get("agents", []):
    auto = agent.get("auto")
    if not isinstance(auto, dict) or not auto.get("marker"):
        continue
    marker = os.path.join(project, auto["marker"])
    if not os.path.exists(marker):
        continue
    try:
        d = json.load(io.open(marker, encoding="utf-8"))
    except (OSError, ValueError):
        d = {}
    aid = d.get("agent") or agent["id"]
    emoji = d.get("emoji") or agent.get("emoji", "")
    stype = d.get("subagent_type") or agent.get("subagent_type", aid)
    print(f"\n## {emoji} Avaliacao de sistema vencida — `{aid}` (nao bloqueante)")
    print(f"<!-- maestro:agent-due: agent={aid} -->")
    print(f"Motivo: {d.get('reason', 'vencida')}.")
    if d.get("when"):
        print(f"Escopo: {d['when']}")
    if d.get("packet"):
        print("\nPacote fechado a montar (vai inteiro no prompt):")
        for item in d["packet"]:
            print(f"- {item}")
    print(f"\n**Acao sugerida:** despache `{stype}` com a ferramenta Agent quando "
          f"o pedido do dono nao estiver no meio do caminho — anunciando no chat "
          f"antes ({emoji} **{aid} ativado** — motivo) — e reporte so o que muda "
          f"algo para ele. Nao trave o trabalho dele por causa disto.")
    try:
        os.remove(marker)
    except OSError:
        pass
PY
fi

# Profile (highest routing priority — who the user is and how they prefer to work)
emit_profile_json "Identidade do usuário" "$PROFILE_DIR/identity.json"
emit_profile_json "Preferências e estilo" "$PROFILE_DIR/style.json"

# Blocos de memoria — SELF, lifetime, projecao L3, aprendizados e craft.
#
# Os quatro emissores em shell viravam ~350 processos externos (um `sed` por
# arquivo em emit_titles, `basename`+`grep`+`awk` por arquivo em emit_all_files)
# e respondiam por quase todo o custo de 35-39 s deste hook. Agora e um spawn.
# O emissor tambem corrige o `find | sort | tail -1` que devolvia
# `weekly_index.md` no lugar da semana, mede o EMITIDO em vez da origem, e corta
# em bytes em vez de caracteres.
emit_memory_blocks() {
  [ -n "$MEM_EMITTER" ] || return 0
  PYTHONIOENCODING=utf-8 maestro_py "$MEM_EMITTER" \
    --brain "$BRAIN_DIR" --envelope "$ENVELOPE_FILE" \
    --budgets "self=$CAP_self,lifetime=$CAP_lifetime,l3=$CAP_l3,learnings=$CAP_learnings,craft=$CAP_craft,l2=$CAP_l2,l1=$CAP_l1" \
    "$@" 2>/dev/null
}

emit_memory_blocks --blocks self,lifetime,l3,learnings,craft

# Ponteiro para o indice navegavel — nao injeta o indice, so diz onde ele esta.
# Ponteiro para o indice navegavel — nao injeta o indice, so diz onde ele esta.
if [ -f "$BRAIN_DIR/index.md" ]; then
  printf '\n## Indice do brain\n'
  printf 'Catalogo completo com resumo de cada pagina: `brain/index.md`.\n'
  printf 'Cada caso tem indice proprio em `brain/accounts/<conta>/cases/<caso>/index.md`.\n'
  if [ -f "$BRAIN_DIR/tasks/tasks.md" ]; then
    # Le tasks.json em vez de raspar o markdown: o formato da pagina muda, os
    # dados nao. Raspar cabecalho ja quebrou uma vez quando as secoes mudaram.
    TASK_LINE=$(PYTHONIOENCODING=utf-8 maestro_py - "$BRAIN_DIR/.maestro/tasks.json" <<'PY' 2>/dev/null
import json, sys, collections
try:
    t = json.load(open(sys.argv[1], encoding="utf-8"))
except Exception:
    sys.exit(0)
live = [x for x in t if x["status"] != "concluida"]
c = collections.Counter(x["status"] for x in live)
p = collections.Counter(x["priority"] for x in live)
parts = [f"{len(live)} em aberto"]
if c.get("em-andamento"):
    parts.append(f"{c['em-andamento']} em andamento")
for lvl in ("P0", "P1", "P2"):
    if p.get(lvl):
        parts.append(f"{p[lvl]} {lvl}")
blocked = sum(1 for x in live if x.get("blocked_external"))
if blocked:
    parts.append(f"{blocked} dependem de terceiro")
print(" · ".join(parts))
PY
)
    if [ -n "$TASK_LINE" ]; then
      printf 'Tarefas: %s — visao completa em `brain/tasks/tasks.md`.\n' "$TASK_LINE"
    fi
  fi
fi

# L2 (sintese da semana) e L1 (ultimo log diario consolidado). O mesmo emissor,
# segunda chamada: `--finalize` fecha o envelope com o que os dois lotes mediram.
emit_memory_blocks --blocks l2,l1 \
  --finalize --out "$BRAIN_DIR/.maestro/context-envelope.json" --cap-total "$CAP_total"

printf '\n<!-- maestro:session-context:end -->\n'

# O envelope e escrito pelo proprio emissor (--finalize acima): medir num
# lugar e publicar em outro foi o que fez o arquivo reportar o tamanho da
# origem sob o nome do que teria sido injetado.
[ -n "$ENVELOPE_FILE" ] && rm -f "$ENVELOPE_FILE" 2>/dev/null

exit 0
