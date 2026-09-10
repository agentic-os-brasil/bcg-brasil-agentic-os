#!/usr/bin/env python3
"""Verifica se todo arquivo de configuracao do produto tem quem o leia.

Irmao de paths-check.py, e existe porque aquele responde a pergunta errada. O
paths-check pergunta "este caminho existe?"; o cabecalho dele ja carrega a frase,
escrita quando os 8 templates de agente foram removidos: *"verificava que o
caminho existia, que e a pergunta errada. Existir nao e ser usado."* Este aqui
pergunta **quem le**.

O padrao que ele fecha e o mais caro deste sistema, e ja apareceu quatro vezes so
em 2026-09-06: os templates de agente (8 arquivos, 0 leitores), os tetos de
contexto de `runtime.json` em versoes anteriores, o hook de anuncio que imprimia
num canal que ninguem le, e os sete arquivos que a primeira execucao do darwin
achou — todos empacotados e entregues a todo usuario. Achar um por vez, por
tropeco, custa uma sessao de diagnostico cada. Perguntar "quem le" custa
segundos, e roda no portao de aceite.

## Duas classes de leitor, e a ausencia das duas e o achado

- **code**   — um `.sh` de hook ou um `.py` de tool abre o arquivo em execucao.
- **prompt** — um `SKILL.md`, `AGENT.md` ou o `CLAUDE.md` **manda** o modelo ler,
  com verbo de leitura na mesma linha. `pa-expert-registry.json` so e lido assim,
  e e o desenho certo para ele. Mencao sem verbo nao conta: o `maestro-operator`
  cita `agent-skill-policy.json` como "o mapa papel -> skill" e nao manda ninguem
  abrir — o arquivo continua sem leitor.
- **nenhum** — ninguem le. E o achado.

Nao contam como leitor: o proprio verificador, o `darwin` (que enumera o que pode
AUDITAR — aceitar isso absolveria tudo que ele audita) e os testes (nomear um
caminho numa assercao e estaticamente identico a abri-lo; ver TEST_READERS).

## Como o casamento e feito

Casar segmento solto nao funciona nas duas direcoes ao mesmo tempo, e as duas
falhas foram medidas nesta base antes de chegar a forma atual. Entao o caminho e
**reconstruido**: resolve `NOME = "literal"` do proprio arquivo leitor, remonta
cada `os.path.join(...)` peca por peca (identificador que nao resolve vira `*`),
corta o prefixo variavel e compara por sufixo — o que resolve
`"$PROJECT_DIR/..."` e `os.path.join(ROOT, ...)` de uma vez. Nome de arquivo solto
nunca e evidencia, e caminho que so aparece como valor de JSON e dado escrito,
nao arquivo aberto.

## A catraca

Os orfaos que ja existiam quando isto nasceu estao em KNOWN_ORPHANS, datados e
com a razao: o portao fica verde hoje e qualquer orfao NOVO o derruba. Nascer
vermelho so ensinaria o dono a ignorar vermelho. A divida continua sendo
reportada em toda execucao — registrada, nao absolvida.

Uso:  python3 bundles/base/tools/readers-check.py           # so os orfaos
      python3 bundles/base/tools/readers-check.py --all     # todos, com o leitor
      python3 bundles/base/tools/readers-check.py --count   # so o numero, para hook
Saida: 0 limpo, 1 com achado.
"""
import io
import os
import re
import sys
import glob
import fnmatch

for _s in (sys.stdout, sys.stderr):
    try:
        _s.reconfigure(encoding="utf-8", errors="replace")
    except (AttributeError, ValueError):
        pass

# bundles/base/tools/readers-check.py -> tres niveis ate a raiz do produto.
ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)),
                                    "..", "..", ".."))

# O que precisa de leitor: configuracao declarativa do produto.
TARGETS = ["bundles/**/*.json"]

# Quem conta como leitor de codigo — abre o arquivo em execucao do produto.
#
# O `embed.go` esta na lista por um motivo que so aparece neste repositorio, e
# nao no ZIP: aqui o bundle e compilado dentro do harness Go, e um `//go:embed`
# e leitura de verdade — o arquivo entra no binario e um teste de contrato o
# valida. Sem esta linha, `pa-expert-registry.json` e `memory/runtime.json`
# ficam eternamente reportados como orfaos, os dois lidos por um `embed.go` a
# poucas pastas de distancia. Ferramenta que acusa dois falsos para sempre e
# ferramenta que se aprende a ignorar, que e exatamente o que o cabecalho deste
# arquivo existe para combater.
CODE_READERS = [".claude/hooks/*.sh", "bundles/*/tools/*.py",
                "bundles/*/*/embed.go", "bundles/*/embed.go"]

