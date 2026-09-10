#!/usr/bin/env python3
"""Roteador de ativacao de agentes.

Terceiro irmao de brain-route.py e skill-route.py. Os dois primeiros respondem
"que pagina importa" e "que skill resolve". Este responde a pergunta que nao
tinha mecanismo nenhum: **que agente o hub deve despachar, e com que pacote**.

Por que existe. Ate 2026-09-06 os quatro spokes eram despachaveis e nenhum
caminho automatico levava a eles. Medido ao vivo: "yoda check nesse deck"
devolvia a *skill* `yoda`; "como anda a saude do sistema" devolvia
`maestro-doctor` e `brain-index`, nunca `darwin`. O unico mecanismo por mensagem
que existia era cego para a camada de spoke, e a regra de quando chamar cada um
vivia em prosa numa skill que depende de alguem lembrar de abrir. Isso e
exatamente o padrao que `brain/learnings/especificacao-nunca-lida-e-padrao.md`
nomeia.

Fonte unica: bundles/base/agents/activation-policy.json. Nao ha corpus paralelo
para manter em sincronia — os gatilhos, o pacote exigido e o status de cada
agente saem de la e de nenhum outro lugar. Agente com `status` diferente de
`active` ou com `dispatchable: false` nunca e roteado.

Conservador de proposito. Um falso positivo aqui custa uma chamada de modelo
inteira, nao uma linha de texto: o piso de evidencia e o mesmo medido para o
roteador de skills (dois termos, ou um bigrama), e o corte relativo ao topo e
mais duro.

Limite conhecido e aceito, nao defeito a corrigir em silencio. Pedido curto cujo
unico termo distintivo e uma palavra so cai abaixo do piso: "o que esta inerte no
maestro" casa apenas `inert` — "que", "esta" e "no" sao palavras de cola e
`maestro` esta na lista de descarte do irmao, porque toda skill do sistema o
menciona. Baixar o piso para um termo resolveria essa frase e quebraria mais do
que resolve: `deck` e `cliente` tambem sao termo unico de yoda, e "monta o deck
do steerco" passaria a pedir revisao senior. A frase mais longa funciona ("tem
deriva entre o especificado e o que roda", "quero uma auditoria do proprio
sistema"), e o caminho de tras continua inteiro: o pedido ainda roteia por skill,
e a tabela de spokes do maestro-operator continua nomeando darwin. Este roteador
e acelerador, nao a unica porta.

Uso:  echo '<texto>' | python3 bundles/base/tools/agent-route.py [--max N]
      python3 bundles/base/tools/agent-route.py --max 2 "texto"
      python3 bundles/base/tools/agent-route.py --list    # o que a politica declara
"""
import io
import os
import re
import sys
import json
import importlib.util
import unicodedata

HERE = os.path.dirname(os.path.abspath(__file__))
POLICY = os.path.join("bundles", "base", "agents", "activation-policy.json")

# A saida carrega emoji (🧙 🧬 🧪) e acento. No Windows o console entrega cp1252
# e o print morre com UnicodeEncodeError na primeira linha — o hook fica sem
# rota e ninguem ve o erro, porque hook falha em silencio. Os irmaos deste tool
# resolvem isso exigindo PYTHONIOENCODING=utf-8 de quem chama; isso e um
# contrato que depende de lembranca, e a lembranca e o que falha. Aqui a
# codificacao e responsabilidade do proprio tool.
for _s in (sys.stdout, sys.stderr):
    try:
        _s.reconfigure(encoding="utf-8", errors="replace")
    except (AttributeError, ValueError):
        pass


# --- helpers de casamento ----------------------------------------------------
# Emprestados de skill-route.py em vez de recopiados. Duplicar o stemmer criaria
# duas definicoes de "o que casa" que divergem em silencio — o mesmo defeito que
# esta correcao existe para fechar. Se o irmao sumir, o fallback local mantem o
# roteador vivo com a mesma semantica.
def _load_sibling():
    path = os.path.join(HERE, "skill-route.py")
    try:
        spec = importlib.util.spec_from_file_location("_skill_route", path)
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        return mod.fold, mod.stem, mod.terms_of
    except Exception:
        return None


