# Proposal 010 — Owner daily segment

**Status:** request for decision. Declares the shape of a page the shipped
owner-atlas operations can already write; it moves one page kind to owner scope,
removes nothing from workspace scope and registers no ritual.

**Original contribution:** Marcelo Petrof Sanches.

**Depends on:** Proposal 006 (owner scope contract), Proposal 007 (owner atlas
operations), PR #286 (the shipped operations and standing grants) and
decision `OATL`.

**Unblocks:** a start-of-day briefing and an end-of-day close; a ritual that
routes the day's learning candidates into owner knowledge; and a maintenance
pass over the owner atlas. None of the four is proposed yet. The shipped `retro`
ritual already reads this segment when the owner keeps it.

## The segment is already writable; this declares its shape

PR #286 shipped `internal/atlasops` with `collect`, `create-page` and
`append-entry`, revocable standing grants, and the `bcgos atlas owner` and
`bcgos atlas grant` verbs. `create-page` creates parent directories on demand,
so `owner/daily/` exists the moment a day is written into it. The segment needs
no bootstrapping and no admission, and the shipped `retro` ritual already lists
"this week's pages in `owner/daily/`, if the owner keeps them" among its inputs.

So the question this proposal settles is not whether the owner may keep a daily
page in owner scope. It is what that page contains, what it must not contain,
and which headings a named operation can address — because `append-entry` never
creates a section, and a day recorded without a declared shape cannot be
appended to during the day it describes.

## The gap this closes

Spec 014 places the daily log at workspace scope only: `daily/index.md` and
`daily/template-daily.md` exist under `<workspace>/brain/`, and the owner atlas
has no daily at all.

That arrangement works for one engagement and fails for the real case. A
consultant running several engagements in parallel has **one working day**, not
one per workspace. Split across roots, the day becomes unanswerable: "what did
I do on day Z" requires reading every workspace root and merging the results.

Proposal 006 keeps exactly that operation out of scope: the owner atlas does not
enumerate workspaces or copy their bodies, and automatic workspace crawling is
not admitted. Proposal 007 makes the exclusion structural, and the shipped
implementation makes it mechanical — page resolution is canonical and rejects
anything that escapes the owner root, and a standing grant authorizes writes
only inside the one segment it names.

So the current arrangement produces a question the system cannot answer without
performing the one operation the scope contract excludes. The fix is not to
permit the aggregation. It is to record the day once, at the scope that owns it.

## The arrangement

| Page | Scope | Authority | Content |
| --- | --- | --- | --- |
| `owner/daily/YYYY-MM-DD.md` | Owner | **Authoritative** record of the owner's working day | What the owner did, decided and carried forward, across every engagement |
| `<workspace>/brain/daily/…` | Workspace | **Optional** per-case notes | Case-specific working notes that belong with the engagement |

"Optional" is a commitment, not a hedge:

- no ritual requires a workspace daily to exist;
- nothing degrades if one never exists;
- the owner daily is never composed by reading one. There is no fallback path
  that reaches into a workspace root.

Spec 014's workspace daily and its reviewed template are neither removed nor
modified. A user working in a single workspace who ignores the owner daily is
unaffected.

## The confidentiality consequence, stated plainly

Owner scope crosses engagements. A page in owner scope is present in a session
about a different client. An owner daily that accumulates client-confidential
detail therefore becomes a cross-engagement leak surface — and it becomes one
gradually, one convenient paste at a time, which is why the rule has to be
structural rather than a reminder.

**The admission rule: the owner daily records what the owner did and decided,
and may name the client and project it was done for. Engagement content is
referenced by link and never copied.**

The rule turns on Proposal 006's distinction between engagement identity and
engagement content. Proposal 006 admits a pointer, an identifier or an
owner-authored sanitized synthesis into owner scope, and keeps raw findings,
figures, deliverable bodies, credentials and stakeholder dynamics in the
workspace that owns them. A client or project name is an identifier: it is what
lets a day be reconstructed and a career trajectory read, and it carries none of
the material the engagement is confidential about. It remains confidential
metadata even so, to be labelled and minimized rather than treated as free.