# Teste NAO conta como leitor, e a razao vale a linha. Primeiro porque ler num
# teste prova que alguem confere o arquivo, nao que ele muda comportamento —
# que e a pergunta inteira aqui. Segundo, e decisivo: um teste que NOMEIA o
# caminho numa asserção e estaticamente identico a um teste que o abre. Com
# tests/ na lista, `tests/test-agent-activation.py` — que existe justamente para
# afirmar que `agents/catalog.json` esta sem leitor — passava a ser o leitor
# dele, e o caso de teste que planta um orfao novo absolvia o proprio orfao que
# plantou (medido: exit 0 onde tinha de ser 1). Mesma circularidade ja excluida
# para o auditor em NOT_A_READER.
TEST_READERS = []

# Quem conta como leitor de prompt — manda o modelo ler.
PROMPT_READERS = [".claude/agents/*.md", "bundles/*/skills/*/SKILL.md",
                  "bundles/*/agents/*/AGENT.md", "CLAUDE.md"]

# O auditor nao conta como leitor. `darwin` enumera, na propria especificacao, o
# estado de sistema que ele pode abrir para AUDITAR — o que inclui quase todo
# arquivo que este verificador fiscaliza. Aceitar isso como leitura tornaria o
# verificador circular: a existencia do auditor absolveria tudo que ele audita, e
# os sete orfaos que ele mesmo achou passariam verdes. Foi o que aconteceu na
# segunda execucao deste arquivo.
NOT_A_READER = {".claude/agents/darwin.md",
                "bundles/base/agents/darwin/AGENT.md"}

# Declarativos por desenho: nao tem leitor em execucao e isso e intencional.
# Toda entrada carrega a razao, porque allowlist sem razao vira o proprio
# problema — uma lista que ninguem revisa e que absolve qualquer coisa nova.
# Rever quando a razao deixar de valer.
DECLARED_NO_READER = {
    "bundles/base/distribution.json":
        "e o mapa de build; quem o le e o make-release.py, que roda a mao e nao "
        "no runtime do produto. Coberto pelo proprio make-release --check.",
    "bundles/base/atlas/managed/backlinks.json":
        "semente vazia do atlas gerido; o artefato vivo e brain/.maestro/backlinks.json, "
        "escrito e lido pelo brain-index.py.",
    "bundles/base/atlas/managed/diagnostics.json":
        "semente vazia do atlas gerido; o artefato vivo e brain/.maestro/diagnostics.json.",
}

# Divida conhecida, 2026-09-06 — NAO e absolvicao. Sao os orfaos que existiam
# quando este verificador nasceu, e o destino de cada um e decisao do dono: dar
# leitor, tirar do produto, ou declarar acima com a razao.
#
# Ficam aqui, e nao em DECLARED_NO_READER, de proposito: misturar "sem leitor por
# desenho" com "sem leitor e ninguem decidiu ainda" apagaria a divida em vez de
# registra-la, e em seis meses ninguem distinguiria as duas.
#
# A catraca e o ponto: com esta lista o portao fica verde hoje e QUALQUER orfao
# novo o derruba. Nascer vermelho so ensinaria a ignorar vermelho.
KNOWN_ORPHANS = {
    "bundles/base/agents/catalog.json":
        "grafo de delegacao autorizado; citado em prosa pelo maestro-operator, "
        "imposto por nada. Mantido em sincronia de schema com o dispatch em Go do "
        "repo-fonte de propósito (ver PROPOSTA-AGENTES-NATIVO-VS-DISPATCH-GO.md).",
    "bundles/base/skills/agent-skill-policy.json":
        "mapa papel -> skill; sem leitor desde que existe. Ja registrado como sem "
        "leitor no diagnostico de 2026-09-05 e ainda assim empacotado.",
    "bundles/base/manifest.json":
        "declaracao do release; nada em execucao o abre.",
    "bundles/base/memory/policy.json":
        "declara context_injection e ordem de camada; quem aplica teto de fato e "
        "memory/runtime.json. Os dois coincidem por acaso, nao por mecanismo.",
    "bundles/base/profile/policy.json": "sem nenhuma referencia, nem em codigo nem em prosa.",
    "bundles/base/release/provider.json": "so citado em pagina de atlas.",
    "bundles/base/runtime/capabilities.json": "sem leitor.",
    "bundles/base/runtime/hook-policy.json":
        "declara may_block / may_use_network / may_call_model por evento — um "
        "contrato de seguranca sobre hooks que nenhum hook consulta.",
    "bundles/base/runtime/maintenance.json": "sem leitor.",
    "bundles/catalog/catalog.json":
        "catalogo de bundles; nem o darwin nem a auditoria manual o tinham achado.",
}

