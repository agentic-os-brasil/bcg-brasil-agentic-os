#!/usr/bin/env python3
"""Compilador do indice do brain (spec 007, escopo privado).

Le todas as paginas de brain/ e produz uma camada de navegacao DERIVADA:

  brain/brain_index.md                indice do escopo do dono
  brain/accounts/<a>/<a>_index.md         indice de cada conta
  brain/accounts/<a>/cases/<c>/<c>_index.md   indice de cada caso
  brain/.maestro/backlinks.json       links reversos
  brain/.maestro/diagnostics.json     links quebrados, orfas, contrato incompleto
  brain/.maestro/log.md               log append-only com fingerprint

Invariantes (spec 007):
  - Nada aqui e autoridade. Toda pagina aponta; nenhuma repete conteudo.
  - Idempotente para o mesmo conjunto de fontes.
  - Compila em staging e publica de uma vez; em falha, preserva o ultimo bom.
  - Escopos nunca se misturam: o indice de um caso so ve o proprio caso.

Uso:  python3 bundles/base/tools/brain-index.py [--check]
      --check  compila e reporta, sem publicar.
"""
import io, os, re, sys, json, glob, hashlib, datetime, shutil, tempfile, unicodedata

BRAIN = "brain"
MACHINE = os.path.join(BRAIN, ".maestro")
REQ = ["id", "title", "summary", "type", "scope", "status", "sensitivity", "updated"]

TYPE_LABELS = {
    "daily": "Dias de trabalho", "learning": "Aprendizados",
    "craft-method": "Métodos", "craft-style": "Estilo", "person": "Pessoas",
    "objectives": "Objetivos", "cdc": "Avaliações (CDC)", "retro": "Retrospectivas",
    "project-feedback": "Feedback de projeto", "upward-feedback": "Feedback para cima",
    "owner-facet": "Facetas SELF", "operating": "Estado operacional",
    "memory-l1": "Memória recente (L1)", "memory-l2": "Memória semanal (L2)",
    "memory-l3": "Trilhas de médio prazo (L3)", "memory-lifetime": "Memória permanente",
    "canon": "Canon", "data": "Canon — dados", "framework": "Canon — frameworks",
    "hypothesis": "Canon — hipóteses", "benchmark": "Canon — benchmarks",
    "interview": "Canon — entrevistas", "decision-log": "Decisões",
    "task-list": "Tarefas", "deliverable": "Entregáveis", "source": "Fontes",
    "project-brief": "Brief do projeto", "account": "Conta",
}
# Ordem de apresentacao. O que nao estiver aqui vai para o fim, alfabetico.
OWNER_ORDER = ["operating", "owner-facet", "memory-lifetime", "memory-l3", "memory-l2",
               "memory-l1", "learning", "craft-method", "craft-style", "person",
               "objectives", "retro", "cdc", "project-feedback", "upward-feedback", "daily"]
CASE_ORDER = ["project-brief", "decision-log", "task-list", "data", "framework",
              "hypothesis", "benchmark", "interview", "canon", "deliverable", "source"]

TREE_DESC = {
    "memory": "memória consolidada pelo motor de dreaming",
    "owner": "quem é o dono: identidade, estilo, estado de trabalho",
    "daily": "a página de cada dia de trabalho",
    "learnings": "aprendizados profissionais duráveis",
    "craft": "métodos e calibrações de estilo",
    "people": "perfis de colegas",
    "development": "objetivos, retros e feedback",
    "accounts": "clientes e seus projetos",
    "tasks": "visão de tarefas derivada dos casos, por status e prioridade",
}



def write_atomic(path, text):
    """Grava por .tmp + os.replace.

    Os arquivos de maquina eram gravados direto, inclusive os ~2 MB do
    route-index. Um Stop interrompido, um lock do OneDrive ou disco cheio
    deixava o arquivo truncado — e o `brain-route` morria com JSONDecodeError
    dentro de um hook que engole stderr, entao o roteamento sumia em silencio.
    O `skill-route` ja publicava assim, e o comentario dele dizia "como no
    brain"; o brain e que nao fazia.
    """
    tmp = path + ".tmp"
    with io.open(tmp, "w", encoding="utf-8", newline=chr(10)) as fh:
        fh.write(text)
    os.replace(tmp, path)


def read(p):
    """Le uma pagina. `errors="replace"` nao e detalhe.

    Sem isso, UM arquivo mal codificado — colado do Outlook, exportado de sistema
    legado, salvo por Excel em ANSI — levanta UnicodeDecodeError e derruba o
    compilador inteiro. O hook do Stop descartava stderr e nao testava o exit,
    entao o indice congelava, o roteamento congelava, e o SessionStart seguia
    reportando o diagnostico da ultima compilacao bem-sucedida como se fosse o
    atual. Falha silenciosa e permanente a partir de um byte.

    Substituir o byte invalido degrada aquela pagina e preserva as outras 200.
    `paths-check.py` e `skill-route.py` ja liam assim.
    """
    return io.open(p, encoding="utf-8", newline="", errors="replace").read()


def parse(p):
    s = read(p)
    # `read()` preserva o final de linha original (newline=""), de proposito: ele
    # nao pode reescrever o arquivo do dono. Isso torna o delimitador sensivel a
    # CRLF, e o dono edita estas paginas — abrir uma no Notepad do Windows e
    # salvar troca LF por CRLF. Sem o \r? aqui, aquela pagina passa a nao ter
    # frontmatter aos olhos do compilador: sai do indice, e o diagnostico a
    # reporta como lacuna de contrato mesmo com os oito campos no lugar. Falha
    # silenciosa e permanente a partir de um final de linha — a mesma classe de
    # problema que `read()` acima existe para evitar.
    m = re.match(r"^---\r?\n(.*?)\r?\n---\r?\n", s, re.S)
    if not m:
        return None, s
    fm, body = {}, s[m.end():]
    for line in m.group(1).split("\n"):
        km = re.match(r"^([A-Za-z_][\w-]*):\s*(.*)$", line)
        if not km:
            continue
        k, v = km.group(1), km.group(2).strip()
        if len(v) > 1 and v[0] == v[-1] and v[0] in "\"'":
            v = v[1:-1]
        fm[k] = v
    return fm, body


