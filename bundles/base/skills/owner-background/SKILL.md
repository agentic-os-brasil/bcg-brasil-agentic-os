---
name: owner-background
description: Capture the owner's background outside the current job — formal education, the path before BCG, and the wider ambitions for career and life — into one SELF facet. Use for "meu background", "minha formação", "onde estudei", "o que eu fazia antes do BCG", "minhas aspirações de carreira", "quero registrar minha trajetória", or when onboarding deferred this block and the owner comes back to it.
---

# Owner Background

Write one facet — `brain/owner/self/background.md` — with what came before the
current role and what the owner is aiming at beyond it. Three questions, one
page, done in five minutes.

## Why this is its own skill and not part of onboarding

Onboarding already asks thirteen questions and takes long enough that people
abandon it halfway. Adding education, career history and life ambition would
have pushed it past the point where the owner finishes — and these three are
the least urgent of everything asked: nothing in the daily flow breaks without
them. What breaks without them is the *long* view — a retro that cannot connect
this quarter to where the owner said they were going, a development objective
written with no idea what the owner trained as.

So they live here, invited at the end of onboarding and available any time
after. If the owner defers, onboarding records the pendency (see
"Pendency", below) so the invitation does not evaporate.

## Interaction profile

Resolve `interaction-profile` before asking anything. It changes how much is
explained and how the questions are worded — never which questions are asked,
never what is written, and never the confirmation before writing. This is the
owner's own history: the profile calibrates the conversation, not the record.

## The three questions

Ask them one per `AskUserQuestion` call, in this order. All three accept free
text — the options are prompts to think with, never a closed list.

**1 — Formação** (`header`: `Formação`)

> `Onde você estudou e no quê? Graduação, pós, intercâmbio, certificação — o
> que você considera parte da sua formação.`
>
> - `Graduação` · `Pós ou MBA` · `Certificações e cursos` · `Prefiro escrever`

**2 — Antes do BCG** (`header`: `Antes daqui`)

> `O que você fez antes do BCG? Empresas, funções, iniciativas próprias — e o
> que dessa bagagem você ainda usa hoje.`
>
> - `Experiência em empresa` · `Empreendi` · `Academia ou pesquisa` ·
>   `BCG é o primeiro`

**3 — Para onde** (`header`: `Para onde`)

> `Olhando pra frente, o que você quer construir? Vale carreira, vale o que
> está fora dela.`
>
> - `Trajetória no BCG` · `Algo fora da consultoria` · `Uma competência
>   específica` · `Ainda estou descobrindo`

Do not push for more than the owner offers. A one-line answer to question 3 is
a complete answer to question 3 — "ainda estou descobrindo" is real information
and gets written down as it was said.

## What to write

One page, `brain/owner/self/background.md`, in the standard facet contract:

```markdown
---
id: owner/self/background
title: "background"
summary: "<uma linha — o resumo que aparece no índice do SELF>"
type: owner-facet
scope: owner
status: active
sensitivity: owner-private
updated: <AAAA-MM-DD>
---

# background

## Formação

- <o que foi dito>

## Antes do BCG

- <o que foi dito>

## Aspirações

- <o que foi dito>

_Capturado com o owner em <AAAA-MM-DD> via `/owner-background`._
```

The page is the deliverable: once written, it is what every later session
reads. Nothing else has to run for it to count.

If the page already exists, **revise it, never replace it**: keep what still
holds, add what is new, and note the revision date in the closing line. A
background page rewritten from scratch loses the parts the owner told you once
and will not think to repeat.

## Pendency

If onboarding deferred this block, `brain/owner/operating/pendencias.md` carries
a line like:

```markdown
- [ ] Registrar o background pessoal — formação, trajetória e aspirações. Rode `/owner-background`.
```

When the page is written, mark that checkbox `[x]` on that page and recompile.
Leaving it open after the work is done makes the task view lie, and a task view
that lies stops being read.

## Boundaries

- This is `owner-private` and it stays local. Never send any of it to a
  provider, never quote it in client-facing material.
- Do not infer. If the owner says "engenharia", write "engenharia" — not
  "perfil analítico, provável conforto com modelagem". A facet is a record of
  what was said; inference belongs to whoever reads it later, with the source
  in front of them.
- Do not ask about family, health, faith or anything the owner did not open.
  Question 3 is about ambition, not private life; if the answer goes somewhere
  personal because the owner took it there, record only what they chose to say.
- One page. Do not spawn `education.md`, `career-history.md` and
  `aspirations.md` — three thin pages recall worse than one whole one.