# Comprimento MINIMO 1, e o filtro de tamanho vem depois da extracao. Com
# {2,200} um literal de um caractere nao casava e a regex reiniciava no meio do
# par de aspas: em `os.path.join(BUNDLES, "*", "skills", "catalog.json")` ela
# passava a parear a aspa de FECHO de `*` com a de ABERTURA de `skills`, e
# engolia os literais seguintes. `catalog.json` sumia da extracao, e o
# skill-route.py — que le os dois catalogos em toda mensagem do dono — aparecia
# como nao-leitor deles. Filtrar por tamanho DENTRO do pareamento desalinha o
# pareamento.
STRING_RE = re.compile(r"""["']([^"'\n]{1,200})["']""")
BACKTICK_RE = re.compile(r"`([^`\s]{4,200})`")


def rel(path):
    return os.path.relpath(path, ROOT).replace("\\", "/")


def expand(patterns):
    out = []
    for pat in patterns:
        out.extend(glob.glob(os.path.join(ROOT, pat), recursive=True))
    return sorted(set(p for p in out if os.path.isfile(p)))


def read(path):
    try:
        return io.open(path, encoding="utf-8", errors="replace").read()
    except OSError:
        return ""


def literals_of(text):
    """Literais de string do arquivo, com separador normalizado."""
    return {m.replace("\\\\", "/").replace("\\", "/")
            for m in STRING_RE.findall(text) if len(m) >= 2}


def concrete_filename(lit):
    """O literal nomeia UM arquivo, ou e uma varredura generica?

    Sem esta guarda o casamento por glob absolve tudo: `brain-index.py` carrega o
    literal `**`, que casa qualquer alvo, e o resultado foi a primeira execucao
    deste verificador declarando leitor para os 18 arquivos — inclusive os sete
    que o darwin acabara de provar orfaos. Um verificador que nunca acha nada e
    pior que verificador nenhum: da a garantia sem prestar o servico.

    Glob sobre DIRETORIO continua valendo (`bundles/*/skills/catalog.json` e como
    o skill-route.py chega nos catalogos). Glob sobre NOME DE ARQUIVO nao e
    evidencia de que aquele arquivo especifico e lido.
    """
    last = lit.rstrip("/").split("/")[-1]
    return "*" not in last and "?" not in last and len(last) >= 4


ASSIGN_RE = re.compile(r"""^\s*([A-Z_][A-Z0-9_]*)\s*=\s*["']([^"'\n]+)["']""", re.M)
JOIN_RE = re.compile(r"os\.path\.join\(([^()]*)\)")
# Valor de JSON: `"chave": "valor"`. Ver a razao em path_candidates.
JSON_VALUE_RE = re.compile(r"""["'][\w.\-]+["']\s*:\s*["']([^"'\n]{2,200})["']""")