def collect():
    pages = {}
    for p in sorted(glob.glob(os.path.join(BRAIN, "**", "*.md"), recursive=True)):
        rel = p.replace("\\", "/")[len(BRAIN) + 1:]
        # Exclui apenas os indices GERADOS (raiz do brain e raiz de cada caso).
        # Indices curados a mao — people/, learnings/, craft/ — continuam sendo
        # paginas: seus links contam como backlink e sua curadoria tem valor que
        # o indice gerado nao reproduz.
        if rel.startswith(".maestro/"):
            continue
        _d, _b = os.path.split(rel)
        if rel in ("brain_index.md", "tasks/tasks.md"):
            continue
        if _d and _b == index_basename(_d):
            continue
        # README de pasta e documento de navegacao — "o que e este diretorio",
        # para humano olhando a arvore. Nao e conteudo do dono, nao deve ganhar
        # frontmatter so para satisfazer o contrato, e nao deve ser reportado
        # como lacuna. Sem esta linha, toda instalacao NOVA abria com
        # "16 pagina(s) sem frontmatter completo" — e as 16 tinham acabado de ser
        # escritas pelo proprio scaffold, incluindo dois README.
        if _b == "README.md":
            continue
        fm, body = parse(p)
        if fm is None:
            pages[rel] = {"_path": rel, "_nofm": True, "_body": body}
            continue
        fm["_path"] = rel
        fm["_body"] = body
        pages[rel] = fm
    return pages


LINK_RE = re.compile(r"\[([^\]]*)\]\((?!https?:)([^)#]+?\.md)(?:#[^)]*)?\)")

# Fronteiras de escopo que viraram arquivo unico (conta, caso): a curadoria do
# dono (ver curated_index) E o indice de navegacao. Modulo-level porque
# index_name precisa delas, nao so o loop de publicacao em main().
CASE_ROOT = re.compile(r"^accounts/[^/]+/cases/[^/]+$")
ACCOUNT_ROOT = re.compile(r"^accounts/[^/]+$")


def resolve(src_rel, target):
    base = os.path.dirname(src_rel)
    return os.path.normpath(os.path.join(base, target)).replace("\\", "/")


def build_links(pages):
    forward, backlinks, broken = {}, {}, []
    for rel, fm in pages.items():
        outs = []
        for m in LINK_RE.finditer(fm.get("_body", "")):
            tgt = resolve(rel, m.group(2))
            outs.append(tgt)
            if tgt in pages:
                backlinks.setdefault(tgt, []).append(rel)
            elif not os.path.exists(os.path.join(BRAIN, tgt)):
                broken.append({"from": rel, "to": tgt, "text": m.group(1)[:60]})
        if outs:
            forward[rel] = sorted(set(outs))
    for k in backlinks:
        backlinks[k] = sorted(set(backlinks[k]))
    return forward, backlinks, broken


def scope_of(fm):
    return fm.get("scope", "owner")


def case_key(scope):
    m = re.match(r"^account/([^/]+)/case/(.+)$", scope or "")
    return (m.group(1), m.group(2)) if m else None


def short(text, n):
    """Corta em fronteira de palavra. Cortar no meio da palavra deixa o titulo
    ilegivel — 'Diagnostico do Modelo de Compr' nao ajuda ninguem a reconhecer
    a pagina."""
    text = (text or "").strip()
    if len(text) <= n:
        return text
    cut = text[:n].rsplit(" ", 1)[0].rstrip(" ,;:—-")
    return (cut or text[:n]) + "…"



def entry_line(fm, from_dir):
    rel = fm["_path"]
    href = os.path.relpath(os.path.join(BRAIN, rel), os.path.join(BRAIN, from_dir) if from_dir else BRAIN)
    href = href.replace("\\", "/")
    title = fm.get("title", rel)
    summary = fm.get("summary", "")
    flag = " `[encerrado]`" if fm.get("status") == "closed" else ""
    return f"- [{title}]({href}){flag} — {summary}"


def group(pages_list, order):
    buckets = {}
    for fm in pages_list:
        buckets.setdefault(fm.get("type", "page"), []).append(fm)
    keys = [k for k in order if k in buckets] + sorted(k for k in buckets if k not in order)
    return [(k, sorted(buckets[k], key=lambda f: (f.get("updated", ""), f.get("title", "")), reverse=True))
            for k in keys]


_STOPWORDS = """de da do das dos e o a os as um uma uns umas para por com em no na nos nas
que se ao aos sem sobre entre como mais menos ja nao ou eh foi ser sao era sera pelo pela
pelos pelas ate apos antes cada todo toda todos todas isso isto esse essa este esta aquele
aquela seu sua seus suas meu minha nosso nossa qual quais quando onde porque tem ter tinha
havia ha fica ficar vai ir vao dois duas tres mes ano ver
ainda sempre apenas tambem so muito bem mesmo mesma outro outra outros outras
novo nova entao assim depois hoje ontem amanha relacionado relacionada relacionados
migrado migrada migrados nenhum nenhuma algum alguma varios varias primeiro primeira
segundo segunda proprio propria durante ainda_que sim tal tao pouco quase logo bastante"""


def _fold(w):
    """Remove acento para comparar com a lista de stopwords."""
    return unicodedata.normalize("NFKD", w).encode("ascii", "ignore").decode().lower()


STOP = {_fold(w) for w in _STOPWORDS.split()}

_STEM_AT = 5
SHORT_OK = {"tmo", "bcg", "pdf", "eod", "cdc", "ipa", "fpa", "dre", "kpi", "roi",
            "csv", "xls", "ppt", "sql", "api", "qa", "pr"}


def _stem(w):
    """Prefixo de 5 letras — o mesmo stemmer pobre do skill-route.

    Aplicado no indice E na consulta, ou nao casa nada. Foi a divergencia entre
    os dois roteadores irmaos: um radicalizava, o outro nao.
    """
    return w[:_STEM_AT]


TERM_RE = re.compile(r"[A-Za-zÀ-ÿ][A-Za-zÀ-ÿ0-9\-]{2,}")


