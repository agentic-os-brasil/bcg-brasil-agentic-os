# Proposal 008 — Owner development segment

**Status:** request for decision. Declares the shape of the pages the shipped
owner-atlas operations already write into `owner/development/`; it registers no
skill and schedules no ritual.

**Original contribution:** Marcelo Petrof Sanches.

**Depends on:** Proposal 006 (owner scope contract), Proposal 007 (owner atlas
operations), PR #286 (the shipped operations, standing grants and `retro`
ritual) and decision `OATL`.

**Unblocks:** the project-feedback and CDC capture rituals these page kinds
imply; a bounded development projection for a future start-of-day briefing; and
a future maintenance pass over the owner atlas. None of the three is proposed
yet. The `retro` ritual Proposal 005 deferred now ships and writes into this
segment.

## Reading the proposals this document cites

Proposals 003, 004 and 005 were accepted and then removed from the working tree.
They are still readable from history:

```sh
git show 760abd8:docs/proposals/003-people-cross-project-scope/README.md
git show 760abd8:docs/proposals/004-spoke-agent-roster/README.md
git show 760abd8:docs/proposals/005-skill-consolidation/README.md
```

Two cautions. `docs/proposals/003-qualification-unlock.md` is in the tree and is
a different document; the 003 cited below is the people cross-project scope
proposal above. And 005 survives in two forms — the reconciled disposition
matrix quoted below is the one at `760abd8`, while `2fe2a50` carries a later
unreconciled draft of the same proposal on another branch.

## The segment is already writable; this declares its shape

PR #286 shipped `internal/atlasops` with `collect`, `create-page` and
`append-entry`, revocable standing grants, and the `bcgos atlas owner` and
`bcgos atlas grant` verbs. `create-page` creates parent directories on demand,
so `owner/development/retros/` exists the moment a page is written into it. A
segment needs no bootstrapping, no registration and no admission to be writable.

The `retro` skill in `bundles/base/skills/retro/` already uses that surface. It
writes the weekly retrospective to `owner/development/retros/<YYYY-MM-DD>.md`,
appends evidence under an objective's evidence section in
`owner/development/objectives.md`, and then offers to promote a durable claim
drafted on the retrospective into `owner/learnings/` (Proposal 033). These pages
are written today, by working code, in whatever shape the caller happens to
pass.

So this proposal does not make the segment possible. It gives its pages a
recognizable, retrievable shape: what each page is called, which sections it
declares, and which of those a named operation can address. `append-entry`
never invents a section, so a heading a page does not declare is a heading no
ritual can write under. That is what is being decided here.

Two accepted reconciliations deferred professional-development features for one
reason, and the owner scope contract has now removed it.

| Deferred item | Source | Stated blocker | Status today |
| --- | --- | --- | --- |
| `career-keeper` | Proposal 004 | "requires an owner-private professional-development scope, not an account scope" | Scope exists; the role stays deferred and this proposal does not revive it |
| `retro` | Proposal 005 | owner-private development authority absent | Authority exists and the ritual ships; only the page shape was left undeclared |
| `record-learning` | Proposal 005 | same | Authority exists; the ritual stays unregistered (Proposal 033) |

Spec 014 created `owner/development/index.md` and described it as holding
professional-development objectives, retrospectives and evidence — a name with
no page kinds and no recommended shapes. This proposal supplies them. It does
not add to the role catalogue.

## Why this content is inside Proposal 006's admitted set

Proposal 006 admits content the owner authored whose data subject is not a third
party, and names two shapes that content takes: pages about the owner, and pages
about the work itself. Every page kind below is of the first shape and satisfies
the test by construction.

| Page kind | Data subject | Author |
| --- | --- | --- |
| Objectives | The owner | The owner |
| Retrospective | The owner's week | The owner |
| Project feedback | The owner's performance | The owner, recording what was said to them |
| CDC review | The owner's trajectory | The owner, recording what was communicated to them |

Feedback and review pages are the only ones that touch a second person, and
only as a **source**. Proposal 006 admits that attribution and then qualifies
it. A record of feedback the owner received may name who gave it, because the
subject of the page is the owner's own performance and the giver is what makes
the record verifiable. The name nevertheless stays personal metadata even though
it is not the page's subject: it must be labelled and minimized, and naming a
giver does not eliminate the third-party considerations attached to them.

The boundary that still holds is stated in the templates and repeated here:
**the page's subject must remain the owner's own performance.** Naming the giver
is attribution; assessing the giver is a page about a non-consenting third
party, which Proposal 003 deferred and Proposal 006 still denies — as is
feedback concerning anyone other than the owner. The test is whose conduct the
page describes.

## Distinct from `owner/self/`

Spec 013 already defines ten owner facets — identity, personal context,
professional role, communication style, voice, preferences, motivations,
quality bar, decision rules, working boundaries. The overlap is apparent, not
real.

