---
name: eod
description: Close the working day on the owner's daily page — what got done, what moved, what carries to tomorrow — and surface anything that deserves a durable home elsewhere. Use for "eod", "fechando o dia", "wrapping up", "done for today", or when the owner is stopping.
---

# End of Day

Close the day honestly and leave tomorrow a starting point.

All reads and writes use direct file operations on the owner atlas paths (`brain/owner/`). Never skip the confirmation gate or edit atlas files directly outside the skill's write sequence.

## Interaction profile

Resolve [`interaction-profile`](../interaction-profile/SKILL.md) before presenting. The reads, the write and the
bounds never vary by profile.

- `standard`: the three-line recap and tomorrow's first priority.
- `advanced`: add what was surfaced for a durable home and what the owner
  declined to record.
- `power`: add the page revision, the idempotency key and the authority the
  write ran under.

## Inputs

Obtained with `collect`: today's page in `brain/daily/`, and current objectives
from `brain/development/objectives.md` when the day touched one.

## Workflow

1. Ask what actually happened, rather than summarizing what was logged earlier.
   The morning plan is a hypothesis; the point of closing is to record the day
   that occurred.
2. Record on today's page: what got done, what moved and did not finish, and
   what carries to tomorrow. `create-page` if the day has no page yet, then
   `append-entry`.
3. **Surface durable decisions, do not file them.** If the day produced a
   decision that outlives it, say so and name where it belongs. A decision
   about an engagement belongs to that workspace, and this skill does not reach
   across scope to write it.
4. Note evidence bearing on a development objective, good or missed. Offer to
   append it under that objective's own heading, confirming each separately. If
   the objectives page repeats an evidence heading, name the objective's
   heading — the operation refuses an ambiguous target rather than guessing.
5. If the day produced a durable claim about how this kind of work goes, say so
   and offer to promote it. Promotion is a separate, attended act.
6. Preview tomorrow's first priority in one line.
7. Give a three-line recap: what closed, what carries, what is first tomorrow.

## Formato da entrada de fechamento

A página do dia é a mesma que [`start-day`](../start-day/SKILL.md) cria, com o mesmo cabeçalho e as
mesmas seções. Esta skill não redefine a página: quando o dia ainda não tem
página, ela é criada na forma que [`start-day`](../start-day/SKILL.md) declara, e o fechamento é sempre
uma entrada anexada.

**Entrada de fechamento**, anexada sob `## Notas` da página do dia. Fechar duas
vezes acrescenta uma segunda entrada com outro horário; nenhuma entrada
anterior é reescrita:

```markdown
### HH:MM — fechamento
- **Concluído:** <o que fechou hoje>
- **Andou e não terminou:** <o que avançou, e onde parou>
- **Carrega para amanhã:** <uma linha por item>
- **Decisão a registrar em outro lugar:** <a decisão e onde ela pertence> | nenhuma
- **Página de workspace a atualizar:** <link para a página que precisa da atualização> | nenhuma
- **Evidência de objetivo a confirmar:** <o objetivo e a evidência do dia> | nenhuma
```

As três últimas linhas são ponteiros, não registros. Uma decisão durável
pertence ao workspace que a possui e é registrada lá, num ato separado; um fato
de workspace que mudou hoje é apenas anotado aqui como pendência. A terceira
existe porque a evidência que o owner adiou — ou que uma rodada agendada não
tinha autoridade para gravar — se perdia sem ela: o passo 4 oferece anexar a
evidência ao objetivo, e um "não agora" não deixava rastro nenhum. Nada entra
na entrada que o owner não tenha confirmado na conversa, e um fechamento que
não foi gravado nunca é reportado como gravado.

## What this skill narrowed

An earlier design reconciled an external task system and a project's current
truth as part of closing. It does neither.

Where a workspace fact changed today, the day's page notes that the workspace
page needs updating. It does not reach across scope to update it: workspace
content belongs to the workspace that owns it, and closing the owner's day is
not authority over an engagement's record.

## Auto-trigger (SessionStart)

When the session context carries a "Fechamento de dia pendente" block (the
`brain/owner/.eod-requested` marker was present, written by
`session-stop-eod-check.sh`), this is never a mandatory pre-task action —
closing a day always needs the owner's confirmation, so it cannot run
silently the way [`dream-memory`](../dream-memory/SKILL.md)'s daily-light cycle does. Mention it once,
early in the first reply of the session, without blocking whatever the owner
opened the session to do: the same hook also writes `eod_open_dates` into
`brain/.maestro/day-brief.json`, surfaced as the "Day brief pré-computado"
block — that already names the open date(s), computed once at the previous
Stop, so use it instead of running the full reconstruction just to find out
which dates are open. Only fall back to the reconstruction step above when
the day-brief block is absent, or when what it lists doesn't match what the
owner describes (e.g. a day closed by editing the page directly, outside a
Maestro session).

1. If the owner agrees, run the workflow below for each open date and delete
   `brain/owner/.eod-requested` once every open date up to today has a closing
   entry.
2. If the owner declines or defers, leave the marker in place — a repeated
   reminder is recoverable, a silently dropped day is not — and do not raise
   it again later in the same session.

## Autonomous mode (scheduled run)

Entered only when the invoking prompt states explicitly that this is an
unattended, scheduled run (`scheduled-tasks`) with no owner present to answer
or confirm. Never inferred. If the prompt does not say so, the run is attended
and the workflow above applies as written. This mode relaxes no invariant
except the two named at the end of this section.

