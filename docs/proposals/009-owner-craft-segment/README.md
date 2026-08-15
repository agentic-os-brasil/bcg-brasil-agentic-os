# Proposal 009 — Owner craft segment

**Status:** request for decision. Declares the shape of a segment the shipped
owner-atlas operations can already write into, and absorbs one bare index; it
registers no skill and promotes nothing to shared knowledge.

**Original contribution:** Marcelo Petrof Sanches.

**Depends on:** Proposal 006 (owner scope contract), Proposal 007 (owner atlas
operations), PR #286 (the shipped operations and standing grants) and
decision `OATL`.

**Unblocks:** the `record-concept` ritual deferred in Proposal 005; a bounded
craft projection for a future start-of-day briefing; a declared destination for
a future ritual that routes the owner's learning candidates; and a future
maintenance pass over the owner atlas. None of the four is proposed yet.

## Reading the proposals this document cites

Proposal 005 was accepted and then removed from the working tree. It is still
readable from history:

```sh
git show 760abd8:docs/proposals/005-skill-consolidation/README.md
```

That is the reconciled version, and the `record-concept` deferral quoted below
is in it. `2fe2a50` carries a later unreconciled draft of the same proposal on
another branch.

## The segment is already writable; this declares its shape

PR #286 shipped `internal/atlasops` with `collect`, `create-page` and
`append-entry`, revocable standing grants, and the `bcgos atlas owner` and
`bcgos atlas grant` verbs. `create-page` creates parent directories on demand,
so `owner/craft/methods/` and `owner/craft/style/` come into existence the first
time a page is written into either. Neither needs bootstrapping, registration or
admission, and nothing in the operation set has to be extended to allow them.

What is missing is not permission but shape. A method page and a style page are
different kinds of thing, they age differently, and only one of them may ever
become a candidate for shared knowledge. Without a declared shape the difference
is invisible to whoever later looks for promotion candidates. This proposal
declares it.

Proposal 005 deferred `record-concept` pending an "owner/practice canon". No
such canon was ever defined. Spec 014 created `owner/concepts/index.md` and
described it as holding reusable personal methods and playbooks, noting that
promotion to shared knowledge requires separate governance — an accurate
description of a folder that has no page kind and no recommended shape.

A name is not a canon. This proposal defines one, and adds a second layer that
the deferral did not anticipate.

## Two kinds of knowledge, deliberately separated

```text
owner/craft/
  index.md
  methods/<method-slug>.md
  style/<situation-slug>.md
```

| | `methods/` | `style/` |
| --- | --- | --- |
| What it holds | Reusable techniques, frameworks and playbooks the owner developed or adopted | The owner's preferences for *how* they do something |
| Example | How a cost-baseline analysis is built | How this owner likes a cost-baseline analysis structured, phrased and checked before it goes out |
| Data subject | The technique | The owner |
| Another practitioner could adopt it | Yes — that is the test | No — adopting it would import one person's calibration |
| Eligible for a future shared-skill proposal | Yes, under separate governance | **Never** |
| Promotion path | Exists, and is out of scope here | Deliberately absent |

The separation is the point of the proposal. A method can become a shared
capability someday; a style must not, because it is one person's preference and
not a capability. Storing them in one folder guarantees that the boundary is
crossed the first time anyone looks for promotion candidates. Storing them
apart makes the wrong promotion visible before it happens.

The style template works the boundary from the other direction: it asks the
author to state why the page is *not* generalizable. A page that cannot answer
that prompt is very likely a method filed in the wrong folder. That is a review
finding, not a rejection — the field is a discipline, not a validator, and no
page is refused for leaving it blank.

## Relationship to `owner/concepts/`

**`craft/methods/` absorbs `concepts/`.** Absorption, not supersession, and the
reason is that supersession leaves an orphan.

The case for absorbing is short. Spec 014's own description of `concepts/` —
reusable personal methods and playbooks, with shared promotion held behind
separate governance — is a description of `craft/methods/` with the shape
missing. Keeping both would create two homes for one page kind, which the
single-source rule already forbids. Nothing is lost: `concepts/` is a bare
index with no defined page, so there is no content to migrate.

Two guarantees follow, and both matter more than the naming:

- **No page is moved and none is deleted.** If a user has already written pages
  under `owner/concepts/`, they stay where they are. This is a commitment of
  this proposal, not a limit of the operation set: Proposal 007 accepts
  `archive`, `restore`, `redact`, `delete` and `export`, none of which is built,
  and a permanent delete would in any case require an explicit owner
  confirmation. The `concepts/` index is retained and carries a pointer to
  `craft/methods/`.
- **No bootstrap change is decided here.** What `bcgos atlas init` creates is
  Spec 014's business. Changing it requires a spec revision with its own tests.
  Until then, `craft/methods/` is the declared single home for new method pages
  and `concepts/` is a pointer to it.

## Why `style/` is not a Spec 013 facet

Spec 013 bounds the SELF deliberately: exactly one `## Current` section per
facet, 12 KiB and 120 lines, duplicate-paragraph and transcript-shape
rejection, a closed and versioned ten-facet set, and the explicit rule that
SELF is bounded current truth and "not an append-only diary."

That bound is correct and this proposal does not touch it. It is also the
reason a personalization surface is needed elsewhere: the cold-start interview
can capture that the owner wants to be challenged early and terse prose. It
cannot capture how the owner structures a benchmark, what they always check
before a model leaves their hands, or the three phrasings they refuse to use in
a recommendation. Those are numerous, artefact-specific and accumulate — the
exact shape Spec 013 rejects.

