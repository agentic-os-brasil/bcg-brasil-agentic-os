#!/usr/bin/env python3
"""Roteador de intencao das skills.

Irmao de brain-route.py: mesmo mecanismo, outro corpo. Le o texto do dono e
devolve as skills que provavelmente resolvem o pedido, em vez das 56 injetadas
sem criterio no inicio de toda sessao.

Corpus por skill, bilingue de proposito. O dono escreve em portugues; a maioria
das descricoes de SKILL.md esta em ingles. Casar "fecha o dia" contra "Close the
working day" nao acontece. Os catalogos (base e tech-core) carregam
display_name, trigger e default_prompt em portugues — sao a metade que de fato
casa, e por isso pesam mais.

Indice cacheado em brain/.maestro/skill-route-index.json, invalidado pelo mtime
mais novo entre os SKILL.md e os catalogos. Sem hook para manter: se uma skill
muda, o proximo roteamento reconstroi sozinho.

Uso:  echo '<texto>' | python3 bundles/base/tools/skill-route.py [--max N]
      python3 bundles/base/tools/skill-route.py --max 5 "texto"
      python3 bundles/base/tools/skill-route.py --names   # so os nomes, para o rollup
      python3 bundles/base/tools/skill-route.py --rebuild
"""
import io, os, re, sys, json, glob, unicodedata

CACHE = os.path.join("brain", ".maestro", "skill-route-index.json")
BUNDLES = "bundles"

STOP = set("""de da do das dos e o a os as um uma para por com em no na nos nas que se ao
aos sem sobre entre como mais menos ja nao ou eh foi ser sao era pelo pela ate apos antes
cada todo toda todos todas isso isto esse essa este esta seu sua qual quais quando onde
porque tem ter fica vai ir ver quero preciso fazer faz pode posso vamos vou gostaria
the and for with from that this into only never always use used using when what which
your you are was were has have had not but any all one two its their there here
skill skills maestro owner dono usuario user
tudo dias horas hora reais real coisa jeito""".split())

# Siglas curtas que SAO gatilho. O filtro de 4 letras existe para descartar
# ruido ("uma", "isso"), e de quebra apagava as palavras mais especificas do
# vocabulario: `pdf` some da linha "PDF, Office, pagina salva, imagem" que a
# tabela da a `ingest-content`, e a skill fica invisivel para o pedido mais
# obvio dela. Allowlist em vez de baixar o piso para todos: um `eod` entra, um
# `uma` continua fora.
SHORT_OK = {"pdf", "eod", "bcg", "tmo", "cdc", "ipa", "fpa", "csv", "xls", "ppt",
            "doc", "sql", "api", "kpi", "roi", "dre", "iva", "npv", "qa", "pr"}

TERM_RE = re.compile(r"[A-Za-zÀ-ÿ][A-Za-zÀ-ÿ0-9\-]{2,}")

# Peso por origem do termo. O catalogo e portugues e curado: e o que casa com o
# pedido real. A descricao de frontmatter e inglesa e prolixa; vale menos.
FIELD_WEIGHT = {"operator": 4.0, "catalog": 3.0, "name": 2.5, "heading": 1.5,
                "description": 1.0, "body": 0.6}


def fold(w):
    return unicodedata.normalize("NFKD", w).encode("ascii", "ignore").decode().lower()


STEM_AT = 5


def stem(w):
    """Prefixo de 5 letras. Stemmer pobre, e de proposito.

    Portugues flexiona demais para casamento exato: o dono escreve "registrar"
    e o gatilho diz "registra", "decisao" contra "decisoes", "monta" contra
    "montar". Sem isso o roteador erra por conjugacao, nao por sentido.
    Cortar no quinto caractere une essas familias e custa uma colisao ocasional
    que o idf e o peso por origem absorvem. Aplicado dos dois lados — indice e
    consulta — ou nao casa nada.
    """
    return w[:STEM_AT]


def terms_of(text):
    """Unigramas distintivos e bigramas de um trecho, ja radicalizados."""
    words = [fold(w) for w in TERM_RE.findall(text or "")]
    uni = {stem(w) for w in words
           if w not in STOP and (len(w) >= 4 or w in SHORT_OK)}
    bi = set()
    for i in range(len(words) - 1):
        if words[i] in STOP or words[i + 1] in STOP:
            continue
        bi.add(f"{stem(words[i])} {stem(words[i + 1])}")
    return uni, bi


def skill_files():
    return sorted(glob.glob(os.path.join(BUNDLES, "*", "skills", "*", "SKILL.md")))


def catalog_files():
    return sorted(glob.glob(os.path.join(BUNDLES, "*", "skills", "catalog.json")))


def newest_mtime():
    m = 0.0
    for p in skill_files() + catalog_files():
        try:
            m = max(m, os.path.getmtime(p))
        except OSError:
            pass
    return round(m, 3)


