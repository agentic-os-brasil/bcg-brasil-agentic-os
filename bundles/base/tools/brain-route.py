#!/usr/bin/env python3
"""Roteador de intencao do brain.

Le o texto do dono e devolve as paginas do brain que provavelmente importam,
como titulo + resumo + caminho. Nao le nenhuma pagina: tudo vem do
route-index.json que o compilador produziu.

Escopo: sempre o do dono, mais o caso ativo em brain/accounts/.active.
Nunca um caso que nao seja o ativo — o isolamento vale aqui como vale na escrita.

Uso:  echo '<texto>' | python3 bundles/base/tools/brain-route.py [--max N]
      python3 bundles/base/tools/brain-route.py --max 5 "texto"
"""
import io, os, re, sys, json, unicodedata

BRAIN = "brain"
ROUTE = os.path.join(BRAIN, ".maestro", "route-index.json")
ACTIVE = os.path.join(BRAIN, "accounts", ".active")

# Stoplist. Compartilha com o skill-route as palavras de cola genericas, e
# diverge NELE de proposito em um ponto: `dia`, `dias`, `semana`, `caso`,
# `fonte`, `nota`, `owner` e `dono` NAO entram aqui. Numa base cujo assunto e
# caso, fonte e semana, descartar essas e apagar a pergunta — "ir a fonte
# primaria" perdia metade dos termos. No corpus de skills elas sao cola; aqui
# sao conteudo. A assimetria e a decisao, nao o esquecimento.
STOP = set("""de da do das dos e o a os as um uma para por com em no na nos nas que se ao
aos sem sobre entre como mais menos ja nao ou eh foi ser sao era pelo pela ate apos antes
cada todo toda todos todas isso isto esse essa este esta seu sua qual quais quando onde
porque tem ter fica vai ir ver ainda sempre tudo horas hora coisa jeito nada algo
alguem the and for with from that this into when what which your you are was were
apenas tambem muito bem mesmo outro outra novo nova entao assim depois hoje ontem quero
preciso fazer faz pode posso vamos vou gostaria sobre qual quero""".split())


# Peso por tipo. Uma pergunta de trabalho quer canon, decisao e tarefa — nao o
# daily que mencionou o assunto de passagem, nem o perfil de um colega que
# trabalhou no caso. Sem isso o ranking premia quem repete o termo, nao quem
# responde.
TYPE_WEIGHT = {
    "data": 1.6, "framework": 1.6, "hypothesis": 1.6, "benchmark": 1.6,
    "interview": 1.4, "canon": 1.5, "decision-log": 1.5, "task-list": 1.5,
    "project-brief": 1.4, "deliverable": 1.4, "source": 1.1,
    "learning": 1.4, "craft-method": 1.4, "craft-style": 1.3,
    "memory-lifetime": 1.2, "memory-l3": 1.1, "memory-l2": 0.9,
    "owner-facet": 0.9, "operating": 0.9, "objectives": 1.0,
    "index": 0.8, "account": 0.8,
    "daily": 0.5, "memory-l1": 0.5, "person": 0.4,
}

TERM_RE = re.compile(r"[A-Za-zÀ-ÿ][A-Za-zÀ-ÿ0-9\-]{2,}")


def fold(w):
    return unicodedata.normalize("NFKD", w).encode("ascii", "ignore").decode().lower()


STEM_AT = 5
SHORT_OK = {"tmo", "bcg", "pdf", "eod", "cdc", "ipa", "fpa", "dre", "kpi", "roi",
            "csv", "xls", "ppt", "sql", "api", "qa", "pr"}


def stem(w):
    """Prefixo de 5 letras — o mesmo stemmer do skill-route.

    Os dois roteadores foram escritos como "mesmo mecanismo, outro corpo" e
    divergiram exatamente aqui: um radicalizava e o outro nao. O efeito era o
    dono escrever "como eu classifico spend" e nao achar
    `classificacao-spend-por-razao-social`, porque `classifico` nao e
    `classificacao`. Aplicado no indice (brain-index.py) e aqui, ou nao casa
    nada.
    """
    return w[:STEM_AT]


def query_terms(text):
    words = [fold(w) for w in TERM_RE.findall(text)]
    uni = {stem(w) for w in words
           if w not in STOP and (len(w) >= 4 or w in SHORT_OK)}
    bi = set()
    for i in range(len(words) - 1):
        if words[i] in STOP or words[i + 1] in STOP:
            continue
        bi.add(f"{stem(words[i])} {stem(words[i + 1])}")
    return uni, bi


def active_scope():
    try:
        v = io.open(ACTIVE, encoding="utf-8").read().strip()
        return v if v else None
    except OSError:
        return None