def page_terms(fm):
    """Texto de alta densidade de sinal: titulo, resumo, cabecalhos e inicio do corpo."""
    body = fm.get("_body", "")
    heads = " ".join(re.findall(r"^#{2,3}\s+(.+)$", body, re.M))
    return " ".join([fm.get("title", ""), fm.get("summary", ""), heads, body[:2500]]).lower()


def ngrams(text, n):
    words = [w for w in TERM_RE.findall(text)]
    for i in range(len(words) - n + 1):
        gram = words[i:i + n]
        folded = [_fold(w) for w in gram]
        if folded[0] in STOP or folded[-1] in STOP:
            continue
        if sum(1 for w in folded if w in STOP) > len(gram) - 2:
            continue
        yield " ".join(gram)


def concept_hubs(pages_list, min_pages=4):
    """Conceitos recorrentes, detectados por n-grama sobre as paginas do escopo.

    Sem embeddings — a spec 007 coloca busca vetorial fora do V1. Um conceito e
    um n-grama que aparece em min_pages ou mais paginas DISTINTAS do escopo.
    N-gramas curtos sao descartados quando um mais longo cobre o mesmo conjunto,
    para nao listar 'modelo' e 'modelo de impacto' como coisas diferentes.
    """
    seen, types = {}, {}
    for fm in pages_list:
        text = page_terms(fm)
        found = set()
        for n in (3, 2):
            for g in ngrams(text, n):
                found.add(g)
        for g in found:
            seen.setdefault(g, set()).add(fm["_path"])
            types.setdefault(g, set()).add(fm.get("type", "page"))

    total = max(len(pages_list), 1)
    max_ratio = 0.15 if total > 120 else (0.35 if total > 40 else 0.5)

    # Dois filtros, ambos necessarios:
    #  - frequencia alta demais = estrutura de template, nao conceito. Toda daily
    #    tem "decisoes que emergiram"; isso nao liga nada a nada.
    #  - conceito precisa ATRAVESSAR tipos. Um hub que so aparece em daily/ e um
    #    cabecalho repetido; um que aparece em canon + decisions + tasks e um tema.
    hits = {g: ps for g, ps in seen.items()
            if len(ps) >= min_pages
            and len(ps) / total <= max_ratio
            and len(types[g]) >= 2}
    # descarta o curto quando o longo cobre o mesmo conjunto de paginas
    # Colapsa n-gramas que descrevem a mesma frase. "decision log",
    # "decision log original" e "original maestro" sao pedacos de uma unica
    # frase de boilerplate da migracao do v1; listar os tres como conceitos
    # distintos e ruido com cara de sinal.
    drop = set()
    # Desempate pelo proprio termo: sem ele a ordem vem da iteracao de um set,
    # que varia com o hash seed do processo — e a saida deixa de ser idempotente.
    items = sorted(hits.items(), key=lambda x: (-len(x[1]), x[0]))
    for a, (g, ps) in enumerate(items):
        if g in drop:
            continue
        for h, qs in items[a + 1:]:
            if h in drop:
                continue
            overlap = len(ps & qs) / max(len(qs), 1)
            if g in h or h in g or overlap >= 0.8:
                drop.add(h)
    hits = {g: ps for g, ps in hits.items() if g not in drop}

    by_path = {fm["_path"]: fm for fm in pages_list}
    return sorted(((g, [by_path[x] for x in sorted(ps)]) for g, ps in hits.items()),
                  key=lambda x: (-len(x[1]), x[0]))


HEADER = ["> Página gerada. Não edite à mão — recompilada a partir do frontmatter",
          "> de cada página. Nada aqui é autoridade: o índice aponta, a página aponta a fonte.",
          ""]


def _index_prefix(folder):
    """Prefixo de contexto para o nome do indice de uma pasta, ou None quando
    o basename sozinho ja e globalmente unico.

    Pastas dentro de uma conta ou de um caso (canon/, decisions/, tasks/...)
    usam nomes genericos que se repetem em toda conta/caso — sem prefixo,
    'canon_index.md' nasceria identico em cada caso que tem um canon/. O slug
    da conta ou do caso, ja unico por construcao, remove a colisao.
    """
    parts = folder.split("/")
    if parts and parts[0] == "accounts" and len(parts) >= 2:
        if len(parts) >= 4 and parts[2] == "cases":
            return parts[3]      # slug do caso
        return parts[1]          # slug da conta
    return None


def index_basename(folder):
    """So o nome do arquivo de indice de uma pasta, sem o caminho."""
    base = os.path.basename(folder)
    prefix = _index_prefix(folder)
    if prefix and prefix != base:
        return f"{prefix}-{base}_index.md"
    return f"{base}_index.md"


def index_name(folder):
    """Caminho (a partir de brain/) do que representa uma pasta na navegacao.

    Nome distinto por pasta (daily_index.md, learnings_index.md) em vez de
    index.md em toda parte: evita colisao de nome no editor e no grafo, e
    deixa a curadoria do dono — em learnings/, people/ e craft/, guardada como
    o proprio nome da pasta ('<pasta>.md', ver curated_index) — conviver com o
    gerado sem se sobrescreverem. A raiz do brain e sempre 'brain_index.md': e
    o unico indice sem pasta dona, entao 'index.md' sozinho colidiria com
    qualquer curadoria de mesmo nome.

    Conta e caso sao a excecao a "indice e curadoria sao arquivos separados":
    quando a curadoria existe, ELA e o indice — nao ha arquivo gerado a parte
    para essas duas fronteiras (ver main(): conta nao publica nada, caso cola
    a secao gerada dentro do proprio brief via splice_generated). Por isso todo
    link que passa por aqui — breadcrumb, 'Subpastas', 'Casos' da raiz — cai
    direto no brief/pagina de conta, em vez de num indice-casca que so repetia
    o que a curadoria ja dizia.
    """
    if not folder:
        return "brain_index.md"
    if CASE_ROOT.match(folder) or ACCOUNT_ROOT.match(folder):
        cur = curated_index(folder)
        if cur:
            return cur
    return f"{folder}/{index_basename(folder)}"


def rel_href(target, from_dir):
    a = os.path.join(BRAIN, target.replace("/", os.sep))
    b = os.path.join(BRAIN, from_dir.replace("/", os.sep)) if from_dir else BRAIN
    return os.path.relpath(a, b).replace("\\", "/")