_sib = _load_sibling()

if _sib:
    fold, stem, terms_of = _sib
else:                                          # fallback: mesma semantica, local
    _STOP = set("""de da do das dos e o a os as um uma para por com em no na nos nas que se
ao aos sem sobre entre como mais menos ja nao ou eh foi ser sao era pelo pela ate apos antes
cada todo toda todos todas isso isto esse essa este esta seu sua qual quais quando onde
porque tem ter fica vai ir ver quero preciso fazer faz pode posso vamos vou gostaria
the and for with from that this into only never always use used using when what which
your you are was were has have had not but any all one two its their there here""".split())
    _TERM_RE = re.compile(r"[A-Za-zÀ-ÿ][A-Za-zÀ-ÿ0-9\-]{2,}")

    def fold(w):
        return unicodedata.normalize("NFKD", w).encode("ascii", "ignore").decode().lower()

    def stem(w):
        return w[:5]

    def terms_of(text):
        words = [fold(w) for w in _TERM_RE.findall(text or "")]
        uni = {stem(w) for w in words if w not in _STOP and len(w) >= 4}
        bi = set()
        for i in range(len(words) - 1):
            if words[i] in _STOP or words[i + 1] in _STOP:
                continue
            bi.add(f"{stem(words[i])} {stem(words[i + 1])}")
        return uni, bi


# Peso por origem. `triggers` e a lista curada de como o pedido real soa — e o
# unico corpus escrito contra o pedido, e nao contra o nome do agente. `when` e
# a regra em prosa: vale, mas menos. O id vale porque o dono as vezes digita o
# nome ("chama o darwin").
FIELD_WEIGHT = {"trigger": 4.0, "name": 3.0, "when": 1.5}


def load_policy(path=POLICY):
    try:
        return json.load(io.open(path, encoding="utf-8"))
    except (OSError, ValueError):
        return None


def routable(policy):
    """Agentes que o hub pode de fato despachar agora.

    Tres filtros, nesta ordem: camada spoke (hub e instancia nao se despacham),
    status active (dormente fica no disco e fora do roteamento) e dispatchable.
    """
    out = []
    for a in (policy or {}).get("agents", []):
        if a.get("layer") != "spoke":
            continue
        if a.get("status") != "active" or not a.get("dispatchable"):
            continue
        out.append(a)
    return out


def index(policy):
    """termo -> {agent_id: peso}, mais os metadados que o hook imprime."""
    agents, per_agent = {}, {}
    for a in routable(policy):
        aid = a["id"]
        agents[aid] = {
            "emoji": a.get("emoji", ""),
            "subagent_type": a.get("subagent_type", aid),
            "when": a.get("when", ""),
            "never": a.get("never", ""),
            "packet": a.get("packet", []),
            "gate": a.get("gate", ""),
        }
        weighted = {}
        fields = [("name", aid.replace("-", " ")), ("when", a.get("when", ""))]
        fields += [("trigger", t) for t in a.get("triggers", [])]
        for field, text in fields:
            w = FIELD_WEIGHT[field]
            uni, bi = terms_of(text)
            for t in uni:
                weighted[t] = max(weighted.get(t, 0.0), w)
            for t in bi:                       # bigrama e especifico: vale o triplo
                weighted[t] = max(weighted.get(t, 0.0), w * 3)
        per_agent[aid] = weighted

    inverted = {}
    for aid, weighted in per_agent.items():
        for t, w in weighted.items():
            inverted.setdefault(t, {})[aid] = round(w, 3)
    return {"agents": agents, "terms": inverted}


