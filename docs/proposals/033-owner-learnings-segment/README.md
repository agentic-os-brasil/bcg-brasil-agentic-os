# Proposal 033 — Owner learnings segment

**Status:** request for decision. Declares the shape of the pages the shipped
`retro` ritual already promotes into `owner/learnings/`; it registers no skill
and schedules no ritual.

**Original contribution:** Marcelo Petrof Sanches.

**Depends on:** Proposal 006 (owner scope contract), Proposal 007 (owner atlas
operations), PR #286 (the shipped operations, standing grants and `retro`
ritual) and decision `OATL`.

**Unblocks:** a future ritual that routes the learning candidates written on
daily pages into this segment, which needs the shape defined here; and a future
maintenance pass over the owner atlas, which would read it. Neither is proposed
yet.

## Reading the proposals this document cites

Proposal 005 was accepted and then removed from the working tree. It is still
readable from history:

```sh
git show 760abd8:docs/proposals/005-skill-consolidation/README.md
```

That is the reconciled version, and the `record-learning` deferral cited below
is in it. `2fe2a50` carries a later unreconciled draft of the same proposal on
another branch.

## The segment is already writable; this declares its shape

PR #286 shipped `internal/atlasops` with `collect`, `create-page` and
`append-entry`, revocable standing grants, and the `bcgos atlas owner` and
`bcgos atlas grant` verbs. `create-page` creates parent directories on demand,
so a claim page written to `owner/learnings/<claim-slug>.md` brings the segment
into existence with it. Nothing has to bootstrap the folder and nothing has to
admit it.

The shipped `retro` ritual is already the intake. After recording the
retrospective and the week's objective evidence, it offers to promote a durable
claim into `owner/learnings/` with `create-page`, one page per claim, on the
shape declared below. Two mechanical facts govern that promotion and are worth
stating up front. A standing grant covers a single page family, so an occurrence
woken under a weekly retro grant scoped to `development/retros/` can write the
retrospective but not a learning — the promotion is attended only, and the skill
says so. And `create-page` preserves an existing page rather than replacing it,
so a re-promoted claim converges instead of overwriting the claim already there.

What was missing was never permission. Spec 014 creates
`owner/learnings/index.md` and describes it as holding durable professional
learnings, correctable and linked to their sources where applicable. That is a
name, an index and a sentence: no page kind, no recommended shape, and no
declared headings for anything to append under. This proposal supplies them.

Two lines of work already point at the segment:

| Item | Source | What it assumes |
| --- | --- | --- |
| The `retro` promotion step | Shipped, in `bundles/base/skills/retro/` | `owner/learnings/` as the destination for a durable claim, one page per claim |
| `record-learning` | Proposal 005, deferred | A durable owner-private destination for a learning |

The other two owner knowledge segments were given contracts. Proposal 008
defined `owner/development/`; Proposal 009 defined `owner/craft/` and, in
passing, absorbed the one other bare index Spec 014 created. `owner/learnings/`
was left as it was found — and it is the one segment a shipped ritual already
writes into, which is why the shape should be decided rather than fixed by
whichever caller gets there first.

It does not revive `record-learning` and it registers no ritual of its own. It
declares the page the shipped promotion writes, and the page any future routing
ritual would write.

## Why this content is inside Proposal 006's admitted set

Proposal 006 admits content the owner authored whose data subject is not a third
party, and names two shapes that content takes: pages about the owner, and pages
about the work itself. Methods and working preferences are the second shape —
the owner is plainly not their subject — and the operative test for them is
*would this page require a second person's consent to exist, and does it carry
engagement content*.

A learning is of that second shape, and passes both tests on the same footing as
a method.

| Page kind | Data subject | Author | Requires third-party consent |
| --- | --- | --- | --- |
| Method (Proposal 009) | The technique | The owner | No |
| Style (Proposal 009) | The owner | The owner | No |
| Learning (this proposal) | A class of work | The owner | No |

The subject of a learning is a kind of work, not a person and not an
engagement. Nothing in it belongs to anyone who could ask for it to be
corrected or withdrawn. That is what makes the segment admissible, and it is
also what the segment's central discipline — stated below — exists to protect.