**The declaration is a fixed marker, not a paraphrase.** The invoking prompt
must contain this line verbatim:

```
MAESTRO_RUN: scheduled-unattended
```

Whoever creates the scheduled task writes that line; this skill enters
autonomous mode on it and on nothing else. A prose description of being
scheduled is not enough, because the failure is silent in the worst
direction: reworded, the skill reads the run as attended, asks its first
question, and waits for an owner who is not there — a routine that appears
configured and quietly produces nothing. A marker either matches or it does
not, and it can be asserted by a check.

Closing is the ritual that most depends on the owner, because step 1 asks what
actually happened. Unattended, that question has no answer, so the day is
reconstructed instead of recounted — and the difference is marked on the page.

1. **Step 1 becomes reconstruction, not dialogue.** Draft the day from what is
   already on today's page and from the session's own activity. Mark each line
   for what it rests on: `evidência direta` when the page supports it,
   `inferido, não confirmado` when it does not. Never invent an outcome to
   round out a quiet day.
2. **The closing entry is written, and marked as unconfirmed.** Step 2 runs as
   usual — `create-page` if the day has no page, then `append-entry` — with one
   added line at the top of the entry: `**Registro:** fechamento automático,
   não confirmado pelo dono`. The rest of the entry's shape is identical to an
   attended closing.
3. **Steps 3, 4 and 5 do not run.** No decision is filed, no evidence is added
   to an objective's own heading, and no learning is promoted. These are
   skipped outright rather than approximated: each one asks the owner to agree
   to something durable about their own work, and an unattended run cannot
   stand in for that. Every one of them leaves a pointer instead, in a place
   that already exists on the page — nothing is dropped for lack of
   authority:
   - a durable decision → the closing entry's `Decisão a registrar em outro
     lugar` line;
   - a workspace page that went stale today → `Página de workspace a
     atualizar`;
   - evidence bearing on a development objective → `Evidência de objetivo a
     confirmar`;
   - a durable claim about how this kind of work goes → the page's
     `## Candidatos a aprendizado` section, as a pointer, never as a promoted
     learning.
4. **A `proposed` result is final in this mode.** Attended, the proposal goes
   to the owner. Here there is nobody to show it to, so the page moved under
   the read and nothing is written: leave the owner's version alone, do not
   retry, and name it in the report.
5. **Step 6 stays, step 7 is replaced by the report below.** Tomorrow's first
   priority is still previewed in one line, inside the entry — it is a reading
   of the day, not a commitment made on the owner's behalf.
6. **Report the closing in full to the chat of this execution.** What closed,
   what carries, what is first tomorrow, which lines were inferred rather than
   evidenced, everything deferred to the next attended pass, and whether the
   entry was written, proposed or skipped. The owner reads this chat later; it
   has to carry what the attended three-line recap would have said, plus what
   this mode could not do.

   **Why this is a hard requirement and not a preference.** Scheduled runs
   were observed opening a session, doing the work, and reporting a fraction
   of what the skill was supposed to produce — the chat existed, the page was
   written, and the owner still could not see what the routine had actually
   concluded. The failure is silent from the owner's side and looks identical
   to the routine working. So the bar is not "announce that the run
   happened": it is that this chat carries **everything an attended run of
   this skill would have said out loud**, at the same level of detail. If a
   line would have been spoken to the owner, it is written here.

The owner reviewing an autonomously closed entry later, in an ordinary
attended session, can correct it and act on its pointer lines exactly as with
any other closing entry. This mode defers those acts; it never forecloses
them.


The permission for this mode and its five bounding conditions are recorded as
decision UNAT in the project decision log. This section implements that
decision; it does not extend it.

### Invariants (autonomous mode)

- Autonomous mode is entered only on an explicit, self-declared unattended
  run. Never inferred from context.
- An autonomously closed entry is always marked as such in its own text, never
  indistinguishable from an attended closing.
- Decisions, objective evidence and learning promotion are never touched in
  this mode, without exception. All three stay attended-only acts.
- Every other invariant of the attended skill still holds, with two named
  exceptions: the confirmation gate is replaced by the unconfirmed marker in
  point 2, and a `proposed` result ends the write instead of starting a
  conversation.
- The interaction profile calibrates how much is explained, never how much of
  this run is reported. A concise profile shortens the prose, not the record:
  the report still carries every item, every marker and everything deferred.
## Invariants

- Append-only per day. Closing twice adds a second timestamped entry rather
  than overwriting the first; the two attended check-ins remain auditable.
- Nothing is recorded that the owner did not confirm in the conversation.
  The single exception is a scheduled run, which records a reconstruction
  marked as unconfirmed — see "Autonomous mode" above.
- An engagement may be named. Findings, figures and deliverable material stay
  in the workspace that owns them.
- A result of `proposed` rather than `written` means the page moved under the
  read. Show the owner the proposal; do not retry over their edit. In a
  scheduled run there is nobody to show it to — see "Autonomous mode" above.
- If an operation is unavailable, close the day in conversation and say the
  recording did not happen. A day discussed and not written is an honest
  outcome; a day reported as written when it was not is a lie the owner will
  discover later.

## Contrato de página do brain

Toda página escrita em `brain/` precisa do frontmatter definido em
`bundles/base/brain-contract.md` — `id`, `title`, `summary`, `type`, `scope`, `status`,
`sensitivity`, `updated`. Leia esse arquivo antes de gravar e escreva o bloco junto com a
página, nunca depois.

Uma página sem esse bloco não aparece no índice do brain e não recebe backlinks: o
trabalho fica gravado e invisível.