def curated_index(folder):
    """'<pasta>.md' escrito a mao dentro da propria pasta, se existir.

    Nome = o nome da propria pasta, sem sufixo. Nao colide com o indice
    gerado (que carrega o sufixo '_index', ver index_name) nem com a
    curadoria de outra pasta, desde que pastas de topo nao repitam nome —
    verdade por construcao para craft/, learnings/, people/. Nao usar este
    padrao para pastas aninhadas cujo basename se repete entre casos ou
    contas (canon/, decisions/, tasks/...): ali o nome sozinho colidiria.
    """
    if not folder:
        return None
    name = f"{os.path.basename(folder)}.md"
    p = os.path.join(BRAIN, folder.replace("/", os.sep), name)
    return f"{folder}/{name}" if os.path.exists(p) else None


def render_folder(folder, own_pages, subfolders, order, backlinks, fingerprint, tree_counts,
                   parent_index):
    """Indice de uma pasta: as paginas dela, mais ponteiro para cada subpasta.

    `parent_index` e a pasta que RESPONDE pelo pai (ja resolvida pelo chamador
    via nearest_index) — nao o pai literal. Um pai de passagem (ex.: 'cases/'
    quando a conta tem um so caso) nunca ganha indice proprio, entao apontar
    '↑' para ele produzia um link morto; o breadcrumb agora sobe direto para
    quem de fato tem pagina.
    """
    label = os.path.basename(folder) if folder else "brain"
    out = [f"# Índice — {label}", ""] + HEADER

    out.append(f"[↑ {os.path.basename(parent_index) or 'brain'}]({rel_href(index_name(parent_index), folder)})")
    out.append("")


    cur = curated_index(folder)
    if cur:
        out += [f"> Curadoria do dono, escrita à mão: [{cur}]"
                f"({rel_href(cur, folder)}). Esta página aqui é a listagem completa; "
                f"aquela é a opinião.", ""]
        # A curadoria ja foi anunciada acima. Lista-la de novo mais abaixo, sob
        # o proprio 'type' dela, duplicava o ponteiro e criava um segundo
        # cabecalho de cara de indice (## index) dentro do indice gerado —
        # exatamente a confusao de "dois indices" que este arquivo existe para
        # evitar.
        own_pages = [fm for fm in own_pages if fm["_path"] != cur]

    if subfolders:
        out += ["## Subpastas", ""]
        for s in sorted(subfolders):
            n = tree_counts.get(s, 0)
            out.append(f"- [{os.path.basename(s)}/]({rel_href(index_name(s), folder)})")
        out.append("")

    for typ, items in group(own_pages, order):
        out += [f"## {TYPE_LABELS.get(typ, typ)}", ""]
        for fm in items:
            out.append(entry_line(fm, folder))
        out.append("")


    out += ["---", f"<!-- fingerprint: {fingerprint} -->"]
    return "\n".join(out) + "\n"


GEN_START = "<!-- maestro:generated:start -->"
GEN_END = "<!-- maestro:generated:end -->"
GEN_NOTE = ("> Do daqui para baixo: gerado, não edite à mão — recompilado a partir do "
            "frontmatter de cada página do caso. O brief acima desta marca é do dono.")


def render_case_section(folder, own_pages, subfolders, order, parent_index):
    """Corpo gerado para colar dentro do brief do caso (ver splice_generated).

    Caso e conta sao a fronteira de escopo que virou arquivo unico: sem
    titulo, sem callout de curadoria — isso tudo ja vive no brief que este
    bloco entra dentro. So a navegacao: subpasta, e o resto do material do
    caso agrupado por tipo (decisoes, tarefas, entregaveis, fontes — canon
    tem pasta e indice proprios porque tem massa para isso).
    """
    out = [f"[↑ {os.path.basename(parent_index) or 'brain'}]({rel_href(index_name(parent_index), folder)})", ""]
    if subfolders:
        out += ["## Subpastas", ""]
        for s in sorted(subfolders):
            out.append(f"- [{os.path.basename(s)}/]({rel_href(index_name(s), folder)})")
        out.append("")
    for typ, items in group(own_pages, order):
        out += [f"## {TYPE_LABELS.get(typ, typ)}", ""]
        for fm in items:
            out.append(entry_line(fm, folder))
        out.append("")
    return "\n".join(out).rstrip() + "\n"


def splice_generated(existing_text, body):
    """Atualiza (ou cria, na primeira vez) o bloco gerado dentro de um arquivo
    curado, preservando tudo fora dos marcadores byte a byte. E o mesmo
    principio do bloco `maestro:session-scope` nos AGENT.md: uma secao
    marcada dentro de um arquivo autoral e a unica parte que o compilador
    escreve; o resto e do dono e nunca e tocado.
    """
    block = f"{GEN_START}\n{GEN_NOTE}\n\n{body}{GEN_END}\n"
    if GEN_START in existing_text and GEN_END in existing_text:
        pre, rest = existing_text.split(GEN_START, 1)
        _, post = rest.split(GEN_END, 1)
        pre, post = pre.rstrip("\n"), post.lstrip("\n")
        return f"{pre}\n\n{block}\n{post}" if post else f"{pre}\n\n{block}"
    return f"{existing_text.rstrip()}\n\n{block}"


def render_root(trees, case_rows, all_pages, backlinks, fingerprint, tree_counts):
    """Raiz: so as arvores, os casos e o que atravessa tudo. Nada de listar pagina."""
    out = ["# Índice do brain", ""] + HEADER
    out.append("Cada árvore tem índice próprio; esta página só aponta para eles.")
    out.append("")

    out += ["## Árvores", ""]
    for t in trees:
        n = tree_counts.get(t, 0)
        desc = TREE_DESC.get(t)
        tail = f" — {desc}" if desc else ""
        target = "tasks/tasks.md" if t == "tasks" else index_name(t)
        out.append(f"- [{t}/]({rel_href(target, '')}){tail}")
    out.append("")

    out += ["## Casos", "",
            "Cada caso vê apenas a si mesmo. O escopo do dono não se mistura com eles.", ""]
    out += case_rows
    out.append("")

    out += ["## Tarefas", "",
            "Visão que atravessa todos os casos, em dois eixos — status e prioridade:",
            "[tasks/tasks.md](tasks/tasks.md)", ""]


    linked = [fm for fm in all_pages if backlinks.get(fm["_path"])]
    if linked:
        out += ["## Páginas mais referenciadas", ""]
        for fm in sorted(linked, key=lambda f: -len(backlinks[f["_path"]]))[:8]:
            n = len(backlinks[fm["_path"]])
            out.append(f"- [{fm.get('title', fm['_path'])}]({rel_href(fm['_path'], '')})")
        out.append("")

    out += ["---", f"<!-- fingerprint: {fingerprint} -->"]
    return "\n".join(out) + "\n"


