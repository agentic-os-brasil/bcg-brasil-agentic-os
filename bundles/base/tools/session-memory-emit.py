#!/usr/bin/env python3
"""Emissor dos blocos de memoria do SessionStart.

Substitui quatro laços por-arquivo em `session-start-memory-inject.sh`
(`emit_all_files`, `emit_titles`, `emit_l3_projection`, `emit_latest_file`) e o
corte por orcamento (`cap_and_emit`).

**Por que existe.** O hook levava 35-39 s antes da primeira resposta da sessao.
Nada nele era lento: eram ~350 processos externos a ~100 ms cada no Cygwin —
`sed` uma vez por arquivo em `emit_titles`, `basename`+`grep`+`awk` por arquivo
em `emit_all_files`. Perfilado com `bash -x`: 763 passos de shell. Aqui e um
spawn so.

Tres defeitos foram corrigidos junto, porque viviam exatamente nas funcoes
reescritas:

  1. `emit_latest_file` fazia `find | sort | tail -1` e devolvia
     `weekly_index.md` — digito ordena antes de letra. A sessao recebia o indice
     gerado da pasta no lugar da sintese da semana, e o mesmo valia para
     `medium-term/`. `emit_all_files` ja tinha a guarda contra pagina gerada;
     este emissor nao a recebeu. A exclusao agora e compartilhada por todos, e a
     ordenacao continua sendo por NOME — que e a certa, porque os arquivos sao
     `2026-W36.md` e `2026-09-05.md`. Ver a nota em `emit_latest_file`: trocar
     para mtime foi tentado e devolveu a semana errada.
  2. O envelope registrava o tamanho de ANTES do corte, entao `over_budget`
     nunca podia significar "vazou" — so "a fonte e maior que o teto". Agora ha
     `bytes` (emitido) e `source_bytes` (origem), e os dois sinais existem
     separados: um mede o envelope, o outro mede o crescimento da fonte.
  3. O corte usava `awk length($0)`, que conta CARACTERES sob locale UTF-8. Com
     prosa em portugues cheia de acento, o corte deixava passar ate ~10% a mais
     de bytes do que o teto. Aqui a conta e em bytes, sempre.

Uso:
  session-memory-emit.py --brain DIR --envelope FILE --budgets k=v,... \\
                         --blocks self,lifetime,l3,learnings,craft
  session-memory-emit.py ... --blocks l2,l1 --finalize --out FILE --cap-total N

`--finalize` grava o JSON do envelope a partir do arquivo acumulado.
Fail-open: qualquer erro sai 0 sem emitir o bloco. Um SessionStart sem memoria
e ruim; um SessionStart travado e pior.
"""
import io
import json
import os
import sys
import argparse
import datetime

# Pagina gerada pelo compilador de indice nao e conteudo do dono. A regra vale
# para todos os emissores — foi por faltar aqui que a memoria semanal sumiu.
SKIP_NAMES = {"README.md", "index.md"}
GENERATED_MARK = "Página gerada"


def is_generated(path, name):
    if name in SKIP_NAMES or name.endswith("_index.md"):
        return True
    try:
        with io.open(path, encoding="utf-8", errors="replace") as f:
            return GENERATED_MARK in f.read(400)
    except OSError:
        return True


def read_text(path):
    try:
        with io.open(path, encoding="utf-8", errors="replace") as f:
            return f.read()
    except OSError:
        return ""


def frontmatter_title(text, fallback):
    """Titulo do frontmatter, sem abrir mao de arquivo sem frontmatter."""
    lines = text.split("\n")
    if not lines or lines[0].strip() != "---":
        return fallback, False
    for ln in lines[1:]:
        if ln.strip() == "---":
            break
        if ln.startswith("title:"):
            t = ln[6:].strip().strip('"').strip("'")
            if t:
                return t, True
    return fallback, True


def md_files(dirpath, recursive=False):
    out = []
    try:
        if recursive:
            for root, _dirs, names in os.walk(dirpath):
                for n in names:
                    if n.endswith(".md"):
                        out.append(os.path.join(root, n))
        else:
            for n in os.listdir(dirpath):
                p = os.path.join(dirpath, n)
                if n.endswith(".md") and os.path.isfile(p):
                    out.append(p)
    except OSError:
        return []
    return sorted(out)


# --------------------------------------------------------------------------
# Emissores
# --------------------------------------------------------------------------