## Three segments, one question each

Owner knowledge is now three segments. The boundary between them is the part a
reviewer should probe, so it is stated as a single question per segment and
then worked through an example.

| | `owner/development/` (Proposal 008) | `owner/craft/` (Proposal 009) | `owner/learnings/` (this proposal) |
| --- | --- | --- | --- |
| Question answered | How the owner is **growing** | **How** the owner works | **What** the owner has come to know |
| Subject | The owner's trajectory | Procedure | A claim about the world of the work |
| Holds | Objectives, evidence, feedback, career reviews | `methods/` for reusable technique, `style/` for personal calibration | Durable professional insight carried across engagements |
| Can be wrong | No — it is a record of what happened | No — a procedure is well or badly suited, not false | **Yes** — a claim about the world can turn out false |
| Ages by | Cycle boundary | Disuse | Contradiction |

The worked contrast, on one subject:

| Statement | Segment | Why |
| --- | --- | --- |
| "Run the stakeholder map before the kickoff, not after." | `craft/methods/` | An instruction another practitioner could follow and get a comparable result |
| "I write the recommendation first and the analysis second." | `craft/style/` | True of this owner, not of the craft; adopting it would import one person's calibration |
| "Procurement transformations stall at the category-owner layer, not at the executive layer." | `owner/learnings/` | A claim about how a kind of work actually goes; it could be shown to be false |
| "Sharpen how I take a room from analysis to decision." | `owner/development/` | An objective for the owner, with evidence accumulating beneath it |

**The operational test is falsifiability.** A learning is the only one of the
three knowledge kinds that can be *wrong*. If a statement could be contradicted
by the next engagement, it is a learning and needs a confidence and an as-of
date. If it could only be found unsuited, ignored or outgrown, it is a method, a
style or an objective. A statement that cannot be wrong and cannot be followed
is neither; it stays on the daily page.

## Segment layout

```text
owner/learnings/
  index.md                                   # created by Spec 014
  <claim-slug>.md
```

**One page per claim, not a running log.** A claim carries its own scope,
confidence, as-of date and revision history, and it is retrieved, projected and
superseded as a unit. A line in a shared log has none of those. It is also
unreachable by the operations that exist: `append-entry` adds a line under a
heading a page already declares and cannot rewrite one, so a claim buried in a
log could not be corrected in place by any implemented operation. The slug names
the claim, not the engagement it came from.

`index.md` is retained as created by Spec 014 and points at each claim page —
written today as an owner edit, since `link` is accepted and not yet built. It
is a directory, never a second copy of the claims.

## Source without engagement content — the segment's central discipline

A learning is almost always drawn from a specific engagement. That is where the
risk lives, and it is why this section is the load-bearing one.

Under Proposal 006, engagement identity is not engagement content: the contract
admits a pointer, an identifier or an owner-authored sanitized synthesis, and
keeps raw findings, figures, deliverable bodies and stakeholder dynamics in the
workspace that owns them. The engagement may therefore be **named as the source**
of a claim, so that the claim can be traced and weighed. The name remains
confidential metadata to be labelled and minimized; it is not made free by being
attribution.

**The claim itself must be stated as a generalization.** A page that can only be
written by restating what was found at a client is not a learning; it is an
engagement finding filed in the wrong scope.

| Admitted | Denied |
| --- | --- |
| "Procurement transformations stall at the category-owner layer" | The client's category structure, its spend, its named owners |
| "Observed on <engagement>, <date>" as attribution | What was concluded on that engagement |
| A link to the workspace page holding the underlying analysis | A copy of that analysis, its figures or its exhibits |
| "Three engagements, two sectors" as the weight behind a claim | A per-engagement account of each |

This is the reason a learning is safe to carry across engagements when the
engagement's own pages are not. A page in owner scope is present in a session
about a different client. A generalization can be, because it belongs to the
owner's professional judgement; a finding cannot, because it belongs to the
client. The generalization is performed by the owner, at the moment of writing.
Nothing in the system performs it on the owner's behalf.

When a claim cannot be stated without the confidential detail, it is not yet a
learning. It stays where the detail lives until the owner can state it without.

## Learnings are revisable