| Question | Home |
| --- | --- |
| How should the owner be talked to? | `owner/self/communication-style.md` (Spec 013) |
| What must be true before the owner calls anything done? | `owner/self/quality-bar.md` (Spec 013) |
| How does the owner structure *this kind* of analysis? | `owner/craft/style/` |
| How is that analysis performed at all? | `owner/craft/methods/` |

`craft/style/` is therefore the surface on which the system becomes
progressively personalized after onboarding, without enlarging the facet set,
without turning current truth into history, and without a continuous-learning
lifecycle: every page is owner-authored, written directly or through a named
operation.

## Operations

| Moment | Operation | Result |
| --- | --- | --- |
| A method or style page is first written | `create-page` | The page exists once; a second call preserves what is there and never overwrites it |
| A dated line is added under `## Evidence of use` | `append-entry` | Entry added under a heading the page already declares; the same idempotency key produces one entry |
| `Last used`, `Last confirmed` or `Maturity` changes | owner edit today | `set-field` is accepted and not yet built |
| The `concepts/` index points at `craft/methods/` | owner edit today | `link` is accepted and not yet built |

Three of Proposal 007's ten operations are implemented: `collect`, `create-page`
and `append-entry`. The rest are accepted and unbuilt, and where one is named
above the effect is available today as a direct owner edit. `append-entry` never
creates a heading, so a section a page does not declare is a section nothing can
append under — which is the practical reason the recommended shapes below fix
their headings.

An unavailable operation degrades automation and managed persistence. It does
not block conversation, read-only reasoning, direct owner edits or preparation
of a reviewable draft. A capture conversation about a method still happens and
still produces a draft; only the recorded write is lost, and the runtime must
distinguish "proposed" from "written" rather than reporting a page as filed.

## Templates

The templates below are recommended shapes, not admission gates. Decision `OATL`
is explicit that templates and named operations are reliability aids, and the
owner may author free-form private Markdown here. What a template buys is
retrievability, a visible methods/style boundary, and stable headings a named
operation can address.

### `methods/<method-slug>.md`

```md
# Method — <name>

> A reusable technique. Record the technique, never the engagement it was used
> on. No client name, no client data, no deliverable content.

## Snapshot
- **Kind:** technique | framework | playbook
- **Origin:** developed | adapted from <public or managed source>
- **Maturity:** draft | used once | repeatable
- **Last used:** YYYY-MM-DD

## Problem it solves
-

## When to use it, and when not to
- **Use when:**
- **Do not use when:**

## Steps
1.
2.

## Inputs required
-

## Failure modes
-

## Evidence of use
- YYYY-MM-DD — <what it was applied to, described without identifying detail>

## Shareability
- **Generalizable:** yes | not yet
- **What would have to change before sharing:**

## Related
- [Craft index](../index.md)
```

### `style/<situation-slug>.md`

```md
# Style — <artefact kind or recurring situation>

> A personal calibration, not a capability. This page is never a promotion
> candidate for shared knowledge.

## Snapshot
- **Applies to:** <artefact kind or recurring situation>
- **Strength:** always | usually | when time allows
- **Last confirmed:** YYYY-MM-DD

## Preference
-

## Why the owner works this way
-

## Always check before it goes out
- [ ]

## Anti-patterns the owner rejects
-

## Not generalizable because
- <one line. If this cannot be completed, the page probably belongs in methods/.>

## Related
- [Craft index](../index.md)
```

## Staleness threshold

A maintenance pass over the owner atlas would report a durable fact whose as-of
date has aged past the threshold its segment declares. No such pass is proposed
yet. Craft pages age by disuse rather than by contradiction, so this segment's
clock is a long one.

| Page kind | Threshold | Why |
| --- | --- | --- |
| `methods/<method-slug>.md` | **12 months** since `Last used` | A technique untouched for a year has been superseded by a better one or has dropped out of the work; either way the page no longer describes how the owner works |
| `style/<situation-slug>.md` | **12 months** since `Last confirmed` | A calibration outlives any single engagement, but a preference carried a year without being restated is worth re-reading before it is trusted |

Twelve months is a **proposed default and open to reviewer adjustment**, not a
settled number. A stale craft page is never wrong — it is unconfirmed — so the
finding is "re-confirm this", never "this is out of date".

## Consequences

- Proposal 005's `record-concept` deferral loses its stated blocker. The ritual
  may be proposed against `craft/methods/` as its own document; the destination
  is already writable without it.
- `owner/concepts/` has one successor rather than a competing sibling, and no
  user page is moved to achieve it.
- Personalization has a growth surface that leaves Spec 013's bounded SELF
  intact.
- A future shared-knowledge proposal inherits a defined intake — methods marked
  generalizable — and an equally defined exclusion: style pages, categorically.
- A start-of-day briefing, whenever one is proposed, gains a bounded craft
  projection to read through `collect`, under Proposal 007's reader rules.
- No role is affected. A skill does not edit a craft page; it asks for a named
  operation. Owner content reaches other readers only as a bounded,
  purpose-declared projection: the owner session and Maestro may receive one;
  Yoda may receive a stale-checked self-proxy projection; Case, Client Account
  and PA Expert agents receive an explicitly authorized attenuated excerpt or
  pointer; nobody receives the owner root.

## Explicit non-decisions

- no method is promoted to managed, shared or organizational knowledge, and no
  promotion mechanism is created;
- no bootstrap output is changed — Spec 014 continues to create
  `owner/concepts/index.md` until a spec revision says otherwise;
- no page is moved, renamed, archived or deleted by this document, and no
  operation is added to Proposal 007's accepted set of ten;
- no skill is registered, and no capture ritual or standing grant is created;
- no Spec 013 facet, policy or bound is changed;
- no client, engagement or deliverable content is admitted — a method records
  the technique, never the case;
- merging this document activates no runtime capability. The three implemented
  operations are available because PR #286 shipped them, not because of
  this text.
