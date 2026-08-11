# Proposal 006 — Owner scope contract

**Status:** request for decision. Defines an authority; activates no runtime capability.

**Original contribution:** Marcelo Petrof Sanches.

**Depends on:** nothing. This proposal is the entry point of the owner layer.

**Unblocks:** Proposals 007–010, 018–023, 028–029 and 032–033 (see *Consequences*).

## Executive summary

Three accepted reconciliations deferred three different features for the same
stated reason: no owner-private scope exists.

| Deferred item | Source | Stated blocker |
| --- | --- | --- |
| Cross-project people record | Proposal 003 | "requires a future owner-private contract" |
| `career-keeper` | Proposal 004 | "requires an owner-private professional-development scope, not an account scope" |
| `retro`, `record-learning` | Proposal 005 | owner-private development authority absent |

Proposal 003 also enumerated seven requirements that any owner-private contract
must satisfy. This proposal answers those requirements for the **narrowest
useful subset**: content the owner authored whose data subject is not a third
party.

It deliberately does **not** attempt the hardest case. Third-party colleague
records stay deferred under Proposal 003's full requirement set.

## The narrowing that makes this tractable

Proposal 003's requirements were written against a people record — a document
about someone who is not the owner, who did not consent, and who has correction
rights. Almost all of the difficulty lives in that fact.

This proposal admits into owner scope only content that the owner authored and
**whose data subject is not a third party**:

| Admitted | Denied |
| --- | --- |
| The owner's development objectives, retrospectives and self-assessment | Any page whose subject is a third party |
| Feedback the owner received, with its source attributed | Feedback about other people; any assessment of a third party |
| Methods and working preferences the owner authored | Engagement content — findings, figures, deliverables, stakeholder dynamics |
| The owner's own daily work log, naming the engagements worked on | Cross-workspace aggregation of any kind |

A page that would require a second person's consent to exist is out of scope by
construction, not by policy reminder.

Admitted content takes two shapes, and both are intended. Some pages are **about
the owner** — objectives, retrospectives, the record of a working day. Others are
**about the work itself** — a method the owner uses, a preference for how to do
something, a durable claim about how a kind of engagement tends to go. The second
shape has no personal data subject at all, which is why it raises none of the
difficulty Proposal 003 identified. What unites them is negative and is the
operative test: **no page in owner scope has a third party as its data subject.**

### Two boundaries that are easy to misread

**Source is not subject.** A record of feedback the owner received may name who
gave it. The subject of that page is the owner's own performance; the giver is
attribution, which is what makes the record verifiable and useful. This does not
admit a page *about* that person, an assessment of them, or feedback concerning
anyone other than the owner. The test is whose conduct the page describes.

**Engagement identity is not engagement content.** The owner's own record of
which engagements they worked on is their professional history, and an
organization ordinarily maintains it. Owner scope therefore admits the name of a
client or project as an identifier, so a day can be reconstructed and a career
trajectory read. It does not admit what was learned inside that engagement:
findings, figures, deliverable material and stakeholder dynamics stay in the
workspace that owns them, referenced by link rather than copied.

Both boundaries are narrow on purpose. Widening either one requires amending
this proposal, not a local exception in a segment proposal.

## Answering Proposal 003's seven requirements

1. **Independent owner scope.** Owner content is stored under the existing
   local owner atlas root created by Spec 014, distinct from workspace, client
   account and managed bundle roots. This proposal does not reuse
   `client_account_agent`, or any account-scoped role, as a global owner.
2. **Minimal record.** Admission is closed by the table above. Fields outside
   the accepted segment templates are rejected rather than stored as free text.
3. **Explicit promotion.** Nothing enters owner scope automatically. Every write
   originates from a user-invoked operation. There is no workspace enumeration,
   no cross-bundle link and no automatic memory aggregation.
4. **Purpose and readers.** The reader set is closed to the owner's own session.
   Owner content is never placed in a packet sent to a case, account, expert or
   review role.
5. **Human rights over the record.** Content is owner-authored and locally
   stored; the owner may read, correct and delete any page directly. Writes are
   non-destructive and carry provenance, so correction never silently loses a
   prior value.
6. **No reverse leakage.** Owner content is never copied into a workspace or
   account root. The prohibition is symmetric: workspace and client material is
   never copied into owner scope either.
7. **Runtime-neutral enforcement.** Enforcement lives in the shared local
   authorization core invoked by the command layer, not in prompt instructions.
   A runtime without that core reports the capability unavailable.

## Execution route — no new role is created

The role catalogue is a closed list with `max_depth: 1`. Maestro is a hub with
no tools and no direct read of owner facets. Neither is changed here.

Owner-scope reads and writes are performed by the **command layer**, exactly as
canonical memory already works: `bcgos memory dream daily` is a deterministic
operation that a skill invokes through the installed adapter, and the shipped
skill states plainly that it must never write memory files itself.

This proposal adopts that precedent unchanged:

```text
skill  →  installed runtime adapter  →  named owner-scope operation  →  write
```

No role gains a persistence grant. No agent-to-agent edge is added. No agent
reads owner content. Proposal 007 specifies the operation set.

## Consequences

- Proposals 008–010 may define owner atlas segments within an authorized scope.
- Proposals 018–023 may specify skills that read and write those segments
  through the command layer.
- Proposal 003's people record stays deferred and unchanged. Admitting it
  requires its own proposal satisfying requirement 2 and requirement 5 for a
  non-consenting data subject.
- An owner-scoped agent role remains unavailable. Should one ever be proposed,
  it must satisfy Proposal 004's governed rules in full and is not implied by
  this document.
- Spec 014 is not superseded. Its bootstrap and its non-overwriting guarantee
  remain authoritative.

## Explicit non-decisions

- no role is added to, or removed from, the agent catalogue;
- no agent receives a tool grant, a new packet type or a user channel;
- no people, task, calendar, email or chat authority is created;
- no memory layer, promotion rule or eligibility policy is changed;
- no runtime is reported available by merging this document.