def emit_all_files(label, dirpath):
    """Corpo de todo *.md da pasta — usado por SELF e pela memoria lifetime.

    Emite o CORPO, nao o arquivo: frontmatter, procedencia e cabecalho duplicado
    existem para o compilador e para quem navega, nao para a sessao.
    """
    files = [p for p in md_files(dirpath)
             if not is_generated(p, os.path.basename(p))]
    if not files:
        return ""
    parts = ["\n## %s\n" % label]
    for p in files:
        base = os.path.basename(p)
        text = read_text(p)
        stem = base[:-3]
        # Fallback é o stem, não o `base`: com `base`, uma página sem `title:`
        # saía como `### sem-fm.md · sem-fm.md`, porque o teste de duplicação
        # compara com o stem. `emit_titles` já usava o stem — era inconsistência
        # dentro do mesmo arquivo.
        title, had_fm = frontmatter_title(text, stem)
        body, blank = [], True
        lines = text.split("\n")
        if had_fm:
            end = 0
            for i, ln in enumerate(lines[1:], 1):
                if ln.strip() == "---":
                    end = i
                    break
            lines = lines[end + 1:]
        for ln in lines:
            s = ln.strip()
            if ln.startswith("# "):
                continue
            if s == "## Current":
                continue
            if len(s) > 1 and s.startswith("_") and s.endswith("_"):
                continue  # linha de procedencia
            if not s:
                if blank:
                    continue
                blank = True
            else:
                blank = False
            body.append(ln)
        # Cabeçalho só sai se houver corpo. Numa instalação virgem as 10 facetas
        # SELF são placeholders cujo conteúdo inteiro é a linha de procedência,
        # que este emissor descarta — o resultado eram 10 subtítulos com 9
        # completamente vazios, ~450 bytes de ruído no bloco de maior prioridade
        # da primeira sessão do dono.
        text_body = "\n".join(body).strip("\n")
        if not text_body:
            continue
        parts.append("\n### %s\n" % title if title == stem
                     else "\n### %s · %s\n" % (title, base))
        parts.append(text_body + "\n")
    # Só o cabeçalho da seção e nada abaixo dele não é informação.
    return "".join(parts) if len(parts) > 1 else ""


def emit_titles(label, dirpath, note):
    """Uma linha por pagina, titulo so, arvore inteira.

    Aprendizados e craft precisam estar em contexto ANTES do pedido: "nunca
    automatizar Office via COM" tem de estar la antes da tentativa, e nenhum
    roteador buscaria isso quando o pedido e "gera esse slide".
    """
    files = [p for p in md_files(dirpath, recursive=True)
             if not (os.path.basename(p) in SKIP_NAMES
                     or os.path.basename(p).endswith("_index.md"))]
    if not files:
        return ""
    parts = ["\n## %s\n" % label]
    if note:
        parts.append(note + "\n")
    for p in files:
        base = os.path.basename(p)
        title, _ = frontmatter_title(read_text(p), base[:-3])
        parts.append("- %s\n" % title)
    return "".join(parts)


def emit_l3_projection(label, dirpath):
    """Projecao compacta do medio prazo: titulo e ponteiro, nunca o corpo."""
    files = md_files(dirpath)
    gen = 0
    for p in files:
        base = os.path.basename(p)
        if not base.startswith("g") or "-" not in base:
            continue
        n = base[1:base.index("-")]
        if n.isdigit() and int(n) > gen:
            gen = int(n)
    if gen == 0:
        return ""
    pref = "g%d-" % gen
    sel = [p for p in files if os.path.basename(p).startswith(pref)]
    if not sel:
        return ""
    parts = ["\n## %s\n" % label,
             "<!-- projecao: titulos e ponteiros; leia o arquivo para a evidencia -->\n"]
    for p in sel:
        base = os.path.basename(p)
        title, _ = frontmatter_title(read_text(p), base)
        parts.append("- **%s** -- `memory/medium-term/%s`\n" % (title, base))
    return "".join(parts)