def read_frontmatter(path):
    """name, description e o primeiro paragrafo util depois do H1."""
    try:
        raw = io.open(path, encoding="utf-8", errors="replace").read()
    except OSError:
        return None
    fm, name, desc = {}, "", ""
    lines = raw.split("\n")
    if lines and lines[0].strip() == "---":
        for i, ln in enumerate(lines[1:], 1):
            if ln.strip() == "---":
                lines = lines[i + 1:]
                break
            if ":" in ln and not ln.startswith((" ", "\t")):
                k, v = ln.split(":", 1)
                fm[k.strip()] = v.strip().strip('"\'')
    name = fm.get("name", "")
    desc = fm.get("description", "")
    heading, body = "", []
    for ln in lines:
        s = ln.strip()
        if not s:
            continue
        if s.startswith("# ") and not heading:
            heading = s[2:]
            continue
        if heading and not s.startswith(("#", "|", "-", "```", "<!--")):
            body.append(s)
            if len(body) >= 3:
                break
    return {"name": name, "description": desc, "heading": heading, "body": " ".join(body)}



OPERATOR = os.path.join(BUNDLES, "base", "skills", "maestro-operator", "SKILL.md")
ROW_RE = re.compile(r"^\|(.+?)\|(.+?)\|\s*$")
ID_RE = re.compile(r"`/?([a-z][a-z0-9-]{2,})`")


def parse_operator_table():
    """Gatilhos em portugues, lidos da tabela de roteamento do maestro-operator.

    A tabela existe para um humano ler: coluna esquerda "o pedido soa como",
    coluna direita a skill. E o unico corpus de gatilho em portugues escrito
    contra pedido real, e nao contra o nome da skill — por isso pesa mais que o
    catalogo. Manter os dois em sincronia seria dividir a verdade em dois; aqui
    a tabela e a fonte e o indice e derivado.

    Fail-open: sem o arquivo, sem tabela reconhecivel ou com formato mudado,
    devolve vazio e o roteamento cai para catalogo + frontmatter.
    """
    try:
        raw = io.open(OPERATOR, encoding="utf-8", errors="replace").read()
    except OSError:
        return {}
    out, in_table = {}, False
    for ln in raw.splitlines():
        m = ROW_RE.match(ln)
        if not m:
            in_table = False
            continue
        left, right = m.group(1).strip(), m.group(2).strip()
        head = fold(left)
        if head.startswith(("o pedido soa como", "tipo de pedido")):
            in_table = True                   # cabecalho: as linhas seguintes valem
            continue
        if set(left) <= set("-: ") or not in_table:
            continue                          # separador, ou tabela que nao e de rota
        for sid in ID_RE.findall(right):
            out[sid] = (out.get(sid, "") + " " + left).strip()
    return out


def build():
    """Indice invertido termo -> {skill_id: peso}, com idf embutido no ranking."""
    catalog, triggers = {}, {}
    operator = parse_operator_table()
    for cf in catalog_files():
        try:
            data = json.load(io.open(cf, encoding="utf-8"))
        except (OSError, ValueError):
            continue
        for s in data.get("skills", []):
            sid = s.get("id")
            if sid:
                catalog[sid] = " ".join(filter(None, [
                    s.get("display_name", ""), s.get("trigger", "")]))
                triggers[sid] = s.get("trigger", "") or s.get("display_name", "")

    skills, per_skill = {}, {}
    for path in skill_files():
        sid = os.path.basename(os.path.dirname(path))
        fmt = read_frontmatter(path)
        if not fmt:
            continue
        bundle = path.replace("\\", "/").split("/")[1]
        skills[sid] = {
            "bundle": bundle,
            "path": path.replace("\\", "/"),
            "label": fmt["heading"] or sid,
            # resumo curto para o hook nao reabrir arquivo nenhum
            "summary": (triggers.get(sid) or fmt["description"])[:110],
        }
        weighted = {}
        for field, text in (("operator", operator.get(sid, "")),
                            ("catalog", catalog.get(sid, "")), ("name", sid.replace("-", " ")),
                            ("name", fmt["name"]), ("heading", fmt["heading"]),
                            ("description", fmt["description"]), ("body", fmt["body"])):
            w = FIELD_WEIGHT[field]
            uni, bi = terms_of(text)
            for t in uni:
                weighted[t] = max(weighted.get(t, 0.0), w)
            for t in bi:                      # bigrama e especifico: vale o triplo
                weighted[t] = max(weighted.get(t, 0.0), w * 3)
        per_skill[sid] = weighted

    inverted = {}
    for sid, weighted in per_skill.items():
        for t, w in weighted.items():
            inverted.setdefault(t, {})[sid] = round(w, 3)

    return {"schema_version": 1, "built_from_mtime": newest_mtime(),
            "skills": skills, "terms": inverted}


def load(rebuild=False):
    if not rebuild and os.path.exists(CACHE):
        try:
            idx = json.load(io.open(CACHE, encoding="utf-8"))
            if not isinstance(idx, dict):
                raise ValueError("cache com forma inesperada")
            if idx.get("built_from_mtime") == newest_mtime():
                return idx
        except (OSError, ValueError):
            pass
    idx = build()
    try:
        os.makedirs(os.path.dirname(CACHE), exist_ok=True)
        tmp = CACHE + ".tmp"
        io.open(tmp, "w", encoding="utf-8", newline="\n").write(
            json.dumps(idx, ensure_ascii=False) + "\n")
        os.replace(tmp, CACHE)                # publicacao atomica, como no brain
    except OSError:
        pass
    return idx


