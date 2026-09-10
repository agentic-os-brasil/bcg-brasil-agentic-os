#!/usr/bin/env python3
"""Verifica se todo caminho citado pelo Maestro existe de verdade.

Existe por causa de um padrao de bug, nao por gosto de teste. A migracao de
2026-09-01 mudou o disco e as skills mas nao o first-run-scaffold.sh; o refactor
de 2026-09-05 subiu quatro arvores para o topo do brain e sete skills continuaram
apontando para `owner/daily/`, `owner/craft/`, `owner/development/` e
`owner/learnings/`, que deixaram de existir. Nos dois casos o codigo estava
correto e o caminho tinha morrido — e nada reclamou, porque nada verificava.

O que faz: le todo caminho entre crases em skills, agents, hooks e CLAUDE.md e
confere contra o disco. Segmento-template (`<case-id>`, `{slug}`, `${VAR}`) vira
glob. Arquivo que ainda nao existe passa se a pasta-mae existir — a skill vai
cria-lo. Pasta-mae que nao existe e o achado.

Uso:  python3 bundles/base/tools/paths-check.py            # so o que quebrou
      python3 bundles/base/tools/paths-check.py --all      # tudo que foi visto
      python3 bundles/base/tools/paths-check.py --count    # so o numero, para hook
Saida: 0 limpo, 1 com achado. Sempre com PYTHONIOENCODING=utf-8 no Windows.
"""
import io, os, re, sys, glob

SOURCES = [
    "bundles/*/skills/*/SKILL.md",
    "bundles/*/agents/*/AGENT.md",
    # `bundles/*/agents/templates/*/AGENT.md` saiu daqui em 2026-09-06 junto com
    # a pasta. Os 8 templates eram uma geracao morta — 4 deles declarados
    # `compatibility_only` ou `deprecated_rejected` pelo proprio catalogo — e
    # nenhum era lido por nada. Esta linha os fazia parecer saudaveis: verificava
    # que o caminho existia, que e a pergunta errada. Existir nao e ser usado.
    ".claude/agents/*.md",
    ".claude/hooks/*.sh",
    "CLAUDE.md",
]

# Caminho entre crases que comece por uma raiz que o projeto de fato tem. Sem a
# ancora, a varredura pega `git status`, `--check` e meia duzia de falsos.
PATH_RE = re.compile(r"`((?:brain|bundles|schemas|\.claude|\.maestro)/[^`\s]+)`")
# Formas de template que aparecem no corpus: <case-id>, {slug}, ${VAR} nas
# skills; %s nos printf dos hooks; ** nos globs dos agentes. Todas viram *.
TPL_RE = re.compile(r"<[^>]+>|\{[^}]+\}|\$\{[^}]+\}|%[sd]|\*\*")

# Fora de escopo nesta release: a ingestao de SharePoint e o cursor do
# learn-from-logs escrevem em arvore que so nasce no primeiro uso. Nao e
# caminho morto, e feature desligada.
IGNORE_PREFIX = ("brain/knowledge/", "brain/memory/sharepoint-rationales/",
                  "brain/memory/learn-from-logs/")


def resolve(p):
    """True se o caminho existe, ou se a pasta que o conteria existe."""
    p = p.rstrip("/")
    if p.startswith(IGNORE_PREFIX):
        return True
    if TPL_RE.search(p):
        g = TPL_RE.sub("*", p)
        if glob.glob(g) or glob.glob(os.path.dirname(g) or "."):
            return True
        # Nada casou. Duas coisas muito diferentes produzem isso, e confundi-las
        # fazia toda instalacao NOVA nascer com 30 "caminhos mortos":
        #
        #   (a) template ainda nao instanciado — `brain/accounts/<account-id>/
        #       cases/<case-id>/canon/` nao casa nada enquanto nao existe a
        #       primeira conta. Nao e caminho morto, e uma pasta que o dono ainda
        #       nao criou. Como o resultado entra na saude como ERRO, o primeiro
        #       contato com o produto era um alerta falso que nunca sumia.
        #   (b) template errado — `owner/daily/<x>.md` depois que `daily/` subiu
        #       para o topo do brain. Esse e o achado que esta ferramenta existe
        #       para pegar, e ele tem de continuar sendo pego.
        #
        # A distincao se faz em duas perguntas, nesta ordem. A primeira versao
        # desta correcao fez so a segunda e ficou cega: quando o primeiro
        # segmento variavel e o ULTIMO, o teste de ocupantes vira o mesmo glob
        # que ja falhou duas linhas acima, e o resultado era `True`
        # incondicional. `brain/owner/daily/<data>.md` — uma das quatro arvores
        # que o refactor moveu, e o motivo de esta ferramenta existir — voltou a
        # passar.
        parts = g.split("/")
        first_wild = next((i for i, s in enumerate(parts) if "*" in s), -1)
        if first_wild < 0:
            return False

        # 1. A pasta-mae LITERAL, antes de qualquer curinga, existe? Se nao, o
        #    caminho esta morto independentemente do que o template resolveria.
        #    E aqui que `owner/daily/`, `owner/craft/` e `atlas/` sao pegos.
        literal = "/".join(parts[:first_wild])
        if literal and not os.path.isdir(literal):
            return False

        # 2. So o nome do arquivo e template: a pasta existe, a skill vai criar.
        if first_wild == len(parts) - 1:
            return True

        # 3. O nivel variavel tem ocupante? Sem nenhum, a arvore ainda nao foi
        #    instanciada e nao ha o que julgar — instalacao nova. Com ocupante,
        #    o glob completo falhou por outro motivo, e ai e achado de verdade.
        return not glob.glob("/".join(parts[:first_wild + 1]))
    return os.path.exists(p) or os.path.isdir(os.path.dirname(p) or ".")


def main():
    argv = sys.argv[1:]
    show_all = "--all" in argv
    count_only = "--count" in argv          # modo para hook: so o numero
    files = sorted({f for pat in SOURCES for f in glob.glob(pat)})
    bad, seen = {}, 0
    for f in files:
        try:
            s = io.open(f, encoding="utf-8", errors="replace").read()
        except OSError:
            continue
        for p in sorted(set(PATH_RE.findall(s))):
            seen += 1
            if not resolve(p):
                bad.setdefault(f.replace("\\", "/"), []).append(p)

    if count_only:
        print(sum(len(v) for v in bad.values()))
        return 1 if bad else 0
    if show_all:
        print(f"{len(files)} arquivos varridos, {seen} caminhos citados.")
    if not bad:
        print(f"paths-check: OK — {seen} caminhos, nenhum morto.")
        return 0
    print(f"paths-check: {sum(len(v) for v in bad.values())} caminho(s) morto(s) "
          f"em {len(bad)} arquivo(s).\n")
    for f, ps in sorted(bad.items()):
        print(f)
        for p in ps:
            print(f"    {p}")
    print("\nHipotese default: o caminho existia e foi movido. Confira o layout "
          "atual antes de assumir que a skill esta errada.")
    return 1


if __name__ == "__main__":
    sys.exit(main())
