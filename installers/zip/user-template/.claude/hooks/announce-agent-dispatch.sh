#!/usr/bin/env bash
# Maestro PreToolUse — visibilidade do despacho de agente.
#
# Marcelo, 2026-09-06: "toda vez que o hub disparar um agente, gostaria que ele
# mandasse no chat para o usuario saber que determinado agente foi ativado".
#
# **O que este hook nao faz, e e importante nao mentir sobre isso.** Ele nao
# escreve no chat. Nenhum hook escreve: o texto que o dono le vem do hub, e so
# do hub. O que este hook faz e devolver a exigencia AO HUB — nao por lembranca,
# nao por uma linha de skill lida ha vinte turnos.
#
# **Correcao de 2026-09-06, mesma tarde.** A primeira versao imprimia texto puro
# em stdout e saia 0. Isso NAO chega ao modelo: em PreToolUse, stdout com exit 0
# vai para o transcript, e o unico canal de volta e um objeto JSON (ou stderr com
# exit 2, que aqui bloquearia o despacho e e a correcao errada). O hook rodava,
# imprimia e falava para o vazio — com teste verde, porque o teste capturava o
# stdout do proprio hook e confirmava que o texto EXISTIA, nunca que CHEGAVA.
# Achado do darwin na primeira execucao dele, confirmado ao vivo: um despacho
# real nao trouxe uma linha sequer ao contexto do hub. Agora sai
# `hookSpecificOutput.additionalContext`, e o teste exige JSON parseavel com o
# campo — nao texto solto.
#
# **E uma correcao de honestidade junto.** PreToolUse dispara DEPOIS que o hub
# ja compos a chamada, entao este hook nao pode garantir anuncio "antes" coisa
# nenhuma: naquela rodada a mensagem ja saiu. Ele e REDE, nao portao — pega o
# anuncio que faltou e cobra na resposta da mesma rodada, junto do resultado.
# Quem cobra de verdade ANTES e o agent-route.py, e so quando o despacho nasceu
# de uma mensagem do dono.
#
# Duas coberturas, com buracos complementares:
#   - agent-route.py cobra ANTES, mas so quando a mensagem do dono casou os
#     gatilhos da politica;
#   - este hook cobra NO ATO, inclusive quando o despacho nasceu de um marcador
#     periodico, de um portao ou de decisao propria do hub — casos em que o
#     roteador nunca chegou a falar — mas so depois da chamada composta.
#
# A regra e o formato vivem em bundles/base/agents/activation-policy.json, no
# bloco `announce`. Este hook nao tem texto proprio: se a politica mudar o
# formato, muda aqui junto. Uma fonte.
#
# Fail-open e silencioso: sem interpretador resolvivel, sem politica, ou payload
# que nao parseia,
# sai 0 sem bloquear o despacho. Visibilidade e desejavel; travar o trabalho do
# dono por causa dela nao e.

set +e

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
POLICY="$PROJECT_DIR/bundles/base/agents/activation-policy.json"

HOOK_INPUT=$(cat 2>/dev/null)
[ -z "$HOOK_INPUT" ] && exit 0
[ -f "$POLICY" ] || exit 0
# shellcheck source=lib/python.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/python.sh" 2>/dev/null || true
maestro_python >/dev/null 2>&1 || exit 0

# Caminho rapido: se o payload nao menciona subagent_type, nao e despacho de
# agente e nao ha o que anunciar. Evita abrir python em toda chamada de tool.
case "$HOOK_INPUT" in
  *subagent_type*) ;;
  *) exit 0 ;;
esac

# O parser vem por heredoc CITADO e e usado por expansao simples de variavel.
# Nao e estilo: dentro de `maestro_py -c "..."` o bash reinterpreta o conteudo, e
# um par de crases ou de aspas num comentario em portugues ja quebrou o hook
# irmao duas vezes. Com heredoc citado nao ha o que escapar.
read -r -d '' ANNOUNCE_PY <<'PY'
import io, json, sys

try:
    payload = json.load(sys.stdin)
except Exception:
    sys.exit(0)

tool = payload.get("tool_name", "")
# A ferramenta de subagente ja se chamou Task e hoje se chama Agent. Aceitar as
# duas custa nada e evita que o anuncio suma numa renomeacao do runtime.
if tool not in ("Agent", "Task"):
    sys.exit(0)

inp = payload.get("tool_input") or {}
stype = inp.get("subagent_type") or ""
if not isinstance(stype, str) or not stype:
    sys.exit(0)

policy_path = sys.argv[1]
try:
    policy = json.load(io.open(policy_path, encoding="utf-8"))
except Exception:
    sys.exit(0)

announce = policy.get("announce") or {}
if not announce.get("required"):
    sys.exit(0)

agent = next((a for a in policy.get("agents", [])
              if a.get("subagent_type") == stype or a.get("id") == stype), None)


def emit(text):
    """Devolve ao hub pelo unico canal que ele le em PreToolUse.

    Texto puro em stdout aqui vai para o transcript e nao chega ao modelo. O
    objeto JSON chega. `permissionDecision` fica de fora de proposito: este hook
    nunca decide permissao, so acrescenta contexto — declarar `allow` aqui
    aprovaria silenciosamente todo despacho de agente, o que ninguem pediu.
    """
    print(json.dumps({
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "additionalContext": text,
        }
    }, ensure_ascii=False))


rule = announce.get("rule", "")

# Subagente que a politica do Maestro nao declara (os nativos do Claude Code,
# por exemplo). Nao inventa identidade: cobra o anuncio com o nome cru.
if agent is None:
    emit(f"[maestro] Voce despachou o subagente `{stype}`, que nao esta na "
         f"politica de agentes do Maestro ({policy_path}). {rule} "
         f"Se ainda nao anunciou, diga na resposta desta rodada qual subagente "
         f"foi ativado e por que.")
    sys.exit(0)

emoji = agent.get("emoji", "")
display = agent.get("id", stype)

if agent.get("status") != "active" or not agent.get("dispatchable"):
    # Dormente ou nao despachavel: isto e erro de rota, nao pedido de anuncio.
    reason = agent.get("dormant_reason") or agent.get("never") or ""
    emit(f"[maestro] ATENCAO: `{display}` esta declarado como NAO despachavel na "
         f"politica de agentes. {reason} Reveja se este despacho deveria ter "
         f"acontecido antes de usar o resultado, e diga isso ao dono.")
    sys.exit(0)

# Formato ja substituido: o hub recebe a linha pronta, nao um template. A
# primeira versao mandava `{emoji} **{display} ativado**` literal, com o agente
# resolvido em maos duas linhas acima.
fmt = announce.get("format") or "{emoji} **{display} ativado** — {motivo}."
linha = fmt.replace("{emoji}", emoji).replace("{display}", display)

partes = [f"[maestro] Despacho de {emoji} `{display}` em curso. {rule}",
          f"Anuncie assim, se ainda nao anunciou: {linha}"]
on_return = announce.get("on_return")
if on_return:
    partes.append(f"Na volta: {on_return}")
emit(" ".join(partes))
PY

printf '%s' "$HOOK_INPUT" | PYTHONIOENCODING=utf-8 maestro_py -c "$ANNOUNCE_PY" "$POLICY" 2>/dev/null

exit 0
