#!/usr/bin/env python3
"""Migra a workspace `data/` (v0.1.11 e anteriores) para o layout `brain/`.

**Por que existe.** O rename `data/` -> `brain/` foi feito no produto e nao no
ritual de atualizacao. O `README-INSTALL.md` manda copiar a pasta de dados da
versao antiga para dentro da nova; depois do rename ela chega como `data/` num
produto que so enxerga `brain/`. O resultado, para quem ja usa: arvore orfa,
sessao abrindo vazia, e a impressao de ter perdido semanas de trabalho. Nenhuma
mensagem, nenhum erro — o produto simplesmente nao olha para la.

**Invariantes, nesta ordem de prioridade.**

1. **Copia, nunca move.** `data/` fica intacta. Se algo der errado, o estado
   anterior continua no disco, e o dono pode simplesmente apagar `brain/`.
2. **Nunca sobrescreve.** Arquivo que ja existe no destino e preservado e
   contado como `mantido`. Rodar duas vezes nao muda nada.
3. **Recusa ambiguidade em vez de adivinhar.** Se `brain/` ja carrega conteudo
   do dono, a migracao para e reporta. Fundir duas arvores sem saber qual e a
   boa e como se perde trabalho de verdade.
4. **Fail-open.** Qualquer erro sai 0 com relatorio. Uma migracao que nao
   aconteceu e recuperavel; uma sessao travada no start, nao.

**Mapa.** Derivado do backup real pre-refactor, nao de suposicao:

    data/accounts/<a>/cases/<c>/brain/<sub>  -> brain/accounts/<a>/cases/<c>/<sub>
    data/cases/<c>/[brain/]<sub>             -> brain/accounts/_sem-conta/cases/<c>/<sub>
    data/owner/atlas/<arvore>                -> brain/<arvore>
    data/owner/<resto>                       -> brain/owner/<resto>
    data/profile/*.json                      -> brain/owner/*.json
    data/memory                              -> brain/memory
    data/agents|canary|workspaces            -> descartadas (sempre vazias)
    data/README.md                           -> descartado (o scaffold reescreve)

Uso:  migrate-data-to-brain.py --project DIR [--dry-run]
"""
import io
import os
import sys
import shutil
import argparse
import datetime

ATLAS_TREES = ("daily", "craft", "learnings", "people", "development")
DROP_DIRS = ("agents", "canary", "workspaces")
DROP_FILES = ("README.md",)
CASE_SUBS = ("canon", "decisions", "deliverables", "projects", "sources", "tasks")


class Plan:
    def __init__(self):
        self.copies = []      # (src, dst)
        self.skipped = []     # (src, motivo)
        self.notes = []

    def add(self, src, dst):
        self.copies.append((src, dst))


def walk_files(root):
    for base, _dirs, names in os.walk(root):
        for n in names:
            yield os.path.join(base, n)


def rel(path, root):
    return os.path.relpath(path, root).replace("\\", "/")


# craft/index.md e learnings/index.md eram o nome de topo antes de
# 2026-09-07 — colidiam entre si e com o index.md da raiz, indistinguiveis no
# editor. Normalizado aqui, na saida de target_for, em vez de em cada ramo que
# poderia produzi-los: um so lugar continua certo se um novo ramo aparecer.
_LEGACY_TOP_INDEX = {"craft/index.md": "craft/craft.md",
                     "learnings/index.md": "learnings/learnings.md",
                     "people/index.md": "people/people.md"}


def target_for(r, data, brain):
    """Onde um caminho relativo de data/ cai em brain/. None = descartar."""
    t = _target_for_raw(r)
    return _LEGACY_TOP_INDEX.get(t, t) if t else t