def path_candidates(text):
    """Todo caminho que este leitor pode abrir, como glob de sufixo.

    Casar segmento solto nao funciona nas duas direcoes ao mesmo tempo, e as duas
    falhas foram medidas nesta base:

    - exigir os segmentos soltos (nome + pai + raiz) ABSOLVIA
      `bundles/catalog/catalog.json`, porque `catalog` aparece no skill-route.py
      como chave de peso (`FIELD_WEIGHT`), nao como diretorio. Ninguem le aquele
      arquivo.
    - casar so o literal inteiro PERDIA
      `"$PROJECT_DIR/bundles/base/memory/runtime.json"`, que e como o
      session-start-memory-inject.sh de fato aponta para o arquivo de tetos que
      ele le em toda sessao.

    Entao o caminho e reconstruido de verdade: resolve `NOME = "literal"` do
    proprio arquivo, remonta cada `os.path.join(...)` peca por peca (identificador
    que nao resolve vira `*`), e corta o prefixo variavel — comparacao por sufixo
    resolve `$PROJECT_DIR/...` e `ROOT/...` de uma vez.
    """
    consts = dict(ASSIGN_RE.findall(text))
    out = set()

    # Caminho que aparece SO como valor de JSON e dado escrito, nao arquivo
    # aberto. O first-run-scaffold.sh grava `"policy_source":
    # "bundles/base/memory/policy.json"` dentro do .schema-version que ele
    # produz — cita o arquivo sem nunca o ler, e aquele arquivo de politica de
    # memoria de fato nao tem leitor nenhum. Contar isso como leitura absolveria
    # justamente o caso que o darwin achou.
    escritos = set()
    for v in set(JSON_VALUE_RE.findall(text)):
        como_valor = len(re.findall(r'"[\w.\-]+"\s*:\s*"%s"' % re.escape(v), text))
        total = text.count('"%s"' % v)
        if total and como_valor == total:      # nunca aparece fora de um valor
            escritos.add(v)

    for lit in literals_of(text):
        if lit in escritos:
            continue
        out.add(lit)

    for args in JOIN_RE.findall(text):
        pecas = []
        for raw in args.split(","):
            raw = raw.strip()
            if not raw:
                continue
            m = re.fullmatch(r"""["']([^"'\n]*)["']""", raw)
            if m:
                pecas.append(m.group(1))
            elif raw in consts:
                pecas.append(consts[raw])
            else:
                pecas.append("*")          # variavel que nao resolve
        if pecas:
            # Corta o prefixo desconhecido: o que vale e o rabo concreto.
            while pecas and pecas[0] == "*":
                pecas.pop(0)
            if pecas:
                out.add("/".join(pecas).replace("\\", "/"))
    return out


def code_reads(target, candidates):
    """Algum caminho reconstruido deste leitor aponta para o alvo?"""
    for cand in candidates:
        if not cand or cand.endswith("/") or not concrete_filename(cand):
            continue
        # Nome solto nao e evidencia. O literal `catalog.json` do skill-route.py
        # casaria, por sufixo, tanto `bundles/base/skills/catalog.json` (que ele
        # le) quanto `bundles/base/agents/catalog.json` e
        # `bundles/catalog/catalog.json` (que ninguem le). Sufixo so vale para
        # candidato que ja traz o caminho — literal completo ou join remontado.
        if "/" not in cand:
            continue
        if (cand == target
                or cand.endswith("/" + target)
                or fnmatch.fnmatch(target, cand)
                or fnmatch.fnmatch(target, "*/" + cand)):
            return True
    return False


# Verbo de leitura perto do caminho. Sem isto, "mencionado" passa por "lido":
# `maestro-operator/SKILL.md` cita `agent-skill-policy.json` como "o mapa papel →
# skill" e nao manda ninguem abrir — o arquivo continua sem leitor, e a mencao o
# absolvia. Mencao nao e leitura.
READ_VERB = re.compile(
    r"\b(le|ler|leia|leitura|lido|lida|carregue|carregar|carregad|consulte|"
    r"consultar|abre|abrir|read|reads|load|loads|loaded|parse|parses)\b",
    re.I)


def prompt_reads(target, text):
    """Um arquivo de prompt MANDA ler este alvo?"""
    name = target.split("/")[-1]
    for line in text.splitlines():
        hits = [m.replace("\\", "/") for m in BACKTICK_RE.findall(line)]
        if not hits:
            continue
        nomeia = any(lit == target
                     or (lit.endswith("/" + name) and target.endswith(lit.lstrip("/")))
                     or fnmatch.fnmatch(target, lit)
                     for lit in hits)
        if nomeia and READ_VERB.search(line):
            return True
    return False


def _candidates_for(p):
    """Candidatos de um leitor de codigo, com uma correcao para `embed.go`.

    A regra geral desta ferramenta e que nome solto nao e evidencia: o literal
    `catalog.json` dentro do skill-route.py nao prova que ele le
    `bundles/catalog/catalog.json`. A guarda existe para isso e esta certa.

    `//go:embed` e a excecao real, nao uma brecha. A diretiva resolve o nome
    RELATIVO A PASTA do proprio arquivo — `//go:embed pa-expert-registry.json`
    dentro de bundles/base/agents/ nomeia aquele arquivo e nenhum outro, e o
    compilador falha se ele nao existir. Entao aqui o nome solto e qualificado
    com a pasta do leitor, o que e exatamente o que o Go faz, em vez de
    afrouxar a regra para todo mundo.
    """
    lits = path_candidates(read(p))
    if os.path.basename(p) != "embed.go":
        return lits
    # `path_candidates` procura literal entre aspas ou crases, e uma diretiva
    # `//go:embed` nao tem nenhuma das duas — e comentario com nomes soltos. Sem
    # ler a diretiva, o nome nunca entra na lista e nenhum sufixo salva depois.
    here = rel(os.path.dirname(os.path.abspath(p)))
    body = read(p)
    embedded = set()
    for line in body.splitlines():
        t = line.strip()
        if not t.startswith("//go:embed"):
            continue
        for name in t[len("//go:embed"):].split():
            embedded.add(name)
            if "/" not in name:
                embedded.add(here + "/" + name)
            else:
                embedded.add(here + "/" + name)
    return lits | embedded

