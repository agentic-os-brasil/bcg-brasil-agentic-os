---
name: deck-drill
description: Rehearse a finished deck against the questions its audience will actually ask, with candidate answers grounded in the case context and the three weakest points named. Use before presenting — "deck-drill", "prepare me for this meeting", "what will they ask", "poke holes before the client does".
---

# Deck Drill

The deck is finished. This is the rehearsal: what the room will ask, what you
would say, and where your answer is still thin.

Advisory and read-only. It produces questions and candidate answers; it does
not edit the deck, and it does not approve it.

## Not a review

`deck-review` and the quality path judge whether an output is good. This asks
how an audience will attack it — a different question with a different answer.
A deck can be sound and still fall apart in the room, and it can be rough and
survive because the presenter knew where the pressure would come from.

Run this after the deck is settled, not instead of settling it.

## Interaction profile

Resolve `interaction-profile` before presenting. The lenses, the grounding rule
and the honesty about weak answers never vary by profile.

- `standard`: the questions by lens, a candidate answer each, the three weakest.
- `advanced`: add who in the room is likely to ask each one, and why.
- `power`: add which part of the supplied context each candidate answer rests
  on, so the presenter can check the ground before relying on it.

## Inputs

The deck text, and whatever case and client context the session already
provides. Nothing is fetched. Where context is thin, the drill still runs — it
just names more answers as needing input, which is itself useful before a
meeting.

## Workflow

1. Read the deck end to end and identify the decision it is asking for. A deck
   that asks for nothing will be attacked differently, and that is worth saying.
2. Generate **eight to twelve** questions the audience would genuinely ask,
   grouped by three lenses:
   - **content** — is the logic sound, do the numbers hold, what was left out;
   - **process** — why this approach, why trust the method, who was consulted;
   - **implication** — what does this mean for us, what does it cost, what
     happens next, what if we do nothing.
3. For each question, supply **one or two candidate answers** built only from
   context actually available. Where the context does not support an answer,
   say the answer needs input and name what input — do not compose something
   that sounds right.
4. Name the **three weakest points**: where the honest answer is still thin and
   the presenter should prepare before the meeting rather than in it.
5. Where a question would land better if the deck changed, say so separately
   from the answers. That is a note for the author, not a rehearsal line.

## Invariants

- Nothing is invented about the client, the market or the analysis to make an
  answer sound stronger. A fabricated answer is worse than an admitted gap,
  because it will be repeated in the room.
- A thin answer is reported as thin. The point of the drill is to find those
  before the audience does; hiding them defeats it.
- The deck is not edited and not scored. This produces preparation, not a verdict.
- Questions are the ones the audience would ask, not the ones that are easy to
  answer. A comfortable drill has not done its job.