| Admitted in `owner/daily/` | Denied |
| --- | --- |
| "Reviewed the cost-baseline model and rejected the bottom-up build" | The baseline, the numbers, the client's cost structure |
| A link to the workspace project page where the detail lives | A copy of that page's `## Current truth` block |
| "Over-prepared the steering prep and under-decided" | Named attendees, their positions, their reactions |
| A learning candidate stated generically | The engagement finding that produced it |
| The client and project worked on, named as identifiers | Findings, figures, deliverable material and stakeholder dynamics from that engagement |

Two clauses close the obvious gaps:

- **A link is a pointer, not a read.** Referencing a workspace page does not
  grant owner scope access to it. Resolution happens later, in a workspace
  session, under workspace authority.
- **When an entry cannot be written without the confidential detail, it is not
  an owner-daily entry.** It belongs in the workspace daily, and the owner
  daily carries the link and the decision only.

The naming allowance is not an exception carved locally. It is the contract:
Proposal 006 admits engagement identity into owner scope and excludes engagement
content, and the owner daily applies that line unchanged. Widening it — copying
a finding, a figure or a stakeholder position into a page that crosses
engagements — requires amending Proposal 006, not this document.

## Why this creates no cross-workspace aggregation

The owner daily is written **forward**, entry by entry, by the owner at the
moment of work. Its chronology is a property of when the owner wrote, not the
output of a scan.

- nothing enumerates workspaces;
- nothing reads or merges workspace roots;
- the shipped operations cannot address a path outside the owner root, and a
  standing grant cannot authorize a write outside its own segment;
- `collect` has no request shape that means "everything": it requires named
  pages and a declared purpose, and refuses a whole-root projection.

The day is unified because it was recorded once, not because anything was
gathered.

## Divergence from the reviewed template

Proposal 002 put the daily template up for review with six sections. The owner
form keeps that shape, with two changes forced by scope.

| Proposal 002 field | Owner-scope form | Reason |
| --- | --- | --- |
| `Related scope → Clients` | Kept, as identifiers | Proposal 006 admits engagement identity; it is the name only, never the engagement's content |
| `Related scope → Projects` | Kept, as identifiers and links | A link is a pointer, not a copy |
| `Priorities`, `Notes`, `Carry forward` | Unchanged | The reviewed shape holds |
| `Decisions surfaced` | Kept; records the decision and a link, not the facts behind it | Keeps engagement facts out of owner scope |
| `Learning candidates` | Kept; stated generically | Feeds Proposals 009 and 033 without carrying engagement content |

## Template — `owner/daily/YYYY-MM-DD.md`

This is a recommended shape, not an admission gate. Decision `OATL` is explicit
that templates and named operations are reliability aids, and the owner may
keep a free-form day. What the shape buys here is specific: every heading below
is one an operation can append under during the day, and a page created without
them can only be edited by hand.

```md
# Daily — YYYY-MM-DD

> Owner-scope record of the working day. This page crosses engagements: record
> what you did and decided, name the client and project you did it for, link to
> the workspace page that holds the detail, and never copy engagement content
> here.

## Related scope
- **Clients:** <client name as an identifier — no engagement content>
- **Workspaces:** <workspace identity or page link>
- **Projects:** <project name, with a link where one exists>

## Priorities
1.
2.
3.

## Notes
-

## Decisions surfaced
- <what was decided, plus a link to the authoritative record — not the facts behind it>

## Learning candidates
- <stated generically; link to `owner/craft/` or `owner/learnings/` once promoted>

## Carry forward
-
```

## Operations