def _target_for_raw(r):
    parts = r.split("/")
    head = parts[0]

    if head in DROP_DIRS:
        return None
    if len(parts) == 1 and head in DROP_FILES:
        return None

    # data/profile/*.json -> brain/owner/*.json
    if head == "profile":
        return "owner/" + "/".join(parts[1:])

    # data/owner/atlas/<arvore>/... -> brain/<arvore>/...
    if head == "owner" and len(parts) > 2 and parts[1] == "atlas":
        if parts[2] in ATLAS_TREES:
            return "/".join(parts[2:])
        # Qualquer coisa sob atlas/ que nao seja arvore conhecida vai para
        # owner/, que e onde ela morava conceitualmente. Nao se inventa destino.
        return "owner/" + "/".join(parts[2:])

    # data/accounts/<a>/account.md -> brain/accounts/<a>/<a>.md. `account.md`
    # sozinho colidia entre as 4 contas (mesmo nome, pastas diferentes,
    # indistinguivel no editor); nome da propria pasta desde 2026-09-07.
    if head == "accounts" and len(parts) == 3 and parts[2] == "account.md":
        return "accounts/%s/%s.md" % (parts[1], parts[1])

    # data/accounts/<a>/cases/<c>/brain/<sub>/... -> sem o nivel brain/
    if head == "accounts" and len(parts) > 5 and parts[2] == "cases" and parts[4] == "brain":
        case_id = parts[3]
        sub = parts[5:]
        # brain/projects/<case-id>.md era o brief do caso, embrulhado numa
        # pasta so para ele — o caso ja e o projeto, entao a pasta e o
        # arquivo eram redundantes um do outro. Colapsado direto na raiz do
        # caso desde 2026-09-07 — ver case-agent-setup/SKILL.md. So colapsa
        # no formato exato esperado; qualquer outra coisa sob projects/ (nome
        # de arquivo diferente, mais de um arquivo) passa intacta em vez de
        # arriscar um destino errado por suposicao.
        if len(sub) == 2 and sub[0] == "projects" and sub[1] == case_id + ".md":
            sub = [sub[1]]
        return "accounts/" + "/".join(parts[1:4] + sub)

    # data/cases/<c>/... — layout distribuido, sem camada de conta. Nao da para
    # adivinhar o cliente; vai para uma conta-marcador visivel e o relatorio diz
    # o que fazer. Perder o material seria pior; inventar um nome de cliente
    # tambem.
    if head == "cases" and len(parts) > 2:
        rest = parts[2:]
        if rest and rest[0] == "brain":
            rest = rest[1:]
        return "accounts/_sem-conta/cases/" + "/".join([parts[1]] + rest)

    return r


def scaffold_written(brain):
    """O que o scaffold escreveu, lido do log que ele proprio mantem.

    Manter uma segunda lista a mao era o bug, e os oito nomes eram o sintoma: o
    scaffold ganhou frontmatter nas facetas, `tasks/`, `markitdown.json` e
    `registry.json`, e a lista aqui nao acompanhou. Resultado — abrir a pasta
    nova UMA vez antes de copiar a `data/`, que e literalmente o passo 3 do
    fluxo de instalacao, fazia os proprios placeholders do scaffold contarem
    como "conteudo do dono" e travarem a migracao para sempre.

    O log e `<ts>  WRITE OK  brain/<caminho>`; derivar dele mantem uma fonte so.
    """
    out = set()
    try:
        with io.open(os.path.join(brain, ".scaffold.log"),
                     encoding="utf-8", errors="replace") as f:
            for ln in f:
                if "WRITE OK" not in ln:
                    continue
                tail = ln.split("WRITE OK", 1)[1].strip().split(" ")[0]
                for pref in ("brain/", "data/"):
                    if tail.startswith(pref):
                        out.add(tail[len(pref):])
    except OSError:
        pass
    return out