A claim that later proves wrong is **superseded in place, with its date and its
reason.** This segment does not delete it.

That is a policy of this segment, not a limit of the operation set. Proposal 007
accepts `archive`, `restore`, `redact`, `delete` and `export`; none of them is
built, a permanent delete would require an explicit owner confirmation, and the
owner is free to remove their own file at any time. The reason to keep a
superseded claim is professional rather than mechanical:

- a retracted belief is professionally useful — knowing that a claim was held,
  on what basis, and what broke it is worth more than the claim's absence;
- deleting it destroys the only evidence that the owner's judgement changed,
  which is exactly what a career-length record should preserve;
- a claim silently removed will be re-derived, at cost, from the same
  experience.

Three revision shapes are available, and each is a dated line under
`## Revisions`, with the confidence field corrected alongside it:

| Shape | Meaning |
| --- | --- |
| `sharpened` | The claim holds and is now stated more precisely |
| `narrowed` | The claim holds for fewer situations than believed; the scope field is corrected |
| `superseded` | The claim no longer holds; the page is marked superseded and linked to whatever replaced it, if anything did |

A superseded page keeps its full text, its original grounding and its history.
The header states its status; the body is not rewritten.

## Template — `owner/learnings/<claim-slug>.md`

```md
# Learning — <claim in a few words>

> Owner-authored durable claim. State the generalization, never the engagement
> content behind it. The source engagement may be named; its findings, figures
> and deliverable material may not.

## Claim
- <one line. If it needs more than one, it is more than one learning.>

## Snapshot
- **Status:** active | superseded
- **Opened:** YYYY-MM-DD
- **Last confirmed:** YYYY-MM-DD

## Holds for
- **Situations:**
- **Does not hold for, or untested:**

## Grounded in
- YYYY-MM-DD — <engagement named as source> — <what was observed, stated as a generalization> — <link to the workspace page holding the detail, where one exists>

## Confidence
- **Level:** tentative | working | held
- **As of:** YYYY-MM-DD
- **What would change it:**

## Revisions
- YYYY-MM-DD — sharpened | narrowed | superseded — <what changed and why> — <link to the superseding claim, if any>

## Related
- [Learnings index](index.md)
- <link to a method, style or objective page this claim bears on>
```

This is a recommended shape, not an admission gate. Decision `OATL` is explicit
that templates and named operations are reliability aids, and the owner may
author free-form private Markdown here; a page that departs from the shape is
still a page and is never rejected for it. `## Claim`, `## Holds for`,
`## Grounded in` and `## Confidence` are the sections the shape asks for, and a
page that omits them is simply harder to retrieve, to weigh and to append to —
`append-entry` never creates a heading, so grounding cannot be added later to a
page that never declared where it goes. Extra sections are the owner's business.

## Operations

| Moment | Operation | Result |
| --- | --- | --- |
| A claim is first written, or promoted from a retro | `create-page` | The page exists once from this shape; a second call preserves what is there and reports `unchanged`, never an overwrite |
| Further experience bears on the claim | `append-entry` under `## Grounded in` | A dated line; the same idempotency key produces one entry |
| The claim is sharpened, narrowed or superseded | `append-entry` under `## Revisions` | A dated revision line |
| Confidence, scope, status or last-confirmed changes | owner edit today | `set-field` is accepted and not yet built |
| The claim bears on a method, style, objective or successor claim | written inline in the entry today | `link` is accepted and not yet built |

Three of Proposal 007's ten operations are implemented: `collect`,
`create-page` and `append-entry`. The rest are accepted and unbuilt, and where
one is named above the effect is available today as a direct owner edit. A skill
does not edit a claim page; it asks for a named operation and reports what came
back. The owner may always edit the page directly.

An unavailable operation degrades automation and managed persistence. It does
not block conversation, read-only reasoning, direct owner edits or preparation
of a reviewable draft. A promotion conversation still reaches a claim and still
produces a draft the owner can keep; only the recorded write is lost, and the
runtime must distinguish "proposed" from "written" rather than reporting a claim
as filed.

## Who fills and who prunes the segment

Two things act on this segment and neither is created here.

