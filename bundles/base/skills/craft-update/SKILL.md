---
name: craft-update
description: Capture or revise a personal method, framework or working preference in the owner craft segment, on request and outside the daily flow. Use for "document this technique", "write this up as a method", "update my method for X", "record how I prefer to do Y", or when an approach that just worked should be reusable next time.
---

# Craft Update

Write one craft page properly, or revise one that already exists. The owner has
already decided there is something to record; the work here is getting it into
a shape that is still usable a year from now.

All reads and writes use direct file operations on the owner atlas paths (`data/owner/atlas/`). Never skip the confirmation gate or edit atlas files directly outside the skill's write sequence.

## How this differs from `learnings-bridge`

Both fill `owner/craft/`, from opposite directions.

- **`learnings-bridge` is bottom-up and periodic.** It works from what the
  dailies already collected, sweeps a window of candidates the owner wrote
  earlier, and decides where each one belongs.
- **This skill is top-down and immediate.** The owner names the technique or
  the preference now, in this conversation, with no daily page involved.

They neither invoke nor reimplement one another. If the owner is asking "what
did I note down this week", that is the other skill.

## Interaction profile

Resolve `interaction-profile` before presenting a draft. The method-versus-style
test, the operations used, the bounds and the confirmation gate never vary by
profile; only the explanation and optional detail do.

- `standard`: the draft page, its destination, and what will be written.
- `advanced`: add why it classified as method or style, and which existing
  pages it touches or overlaps.
- `power`: add the pages collected and their revisions, the operation behind
  each write, and every change that falls to an owner edit instead.

## Inputs

- The owner's stated intent: a **new method**, a **revision** to a method that
  exists, or a **working preference**. Ask once if it is not explicit; do not
  draft against a guess.
- The substance, in the owner's words. This skill does not invent a technique
  the owner has not described, and it does not generalize an engagement into a
  method on their behalf.
- Obtained with `collect`, purpose declared and pages named:
  `owner/craft/index.md`, and for a revision the target page itself. Keep the
  revision of anything read; a later write uses it to notice that the owner
  edited the page in the meantime.

## Method or style

Answer this before drafting, and say the answer out loud.

- **Method** — `owner/craft/methods/<method-slug>.md`. Another practitioner
  could follow it and get a comparable result. The subject is the technique.
- **Style** — `owner/craft/style/<situation-slug>.md`. A calibration true of
  this owner, not of the craft. The subject is the owner.

The style shape asks the author to state why the page is *not* generalizable. A
page that cannot answer that is very likely a method in the wrong folder. Treat
a blank answer as a finding to raise, never as grounds to refuse the page.

**A style entry is deliberately not a skill.** A skill is a capability any
practitioner can pick up and use; a style entry is one person's calibration,
and installing it would push their preference onto everyone who ran it. So a
style page is never registered as a skill, a catalogue entry or a runtime
setting, and it is never a candidate for shared knowledge. A method may be, one
day, under governance that does not exist yet and is not invoked here.

## Workflow

1. Establish the intent and `collect` the craft index, plus the target page for
   a revision. Name any existing page that covers the same ground — extending
   one beats creating a near-duplicate.
2. Classify method or style, state the classification and the reason behind it.
3. **New method.** Confirm it is genuinely reusable and has been used at least
   once in earnest rather than only imagined. Draft on the segment shape: what
   it solves, when to use it and when not to, the steps in order, the inputs it
   needs, its failure modes, and whether it is generalizable yet. Present the
   draft, then `create-page` on confirmation. An existing page at the path is
   preserved and reported `unchanged`, never replaced.
4. **Working preference.** Keep it short and observable — what the owner wants,
   in what situation, what to do instead of the default, and the checks they
   run before the artefact goes out. Present it, then `create-page`.
5. **Revision.** Say plainly what the implemented operations can and cannot do
   here. A dated line under a heading the page already declares — typically
   `## Evidência de uso` — is an `append-entry` and is written. Rewriting the
   steps, correcting `Maturity`, `Last used` or `Last confirmed`, or retiring
   guidance is **an owner edit**: no implemented operation replaces prose or
   sets a field. Show the current text beside the proposed replacement, hand
   the owner the exact edit, and report it as pending rather than done.