def emit_latest_file(label, dirpath):
    """Conteudo da pagina mais recente da pasta.

    Ordena por NOME, e isso e deliberado: os arquivos destas pastas sao
    `2026-W36.md` e `2026-09-05.md`, nomeados justamente para que a ordem
    lexicografica seja a ordem cronologica. mtime nao serve — uma consolidacao
    que reescreve treze semanas de uma vez deixa os mtimes em ordem arbitraria,
    e foi exatamente o que aconteceu ao testar esta correcao: W35 venceu W36.

    O bug nunca foi a ordenacao. Era a falta da exclusao de pagina gerada, que
    `emit_all_files` tinha e este emissor nao: `weekly_index.md` ordena depois de
    `2026-W36.md` (digito antes de letra) e ganhava o `tail -1`. A sessao recebia
    o indice da pasta no lugar da sintese da semana — em toda sessao, desde que a
    camada passou a existir.
    """
    files = [p for p in md_files(dirpath)
             if not is_generated(p, os.path.basename(p))]
    if not files:
        return ""
    latest = files[-1]
    text = read_text(latest)

    # Frontmatter fora, cabeçalhos rebaixados. Duas razões, ambas medidas:
    #
    #   - L2 e L1 são justamente os dois blocos que estouram o teto e são
    #     cortados. O bloco L2 começava com ~25 linhas de YAML (`period`,
    #     `range`, `sources`, `generator`), então o corte comia o FIM da síntese
    #     para preservar metadado que não roteia nada. `emit_all_files` já tinha
    #     resolvido isso para SELF e lifetime; este emissor não.
    #   - Os `##` internos da página colidiam com o nível dos cabeçalhos do
    #     próprio hook: o SessionStart mostrava 19 seções onde 15 foram emitidas,
    #     e um `## Decisões` da página parecia seção do contexto da sessão.
    lines = text.split("\n")
    if lines and lines[0].strip() == "---":
        for i, ln in enumerate(lines[1:], 1):
            if ln.strip() == "---":
                lines = lines[i + 1:]
                break
    body = "\n".join("#" + ln if ln.startswith("## ") else ln for ln in lines)
    return "\n## %s\n<!-- source: %s -->\n%s" % (
        label, os.path.basename(latest), body.strip("\n") + "\n")


# --------------------------------------------------------------------------
# Corte por orcamento
# --------------------------------------------------------------------------

def cap(body, budget):
    """Corta em limite de LINHA, contando BYTES, com nota do que saiu.

    Um bloco truncado no meio de uma frase mente sobre o que a memoria contem,
    e mentir e pior que cortar. A nota diz quantas linhas ficaram de fora e onde
    esta o integro.
    """
    raw = len(body.encode("utf-8"))
    if not budget or raw <= budget:
        return body, raw, raw
    lines = body.split("\n")

    def note_for(i):
        return ("\n_[%d de %d linhas omitidas pelo teto de contexto. "
                "Íntegro no arquivo de origem.]_\n" % (len(lines) - i, len(lines)))

    # A nota conta para o teto. Sem isto o bloco saia ~20 bytes acima do proprio
    # limite e aparecia como `over_budget` no envelope — um alarme que so
    # apontava para o tamanho do aviso, nunca para o conteudo.
    # Teto menor que a propria nota (~84 bytes): nao ha corte honesto possivel.
    # Emitir a nota assim mesmo faria o bloco exceder o teto que ele existe para
    # respeitar — o unico caminho pelo qual este emissor violava o proprio
    # contrato. Nao alcancavel com os tetos de hoje (o menor e 900), mas eles vem
    # de arquivo e aceitam qualquer inteiro positivo.
    if budget <= len(note_for(0).encode("utf-8")):
        return "", raw, 0

    kept, n, i = [], 0, 0
    for i, ln in enumerate(lines):
        step = len(ln.encode("utf-8")) + 1
        if n + step + len(note_for(i).encode("utf-8")) > budget:
            break
        n += step
        kept.append(ln)
    else:
        return body, raw, raw
    out = "\n".join(kept) + note_for(i)
    return out, raw, len(out.encode("utf-8"))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--brain", required=True)
    ap.add_argument("--envelope", required=True)
    ap.add_argument("--budgets", default="")
    ap.add_argument("--blocks", default="")
    ap.add_argument("--finalize", action="store_true")
    ap.add_argument("--out", default="")
    ap.add_argument("--cap-total", type=int, default=0)
    a = ap.parse_args()

    budgets = {}
    for pair in a.budgets.split(","):
        if "=" in pair:
            k, v = pair.split("=", 1)
            try:
                budgets[k.strip()] = int(v)
            except ValueError:
                pass

    brain = a.brain
    mem = os.path.join(brain, "memory")
    spec = {
        "self":      lambda: emit_all_files("SELF do usuário", os.path.join(brain, "owner", "self")),
        "lifetime":  lambda: emit_all_files("Memória permanente (lifetime)", os.path.join(mem, "lifetime")),
        "l3":        lambda: emit_l3_projection("Trilhas temáticas de médio prazo (L3)", os.path.join(mem, "medium-term")),
        "learnings": lambda: emit_titles("Aprendizados do dono", os.path.join(brain, "learnings"),
                                         "_Regras duras, ja pagas com erro real. Se uma delas se aplica, ela vale._"),
        "craft":     lambda: emit_titles("Metodos e estilo (craft)", os.path.join(brain, "craft"),
                                         "_Como o dono trabalha. Detalhe em brain/craft/._"),
        "l2":        lambda: emit_latest_file("Resumo semanal (L2)", os.path.join(mem, "weekly")),
        "l1":        lambda: emit_latest_file("Último log diário consolidado (L1)", os.path.join(mem, "recent")),
    }

    out = sys.stdout
    try:
        out.reconfigure(encoding="utf-8", newline="\n")
    except Exception:
        pass

    rows = []
    for key in [b.strip() for b in a.blocks.split(",") if b.strip()]:
        fn = spec.get(key)
        if not fn:
            continue
        try:
            body = fn()
        except Exception:
            continue          # fail-open por bloco: um erro nao leva a sessao junto
        if not body:
            continue
        budget = budgets.get(key, 0)
        emitted, raw, emitted_bytes = cap(body, budget)
        out.write(emitted)
        if not emitted.endswith("\n"):
            out.write("\n")
        rows.append("%s %d %d %d" % (key, emitted_bytes, budget, raw))

    if rows:
        try:
            with io.open(a.envelope, "a", encoding="utf-8", newline="\n") as f:
                f.write("\n".join(rows) + "\n")
        except OSError:
            pass

    if a.finalize and a.out:
        write_envelope(a.envelope, a.out, a.cap_total)
    return 0