| Moment | Operation | Result |
| --- | --- | --- |
| First contact of the day | `create-page` | Today's page exists once; a second call preserves what is there and reports `unchanged`, never an overwrite |
| During the day | `append-entry` | Entry added under a heading the page already declares; the same idempotency key produces one entry |
| Closing the day | `append-entry` under `## Carry forward` | The carry-forward is recorded as a dated line |
| Referencing a workspace page | written inline in the entry today | `link` is accepted and not yet built; the reference is a pointer either way, and nothing is read or copied |
| Correcting a field in place | owner edit today | `set-field` is accepted and not yet built |

Three of Proposal 007's ten operations are implemented: `collect`,
`create-page` and `append-entry`. The rest are accepted and unbuilt, and where
one is named above the effect is available today as a direct owner edit.
Idempotency is what lets a start-of-day or end-of-day ritual be invoked twice,
or invoked after a partial session, without producing two pages or a duplicated
day — the implementation replays a completed operation under the same key
instead of repeating it. A skill does not edit the file; it asks for a named
operation and reports what came back. The owner may always edit the page
directly.

An unavailable operation degrades automation and managed persistence. It does
not block conversation, read-only reasoning, direct owner edits or preparation
of a reviewable draft. A start-of-day or end-of-day conversation still happens
and still produces something the owner can keep; what is lost is the recorded
write, and it must be reported as proposed rather than written.

## Staleness threshold

A maintenance pass over the owner atlas would report a durable fact whose as-of
date has aged past the threshold its segment declares. No such pass is proposed
yet. This segment declares that its pages **never go stale**, and the reason is
in what the page is.

A daily page is a dated snapshot of one working day. It is finished when the day
ends, and nothing on it claims to be true today, so there is no as-of date to
age and nothing to re-confirm. Applying a threshold here would flag every page
older than the window — one finding per elapsed day — and bury the findings that
matter under pages that were never going to age.

That is a **proposed default and open to reviewer adjustment**. A reviewer who
wants unresolved `## Carry forward` lines surfaced after some number of days is
asking for a different check rather than a different threshold, and it belongs
with a maintenance pass and its degradation classes, not here.

## Consequences

- A start-of-day briefing and an end-of-day close gain the day record both were
  blocked on, and either can be specified without a task authority or a provider
  contract.
- The shipped `retro` ritual, which already reads `owner/daily/` when it exists,
  gains a declared shape to read rather than whatever the day happens to hold.
- "What did I do on day Z" is answered by opening one page, with no
  cross-root read and no aggregation.
- `<workspace>/brain/daily/` becomes optional per-case notes. Spec 014 still
  creates it and its template; nothing is deleted and the non-overwriting
  guarantee is untouched.
- Proposal 002's reviewed template remains the workspace form. The owner form
  is the variant above, and the two are allowed to differ.
- The owner daily is the only owner-scope page that routinely references
  workspace content. That reference is a link and stays a link. Any proposal
  that would resolve those links into owner scope is a new decision and is not
  implied here.
- No role is affected. Owner content reaches other readers only as a bounded,
  purpose-declared projection: the owner session and Maestro may receive one;
  Walter may receive a stale-checked self-proxy projection; Case, Client Account
  and PA Expert agents receive an explicitly authorized attenuated excerpt or
  pointer; nobody receives the owner root.

## Explicit non-decisions

- no workspace, client account or managed root becomes readable or writable
  from owner scope;
- no aggregation, enumeration, merge or synchronization of workspace dailies is
  created;
- Spec 014's workspace daily and template are neither removed nor modified;
- no task system, mirror or synchronization contract is created;
- no memory layer, promotion rule or sanitization route is changed — the owner
  daily is not memory input;
- no role is added or modified, and no agent gains a tool grant or a projection
  it was not already entitled to;
- no operation is added to Proposal 007's accepted set of ten, and none of the
  seven still unbuilt is reported available;
- no skill is registered, and no standing grant, schedule or headless invocation
  is created;
- merging this document activates no runtime capability. The three implemented
  operations are available because PR #286 shipped them, not because of
  this text.