def build_route_index(pages_list):
    """Indice invertido termo -> paginas, para o hook de roteamento.

    So termos distintivos: o que aparece em mais de 30% do escopo e estrutura,
    nao assunto, e polui o ranking. Guarda titulo e resumo junto para o hook
    nao precisar reabrir arquivo nenhum.
    """
    per_page, df = {}, {}
    for fm in pages_list:
        text = page_terms(fm)
        terms = set()
        for w in TERM_RE.findall(text):
            fw = _fold(w)
            if fw in STOP or (len(fw) < 4 and fw not in SHORT_OK):
                continue
            terms.add(_stem(fw))
        for g in ngrams(text, 2):
            terms.add(" ".join(_stem(x) for x in _fold(g).split()))
        per_page[fm["_path"]] = terms
        for t in terms:
            df[t] = df.get(t, 0) + 1

    total = max(len(pages_list), 1)
    inverted = {}
    for path, terms in per_page.items():
        for t in terms:
            if df[t] / total > (0.30 if total >= 40 else 0.75):
                continue
            inverted.setdefault(t, []).append(path)

    meta = {fm["_path"]: {"title": fm.get("title", ""), "summary": fm.get("summary", ""),
                          "type": fm.get("type", ""), "scope": fm.get("scope", ""),
                          "status": fm.get("status", "")}
            for fm in pages_list}
    return {"terms": {t: sorted(ps) for t, ps in sorted(inverted.items())}, "pages": meta}


TASK_RE = re.compile(r"^\s*[-*]\s*\[([ xX~/\-])\]\s*(.+)$")
# Marcadores de prioridade escritos no texto. O vermelho ja estava em uso
# antes deste vocabulario existir e continua valendo como P0.
PRIO_TAG = re.compile(r"\[(P[012])\]", re.I)
PRIO_RE = re.compile(r"[\U0001F534\u2757]")      # marcador vermelho = urgente
BLOCKED_RE = re.compile(r"[\u2753\u2754]")        # interrogacao = depende de terceiro


# Tipos de pagina que carregam COMPROMISSO. Um checkbox fora daqui quase sempre
# e um portao de qualidade, nao uma tarefa: o guia de estilo tem "Checar sempre
# antes de o artefato sair" com quatro itens que nunca serao marcados, porque
# devem ser reexecutados a cada entrega. Contar isso como tarefa aberta inventa
# trabalho que nao existe. Allowlist, nao denylist: tipo novo entra fora por
# padrao, e nao polui a visao ate alguem decidir que deve entrar.
TASK_TYPES = {"task-list", "project-brief", "objectives", "daily", "operating"}



def extract_tasks(pages):
    """Le os checkboxes de todas as paginas e devolve uma lista plana.

    Continuacoes indentadas viram parte do texto da tarefa: no brain do dono
    uma tarefa costuma ocupar quatro linhas, e cortar na primeira perde o
    essencial.
    """
    out = []
    for rel, fm in sorted(pages.items()):
        if fm.get("type") not in TASK_TYPES:
            continue
        body = fm.get("_body", "")
        blines = body.split("\n")
        i = 0
        while i < len(blines):
            m = TASK_RE.match(blines[i])
            if not m:
                i += 1
                continue
            state, text = m.group(1), m.group(2)
            i += 1
            while i < len(blines) and blines[i].startswith(("      ", "\t\t")) and blines[i].strip():
                text += " " + blines[i].strip()
                i += 1
            raw = text
            text = re.sub(r"~~(.*?)~~", r"\1", text)
            text = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", text)
            text = re.sub(r"[*`]", "", text)
            urgent = bool(PRIO_RE.search(text))
            blocked = bool(BLOCKED_RE.search(text))
            text = PRIO_RE.sub("", BLOCKED_RE.sub("", text)).strip()
            st = state.lower()
            closed_case = fm.get("status") == "closed"
            if st == "x":
                status = "concluida"
            elif st in ("~", "/", "-"):
                status = "em-andamento"
            else:
                status = "a-fazer"

            # Prioridade: marcador explicito vence; vermelho herdado vira P0;
            # o resto cai em P1 POR OMISSAO, nao por decisao de ninguem.
            tag = PRIO_TAG.search(raw)
            if tag:
                priority = tag.group(1).upper()
                explicit = True
            elif urgent:
                priority = "P0"
                explicit = True
            else:
                priority = "P1"
                explicit = False
            text = PRIO_TAG.sub("", text).strip()

            out.append({
                "text": re.sub(r"\s+", " ", text).strip(),
                "status": status,
                "priority": priority,
                "priority_explicit": explicit,
                "blocked_external": blocked,
                "residual": closed_case and status != "concluida",
                "page": rel,
                "page_title": fm.get("title", rel),
                "scope": fm.get("scope", "owner"),
                "updated": fm.get("updated", ""),
            })
    return out


STATUS_ORDER = ["em-andamento", "a-fazer"]
STATUS_LABEL = {
    "em-andamento": ("Em andamento", "Já começou. Fechar antes de abrir frente nova."),
    "a-fazer": ("A ser realizada", "Ainda não começou."),
}
PRIORITY_ORDER = ["P0", "P1", "P2"]
PRIORITY_LABEL = {"P0": "P0 — urgente", "P1": "P1 — alta prioridade",
                  "P2": "P2 — baixa prioridade"}


