---
name: owner-background
description: Capture the owner's background outside the current job — formal education, the path before BCG, and the wider ambitions for career and life — through the confirmed owner context in product MCP, or one SELF facet on the explicit legacy ZIP host. Use for "meu background", "minha formação", "onde estudei", "o que eu fazia antes do BCG", "minhas aspirações de carreira", "quero registrar minha trajetória", or when onboarding deferred this block and the owner comes back to it.
---

# Owner Background

## Choose the admitted runtime

For the product MCP/plugin, use the route below and stop after its readback.
It enriches the existing canonical `personal-context` facet; it creates neither
`background.md` nor a pending-task checkbox. The legacy ZIP route later in this
file applies only when that installed host and its filesystem authority are
explicitly identified. A missing product operation never selects that fallback.

## Product MCP: preserve, enrich and reconfirm

1. Query `owner.status`. Continue only when `onboarding.state` is `complete`;
   otherwise resume [owner onboarding](../maestro-onboarding/SKILL.md) if the
   owner chooses, keeping background capture pending. Metadata and
   `owner.review` do not reveal the existing body and are not read fallbacks.
2. Query `owner.context` and reconstruct every page, following `next_offset`
   with the first page's exact `generation` until `next_offset: -1`. Discard the
   whole read on refusal or generation change. Require `personal-context` in
   the returned `facets`. Its complete body lies after its exact facet marker
   and before the next advertised facet marker; preserve its original newlines
   while removing only the projection's separator newline. Verify advertised
   facet markers occur once and in returned order. Ambiguous/missing markers
   leave preservation unavailable: stop rather than guess, truncate, read owner
   files or replace unseen content.
3. Explain the destination before collecting answers: this adds to the existing
   **sensitive personal context**, whose canonical readers are Maestro sessions
   and Yoda and whose updates require confirmation. Accepted text is returned
   to the admitted model session; this is not a promise that it stays only in a
   local file, nor an attestation of an individual hosted Yoda's identity. Ask
   whether the owner authorizes this professional-personalization purpose and
   reader scope for active use until revised or withdrawn, preserving stricter
   existing limits. Existing private proposal/audit history keeps its canonical
   retention; this route promises no automatic expiry or historical erasure.
   An unsupported retention requirement or refusal keeps the discussion unsaved.
4. Ask one optional question per turn: education/training; career before the
   current organization; career or life aspirations the owner wants considered
   at work. Use the three question prompts below as conversation guidance,
   with ordinary chat if a host question tool is unavailable. Accept omissions
   and uncertainty without inference. If all are skipped, finish without a
   proposal. Do not import documents, old chats or reports through this route.
5. Draft named `Formação`, `Trajetória anterior` and `Aspirações` subsections
   from the explicit answers under a dated background addition. Preserve the
   complete current `personal-context` body and its consent/retention limits;
   revise a prior background section only as explicitly requested. Show the
   complete merged candidate and the agreed purpose/readers/retention. Keep
   its UTF-8 size at most 12KiB and evidence at most 4KiB. If it does not fit,
   ask the owner to choose an explicitly reviewed reduction; never silently
   drop existing history or split it into unauthorized facets.
6. After the owner chooses to save that candidate, call `maestro_apply` with
   `operation: "owner.propose"`, `facet: "personal-context"`, an `evidence`
   summary of the actual answers and consent, and the complete `proposed_body`.
   Use no invented question ID or consent/approval argument. Before application,
   fetch all of `owner.context` again with the original exact generation and
   compare the complete preserved body; a refused or changed read leaves this
   proposal unapplied and requires fresh context and a new merged proposal.
   This closes the gap between reading the old body and creating its proposal.
   Query `owner.review` with the returned `proposal_id` for metadata, then call
   `owner.apply` with that exact ID. The trusted host presents current and
   proposed text, evidence, sensitivity, readers and policy for human approval.
   Decline/unavailable review preserves the active facet. A conflict preserves
   newer work: re-read current confirmed context, or finish reconfirmation via
   onboarding before preparing another merged proposal.
7. Applying a facet invalidates final profile acceptance. Query `owner.status`
   and `owner.onboarding_review`, then explicitly call
   `owner.confirm_onboarding` for a separate trusted review of the whole current
   profile. Saved answers remain pending on decline or unavailable review;
   report that state rather than claiming accepted-context delivery. Never
   reselect a track just to complete this update.
8. Require `owner.status` to return `complete`. Fetch the entire fresh
   `owner.context` generation again and verify the preserved personal context
   plus exactly the approved background addition. Only this readback completes
   the product route. Claim canonical save/reconfirmation separately from any
   native client or future-session delivery proof. This method grants no
   filesystem, external-send, scheduling or automatic-memory authority.

## Legacy ZIP host only

Before owner-state file operations, follow [admitted owner-state paths](../interaction-profile/resources/owner-paths.md). Owner `brain/` paths are logical; confirmed case/workspace-local paths retain their scope.


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

Resolve [`interaction-profile`](../interaction-profile/SKILL.md) before asking anything. It changes how much is
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
