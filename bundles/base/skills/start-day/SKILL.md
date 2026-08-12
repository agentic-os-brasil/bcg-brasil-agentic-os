---
name: start-day
description: Open or resume the working day at whatever hour the owner appears, composing one briefing from the owner atlas plus any calendar, mail or task context the session already has. Use at first contact of the day, for "bom dia", "good morning", "start day", "what does my day look like", or a re-entry later the same day.
---

# Start Day

Compose one briefing scoped to the hours that actually remain, and record it on
today's page.

All reads and writes go through the owner atlas operations exposed by the
installed runtime adapter (`bcgos atlas owner collect`, `create-page`,
`append-entry`). Never edit a page directly from this skill.

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
  read. Show the owner the proposal; do not retry over their edit.
- If an operation is unavailable, say so and give the briefing anyway. The plan
  is still worth having — only the recording is lost, and it must never be
  reported as done.