def looks_like_owner_content(brain):
    """brain/ ja tem conteudo do dono, ou e so o esqueleto recem-criado?

    Pagina gerada, indice, marcador e placeholder do scaffold nao contam. O que
    conta e material que so o dono produz.
    """
    if not os.path.isdir(brain):
        return False
    written = scaffold_written(brain)
    n = 0
    for p in walk_files(brain):
        r = rel(p, brain)
        b = os.path.basename(r)
        # Dotfile em QUALQUER nivel: o teste antigo era sobre o caminho
        # relativo, entao `memory/.gitignore` e `memory/.schema-version`
        # escapavam e contavam como conteudo do dono.
        if r.startswith(".maestro/") or b.startswith(".") or "/." in r:
            continue
        if b in ("README.md", "index.md") or b.endswith("_index.md"):
            continue
        if r in written:
            continue
        # Rede final, para instalacao cujo log se perdeu: os arquivos que o
        # scaffold cria em toda instalacao, por forma e nao por nome exato.
        if r.startswith("owner/self/") or r.startswith("owner/interview/") or r in (
                "development/objectives.md", "owner/operating/work-state.md",
                # craft/index.md e learnings/index.md sao o nome que o scaffold
                # escrevia antes de 2026-09-07; craft/craft.md e
                # learnings/learnings.md sao o nome atual. As duas formas ficam
                # na lista para uma workspace scaffoldada por uma versao
                # anterior nao ser lida como "tem conteudo do dono" so por
                # causa do nome antigo do placeholder.
                "craft/index.md", "learnings/index.md",
                "craft/craft.md", "learnings/learnings.md", "tasks/tasks.md",
                "owner/identity.json", "owner/style.json", "owner/registry.json",
                "owner/onboarding.json", "owner/markitdown.json",
                "owner/observations/observations.jsonl",
                "memory/policies/lifetime.json"):
            continue
        n += 1
    return n > 0