def route(idx, text, top_n=5):
    # Nome exato da skill, sozinho: o dono digitou "eod" ou "retro" e quer aquilo.
    # Sem isto um id de tres letras nao casa com nada — o filtro de termo exige
    # quatro — e o pedido mais literal possivel e o unico que nao funciona.
    exact = fold(text.strip().lstrip("$/")).strip()
    if exact in idx["skills"]:
        return [(exact, 999.0)]

    uni, bi = terms_of(text)
    if not uni and not bi:
        return []
    terms, n = idx["terms"], max(len(idx["skills"]), 1)
    scores, matched, had_bigram, best_df = {}, {}, {}, {}
    for t in list(bi) + list(uni):
        hits = terms.get(t)
        if not hits:
            continue
        # idf: um termo que casa com meia biblioteca nao decide nada
        idf = 1.0 + (n / max(len(hits), 1)) ** 0.5
        for sid, w in hits.items():
            scores[sid] = scores.get(sid, 0.0) + w * idf
            matched[sid] = matched.get(sid, 0) + 1
            best_df[sid] = max(best_df.get(sid, 0), len(hits))
            if t in bi:
                had_bigram[sid] = True

    # Um termo casado nao e evidencia suficiente. Medido contra o conjunto de
    # referencia: dos 19 pedidos que NAO pedem skill nenhuma, 10 recebiam
    # sugestao — e os 10 casavam EXATAMENTE UM termo. "obrigado" achava
    # `excel-financial-model` por "obrigatorio"; "que horas sao" achava
    # `start-day` por "horas"; "qual a raiz quadrada" achava `investigate` por
    # "causa raiz". Do outro lado, todo acerto real carrega bigrama ou dois
    # termos: "fecha o dia" casa o bigrama `fecha dia` mais `fecha`, "monta o
    # deck" casa tres.
    #
    # O idf sozinho nao separa os dois grupos, e por isso a tentativa anterior
    # falhou: `dias` e `renom` sao hapax de peso 4.0 exatamente como um gatilho
    # legitimo seria. O que separa e a QUANTIDADE de evidencia, nao a raridade
    # dela.
    #
    # Bigrama vale sozinho: "bom dia" e um pedido inteiro em duas palavras.
    #
    # E a terceira saida, que parece de tras para frente e nao e: um termo unico
    # so basta quando ele aparece em VARIAS skills. Termo que recorre pela
    # biblioteca e vocabulario do dominio — `deck` (6 skills), `feedback`,
    # `reuniao`, `explicar` (9). Termo unico que casa uma skill so e quase sempre
    # incidental: `dias`, `renom`, `raiz`, `reais`, `loop`, `email`, `obrig`,
    # `horas` — cada um hapax, cada um um falso positivo medido.
    #
    # Vale so para evidencia de UM termo. Com dois ou mais, o idf normal volta a
    # valer, porque ai a raridade e sinal e nao acidente. Exigir dois termos sem
    # esta saida derrubava o recall de 97,7% para 83,7%: "preciso de um deck pra
    # quinta" casa `deck` e mais nada, porque as palavras de cola entre elas
    # impedem o bigrama.
    scores = {s: v for s, v in scores.items()
              if matched.get(s, 0) >= 2 or had_bigram.get(s) or best_df.get(s, 0) >= 2}

    ranked = sorted(scores.items(), key=lambda kv: (-kv[1], kv[0]))
    if not ranked:
        return []
    # corte relativo ao topo: melhor devolver duas certas que cinco por educacao
    top = ranked[0][1]
    return [(s, sc) for s, sc in ranked[:top_n] if sc >= max(6.0, top * 0.35)]


def main():
    argv = sys.argv[1:]
    top_n, rebuild, names_only = 5, False, False
    if "--rebuild" in argv:
        rebuild = True
        argv.remove("--rebuild")
    if "--names" in argv:
        names_only = True
        argv.remove("--names")
    if "--max" in argv:
        i = argv.index("--max")
        try:
            top_n = max(1, int(argv[i + 1]))   # clamp: --max 0 e --max -1 divergiam
        except (IndexError, ValueError):
            pass
        del argv[i:i + 2]

    idx = load(rebuild)

    if names_only:
        by_bundle = {}
        for sid, m in sorted(idx["skills"].items()):
            by_bundle.setdefault(m["bundle"], []).append(sid)
        for bundle, ids in sorted(by_bundle.items()):
            print(f"- **{bundle}**: " + ", ".join(f"`{i}`" for i in ids))
        return 0

    text = " ".join(argv) if argv else sys.stdin.read()
    if not text.strip():
        return 0
    hits = route(idx, text, top_n)
    if not hits:
        return 0

    print("<!-- maestro:skill-route -->")
    print("Skills que provavelmente resolvem este pedido:")
    for sid, _ in hits:
        m = idx["skills"][sid]
        print(f"- `{sid}` — {m['summary']} — `{m['path']}`")
    print("Carregue a que servir com Read. Se nenhuma servir, responda direto.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
