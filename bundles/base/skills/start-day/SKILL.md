---
name: start-day
description: Open or resume the working day at whatever hour the owner appears, composing one briefing from the owner atlas plus any calendar, mail or task context the session already has. Use at first contact of the day, for "bom dia", "good morning", "start day", "what does my day look like", or a re-entry later the same day.
---

# Start Day

Compose one briefing scoped to the hours that actually remain, and record it on
today's page.

All reads and writes use direct file operations on the owner atlas paths (`brain/owner/`). Never skip the confirmation gate or edit atlas files directly outside the skill's write sequence.

## Interaction profile

Resolve `interaction-profile` before presenting. The reads, the write, the
bounds and the omissions never vary by profile; only the explanation does.

- `standard`: the shape of the day, the top three, one first move.
- `advanced`: add why each item ranked where it did, and what was left out.
- `power`: add the page revisions read, the remaining-hours computation, and
  which optional inputs were unavailable.

## Required inputs

Obtained with `collect`, always with a declared purpose and named pages.

- today's page in `owner/daily/`, if it exists — this decides first contact
  versus re-entry;
- the two most recent prior daily pages;
- current objectives from `owner/development/objectives.md`;
- open workplan lines from the project pages the recent dailies reference.

## Optional inputs

Used only if the running session already provides them. Never requested by
name, never required, never persisted.

- **Calendar** — today's events with start, end, title and participant count,
  used to split what already happened from what is still ahead.
- **Mail** — metadata only: sender, subject, receipt time, flag state, used to
  see who is waiting on a reply. Message bodies are never read.
- **External task view** — open items from a task tool the owner keeps, shown
  alongside atlas workplan lines, never merged with them and never treated as
  authoritative.

The skill names no connector, server or runtime feature. It describes the shape
of context it can use, and works without any of it.

## Workflow

1. Resolve the current time and the local workday bounds.
2. `collect` the required inputs. First contact or re-entry is decided by
   whether today's page already carries entries.
3. Take whatever optional context the session already offers. Record each one
   absent as an omission to state plainly in the briefing.
4. Compute the hours actually remaining: workday end minus now, minus any
   protected block still ahead that the atlas declares.
5. Rank what is achievable in the time that is left. Ranking happens here, from
   what was already read.
6. Compose one briefing:
   - **first contact** — the shape of the day with past and upcoming marked,
     the top three for the remaining hours with a one-line reason each,
     watch-outs, one development nudge tied to a moment still ahead, and a
     suggested first move;
   - **re-entry** — lead with what is already logged, then the same structure
     scoped to what remains;
   - **near-zero remaining hours** — say so and offer to close the day instead
     of forcing a fresh plan.
7. Record **the plan, not the enrichment**: `create-page` for today if absent,
   then `append-entry` with a timestamped entry carrying the ranked priorities,
   the reason for each, and the first move. Calendar and mail material shaped
   the ranking and is not written — meeting titles, participant counts and the
   names of people waiting on a reply stay out of the page. Durable capture of
   a meeting or a correspondent is a separate, attended act. Re-running
   appends; it never overwrites.
8. State every optional input that was unavailable, and whether the entry was
   written or came back as a proposal. An incomplete briefing is reported as
   incomplete, and an unrecorded one is never reported as recorded.

## Formato da página

Forma recomendada, não porta de entrada: a página do owner aceita Markdown
livre. O que a template garante é recuperabilidade e headings estáveis, já que
`append-entry` nunca cria um heading.

**Página do dia — `owner/daily/<YYYY-MM-DD>.md`**, criada apenas quando o dia
ainda não tem página:

```markdown
# Daily — YYYY-MM-DD

> Registro humano do dia. Entradas brutas não são insumo de memória.

## Escopo relacionado
- **Projetos:**
- **Clientes:**

## Prioridades
<!-- append-only: uma entrada de briefing por execução do start-day -->

## Notas
<!-- append-only: o fechamento do dia entra aqui -->

## Decisões que emergiram
- <link para a decisão autoritativa na página do workspace que a possui>

## Candidatos a aprendizado
- <ponteiro de aprendizado owner-private; não copie conteúdo privado do owner aqui>

## Levar adiante
-
```

`## Escopo relacionado`, `## Decisões que emergiram`, `## Candidatos a
aprendizado` e `## Levar adiante` são mantidos pelo owner ou por outras skills.
Esta skill escreve apenas sob `## Prioridades`.

**Entrada de briefing**, anexada sob `## Prioridades` a cada execução. A
primeira execução do dia cria a página e anexa a primeira entrada; cada
execução seguinte apenas acrescenta outra entrada, com o horário em que
aconteceu:

```markdown
### HH:MM — briefing
- **Prioridades para as horas restantes:**
  1. <prioridade> — <por que ficou nesta posição>
  2. <prioridade> — <por que ficou nesta posição>
  3. <prioridade> — <por que ficou nesta posição>
- **Primeiro movimento:** <a próxima ação concreta>
```

O que veio de calendário, e-mail ou visão externa de tarefas não entra na
entrada: título de reunião, contagem de participantes e nome de quem espera
resposta ficam fora da página. Uma entrada que não foi gravada nunca é
reportada como gravada.

## Autonomous mode (scheduled run)

Entered only when the invoking prompt states explicitly that this is an
unattended, scheduled run (`scheduled-tasks`) with no owner present to answer
or confirm. Never inferred — not from a quiet chat, not from an unanswered
question, not from the hour. If the prompt does not say so, the run is
attended and the workflow above applies as written. This mode relaxes no
invariant except the two named at the end of this section.

A scheduled opening runs before the owner arrives, which is the whole point:
the briefing is waiting when they get there. It also means every judgment in
it was made without them.

1. **Steps 1 through 6 run unchanged, without dialogue.** Resolve the time,
   `collect`, take whatever optional context the session offers, compute the
   remaining hours, rank, and compose the briefing exactly as attended. The
   ranking is the value of this run; it is not deferred for lack of an
   audience.
2. **Where the attended workflow would ask, state the reading instead** and
   mark which kind it is: `evidência direta` when a page the owner wrote
   supports it, `inferido, não confirmado` when it does not. Never invent a
   priority to fill a thin day. A day with little evidence produces a short
   briefing, and says that it is short because the evidence was thin.
3. **Near-zero remaining hours does not offer to close the day.** There is
   nobody to accept the offer. Say so in the entry and give the briefing for
   the hours that remain; closing the day stays an attended act.
4. **The entry is written, and marked as unconfirmed.** Step 7 runs as usual —
   `create-page` if today has no page, then `append-entry` — with one added
   line at the top of the entry: `**Registro:** briefing automático, não
   confirmado pelo dono`. The rest of the entry's shape is identical to an
   attended briefing, so the page stays readable as one record.
5. **A `proposed` result is final in this mode.** Attended, a proposal is
   shown to the owner and they decide. Here there is nobody to show it to, so
   the page moved under the read and the entry is not written: leave the
   owner's version alone, do not retry, and name it in the report. Silently
   retrying over an edit the owner made is the one failure this mode could
   cause that they would not be able to see.
6. **Step 8 is folded into the report below**, not dropped: every optional
   input that was unavailable, and the true write outcome, are still stated —
   to the chat rather than to a listener.
7. **Report the briefing in full to the chat of this execution.** Not a
   confirmation line — the whole briefing: the ranked priorities with their
   reasons, the first move, every optional input that was unavailable, and
   whether the entry was written, proposed or skipped. The owner reads this
   chat later and it has to carry what an attended briefing would have said
   out loud. A page written and never announced is indistinguishable, from
   where the owner sits, from a run that never happened.

An owner returning to an autonomously written briefing in an ordinary session
can correct it, act on it, or ignore it exactly as with any other entry. This
mode defers the acts that need them; it forecloses none.

### Invariants (autonomous mode)

- Autonomous mode is entered only on an explicit, self-declared unattended
  run. Never inferred from context.
- An autonomously written briefing is always marked as such in its own text,
  never indistinguishable from an attended one.
- Every invariant of the attended skill still holds, with two named
  exceptions: the confirmation gate is replaced by the unconfirmed marker in
  point 4, and a `proposed` result ends the write instead of starting a
  conversation.
- Nothing sourced from optional context is written, here as anywhere. An
  unattended run has less oversight, not more latitude.
- The interaction profile calibrates how much is explained, never how much of
  this run is reported. A concise profile shortens the prose, not the record:
  the report still carries every item, every marker and everything deferred.
## Invariants

- Append-only per day. The first run creates one page; each subsequent run
  adds one timestamped briefing so the record remains auditable rather than
  pretending the earlier briefing never happened.
- Nothing sourced from optional context is written to the atlas. Enrichment
  composes the briefing and is discarded; durable capture is a separate act.
- Mail is never read below metadata level.
- No task record is created, mirrored or synchronized. Workplan lines are read,
  never written — reading a checkbox the owner wrote is atlas reading, and this
  skill does not become a task system.
- An engagement may be named. Findings, figures and deliverable material stay
  in the workspace that owns them.
- A result of `proposed` rather than `written` means the page moved under the
  read. Show the owner the proposal; do not retry over their edit. In a
  scheduled run there is nobody to show it to — see "Autonomous mode" above.
- If an operation is unavailable, say so and give the briefing anyway. The plan
  is still worth having — only the recording is lost, and it must never be
  reported as done.