def build_plan(data, brain):
    plan = Plan()
    for src in sorted(walk_files(data)):
        r = rel(src, data)
        # Os dois marcadores de controle da arvore de contas viajam. O filtro de
        # dotfile os descartava junto com o lixo de ferramenta, e o dono perdia o
        # caso ativo na atualizacao: a primeira sessao abria com "nenhum caso
        # ativo" sem que nada no relatorio explicasse por que.
        if r not in ("accounts/.active", "accounts/.pending"):
            if r.startswith(".") or "/." in r:
                continue                  # marcadores e lixo de ferramenta
        t = target_for(r, data, brain)
        if t is None:
            plan.skipped.append((r, "descartado pelo mapa (pasta vazia do layout antigo)"))
            continue
        plan.add(src, os.path.join(brain, t.replace("/", os.sep)))

    # Colisao de destino. O mapa nao e injetivo: profile/identity.json e
    # owner/identity.json caem os dois em owner/identity.json. Antes, o segundo
    # encontrava o arquivo ja escrito e era contado como "ja existia e foi
    # preservado" — o relatorio afirmava preservar algo que a propria execucao
    # criara segundos antes, e um dos dois sumia sem nome. O invariante de
    # recusar ambiguidade valia para a arvore e nao valia para o arquivo.
    seen, deduped = {}, []
    for src, dst in plan.copies:
        if dst not in seen:
            seen[dst] = src
            deduped.append((src, dst))
            continue
        keeper, loser = seen[dst], src
        # owner/ vence profile/: e a arvore para onde o layout novo aponta. O
        # perdedor e preservado ao lado, com sufixo, e nomeado no relatorio.
        if "/profile/" in keeper.replace(os.sep, "/"):
            keeper, loser = loser, keeper
        root, ext = os.path.splitext(dst)
        alt = root + ".da-pasta-profile" + ext
        seen[dst] = keeper
        deduped = [(a, b) for a, b in deduped if b != dst]
        deduped.append((keeper, dst))
        deduped.append((loser, alt))
        plan.notes.append("%s e %s tinham o mesmo destino; o segundo foi preservado como %s"
                          % (os.path.basename(keeper), os.path.basename(loser),
                             os.path.basename(alt)))
    plan.copies = deduped
    return plan


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--project", required=True)
    ap.add_argument("--dry-run", action="store_true")
    a = ap.parse_args()

    proj = a.project
    data = os.path.join(proj, "data")
    brain = os.path.join(proj, "brain")
    done_marker = os.path.join(data, ".migrated-to-brain")

    if not os.path.isdir(data):
        return 0
    if os.path.exists(done_marker):
        return 0

    # Uma `data/` que nao parece workspace do Maestro nao e migrada. Melhor nao
    # fazer nada do que copiar a pasta `data` de outro projeto qualquer.
    if not any(os.path.isdir(os.path.join(data, d))
               for d in ("owner", "memory", "accounts", "cases", "profile")):
        return 0

    if looks_like_owner_content(brain):
        # As duas arvores tem conteudo. Escolher entre elas nao e decisao de
        # script — mas recusar em silencio tambem nao: sem o marcador, o
        # SessionStart nao dizia nada e o dono ficava com a workspace vazia sem
        # saber que havia uma migracao pendente ao lado.
        report(proj, brain, None, blocked=True)
        try:
            os.makedirs(os.path.join(brain, ".maestro"), exist_ok=True)
            io.open(os.path.join(brain, ".maestro", ".migration-blocked"), "w",
                    encoding="utf-8", newline=chr(10)).write("blocked" + chr(10))
        except OSError:
            pass
        print("migracao data/ -> brain/: recusada (as duas arvores tem conteudo)")
        return 0

    plan = build_plan(data, brain)
    copied = kept = 0
    failures = []
    for src, dst in plan.copies:
        if os.path.exists(dst):
            kept += 1
            continue
        if a.dry_run:
            copied += 1
            continue
        try:
            os.makedirs(os.path.dirname(dst), exist_ok=True)
            shutil.copy2(src, dst)
            copied += 1
        except OSError as e:
            failures.append((rel(src, data), e.__class__.__name__))
    # Marcadores de caso ativo do layout pre-contas.
    #
    # `data/cases/.active` guarda so o id do caso ("hapvida-tmo"); o formato
    # novo guarda o par conta/caso, e o conteudo do caso acabou de ser copiado
    # para `accounts/_sem-conta/cases/<caso>`. Copiar o arquivo como esta
    # deixaria um marcador que nao nomeia conta nenhuma — e o guard de
    # isolamento, que agora recusa o que nao consegue verificar, barraria o dono
    # de escrever no proprio caso logo depois de atualizar.
    #
    # Fora do build_plan de proposito: aqui o conteudo muda, nao so o destino, e
    # o plano copia bytes.
    for marker in (".active", ".pending"):
        src = os.path.join(data, "cases", marker)
        dst = os.path.join(brain, "accounts", marker)
        if not os.path.isfile(src) or os.path.exists(dst):
            continue
        try:
            case_id = io.open(src, encoding="utf-8", errors="replace").read().strip()
        except OSError:
            continue
        if not case_id or "/" in case_id:
            continue
        if a.dry_run:
            copied += 1
            continue
        try:
            os.makedirs(os.path.dirname(dst), exist_ok=True)
            io.open(dst, "w", encoding="utf-8", newline=chr(10)).write(
                "_sem-conta/" + case_id + chr(10))
            copied += 1
            plan.notes.append(
                "caso ativo %s migrado como _sem-conta/%s; renomeie a conta quando souber o cliente"
                % (case_id, case_id))
        except OSError as e:
            failures.append(("cases/" + marker, e.__class__.__name__))

    failed = len(failures)
    plan.failures = failures

    # O marcador de conclusao SO e escrito quando nada falhou. Antes era
    # incondicional: dois arquivos perdidos por lock transitorio — rotina em
    # laptop com OneDrive — marcavam a migracao como concluida, e ela nunca mais
    # tentava. A perda ficava invisivel e permanente.
    if not a.dry_run and not failures:
        try:
            io.open(done_marker, "w", encoding="utf-8", newline=chr(10)).write(
                datetime.datetime.now(datetime.timezone.utc)
                .strftime("%Y-%m-%dT%H:%M:%SZ") + "\n")
        except OSError:
            pass
        # Aviso para a sessao seguinte. Paginas da versao anterior nao carregam o
        # frontmatter que o indice exige — sao 168 num brain maduro. Sem este
        # aviso, o dono atualiza e a primeira tela e "corrigir 168 paginas", sem
        # dizer de onde vieram nem que isso e esperado. O marcador se apaga
        # sozinho quando as lacunas chegam a zero.
        try:
            os.makedirs(os.path.join(brain, ".maestro"), exist_ok=True)
            io.open(os.path.join(brain, ".maestro", ".migration-notice"), "w",
                    encoding="utf-8", newline="\n").write("%d\n" % copied)
        except OSError:
            pass

    # --dry-run imprime e nao grava. Antes ele criava .maestro/ e escrevia um
    # relatorio cujo proprio texto dizia "Simulacao — nada foi escrito".
    if not a.dry_run:
        report(proj, brain, (plan, copied, kept, failed), blocked=False)
    print("migracao data/ -> brain/: %d copiado(s), %d mantido(s), %d falha(s)"
          % (copied, kept, failed))
    return 0