| | `owner/self/` (Spec 013) | `owner/development/` (this proposal) |
| --- | --- | --- |
| Question answered | Who the owner **is** | How the owner is **growing** |
| Shape | Bounded current truth; exactly one `## Current` section per facet | Dated, evidence-bearing, accumulating |
| Facet set | Closed and versioned | Open — an objective is opened and retired by cycle |
| Change path | Interview, draft, digest, confirmation | Owner-authored pages, written directly or through a named operation |
| Growth | Deliberately bounded: SELF is "not an append-only diary" | Deliberately cumulative — the evidence *is* the value |

The two are complementary and must not merge. Folding development evidence into
SELF would break Spec 013's size, duplicate-paragraph and transcript-shape
rejection rules. Folding SELF into development would turn current truth into
history.

## Segment layout

```text
owner/development/
  index.md                                   # created by Spec 014
  objectives.md                              # one page, current cycle
  retros/YYYY-MM-DD.md                       # written by the shipped retro ritual
  project-feedback/YYYY-MM-DD-<project-slug>.md
  cdc/YYYY-MM-DD-cdc.md
```

`objectives.md` is a single page, not a folder. It is the one home for what the
owner is working on; retros, feedback and CDC pages reference it and never
restate it. None of the subdirectories needs to be created in advance.

| Page kind | Cadence | Created by | Maintained by |
| --- | --- | --- | --- |
| `objectives.md` | Continuous | `create-page` | `append-entry` under the objective's own evidence heading; a status change is an owner edit until `set-field` is built |
| Retrospective | Weekly | `create-page`, by the shipped `retro` ritual | `append-entry` |
| Project feedback | Once per project round | `create-page` | an owner edit; `link` to the objective it touches once `link` is built |
| CDC review | ~6 months | `create-page` | an owner edit per affected objective; `set-field` once built |

Proposal 007's accepted set has ten operations. Three are implemented —
`collect`, `create-page` and `append-entry` — and the table uses only those.
`set-field`, `link`, `archive`, `restore`, `redact`, `delete` and `export` are
accepted and not yet built; where one is named above, the effect is available
today as a direct owner edit, and a ritual must say which of the two happened.
A skill does not edit a page: it asks for a named operation and reports what
came back. The owner may always edit the page directly, and the named operations
are a reliability aid rather than a gate on their own Markdown.

## When an operation is unavailable

An unavailable operation degrades automation and managed persistence. It does
not block conversation, read-only reasoning, direct owner edits or preparation
of a reviewable draft. A ritual over this segment still walks the week, still
reaches a conclusion and still produces a draft the owner can keep; what it
loses is the recorded write. The runtime must distinguish "proposed" from
"written" — `atlasops` returns exactly those terminal states, plus `unchanged` —
and must never report a retrospective as filed when nothing was persisted.

## Fold versus reset — the rule that makes the segment coherent

The two feedback inputs are not the same kind of event and must not be handled
the same way.

| Input | Effect on `objectives.md` | Why |
| --- | --- | --- |
| Project feedback | **Folds in.** Existing objectives gain evidence; at most one new objective may be proposed | It is one project's view of one period — evidence, not verdict |
| CDC review | **Resets.** Every objective is retained, retired or replaced against the committee's view | It is the authoritative career-level synthesis and opens a new cycle |

"Reset" never means deletion. This is a policy of the segment rather than a
limit of the operation set: Proposal 007 does include `delete`, gated behind an
explicit owner confirmation and not yet built. A retired objective keeps its
statement and its full evidence log, marked retired and linked to the CDC page
that retired it. The cycle boundary is recorded, not erased. An owner who does
want a page gone remains free to remove it themselves.

## Templates

The templates below are recommended shapes, not admission gates. Decision `OATL`
is explicit that templates and named operations are reliability aids, and the
owner may author free-form private Markdown in this segment; a page that departs
from a template is still a page. What a template buys is retrievability and a
stable set of headings a named operation can address.

### `objectives.md`

```md
# Development objectives

> Owner-authored. One page for the current cycle. Objectives are current truth;
> evidence beneath each objective is append-only. A feedback giver may be named
> as the source; record no assessment of them.

## Snapshot
- **Cycle opened by:** <link to CDC page, or "initial">
- **Cycle opened:** YYYY-MM-DD
- **Next review due:** YYYY-MM-DD

## Objective <n> — <short name>
- **Statement:**
- **Why it matters:**
- **Observable when met:**
- **Status:** active | retired | replaced
- **Opened / last confirmed:** YYYY-MM-DD / YYYY-MM-DD

### Evidence — objective <n>
- YYYY-MM-DD — <what happened> — met | missed — <link to the daily or retro page>

## Retired
- YYYY-MM-DD — <objective> — retired by <link to CDC page> — <reason>

## Related
- [Retros](retros/)
- [Project feedback](project-feedback/)
- [CDC](cdc/)
```

The evidence heading carries the objective number because `append-entry`
**refuses a section heading that appears more than once** on a page. Two
objectives sharing a bare `### Evidence` heading would make the target
ambiguous, and the operation declines the write rather than filing under the
first match — the recommended shape is what keeps every objective addressable.

