# Proposal 027 — Account context root

**Status:** deferred; specifies the acceptance bar for an account atlas root.
This document requests no root, no operation and no role change.

**Original contribution:** Marcelo Petrof Sanches.

**Depends on:** Proposal 003 (people and account scope), Proposal 006 (owner
scope contract), Proposal 007 (owner atlas operations), Spec 014 (human atlas
bootstrap), Spec 024 (governed account context promotion) and decision `OATL`.

**Unblocks:** nothing. It exists so that a future proposal starts from a
requirement list rather than from the original idea, and so that the question is
not reopened once per account-scope proposal.

## Reading the proposals this document cites

Proposals 003 and 004 were accepted and then removed from the working tree. They
are still readable from history:

```sh
git show 760abd8:docs/proposals/003-people-cross-project-scope/README.md
git show 760abd8:docs/proposals/004-spoke-agent-roster/README.md
```

One caution. `docs/proposals/003-qualification-unlock.md` is in the tree and is
a different document; the 003 cited below is the people cross-project scope
proposal above.

## What this document withdraws

An earlier framing of this proposal placed the workspace client page and the
workspace people pages under `client_account_agent`, and reconciled that with
the role's contract by arguing that a bounded `collect` projection of a
packet-named page is itself the "minimum mediated packet" the contract permits.

That argument is withdrawn in full, and not softened, for two reasons.

- It redefined a term against its intent. The contract line —

  > Do not read raw case workspaces; receive only minimum mediated packets.

  denies the role reach into a workspace. Reading a page from
  `<workspace>/brain/` is reading a case workspace, whatever envelope the page
  arrives in. Naming the page in a packet changes how the read is delivered, not
  what is read. Maintenance of a page is moreover a standing responsibility, not
  a single mediated read, so no packet construction disposes of the objection.
- It inverted the one direction of travel Proposal 003 admitted between the two
  layers:

  > client/account context receives only explicit `account_safe` promotions from
  > workspaces under Spec 024.

  Content moves workspace-to-account, reviewed, one statement at a time. A
  maintenance responsibility running account-to-workspace is the reverse of that
  route, not an instance of it.

Workspace client and people pages are therefore outside the account role
entirely. They belong to the workspace root and to `case_agent`, the role bound
to that root; no accepted document assigns them elsewhere. Nothing of the
withdrawn argument is retained below.

## What the account role does own, and where it has to put it

The role's own definition states the ownership plainly:

> The Client Account Agent owns curated account context and relational judgment.

That is real content: the durable, reviewed framing of an account across the
engagements run for it, held above any single workspace and below the owner's
private scope. It is the content that makes the role worth having.

There is no declared atlas root for it.

Spec 014 creates exactly two roots and reports a third:

| Root | Location | Declared by |
| --- | --- | --- |
| Owner | `<local BCGOS data>/atlas/owner/` | Spec 014, `bcgos atlas init` |
| Workspace | `<workspace>/brain/` | Spec 014, `bcgos atlas init` |
| Managed | shipped in the versioned managed OKF bundle | Spec 014, not created in user data |

`internal/atlas` declares those three and no others: its status structure
carries exactly a `managed`, an `owner` and a `workspace` pointer. This was
re-checked against current `main` before the finding was restated here, and it
survives the owner-atlas work: PR #286 added an owner-root resolver and the
first three operations over that root, and added no fourth root. Proposal 007
gives the owner root an operation set of ten and binds every one of them to it —
path resolution is canonical and descriptor-anchored, and cross-root writes are
rejected. No accepted document declares an account root, and no accepted
operation can reach one.

The nearest existing thing is not a root. Spec 024's governed promotion writes a
signed account record per promotion under
`accounts/<account-id>/promotions/<promotion-id>.json`, holding one reviewed
statement of at most 1,000 bytes with its source hash, approver, classification
and validity window. It is deliberately not navigable: reads require a separate
`read_account` capability **and a known promotion ID**, and the core exposes no
enumeration surface at all. That is the correct design for a promotion ledger
and the wrong shape for curated context. A ledger of statements that cannot be
listed is not a place to keep a framing that has to be read as a whole,
corrected over time and superseded.

So the position today is exact, and should be stated rather than engineered
around: **the account role is defined as the owner of curated account context,
and that context has no declared durable home.** Every account-safe promotion
that succeeds under Spec 024 lands in a record that can be fetched by ID and
never browsed. The curation the role is named for has nowhere to accumulate.

## The only admitted inbound path

Whatever an account root eventually is, one thing about it is already settled
and is not reopened here.

| Question | Settled by | Answer |
| --- | --- | --- |
| How does workspace content reach account context? | Proposal 003, Spec 024 | An explicit, reviewed, owner-approved `account_safe` promotion, and nothing else |
| May the account role read a workspace to obtain it? | `client-account-agent/AGENT.md` | No |
| May any role aggregate across workspaces to compose it? | Proposal 003, the closed role catalogue | No; the authorization core forbids joining two roots |
| May the account role be reused as a global owner? | Proposal 003 requirement 1 | No |
| May workspace client or people pages be maintained from account scope? | The workspace root's own binding | No; `case_agent` is the role bound to that root |

A future account root inherits all five answers. A design that needs any of them
relaxed is a different proposal, and a worse one.

## What a future proposal would have to define

The option stays open on stated terms rather than being quietly abandoned. A
proposal creating an account context root must deliver all of the following in
one document, with fixtures.

### 1. The root and its authorization domain

A declared location, distinct from the owner, workspace and managed roots, with
its own authorization predicate in the shared core — not a folder inside an
existing root and not a reuse of the workspace grant. Proposal 007's owner
operations are bounded to the owner root by construction and carry to no other;
the same must be true of this one in both directions. The proposal must state
which account identity binds the root, how that identity is resolved before any
path is touched, and what happens when one account's material is requested under
another's authority.