def report(proj, brain, result, blocked):
    """Relatorio em brain/.maestro/, legivel pelo dono e pela sessao."""
    path = os.path.join(brain, ".maestro", "migration-report.md")
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    try:
        os.makedirs(os.path.dirname(path), exist_ok=True)
    except OSError:
        return

    if blocked:
        # Nao sobrescrever um relatorio de sucesso: se ja houve migracao, o
        # registro do que foi movido vale mais que o aviso de bloqueio, e o
        # bloqueio tem marcador proprio para o SessionStart surfacar.
        try:
            if os.path.exists(path) and "não executada" not in io.open(
                    path, encoding="utf-8", errors="replace").read(200):
                path = os.path.join(brain, ".maestro", "migration-blocked.md")
        except OSError:
            pass
        body = (
            "# Migração `data/` → `brain/` — não executada\n\n"
            f"_{now}_\n\n"
            "Encontrei uma pasta `data/` da versão anterior **e** conteúdo seu já em\n"
            "`brain/`. Não dá para fundir as duas sem saber qual é a boa, e escolher\n"
            "errado apaga trabalho — então não mexi em nada.\n\n"
            "As duas árvores estão intactas no disco. Peça ao Maestro para comparar as\n"
            "duas e decidir o que fica, ou apague a que você sabe estar velha e abra a\n"
            "sessão de novo: a migração roda sozinha quando só uma existe.\n")
    else:
        plan, copied, kept, failed = result
        sem_conta = sorted({c[1].split(os.sep + "cases" + os.sep)[-1].split(os.sep)[0]
                            for c in plan.copies if "_sem-conta" in c[1]})
        lines = [
            "# Migração `data/` → `brain/`\n",
            f"_{now}_\n",
            ("A pasta `data/` **não foi tocada**: tudo foi copiado, nada movido. "
             "Se algo estiver errado, ela continua lá, inteira." + chr(10)),
            f"- {copied} arquivo(s) copiado(s)",
            f"- {kept} já existiam em `brain/` e foram preservados",
            f"- {failed} falha(s) de cópia" if failed else "- nenhuma falha",
            "\n## O que mudou de lugar\n",
            "| Antes | Agora |",
            "|---|---|",
            "| `data/owner/atlas/daily/` e irmãs | `brain/daily/`, `craft/`, `learnings/`, `people/`, `development/` |",
            "| `data/profile/*.json` | `brain/owner/*.json` |",
            "| `.../cases/<caso>/brain/canon/` | `.../cases/<caso>/canon/` (um nível a menos) |",
            "| `data/agents/`, `canary/`, `workspaces/` | descartadas — estavam sempre vazias |",
        ]
        if getattr(plan, "failures", None):
            lines += [
                "",
                "## Não copiados — a migração NÃO foi marcada como concluída",
                "",
                "Estes arquivos falharam na cópia. A pasta `data/` continua com todos",
                "eles, e a migração **vai tentar de novo** na próxima sessão — por isso",
                "o marcador de conclusão não foi escrito. Se o motivo for arquivo em uso,",
                "fechar o programa que o segura e reabrir o Maestro resolve.",
                "",
            ]
            lines += ["- `%s` — %s" % (r, e) for r, e in plan.failures]
        if plan.notes:
            lines += ["", "## Nomes que colidiram", ""]
            lines += ["- " + n for n in plan.notes]
        if sem_conta:
            lines += [
                "\n## Precisa da sua mão\n",
                "Estes casos vinham sem cliente na versão antiga, então ficaram sob uma",
                "conta-marcador chamada `_sem-conta`. Nada se perdeu — só falta dizer de",
                "quem é cada um:\n",
            ]
            lines += [f"- `{c}`" for c in sem_conta]
            lines += ["\nPeça ao Maestro para mover cada caso para a conta certa."]
        body = "\n".join(lines) + "\n"

    try:
        io.open(path, "w", encoding="utf-8", newline="\n").write(body)
    except OSError:
        pass


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception:
        sys.exit(0)          # fail-open: nunca travar o SessionStart
