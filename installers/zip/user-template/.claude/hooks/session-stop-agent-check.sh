#!/usr/bin/env bash
# Maestro Stop hook — gatilho automatico dos agentes de sistema.
#
# Marcelo, 2026-09-06: "darwin e gamma deveriam ser ativados automaticamente".
# Ate aqui os dois eram despachaveis e nada os disparava: dependiam de alguem
# lembrar de pedir. O aprendizado do dono
# (brain/learnings/especificacao-nunca-lida-e-padrao.md) diz o que acontece com
# regra sem mecanismo — foi assim que o subsistema de agentes ficou inerte de
# 16/08 a 05/09 sem ninguem notar.
#
# O que faz: le os blocos `auto` de bundles/base/agents/activation-policy.json e,
# quando um agente esta vencido, escreve o marcador que o SessionStart seguinte
# surfaca. A politica e a fonte — cadencia, condicao e caminho do marcador saem
# de la. Este hook nao tem nenhuma regra propria sobre QUANDO chamar cada
# agente; ele so sabe COMO medir as condicoes que a politica declara.
#
# Duas cadencias, porque as perguntas sao diferentes:
#   - periodic  (darwin) — deriva entre o especificado e o que roda nao se
#     anuncia. Vence por tempo, e antecipa quando o diagnostico do brain ja
#     acusa erro.
#   - on_change (gamma)  — qualidade longitudinal so significa algo medida em
#     serie. Vence por tempo E por codigo de produto ter mudado desde o ultimo
#     pedido; sem mudanca nao ha o que reavaliar.
#
# Anti-insistencia: o marcador e escrito no maximo uma vez por periodo de
# carencia, e o SessionStart o consome e apaga ao emitir. Se o dono ignorar, o
# pedido volta no proximo vencimento — nunca em toda sessao. Este e exatamente o
# defeito que o session-stop-dream.sh documenta no proprio cabecalho: um
# marcador reescrito sem condicao transforma o canal de "isto vem antes de tudo"
# num aviso permanente que o dono aprende a ignorar.
#
# Fail-open e silencioso: qualquer erro sai 0 sem bloquear o encerramento.

set +e

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
BRAIN_DIR="$PROJECT_DIR/brain"
MACHINE="$BRAIN_DIR/.maestro"
POLICY="$PROJECT_DIR/bundles/base/agents/activation-policy.json"
RUNS="$MACHINE/agent-runs.json"
DIAG="$MACHINE/diagnostics.json"

[ -d "$BRAIN_DIR" ] || exit 0
[ -f "$POLICY" ] || exit 0
# shellcheck source=lib/python.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/python.sh" 2>/dev/null || true
maestro_python >/dev/null 2>&1 || exit 0

mkdir -p "$MACHINE" 2>/dev/null

PYTHONIOENCODING=utf-8 maestro_py - "$PROJECT_DIR" "$POLICY" "$RUNS" "$DIAG" <<'PY' 2>/dev/null
import datetime, glob, io, json, os, sys

project, policy_p, runs_p, diag_p = sys.argv[1:5]
now = datetime.datetime.now(datetime.timezone.utc)


def read_json(path, default=None):
    try:
        return json.load(io.open(path, encoding="utf-8"))
    except (OSError, ValueError):
        return default


def parse_ts(s):
    try:
        return datetime.datetime.fromisoformat((s or "").replace("Z", "+00:00"))
    except (ValueError, AttributeError):
        return None


policy = read_json(policy_p)
if not isinstance(policy, dict):
    sys.exit(0)

runs = read_json(runs_p, {})
if not isinstance(runs, dict):
    runs = {}

# Erro de navegacao no brain antecipa Darwin: link quebrado, lacuna de contrato
# e caminho morto sao exatamente a classe de deriva que ele existe para achar.
diag = read_json(diag_p, {}) or {}


def diag_errors():
    n = 0
    for k in ("broken_links", "contract_gaps", "dead_paths"):
        v = diag.get(k)
        n += len(v) if isinstance(v, list) else (v or 0)
    return n


def newest_mtime(patterns):
    newest = 0.0
    for pat in patterns or []:
        for p in glob.glob(os.path.join(project, pat)):
            try:
                newest = max(newest, os.path.getmtime(p))
            except OSError:
                pass
    return newest


def age_days(stamp):
    t = parse_ts(stamp)
    if not t:
        return None                       # nunca pedido: vence na primeira vez
    return (now - t).total_seconds() / 86400.0


written = []
for agent in policy.get("agents", []):
    auto = agent.get("auto")
    if not isinstance(auto, dict):
        continue
    if agent.get("status") != "active" or not agent.get("dispatchable"):
        continue                          # dormente nunca e armado

    aid = agent["id"]
    marker_p = os.path.join(project, auto.get("marker", ""))
    if not auto.get("marker"):
        continue
    if os.path.exists(marker_p):
        continue                          # ja pendente: nao reescreve

    cooldown = float(auto.get("cooldown_days") or 7)
    prior = (runs.get(aid) or {}).get("last_requested")
    age = age_days(prior)

    reason = None
    if auto.get("kind") == "periodic":
        if age is None:
            reason = "primeira avaliacao — nunca foi pedida"
        elif age >= cooldown:
            reason = f"vencida — {int(age)} dia(s) desde o ultimo pedido (carencia {int(cooldown)})"
        elif diag_errors() and age >= 1:
            reason = (f"antecipada — o diagnostico do brain acusa {diag_errors()} "
                      f"erro(s) de navegacao")
    elif auto.get("kind") == "on_change":
        if age is not None and age < cooldown:
            pass                          # dentro da carencia: nao mede mudanca
        else:
            newest = newest_mtime(auto.get("watch"))
            prior_t = parse_ts(prior)
            changed = newest > (prior_t.timestamp() if prior_t else 0.0)
            if age is None:
                reason = "primeira avaliacao — nunca foi pedida"
            elif changed:
                stamp = datetime.datetime.fromtimestamp(
                    newest, datetime.timezone.utc).strftime("%Y-%m-%d")
                reason = (f"codigo de produto mudou desde o ultimo pedido "
                          f"(alteracao mais recente: {stamp})")
            # sem mudanca: nada a reavaliar, e o silencio e a resposta certa

    if not reason:
        continue

    payload = {
        "agent": aid,
        "emoji": agent.get("emoji", ""),
        "subagent_type": agent.get("subagent_type", aid),
        "requested_at": now.strftime("%Y-%m-%dT%H:%M:%SZ"),
        "reason": reason,
        "when": agent.get("when", ""),
        "packet": agent.get("packet", []),
        "policy": "bundles/base/agents/activation-policy.json",
    }
    try:
        os.makedirs(os.path.dirname(marker_p), exist_ok=True)
        tmp = marker_p + ".tmp"
        io.open(tmp, "w", encoding="utf-8", newline="\n").write(
            json.dumps(payload, ensure_ascii=False, indent=2) + "\n")
        os.replace(tmp, marker_p)         # publicacao atomica, como no resto
    except OSError:
        continue

    runs.setdefault(aid, {})["last_requested"] = payload["requested_at"]
    runs[aid]["last_reason"] = reason
    written.append(aid)

if written:
    runs["schema_version"] = 1
    try:
        tmp = runs_p + ".tmp"
        io.open(tmp, "w", encoding="utf-8", newline="\n").write(
            json.dumps(runs, ensure_ascii=False, indent=2) + "\n")
        os.replace(tmp, runs_p)
    except OSError:
        pass
PY

exit 0