def route(idx, text, top_n=2):
    # Nome exato do agente, sozinho: "darwin", "yoda". O pedido mais literal
    # possivel e o que o filtro de termo mais facilmente descartaria.
    exact = fold(text.strip().lstrip("$/@")).strip()
    if exact in idx["agents"]:
        return [(exact, 999.0)]

    uni, bi = terms_of(text)
    if not uni and not bi:
        return []
    terms, n = idx["terms"], max(len(idx["agents"]), 1)
    scores, matched, had_bigram = {}, {}, {}
    for t in list(bi) + list(uni):
        hits = terms.get(t)
        if not hits:
            continue
        idf = 1.0 + (n / max(len(hits), 1)) ** 0.5
        for aid, w in hits.items():
            scores[aid] = scores.get(aid, 0.0) + w * idf
            matched[aid] = matched.get(aid, 0) + 1
            if t in bi:
                had_bigram[aid] = True

    # Mesmo piso de evidencia medido para o roteador de skills: um termo casado
    # nao decide nada. Sem a terceira saida do irmao ("termo que recorre pela
    # biblioteca") de proposito — com quatro agentes, um termo comum a dois
    # deles nao e vocabulario de dominio, e ambiguidade.
    scores = {a: v for a, v in scores.items()
              if matched.get(a, 0) >= 2 or had_bigram.get(a)}

    ranked = sorted(scores.items(), key=lambda kv: (-kv[1], kv[0]))
    if not ranked:
        return []
    top = ranked[0][1]
    # Corte mais duro que o de skills (0.35): despachar um spoke custa uma
    # chamada de modelo inteira. Melhor devolver um certo que dois por educacao.
    return [(a, sc) for a, sc in ranked[:top_n] if sc >= max(12.0, top * 0.6)]


def emit(idx, hits, announce=None):
    print("<!-- maestro:agent-route -->")
    print("Agente(s) que este pedido provavelmente exige:")
    for aid, _ in hits:
        m = idx["agents"][aid]
        print(f"- **{m['emoji']} `{aid}`** — {m['when']}")
        if m["gate"]:
            print(f"  - Portao: passar por `{m['gate']}` antes.")
        if m["packet"]:
            print("  - Pacote fechado (vai inteiro no prompt): "
                  + "; ".join(m["packet"]) + ".")
        print(f"  - Despachar com a ferramenta Agent, `subagent_type: \"{m['subagent_type']}\"`.")
        # Anuncio no chat, com o formato JA SUBSTITUIDO. A primeira versao
        # imprimia `{emoji} **{display} ativado**` literal, uma vez para todos os
        # acertos, com o agente resolvido em maos na mesma linha — o hub recebia
        # um template para preencher em vez de uma linha pronta. Achado do darwin
        # na primeira execucao dele.
        if announce and announce.get("required"):
            fmt = announce.get("format") or ""
            linha = fmt.replace("{emoji}", m["emoji"]).replace("{display}", aid)
            print(f"  - **Anuncie no chat ANTES de despachar**, assim: {linha}")
    print("Regra: o spoke nao busca contexto e o veredito dele e insumo, nao saida.")
    if announce and announce.get("required"):
        print("O anuncio e pedido explicito do dono e a excecao declarada a nao "
              "surfacar mecanica interna. Este e o unico ponto que cobra ANTES da "
              "chamada; o hook de PreToolUse e rede, e so fala depois.")
    print("Se o pedido nao for de alta alavancagem nem de sistema, nao despache — a decisao de pular e sua, com evidência.")


def main():
    argv = sys.argv[1:]
    top_n = 2
    if "--list" in argv:
        pol = load_policy()
        if not pol:
            return 0
        for a in (pol.get("agents") or []):
            flag = "despachavel" if a.get("dispatchable") else "nao despachavel"
            print(f"{a['id']:<22} {a.get('layer',''):<9} {a.get('status',''):<8} {flag}")
        return 0
    if "--max" in argv:
        i = argv.index("--max")
        try:
            top_n = max(1, int(argv[i + 1]))
        except (IndexError, ValueError):
            pass
        del argv[i:i + 2]

    policy = load_policy()
    if not policy:
        return 0                               # fail-open: sem politica, sem rota
    idx = index(policy)
    if not idx["agents"]:
        return 0

    text = " ".join(argv) if argv else sys.stdin.read()
    if not text.strip():
        return 0
    hits = route(idx, text, top_n)
    if not hits:
        return 0
    emit(idx, hits, policy.get("announce"))
    return 0


if __name__ == "__main__":
    sys.exit(main())