- **Promotion is the intake, and it already ships.** The shipped `retro` ritual
  offers a claim at the close of the weekly walk, one page per claim, each
  confirmed individually and attended only. A future ritual could route the
  `## Learning candidates` lines the owner already wrote on daily pages here on
  the same terms. Nothing else fills the segment: there is no listener, no scan
  and no automatic promotion, and a grant scoped to another segment cannot reach
  this one.
- **A maintenance pass**, whenever one is proposed, is what would surface the
  claims that need attention — a page whose last-confirmed date has gone stale,
  or two claims that contradict each other. It would surface them for the
  owner's judgement; it would resolve nothing on its own.

The segment is therefore filled by confirmed promotion and corrected by
confirmed revision. Both ends stay under the owner's hand.

## Staleness threshold

A maintenance pass over the owner atlas would report a durable fact whose as-of
date has aged past the threshold its segment declares. No such pass is proposed
yet. A learning is the one owner knowledge kind that can be *wrong*, so this
segment's clock is the shortest of the three.

| Confidence | Threshold since `Confidence → As of` | Why |
| --- | --- | --- |
| `tentative` | **90 days** | The claim is explicitly waiting for the next engagement to confirm or break it; a quarter without either is itself information |
| `working`, `held` | **180 days** | A claim carried across engagements should meet the world at least twice a year |
| Page marked `superseded` | **Never stale** | It is the record of a belief that was held. It is not re-confirmed and is never re-opened by ageing |

Ninety and 180 days are **proposed defaults and open to reviewer adjustment**,
not settled numbers. The finding is always "re-confirm this", never "this is
false": ageing is what puts a claim back in front of the owner's judgement, and
nothing in the pass can test whether a claim still holds.

## Consequences

- The claim the shipped `retro` promotion already produces acquires a declared
  destination shape, decided here rather than fixed by the first caller to write
  one.
- A future routing ritual inherits that same shape. It can be reviewed against a
  defined page rather than defining one by being merged.
- The operation split is settled here, so no later ritual has to guess it: a new
  claim is a `create-page`, and `append-entry` adds grounding or a revision to an
  existing claim. Both are implemented and no new operation is required.
- Proposal 005's `record-learning` deferral loses its stated blocker. The
  destination exists, is writable, and a shipped ritual already promotes into
  it; the ritual `record-learning` named remains unregistered and belongs to
  whichever proposal takes it up, not to this document.
- Owner knowledge is closed at three segments — development, craft, learnings.
  A fourth requires its own proposal against Proposal 006, not a folder added in
  passing.
- Spec 014 is not superseded. Its `owner/learnings/index.md` and its
  non-overwriting guarantee stand; an existing user page is never replaced, and
  the bootstrap output is unchanged.
- No briefing's input set is widened here. A ritual that reads the daily,
  development and craft projections is not extended to this segment by this
  document; adding a learnings projection is a separate decision.
- No role is affected. Owner content reaches other readers only as a bounded,
  purpose-declared projection: the owner session and Maestro may receive one;
  Walter may receive a stale-checked self-proxy projection; Case, Client Account
  and PA Expert agents receive an explicitly authorized attenuated excerpt or
  pointer; nobody receives the owner root.

## Explicit non-decisions

- no role is added to, or removed from, the agent catalogue, and no deferred
  role is revived;
- no skill is registered — the shipped `retro` ritual is unchanged by this
  document — and no promotion, hygiene or review ritual is scheduled;
- no standing grant is created, and no existing grant is widened to reach this
  segment;
- no operation is added to Proposal 007's accepted set of ten, and none of the
  seven still unbuilt is reported available;
- no client, engagement or deliverable content is admitted — the source
  engagement may be named, its content may not;
- no learning is promoted to managed, shared or organizational knowledge, and no
  promotion mechanism to them is created;
- no workspace, client account or managed root becomes readable or writable from
  owner scope;
- no bootstrap output is changed — Spec 014 continues to create
  `owner/learnings/index.md` until a spec revision says otherwise;
- no memory layer, promotion rule or eligibility policy is changed, and a
  learning is not memory input;
- merging this document activates no runtime capability. The three implemented
  operations are available because PR #286 shipped them, not because of
  this text.