def stamp_task_age(tasks, pages, persist=True):
    """Registra quando cada tarefa foi vista pela primeira vez.

    A data de `updated` e da PAGINA, nao da tarefa: uma pagina tocada hoje faz
    toda tarefa dela parecer nova. Sem idade real por tarefa, a varredura
    semanal nao tem como perguntar "isso ainda e tarefa?" com base em nada.

    O registro acumula em .maestro/tasks-seen.json. Tarefa que muda de texto
    conta como nova — e o comportamento certo: reescrever uma tarefa e
    reafirma-la.

    `persist=False` sob `--check`. A docstring do modulo promete que `--check`
    compila e reporta sem publicar, e esta funcao escrevia mesmo assim: uma
    inspecao que prometeu nao tocar em nada semeava o `first_seen` que a
    varredura semanal depois usa para perguntar "isso ainda e tarefa?". O relogio
    das tarefas passava a contar a partir de um comando de leitura.
    """
    seen_p = os.path.join(MACHINE, "tasks-seen.json")
    try:
        seen = json.load(io.open(seen_p, encoding="utf-8"))
    except Exception:
        seen = {}

    today = datetime.date.today()
    for t in tasks:
        key = hashlib.sha256(f"{t['page']}|{t['text']}".encode()).hexdigest()[:16]
        if key not in seen:
            # Semente: a data da pagina, quando houver. Tarefa migrada nao deve
            # nascer com idade zero so porque o indice rodou hoje.
            page_date = pages.get(t["page"], {}).get("updated", "")
            seen[key] = page_date if re.match(r"^\d{4}-\d{2}-\d{2}$", page_date or "")                 else today.isoformat()
        t["first_seen"] = seen[key]
        try:
            t["age_days"] = (today - datetime.date.fromisoformat(seen[key])).days
        except ValueError:
            t["age_days"] = 0

    if persist:
        os.makedirs(MACHINE, exist_ok=True)
        io.open(seen_p, "w", encoding="utf-8", newline=chr(10)).write(
            json.dumps(seen, ensure_ascii=False, indent=2) + chr(10))
    return tasks



def render_tasks(tasks):
    """Dois eixos: status (nivel 1) e prioridade (nivel 2).

    Bloqueio por terceiro e residuo de caso encerrado nao sao status — sao
    atributos marcados na propria linha. Uma tarefa bloqueada continua sendo
    "a ser realizada"; o que muda e quem precisa agir.
    """
    live = [t for t in tasks if t["status"] != "concluida"]
    done_n = sum(1 for t in tasks if t["status"] == "concluida")

    by_status = {}
    for t in live:
        by_status.setdefault(t["status"], []).append(t)

    counts = {p: sum(1 for t in live if t["priority"] == p) for p in PRIORITY_ORDER}
    blocked_n = sum(1 for t in live if t["blocked_external"])
    residual_n = sum(1 for t in live if t["residual"])
    implicit_n = sum(1 for t in live if not t["priority_explicit"])

    out = ["# Tarefas", "",
           "> Página gerada a partir dos checkboxes do brain. Não edite aqui — marque na",
           "> página de origem e recompile.", "",
           "**Status** vem do checkbox: `[ ]` a ser realizada · `[~]` em andamento · "
           "`[x]` concluída.",
           "**Prioridade** vem de marcador no texto: `[P0]` urgente · `[P1]` alta · "
           "`[P2]` baixa. Sem marcador, cai em P1.", ""]

    resumo = " · ".join(f"{counts[p]} {p}" for p in PRIORITY_ORDER if counts[p])
    out += [f"**{len(live)} em aberto** — {resumo} · {done_n} concluídas.", ""]

    avisos = []
    if blocked_n:
        avisos.append(f"**{blocked_n}** dependem de terceiro (⛔) — não andam por "
                      f"esforço próprio")
    if residual_n:
        avisos.append(f"**{residual_n}** são residuais de caso encerrado (📦)")
    if implicit_n:
        avisos.append(f"**{implicit_n}** estão em P1 por omissão, não por decisão — "
                      f"marque `[P0]`/`[P2]` na origem para classificar")
    if avisos:
        out += ["> " + "  \n> ".join(avisos), ""]

    for st in STATUS_ORDER:
        items = by_status.get(st, [])
        if not items:
            continue
        label, hint = STATUS_LABEL[st]
        out += [f"## {label}", "", f"_{hint}_", ""]
        for pr in PRIORITY_ORDER:
            sub = [t for t in items if t["priority"] == pr]
            if not sub:
                continue
            out += [f"### {PRIORITY_LABEL[pr]}", ""]
            for t in sorted(sub, key=lambda x: (x["scope"], x["text"])):
                tags = ""
                if t["blocked_external"]:
                    tags += " ⛔"
                if t["residual"]:
                    tags += " 📦"
                if not t["priority_explicit"]:
                    tags += " ·_sem marcador_"
                out.append(f"- {t['text']}{tags}")
                out.append(f"  <br>_{t['scope']} — [{t['page_title']}]({t['page']})_")
            out.append("")

    return "\n".join(out) + "\n"