### 2. The promotion operation and its redaction rule

Spec 024 already defines promotion of a single statement into a record. Writing
that statement into a curated page is a second operation and needs its own
contract:

- the unit written is the reviewed statement, redacted as part of the operation
  rather than after it — a promotion that carries the sentence it came from
  carries the workspace with it;
- owner confirmation of the redacted text, shown as it will be stored;
- the target section is declared, and the operation is non-destructive and
  idempotent in the shape Proposal 007 requires of an owner-scope write;
- provenance on every write, carrying the promotion ID, source receipt ID and
  source hash, so a line on the page can always answer how it got there;
- no discovery path: the operation must not be reachable by enumerating a
  workspace, a client folder or a people index.

### 3. The closed reader set

Every field needs a professional purpose, a sensitivity class and an enumerated
set of readers, in the shape Proposal 003 requirement 4 sets out. Proposal 007's
reader rules are the model for how such a set is written down: purpose declared
per call, a named tier per reader, the smallest useful projection, and no
implicit whole-root dump for anyone. "Closed" therefore means enumerated, not
single-reader. The proposal must say whether an account page may enter a runtime
packet at all and, if so, which fields — minimum fields, never the record body —
and it must state which roles are excluded. A root readable by whichever role
happens to be dispatched is not scoped; it is shared.

### 4. Retention and revocation

Spec 024 records already expire on a validity window of at most 366 days and can
be revoked without deleting evidence, with reads linearized against the
revocation. A curated page composed from those records must honour the same
lifecycle, and the proposal must say how:

| Event | What must happen to the page |
| --- | --- |
| A promotion expires | The derived content becomes unavailable, not merely stale |
| A promotion is revoked | The derived content is withdrawn, and the withdrawal is auditable |
| A source is corrected | A correction path exists and preserves the prior value |
| The account relationship ends | Retention is bounded and the end state is defined |

A page that outlives the authority of the statement it was built from converts a
time-boxed, revocable promotion into a permanent record. That is the specific
failure this requirement exists to prevent.

### 5. The guarantees that must survive

Two properties are the reason the root does not exist yet, and a proposal that
cannot enforce them in the authorization core — rather than assert them in a
prompt — has not met the bar.

- **No workspace enumeration becomes possible.** Not from the root, not from the
  promotion path, not as a convenience for composing a page. Proposal 028's
  requirement 3 depends on this staying closed, and the workspace side stays
  closed only for as long as nothing on the account side needs to look in.
- **No cross-client aggregation becomes possible.** One account root is one
  account. No index across accounts, no shared key, no question that spans two.
  Proposal 003 ruled that duplication across boundaries is preferable to
  crossing them, and an account root is exactly where that ruling would be
  eroded first, quietly, as a reporting convenience.

### 6. Proposal 004's governed rules, if a role change is proposed with it

If the same document also amends `client_account_agent` to maintain the new
root, it answers all eight of Proposal 004's rules explicitly — managed
definition and signed instance, registered parent, exact canonical input
contract, exact semantic grants rather than broad `Read`, `Write`, `Bash` or
`MCP` labels, a bounded result or proposed patch, no direct persistence or
delegation or root browsing or user channel, no runtime tool names in the
definition, and unavailability until native Claude and Codex conformance exists.
Persistence stays in the command layer through named operations, as Proposal 007
established for the owner root.

## Verdict

Until a proposal answers all six, the capability should be reported unavailable
rather than simulated. Proposal 003 already recorded the alternative and chose
against it:

> The future feature is deliberately reported as unavailable rather than
> simulated through prompt instructions.

That sentence is the operative conclusion here too. When curated account context
is asked for, the correct answer is that no account root is declared, with the
reason — because an account root improvised inside an existing root, or a
curated page assembled by reading a workspace, has the same collection behaviour
as a governed one and none of the guarantees.

Reporting it unavailable costs the automation, not the conversation. The runtime
may still reason about an account with the owner, work from what Spec 024 has
already promoted and prepare a reviewable draft the owner keeps wherever they
choose. What it may not do is present that draft as a curated account record, or
report a filing that no declared root received.

## Consequences

- The account layer keeps no atlas root, and the gap is recorded with reasons
  rather than left as an omission a later proposal rediscovers.
- The account role's ownership of curated context remains declared in its
  definition and unimplemented in storage. That mismatch is now documented, so
  it is a known deferral rather than an apparent oversight.
- Workspace client and people pages stay with the workspace root and `case_agent`.
  This document creates no competing claim to them and withdraws the earlier one.
- Spec 024 remains the only route from a workspace to the account layer, and its
  records remain fetch-by-ID with no enumeration surface.
- Proposal 003's requirement 1 — that the account role is not reused as a global
  owner — is confirmed rather than stretched, and Proposal 028's deferral of the
  owner people record is unaffected.
- A future proposal has a checklist rather than a blank page, and must answer
  every item as its own document, with fixtures.

## Explicit non-decisions

- no account atlas root, location, segment, template or operation is created or
  approved;
- no role is added to, or removed from, the agent catalogue, and no deferred
  role is revived;
- no agent receives a tool grant, a persistence grant, a new packet type or a
  user channel;
- no account-promotion path, protocol, classification, retention or revocation
  behaviour is created or changed, and Spec 024 is untouched;
- no workspace, owner or managed root becomes readable or writable from account
  scope, and no workspace enumeration becomes possible;
- no cross-workspace, cross-client or owner-level record, index or identity is
  created, implied or approached;
- no workspace atlas page changes maintainer by this document;
- no HR, staffing, directory, profiling or relationship-scoring capability is
  created;
- no runtime is reported available by merging this document.