6. `append-entry` never creates a heading. A page that does not declare the
   target section cannot receive the entry — offer the text as a draft to
   place. Where a page declares the same heading more than once the operation
   refuses the write rather than guessing which one was meant; name that
   section's own heading instead.
7. Cross-references are written inline in the page body today, and the craft
   index line is an owner edit. Do not report either as a managed link.
8. Report what was written, what is waiting on an owner edit, and anything the
   page still leaves unanswered.

Every run is attended. Nothing here is scheduled, and no standing grant reaches
this segment.

## Formato da página

Formas recomendadas, não portas de entrada: o owner pode escrever Markdown
livre no segmento e nenhuma página é recusada por sair da forma. O que a
template garante é recuperabilidade, a fronteira visível entre método e estilo,
e headings estáveis — `append-entry` nunca cria um heading, então uma seção que
a página não declara é uma seção onde nada pode ser anexado depois.

**Método — `owner/craft/methods/<method-slug>.md`**:

```markdown
# Método — <nome>

> Técnica reutilizável. Registre a técnica, nunca a engagement em que ela foi
> usada. Sem nome de cliente, sem dado de cliente, sem conteúdo de entregável.

## Snapshot
- **Tipo:** técnica | framework | playbook
- **Origem:** desenvolvido | adaptado de <fonte pública ou gerenciada>
- **Maturidade:** rascunho | usado uma vez | repetível
- **Último uso:** YYYY-MM-DD

## Problema que resolve
-

## Quando usar, e quando não usar
- **Usar quando:**
- **Não usar quando:**

## Passos
1.
2.

## Insumos necessários
-

## Modos de falha
-

## Evidência de uso
- YYYY-MM-DD — <a que foi aplicado, descrito sem detalhe identificável>

## Compartilhabilidade
- **Generalizável:** sim | ainda não
- **O que teria de mudar antes de compartilhar:**

## Relacionado
- [Índice de craft](../index.md)
```

**Estilo — `owner/craft/style/<situation-slug>.md`**:

```markdown
# Estilo — <tipo de artefato ou situação recorrente>

> Calibração pessoal, não uma capacidade. Esta página nunca é candidata a
> promoção para conhecimento compartilhado.

## Snapshot
- **Aplica-se a:** <tipo de artefato ou situação recorrente>
- **Força:** sempre | em geral | quando há tempo
- **Última confirmação:** YYYY-MM-DD

## Preferência
-

## Por que o owner trabalha assim
-

## Checar sempre antes de o artefato sair
- [ ]

## Anti-padrões que o owner recusa
-

## Não é generalizável porque
- <uma linha. Se não for possível completar, a página provavelmente pertence a methods/.>

## Relacionado
- [Índice de craft](../index.md)
```

`Maturidade`, `Último uso` e `Última confirmação` mudam por edição do owner:
nenhuma operação implementada altera um campo. A linha no índice de craft
também é edição do owner. `## Evidência de uso` é o único destino de
`append-entry` numa página de método já existente.

## Invariants

- The skill never writes a file. Every effect is a named operation through the
  installed adapter.
- Every write is confirmed by the owner first. No silent create, no silent
  revision.
- A method records the technique, never the case. Client and engagement content
  stays in the workspace that owns it: a page may name the engagement as the
  occasion, but findings, figures, stakeholder positions and deliverable
  material do not move into owner scope.
- Writes are additive. Body text the owner wrote by hand is never rewritten by
  an operation, and superseded guidance is never silently dropped.
- A style page is never promoted, registered or shared, and no promotion path
  for one is created here.
- No memory layer, cycle, capture or eligibility rule is read or written, and
  no decision record is created.
- Content from another workspace or another owner is never admitted.
- A write that reports `proposed` rather than `written` means the page moved
  underneath the read and nothing was persisted. Show the owner the proposal
  and let them decide; do not retry over their edit.
- If an operation is unavailable, say so and keep going. The conversation still
  reaches a page worth having, and the owner can keep the draft — only the
  recording is lost, and it must never be reported as done.