### `retros/YYYY-MM-DD.md`

```md
# Retro — YYYY-MM-DD

## Snapshot
- **Period covered:** YYYY-MM-DD to YYYY-MM-DD
- **Previous retro:** <link>

## What worked
-

## What did not
-

## Against each objective
| Objective | Where it showed up | Where it was missed |
| --- | --- | --- |
| <link> | | |

## Pattern across weeks
- <only if the same signal appears in the previous retro; otherwise omit>

## Learning
- <a durable claim, stated generically; drafted here, then promoted to its own page in `owner/learnings/` (Proposal 033)>

## Intention for next week
- **Intention:**
- **Observable when met:**

## Related
- [Objectives](../objectives.md)
```

### `project-feedback/YYYY-MM-DD-<project-slug>.md`

```md
# Project feedback — <project slug> — YYYY-MM-DD

> Records feedback the owner received, as recorded by the owner. The giver may
> be named, as attribution for what was said about the owner. Do not assess the
> giver, and do not record feedback about anyone else.

## Snapshot
- **Project / workstream:** <link to the workspace project page>
- **Period covered:**
- **Round:** mid-project | end-of-project
- **Given by:** <name, role, or both — attribution only, minimized>

## Strengths, as received
-

## Development areas, as received
-

## Owner reading
- **What I accept:**
- **What I would qualify:**

## Folds into
- **Objective touched:** <link> — reinforces | extends | contradicts
- **New objective proposed:** yes | no

## Related
- [Objectives](../objectives.md)
```

### `cdc/YYYY-MM-DD-cdc.md`

```md
# CDC — YYYY-MM-DD

> Career-level synthesis. This page opens a new objective cycle. A committee
> position may be attributed to the member who stated it; record no assessment
> of any member.

## Snapshot
- **Cycle reviewed:** YYYY-MM-DD to YYYY-MM-DD
- **Outcome, as communicated:**
- **Next CDC due:** YYYY-MM-DD

## Trajectory, as communicated
-

## Strengths carried forward
-

## Priorities set for the next cycle
1.
2.

## Objective reset applied
| Prior objective | Disposition | Reason |
| --- | --- | --- |
| <link> | retained \| retired \| replaced | |

## Owner reading
-

## Related
- [Objectives](../objectives.md)
```

## Staleness threshold

A maintenance pass over the owner atlas would report a durable fact whose as-of
date has aged past the threshold its segment declares. No such pass is proposed
yet; this is the declaration for this segment when one is.

| Page kind | Threshold | Why |
| --- | --- | --- |
| Each active objective in `objectives.md` | **90 days** since `last confirmed` | An objective nobody has confirmed for a quarter has been met, abandoned or forgotten; each of the three wants the owner's attention |
| Retrospective, project feedback, CDC review | **Never stale** | Each is a dated record of something that happened, not a claim about the present. It cannot age out of being true |

Ninety days is a **proposed default and open to reviewer adjustment**, not a
settled number. It was chosen to fall well inside the CDC cycle, so an objective
is re-confirmed at least once before the review that resets it. The finding is
always "re-confirm this", never "this is wrong".

## Consequences

- The shipped `retro` ritual acquires a declared page shape and a stable set of
  addressable headings. A retrospective that departs from the shape is still
  valid; it is only harder to retrieve and to append to.
- A start-of-day briefing, whenever one is proposed, gains a bounded development
  projection to read through `collect`, under Proposal 007's reader rules.
- `objectives.md` becomes the single source for development objectives. Any
  later page that restates an objective instead of linking to it is a defect.
- Spec 014 is not superseded. Its `owner/development/index.md` and its
  non-overwriting guarantee stand; an existing user page is never replaced.
- `career-keeper` stays deferred. This proposal deliberately meets the need
  without a role. Owner content reaches other readers only as a bounded,
  purpose-declared projection: the owner session and Maestro may receive one;
  Walter may receive a stale-checked self-proxy projection; Case, Client Account
  and PA Expert agents receive an explicitly authorized attenuated excerpt or
  pointer; nobody receives the owner root.

## Explicit non-decisions

- no role is added to, or removed from, the agent catalogue, and no deferred
  role is revived;
- no skill is registered here — the shipped `retro` ritual is unchanged by this
  document, and no feedback or review ritual is registered;
- no operation is added to Proposal 007's accepted set of ten, and none of the
  seven still unbuilt is reported available;
- no standing grant, cadence or schedule is created;
- no performance, compensation or staffing system is read, mirrored or treated
  as authoritative;
- no third-party record is created — a feedback giver is named as the source of
  feedback about the owner, never made the subject of a page or of an
  assessment, and attribution does not eliminate the labelling and minimization
  Proposal 006 requires;
- no memory layer, promotion rule or eligibility policy is changed;
- merging this document activates no runtime capability. The three implemented
  operations are available because PR #286 shipped them, not because of
  this text.