def main():
    argv = sys.argv[1:]
    show_all = "--all" in argv
    count_only = "--count" in argv

    targets = [rel(p) for p in expand(TARGETS)]
    # O proprio verificador nao conta: ele nomeia os padroes que varre, e sem
    # esta exclusao ele se declara leitor de tudo que fiscaliza.
    me = rel(os.path.abspath(__file__))
    code_files = [(rel(p), _candidates_for(p)) for p in expand(CODE_READERS)
                  if rel(p) != me]
    test_files = [(rel(p), path_candidates(read(p))) for p in expand(TEST_READERS)]
    prompt_files = [(rel(p), read(p)) for p in expand(PROMPT_READERS)
                    if rel(p) not in NOT_A_READER]

    rows = []
    for t in targets:
        code = sorted(r for r, lits in code_files if code_reads(t, lits))
        test = sorted(r for r, lits in test_files if code_reads(t, lits))
        prompt = sorted(r for r, txt in prompt_files if prompt_reads(t, txt))
        rows.append((t, code, test, prompt))

    sem_leitor = [r for r in rows
                  if not r[1] and not r[2] and not r[3]
                  and r[0] not in DECLARED_NO_READER]
    # Divida conhecida nao derruba o portao; orfao novo derruba. Ver KNOWN_ORPHANS.
    conhecidos = [r for r in sem_leitor if r[0] in KNOWN_ORPHANS]
    orphans = [r for r in sem_leitor if r[0] not in KNOWN_ORPHANS]

    if count_only:
        print(len(orphans))
        return 1 if orphans else 0

    if show_all:
        for t, c, tst, p in rows:
            if c:
                print(f"  code      {t}\n              <- {', '.join(c)}")
            elif p:
                print(f"  prompt    {t}\n              <- {', '.join(p)}")
            elif tst:
                # Config lida so pelo proprio teste: o teste confere um arquivo
                # que nao muda comportamento nenhum. Passa, mas e o formato de
                # orfao que mais facilmente se disfarca de saudavel.
                print(f"  SO TESTE  {t}\n              <- {', '.join(tst)}"
                      f"\n              (nenhum hook ou tool le este arquivo)")
            elif t in DECLARED_NO_READER:
                print(f"  declarado {t}\n              {DECLARED_NO_READER[t]}")
            elif t in KNOWN_ORPHANS:
                print(f"  DIVIDA    {t}\n              {KNOWN_ORPHANS[t]}")
            else:
                print(f"  ORFAO     {t}")
        print()

    if conhecidos:
        total = sum(os.path.getsize(os.path.join(ROOT, t)) for t, *_ in conhecidos)
        print(f"readers-check: {len(conhecidos)} arquivo(s) de divida conhecida "
              f"({total} bytes empacotados sem leitor). Nao bloqueiam — cada um "
              f"espera decisao do dono: dar leitor, tirar do produto, ou declarar "
              f"como declarativo. Ver KNOWN_ORPHANS.")

    if not orphans:
        print(f"readers-check: OK — {len(targets)} arquivos de configuracao, "
              f"nenhum orfao novo.")
        return 0

    print(f"readers-check: {len(orphans)} arquivo(s) de configuracao sem leitor.")
    print("Nada em .sh, .py, SKILL.md, AGENT.md ou CLAUDE.md le estes arquivos —")
    print("eles sao empacotados e entregues, e nao mudam comportamento nenhum:\n")
    for t, _c, _t, _p in orphans:
        size = os.path.getsize(os.path.join(ROOT, t))
        print(f"  {t}  ({size} bytes)")
    print("\nPara cada um: dar um leitor, remover do produto, ou declarar em")
    print("DECLARED_NO_READER com a razao (em bundles/base/tools/readers-check.py).")
    return 1


if __name__ == "__main__":
    sys.exit(main())
