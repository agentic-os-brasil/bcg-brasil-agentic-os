---
name: feedback-capture
description: Capture formal feedback the owner received into owner development scope, folding project feedback into the live objectives or resetting them on a career-committee review, and compile a self-review pack before a review. Use for "I got feedback", "capture my project feedback", "log my career review", "update my objectives from this", or "prepare my self-review pack".
---

# Feedback Capture

Record what was said to the owner, then apply the effect it actually has on
their objectives. Getting the capture right matters less than getting the
effect right: the two feedback events are not the same kind of event.

All reads and writes use direct file operations on the owner atlas paths (`data/owner/atlas/`). Never skip the confirmation gate or edit atlas files directly outside the skill's write sequence.

## Interaction profile

Resolve `interaction-profile` before presenting the capture. The reads, the
writes, the bounds and the confirmation behaviour never vary by profile; only
the explanation and optional detail do.

- `standard`: what was captured, and what changed in the objectives.
- `advanced`: add why each objective folded, merged, survived or retired, and
  what the previous cycle looked like.
- `power`: add the pages collected and their revisions, the operation and
  idempotency key behind each write, and every part of the feedback that was
  captured but produced no objective change.

## Establish the artifact first

Two artifacts, two different effects. Establish which one this is before
writing anything, and ask when the answer is not explicit.

| | Project feedback | Career-committee review |
| --- | --- | --- |
| Page | `owner/development/project-feedback/<YYYY-MM-DD>-<project-slug>.md` | `owner/development/cdc/<YYYY-MM-DD>-cdc.md` |
| Cadence | Once per project round | Roughly every six months |
| What it is | One case team's view of one period | The career-level synthesis |
| Effect | **Folds into** the live objectives | **Resets** them |

- **A fold** adds evidence to the objectives that already exist, merges where
  the underlying pattern is the same, and may propose at most one new
  objective. It never retires anything, however strongly the feedback is
  worded. It is evidence, not a verdict.
- **A reset** opens a new cycle: retire what the committee reads as mastered,
  promote the committee's priorities, and update the next review date. Retiring
  is not deleting — a retired objective keeps its statement and its full
  evidence log, marked retired and pointed at the review that retired it.

Never infer a reset from a project's feedback.

## Whose page this is

The subject of every page written here is **the owner's own performance**.

Feedback names who gave it, and that attribution is admitted: it is what makes
the record verifiable and lets the owner weigh it later. The name stays
personal metadata even so — record the minimum that makes the source
identifiable and no more.

Attribution is where it stops. No assessment of the giver is recorded, no
inferred trait, score or rating is produced from their words, and feedback
concerning anyone other than the owner is not admitted at all. The test is
whose conduct the page describes.

## Inputs

- The feedback text, supplied by the owner in session, close to its source.
- Obtained with `collect`, purpose declared and pages named:
  `owner/development/objectives.md`; the prior captures the owner points at;
  and the retrospectives since the last review. There is no whole-root read and
  no folder listing, so name the pages. An absent page is reported as an
  omission — a first capture has no predecessor, and that is not an error.

Keep the revision of each page read. A later write uses it to notice that the
owner edited the page in the meantime.

## Workflow

1. Establish the artifact. Ask if it is not explicit.
2. Capture the feedback with `create-page` on that artifact's shape, as close
   to the owner's source wording as possible. Do not paraphrase the substance
   away, and add no assessment of your own. Separate what was received from the
   owner's own reading of it, and keep the reading in the owner's voice.
3. Compute the effect — a fold, or a reset — and show the whole proposed change
   before any of it is applied.
4. Apply only what the owner confirms, one item at a time:
   - **evidence** goes under the objective's own evidence heading with
     `append-entry`. `append-entry` refuses a heading that appears more than
     once on a page, and an objectives page carries an evidence section beneath
     every objective, so the target must be that objective's own numbered
     heading — `#### Evidência — objetivo <n>`. A bare `#### Evidência` is
     ambiguous and the write is declined rather than filed under objective one;
   - **a retirement** is a dated line under `## Aposentados` with `append-entry`,
     naming the review that retired it;
   - **an objective's status, its last-confirmed date, the next review date and
     any new objective** change the page's fields or its heading structure.
     No implemented operation does that. Hand the owner the exact edit and
     report it as pending, never as written.
5. `create-page` preserves a page that already exists and reports `unchanged`.
   A re-capture of the same feedback therefore converges on one page rather
   than overwriting it; if the owner meant to correct a capture, that is an
   owner edit.
6. Reflect back what changed in the objective set, and one concrete way to
   start practising the newest objective this week.