def main():
    check_only = "--check" in sys.argv
    if not os.path.isdir(BRAIN):
        print("erro: brain/ nao encontrado — rode a partir da raiz do Maestro", file=sys.stderr)
        return 2

    pages = collect()
    good = {k: v for k, v in pages.items() if not v.get("_nofm")}
    forward, backlinks, broken = build_links(good)

    src_fp = hashlib.sha256()
    for rel in sorted(good):
        src_fp.update(rel.encode()); src_fp.update(read(os.path.join(BRAIN, rel)).encode())
    fingerprint = src_fp.hexdigest()[:16]
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    owner_pages = [f for f in good.values() if not f["_path"].startswith("accounts/")]
    cases = {}
    for f in good.values():
        ck = case_key(scope_of(f))
        if ck:
            cases.setdefault(ck, []).append(f)

    outputs = {}

    # --- arvore de pastas -----------------------------------------------------
    # Cada pasta indexa as proprias paginas e aponta para as subpastas. Nenhum
    # indice repete o conteudo do de baixo: e por isso que a raiz cabe numa tela.
    folders, subfolders = {}, {}
    for fm in good.values():
        d = os.path.dirname(fm["_path"])
        folders.setdefault(d, []).append(fm)
        parts = d.split("/") if d else []
        for n in range(len(parts)):
            parent = "/".join(parts[:n])
            child = "/".join(parts[:n + 1])
            subfolders.setdefault(parent, set()).add(child)
            folders.setdefault(child, folders.get(child, []))
    folders.setdefault("", folders.get("", []))

    tree_counts = {}
    for f in folders:
        prefix = f + "/" if f else ""
        tree_counts[f] = sum(1 for fm in good.values() if fm["_path"].startswith(prefix))

    def order_for(folder):
        return CASE_ORDER if folder.startswith("accounts/") else OWNER_ORDER

    # Uma pasta so ganha indice proprio quando tem massa para justificar uma
    # pagina. Abaixo disso o indice do pai lista as paginas dela direto: um
    # indice para uma pasta de uma pagina nao ajuda ninguem a achar nada.
    MIN_OWN = 5

    def qualifies(f):
        if not f:
            return True
        # Fronteira de escopo sempre ganha indice, tenha o tamanho que tiver:
        # a raiz de uma conta e de um caso sao os limites que o isolamento e a
        # navegacao por cliente protegem, e uma arvore do topo e como o dono
        # navega. Nao sao pastas quaisquer. Sem a fronteira de conta, uma conta
        # com so a pagina account.md colapsava direto para dentro do caso — a
        # lista de contas em accounts_index.md desaparecia e sobrava so a lista
        # achatada de casos, sem ligacao visivel entre conta e projeto.
        if f == "tasks":
            return False
        if CASE_ROOT.match(f) or ACCOUNT_ROOT.match(f) or "/" not in f:
            return True
        if len(folders.get(f, [])) >= MIN_OWN:
            return True
        # Pasta de passagem — um unico filho que qualifica — colapsa: o pai
        # aponta direto para o neto. Um indice de 400 bytes que so repassa
        # nao ajuda a achar nada.
        return sum(1 for s in subfolders.get(f, set()) if qualifies(s)) >= 2

    qual = {f for f in folders if qualifies(f)}

    def nearest_index(f):
        """Pasta que responde por f: ela mesma, se qualifica, senao o ancestral."""
        while f and f not in qual:
            f = os.path.dirname(f)
        return f

    absorbed = {}
    for fm in good.values():
        home = nearest_index(os.path.dirname(fm["_path"]))
        absorbed.setdefault(home, []).append(fm)

    for folder in sorted(qual):
        if not folder:
            continue
        own = absorbed.get(folder, [])
        def reachable(f):
            """Descendentes que qualificam, pulando pastas de passagem."""
            out = []
            for s in sorted(subfolders.get(f, set())):
                out += [s] if s in qual else reachable(s)
            return out
        subs = reachable(folder)
        if not own and not subs:
            continue

        # Conta e caso sao arquivo unico quando a curadoria existe: sem
        # indice gerado a parte. Conta nao publica nada (o '## Cases' dentro
        # da propria pagina e mantido a mao); caso cola a secao gerada dentro
        # do proprio brief via splice_generated. Sem curadoria ainda (caso
        # novo sem brief escrito), cai no render_folder de sempre — rede de
        # seguranca, nao o caminho esperado.
        cur = curated_index(folder) if (ACCOUNT_ROOT.match(folder) or CASE_ROOT.match(folder)) else None
        if cur and ACCOUNT_ROOT.match(folder):
            continue
        if cur and CASE_ROOT.match(folder):
            own_wo_cur = [fm for fm in own if fm["_path"] != cur]
            body = render_case_section(folder, own_wo_cur, subs, order_for(folder),
                                        nearest_index(os.path.dirname(folder)))
            outputs[cur] = splice_generated(read(os.path.join(BRAIN, cur.replace("/", os.sep))), body)
            continue

        outputs[index_name(folder)] = render_folder(
            folder, own, subs, order_for(folder), backlinks, fingerprint, tree_counts,
            nearest_index(os.path.dirname(folder)))

    # --- raiz -----------------------------------------------------------------
    trees = [t for t in sorted(subfolders.get("", set())) if t in qual]
    # tasks/ nao tem pagina de conteudo — so a visao gerada — entao nao entra pela
    # travessia normal. Mas e uma pasta do brain e o dono navega por ela, entao
    # aparece na lista de arvores com ponteiro direto para a visao.
    if os.path.isdir(os.path.join(BRAIN, "tasks")):
        trees = sorted(trees + ["tasks"])
    case_rows = []
    for (acct, case), items in sorted(cases.items()):
        closed = sum(1 for f in items if f.get("status") == "closed")
        state = "encerrado" if closed > len(items) / 2 else "ativo"
        d = f"accounts/{acct}/cases/{case}"
        case_rows.append(f"- **{acct} / {case}** — {state} — "
                         f"[índice]({index_name(d)})")
    outputs["brain_index.md"] = render_root(trees, case_rows, list(good.values()),
                                            backlinks, fingerprint, tree_counts)

    all_tasks = stamp_task_age(extract_tasks(good), good, persist=not check_only)
    outputs["tasks/tasks.md"] = render_tasks(all_tasks)


    contract_gaps = []
    for rel, fm in pages.items():
        if fm.get("_nofm"):
            contract_gaps.append({"page": rel, "missing": ["frontmatter"]})
            continue
        miss = [k for k in REQ if not fm.get(k)]
        if miss:
            contract_gaps.append({"page": rel, "missing": miss})

    orphans = sorted(rel for rel in good
                     if not backlinks.get(rel)
                     and good[rel].get("type") not in ("daily", "memory-l1", "memory-l2",
                                                       "memory-l3", "memory-lifetime",
                                                       "owner-facet", "operating"))
    today = datetime.date.today()
    stale = []
    for rel, fm in good.items():
        if fm.get("status") == "closed":
            continue
        try:
            age = (today - datetime.date.fromisoformat(fm.get("updated", ""))).days
        except ValueError:
            continue
        if age > 60 and fm.get("type") in ("task-list", "project-brief", "objectives", "operating"):
            stale.append({"page": rel, "days": age, "type": fm.get("type")})

    diagnostics = {
        "generated_at": now, "source_fingerprint": fingerprint,
        "pages_total": len(pages), "pages_indexed": len(good),
        "links_forward": sum(len(v) for v in forward.values()),
        "links_backlinked_pages": len(backlinks),
        "broken_links": broken,
        "orphans": orphans,
        "contract_gaps": contract_gaps,
        "stale_open_work": sorted(stale, key=lambda x: -x["days"]),
        "scopes": {"owner": len(owner_pages),
                   **{f"{a}/{c}": len(v) for (a, c), v in sorted(cases.items())}},
    }

    if check_only:
        print(json.dumps({k: v for k, v in diagnostics.items() if k != "contract_gaps"},
                         ensure_ascii=False, indent=2))
        print(f"\ncontract_gaps: {len(contract_gaps)}")
        return 0

    # --- publicacao atomica: escreve em staging, valida, só entao move --------
    staging = tempfile.mkdtemp(prefix="brain-index-")
    try:
        for rel, content in outputs.items():
            sp = os.path.join(staging, rel.replace("/", os.sep))
            os.makedirs(os.path.dirname(sp), exist_ok=True)
            io.open(sp, "w", encoding="utf-8", newline="\n").write(content)
        for rel in outputs:
            sp = os.path.join(staging, rel.replace("/", os.sep))
            if os.path.getsize(sp) < 200:
                raise RuntimeError(f"saida suspeita (muito curta): {rel}")
        for rel, content in outputs.items():
            dp = os.path.join(BRAIN, rel.replace("/", os.sep))
            os.makedirs(os.path.dirname(dp), exist_ok=True)
            shutil.copyfile(os.path.join(staging, rel.replace("/", os.sep)), dp)
    finally:
        shutil.rmtree(staging, ignore_errors=True)

    # Coleta de lixo: indice de pasta que esta no disco mas nao foi gerado nesta
    # rodada nao qualifica mais — a pasta encolheu, foi renomeada ou sumiu. Sem
    # isso a camada de navegacao so cresce, e um indice velho segue apontando
    # para paginas que nao existem. So remove o que casa com o padrao do gerado;
    # index.md escrito a mao nunca e tocado.
    removed = []
    for _p in glob.glob(os.path.join(BRAIN, "**", "*_index.md"), recursive=True):
        _rel = _p.replace(os.sep, "/")[len(BRAIN) + 1:]
        _d, _b = os.path.split(_rel)
        if not _d or _b != index_basename(_d):
            continue
        if _rel not in outputs:
            try:
                os.remove(_p)
                removed.append(_rel)
            except OSError:
                pass


    # Publicacao atomica em todos os arquivos de maquina — ver write_atomic.
    # Estes sao lidos por outros processos (roteador, hooks) enquanto este grava;
    # o route-index sozinho tem ~2 MB, e meio arquivo em disco derruba o
    # brain-route dentro de um hook que engole stderr.
    os.makedirs(MACHINE, exist_ok=True)
    write_atomic(os.path.join(MACHINE, "backlinks.json"),
                 json.dumps(backlinks, ensure_ascii=False, indent=2) + chr(10))
    write_atomic(os.path.join(MACHINE, "forward-links.json"),
                 json.dumps(forward, ensure_ascii=False, indent=2) + chr(10))
    write_atomic(os.path.join(MACHINE, "diagnostics.json"),
                 json.dumps(diagnostics, ensure_ascii=False, indent=2) + chr(10))

    # Indice de roteamento: um por escopo, para o hook nunca cruzar caso.
    routes = {"owner": build_route_index(owner_pages)}
    for (acct, case), items in sorted(cases.items()):
        routes[f"{acct}/{case}"] = build_route_index(items)

    write_atomic(os.path.join(MACHINE, "tasks.json"),
                 json.dumps(all_tasks, ensure_ascii=False, indent=2) + chr(10))
    write_atomic(os.path.join(MACHINE, "route-index.json"),
                 json.dumps(routes, ensure_ascii=False) + chr(10))


    # Log so recebe entrada quando algo MUDOU. O hook do Stop dispara ao fim de
    # cada turno do assistente, nao por sessao, entao um append incondicional
    # escrevia ~297 bytes por turno — centenas de entradas "nada mudou" por dia,
    # e um arquivo que so cresce. O fingerprint ja existia e nao era consultado
    # para isto: se ele repete, a compilacao nao produziu diferenca nenhuma e nao
    # ha o que registrar.
    logp = os.path.join(MACHINE, "log.md")
    prev_fp = ""
    try:
        with io.open(logp, encoding="utf-8", errors="replace") as fh:
            for ln in fh:
                if ln.startswith("* fingerprint `"):
                    prev_fp = ln.split("`")[1]
    except OSError:
        pass
    if prev_fp != fingerprint:
        if not os.path.exists(logp):
            io.open(logp, "w", encoding="utf-8", newline="\n").write(
                "# Log de compilação do índice\n\nAppend-only. "
                "Uma entrada por compilação que mudou algo.\n")
        with io.open(logp, "a", encoding="utf-8", newline="\n") as fh:
            fh.write(f"\n## {now}\n\n"
                     f"* fingerprint `{fingerprint}` — {len(good)} páginas indexadas, "
                     f"{len(outputs)} índices publicados.\n"
                     f"* links: {sum(len(v) for v in forward.values())} de saída, "
                     f"{len(backlinks)} páginas com entrada.\n"
                     f"* diagnóstico: {len(broken)} link(s) quebrado(s), {len(orphans)} órfã(s), "
                     f"{len(contract_gaps)} lacuna(s) de contrato, {len(stale)} item(ns) parado(s), "
                     f"{len(removed)} indice(s) obsoleto(s) removido(s)." + chr(10))

    _gc = f", {len(removed)} obsoleto(s) removido(s)" if removed else ""
    print(f"publicados {len(outputs)} índices{_gc}  |  fingerprint {fingerprint}")
    print(f"  paginas indexadas: {len(good)}")
    print(f"  links de saida:    {sum(len(v) for v in forward.values())}")
    print(f"  paginas com backlink: {len(backlinks)}")
    print(f"  quebrados={len(broken)}  orfas={len(orphans)}  "
          f"lacunas={len(contract_gaps)}  parados={len(stale)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