def main():
    argv = sys.argv[1:]
    top_n = 5
    if "--max" in argv:
        i = argv.index("--max")
        # `--max abc` e `--max` sem valor davam traceback. Argumento invalido
        # cai no default: um roteador e uma sugestao, nao deve derrubar nada.
        try:
            top_n = max(1, int(argv[i + 1]))
        except (IndexError, ValueError):
            pass
        del argv[i:i + 2]
    text = " ".join(argv) if argv else sys.stdin.read()
    if not text.strip():
        return 0
    if not os.path.exists(ROUTE):
        return 0

    # Indice corrompido nao pode derrubar o roteamento. `brain-index.py` grava
    # 2,1 MB de route-index sem atomicidade; um Stop interrompido ou um lock do
    # OneDrive deixa o arquivo truncado, e o traceback resultante era engolido
    # pelo `2>/dev/null` do hook — o roteamento de paginas sumia sem uma linha de
    # aviso. O irmao skill-route ja tratava assim.
    try:
        routes = json.load(io.open(ROUTE, encoding="utf-8"))
    except (OSError, ValueError):
        return 0
    if not isinstance(routes, dict):
        return 0
    scopes = ["owner"]
    act = active_scope()
    if act and act in routes:
        scopes.append(act)

    uni, bi = query_terms(text)
    if not uni and not bi:
        return 0

    # Peso por raridade, que o irmao skill-route tem desde sempre e este nao
    # tinha. Sem ele, um termo que aparece em meia arvore valia o mesmo que um
    # que aparece numa pagina so — e como o piso exigia dois termos casados, uma
    # pergunta com UM termo raro e exato ("credencial") voltava vazia enquanto a
    # pagina certa existia. Raridade e o sinal mais forte que uma consulta curta
    # tem; ignorá-lo era jogar fora justamente o caso facil.
    scores, meta = {}, {}
    matched, bigram_hit = {}, {}
    for sc in scopes:
        idx = routes.get(sc)
        if not idx:
            continue
        terms, pages = idx["terms"], idx["pages"]
        n = max(len(pages), 1)
        for t in bi:                      # bigrama vale mais: e especifico
            hits = terms.get(t, [])
            idf = 1.0 + (n / max(len(hits), 1)) ** 0.5
            for p in hits:
                scores[p] = scores.get(p, 0) + 3 * idf
                matched[p] = matched.get(p, 0) + 1
                bigram_hit[p] = True
        for t in uni:
            hits = terms.get(t, [])
            idf = 1.0 + (n / max(len(hits), 1)) ** 0.5
            for p in hits:
                scores[p] = scores.get(p, 0) + 1 * idf
                matched[p] = matched.get(p, 0) + 1
        meta.update(pages)

    if not scores:
        return 0

    # Peso do tipo antes de cortar, e corte relativo ao topo em vez de absoluto.
    #
    # O piso fixo de 2 pontos era intransponivel para um termo unico: um unigrama
    # valia 1 e o maior TYPE_WEIGHT e 1,6, entao NENHUMA pagina casada por um so
    # termo aparecia, por mais raro e exato que ele fosse. Com o idf, um termo
    # que existe numa pagina so ja pontua alto sozinho — que e o comportamento
    # certo — e o corte relativo separa o que se destaca do que so encostou.
    # Dois termos casados, ou um bigrama. E a mesma regra do skill-route, pelo
    # mesmo motivo medido: com o idf ligado, UM termo raro pontua alto sozinho, e
    # ai "qual a capital da Franca" acha pagina porque `capital` aparece uma vez
    # em algum lugar. Exigir duas evidencias derruba o ruido a zero sem custar
    # recall — as perguntas legitimas do conjunto de referencia trazem duas.
    scores = {p: v for p, v in scores.items()
              if matched.get(p, 0) >= 2 or bigram_hit.get(p)}
    scores = {p: s * TYPE_WEIGHT.get(meta.get(p, {}).get("type", ""), 1.0)
              for p, s in scores.items()}
    if not scores:
        return 0
    top = max(scores.values())
    ranked = [(p, s) for p, s in scores.items() if s >= max(3.0, top * 0.45)]
    if not ranked:
        return 0
    ranked.sort(key=lambda x: (-x[1], x[0]))

    print("<!-- maestro:route -->")
    print("Páginas do brain que provavelmente importam para esta mensagem:")
    for p, s in ranked[:top_n]:
        m = meta.get(p, {})
        title = m.get("title") or p
        summary = (m.get("summary") or "")[:110]
        flag = " [encerrado]" if m.get("status") == "closed" else ""
        print(f"- `brain/{p}`{flag} — **{title}** — {summary}")
    print("Leia sob demanda. O índice completo está em `brain/brain_index.md`.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