def write_envelope(src, dst, cap_total):
    """Envelope medido -> brain/.maestro/context-envelope.json.

    E o `context_envelope` que o sensor de apodrecimento do Darwin descreve.
    Agora publica DOIS numeros por bloco: `bytes` e o que de fato entrou na
    sessao, `source_bytes` e o tamanho da origem. O primeiro mede o envelope; o
    segundo preserva o sinal de crescimento da fonte, que era o unico que
    existia antes — e que, publicado sozinho sob o nome do primeiro, fazia
    `over_budget` significar o oposto do que parecia.
    """
    blocks, order, total, src_total = {}, [], 0, 0
    try:
        with io.open(src, encoding="utf-8") as f:
            for line in f:
                parts = line.split()
                if len(parts) != 4:
                    continue
                key, b, budget, raw = parts[0], int(parts[1]), int(parts[2]), int(parts[3])
                blocks[key] = {
                    "bytes": b,
                    "source_bytes": raw,
                    "budget": budget,
                    "ratio": round(b / budget, 3) if budget else None,
                    "over": b > budget > 0,
                    "truncated": raw > b,
                }
                order.append(key)
                total += b
                src_total += raw
    except (OSError, ValueError):
        return

    out = {
        "schema_version": 2,
        "observed_at": datetime.datetime.now(datetime.timezone.utc)
                       .strftime("%Y-%m-%dT%H:%M:%SZ"),
        "source": "session-start-memory-inject.sh",
        "injection_order": order,
        "memory_inject_bytes": total,
        "memory_source_bytes": src_total,
        "memory_inject_budget": cap_total,
        "over_budget": [k for k, v in blocks.items() if v["over"]],
        "truncated": [k for k, v in blocks.items() if v["truncated"]],
        "total_over_budget": bool(cap_total and total > cap_total),
        "blocks": blocks,
        "_nota": ("`bytes` e o que entrou na sessao; `source_bytes` e o tamanho da "
                  "origem. `over_budget` so pode ter entrada se o corte falhar; "
                  "`truncated` e a lista normal de blocos que a origem excedeu. "
                  "Ordem contratada: lifetime -> medium-term (l3) -> weekly (l2) -> "
                  "recent (l1); SELF e as trilhas do dono vem antes por prioridade de "
                  "roteamento, nao por camada. O rollup de skills sai de "
                  "first-run-scaffold.sh e nao esta somado aqui."),
    }
    try:
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        tmp = dst + ".tmp"
        with io.open(tmp, "w", encoding="utf-8", newline="\n") as f:
            json.dump(out, f, ensure_ascii=False, indent=2)
            f.write("\n")
        os.replace(tmp, dst)
    except OSError:
        pass


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception:
        sys.exit(0)          # fail-open: nunca travar o SessionStart