7. **Self-review pack**, when asked for one before a review: `collect` the
   evidence already recorded under each objective, the retrospectives for the
   cycle, and the project feedback pages **the owner names**. A project
   feedback path carries a project slug that no date can derive, and there is
   no folder listing, so the skill cannot know what "all of them" is — the
   owner supplies the list, and the pack states which pages it was built from
   and that anything unnamed is not in it. It is a projection of pages that
   already exist — introduce no claim that is not on one of them, and mark a
   thin objective as thin rather than filling it. Present it in session; write
   it with `create-page` only if the owner names a page for it.

Every run is attended. Nothing here is scheduled, and no standing grant reaches
this segment.

## Formato da página

Formas recomendadas, não portas de entrada: o owner pode escrever Markdown
livre no segmento. O que a template garante é recuperabilidade e headings
estáveis. As duas páginas registram o que foi dito ao owner e o efeito sobre os
objetivos; nenhuma delas avalia quem deu o feedback.

**Project feedback — `owner/development/project-feedback/<YYYY-MM-DD>-<project-slug>.md`**:

```markdown
# Project feedback — <project-slug> — YYYY-MM-DD

> Registra o feedback que o owner recebeu, como o owner registrou. Quem deu o
> feedback pode ser nomeado, como atribuição do que foi dito sobre o owner. Não
> avalie quem deu, e não registre feedback sobre terceiros.

## Snapshot
- **Projeto / workstream:** <link para a página do projeto no workspace>
- **Período coberto:** YYYY-MM-DD a YYYY-MM-DD
- **Rodada:** meio de projeto | fim de projeto
- **Dado por:** <nome, papel, ou ambos — atribuição apenas, minimizada>

## Pontos fortes, como recebidos
-

## Áreas de desenvolvimento, como recebidas
-

## Leitura do owner
- **O que eu aceito:**
- **O que eu qualificaria:**

## Efeito nos objetivos
- **Objetivo tocado:** <link> — reforça | estende | contradiz
- **Novo objetivo proposto:** sim | não

## Relacionado
- [Objetivos](../objectives.md)
```

**Career-committee review — `owner/development/cdc/<YYYY-MM-DD>-cdc.md`**:

```markdown
# CDC — YYYY-MM-DD

> Síntese de carreira. Esta página abre um novo ciclo de objetivos. Uma posição
> do comitê pode ser atribuída a quem a enunciou; não registre avaliação de
> nenhum membro.

## Snapshot
- **Ciclo revisado:** YYYY-MM-DD a YYYY-MM-DD
- **Resultado, como comunicado:**
- **Próximo CDC em:** YYYY-MM-DD

## Trajetória, como comunicada
-

## Pontos fortes que seguem
-

## Prioridades definidas para o próximo ciclo
1.
2.

## Efeito nos objetivos
| Objetivo anterior | Destino | Motivo |
| --- | --- | --- |
| <link> | mantido \| aposentado \| substituído | |

## Leitura do owner
-

## Relacionado
- [Objetivos](../objectives.md)
```

A tabela da página de CDC registra o reset como ele foi comunicado. Aplicar o
reset em `objectives.md` — status, data de última confirmação, próxima revisão,
objetivo novo — é edição do owner, mostrada como pendente e nunca reportada
como escrita. A aposentadoria de um objetivo preserva o enunciado e o log de
evidência dele e aponta para a página de CDC que a causou.

## Invariants

- The skill never writes a file. Every effect is a named operation through the
  installed adapter.
- No objective changes without being shown to the owner first.
- Project feedback never retires an objective. Only a career-committee review
  resets the set, and a reset preserves what it retires.
- Capture is idempotent. Re-capturing the same feedback under the same key
  produces one page and no duplicated objective change.
- The page's subject is the owner's own performance. A giver is named as
  attribution and never assessed; no record whose subject is that person is
  created, and no inferred trait, score or rating is produced.
- Client and engagement content stays in the workspace that owns it. A capture
  may name the project it came from and link to its workspace page; findings,
  figures and deliverable material do not move into owner scope.
- No performance, compensation or staffing system is read or treated as
  authoritative. What the owner reports is what is recorded.
- A write that reports `proposed` rather than `written` means the page moved
  underneath the read and nothing was persisted. Show the owner the proposal
  and let them decide; do not retry over their edit.
- If an operation is unavailable, say so and keep going. The reading of the
  feedback, the fold-or-reset judgement and a draft the owner can keep all
  still stand — only the recording is lost, and it must never be reported as
  done.
