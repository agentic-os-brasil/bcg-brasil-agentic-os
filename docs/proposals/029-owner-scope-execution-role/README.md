# Proposal 029 — Owner-scope execution role

**Status:** deferred; records the acceptance bar for a future decision. This
document requests no role and proposes no catalogue change.

**Original contribution:** Marcelo Petrof Sanches.

**Depends on:** Proposal 006 (owner scope contract), Proposal 007 (owner atlas
operations), PR #286 (the shipped operations, standing grants and `retro`
ritual), PR #289 (the declared owner segment shapes) and decision `OATL`.

**Unblocks:** nothing. It closes a question that would otherwise be reopened
once per owner-scope proposal.

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

## The question

The owner atlas now has a scope (Proposal 006), an accepted operation set of ten
(Proposal 007), three of those operations implemented with revocable standing
grants and one ritual running on them (PR #286), and four declared segment
shapes (PR #289). Every other layer of the system has an agent that owns it:
`case_agent` owns the case, `client_account_agent` owns account framing,
`governance_analyst` owns system health, `quality_guardian` owns longitudinal
quality.

The owner layer has none. The question is whether it should.

**It should not, yet.** Not because an owner-scoped role is objectionable in
principle, but because the three things that would justify one are absent and
the thing it would supposedly provide already exists — no longer as a plan, but
as running code.

## Why the answer is no today

### The catalogue is closed, and closed against this specifically

Two accepted reconciliations already ruled on it, in terms that were not
incidental, and the owner scope contract restated the ruling when it created the
scope.

| Source | Ruling |
| --- | --- |
| Proposal 004 | This proposal "does not add a second graph, grant tools, change Walter, create an owner-global agent or activate any runtime capability" |
| Proposal 003 | "Proposal 004 cannot create owner-level people, career, planning or wellbeing agents on top of the account role" |
| Proposal 006 | "Future segments and rituals may build on this scope without creating an owner-scoped agent role" — under a section headed "Execution model — no new role is created" |

The catalogue reflects those rulings mechanically. Its deterministic delegation
block declares `max_depth: 1`, `max_children_per_agent: 0`, one active branch and
a single allowed edge set from the hub; the `native_advisory` block widens depth
and edges for attended reasoning, and adds no scope kind. There is no owner scope
kind in either: every registered role carries an `ownership_scope` drawn from
`case`, `account`, `governance`, `quality_longitudinal`, `pa_expert_registry` or
the hub's own `system` scope. An owner-scoped role is not merely unregistered —
the property it would need does not exist in the schema.

Adding one is therefore a schema change plus a catalogue change plus an
authorization change, not a file addition.

### Nothing that shipped needs it

This is the part worth stating positively, because it is what makes the deferral
comfortable rather than merely enforced. It is also no longer a prediction. PR
#286 shipped `internal/atlasops` with `collect`, `create-page` and
`append-entry`, revocable standing grants, the `bcgos atlas owner` and
`bcgos atlas grant` verbs, and the first ritual over them. Nothing in it was
harder to build for the lack of a role, and nothing in it is waiting on one.

Every write goes through the command-layer route Proposal 007 established, which
is itself the shape canonical memory already uses:

```text
skill  →  installed runtime adapter  →  named owner-scope operation  →  write
```

| What a ritual needs | Who supplies it today |
| --- | --- |
| Bounded read of owner pages | `collect`, implemented in PR #286 |
| Page creation, idempotent | `create-page`, implemented in PR #286 |
| Timestamped entry under a named section | `append-entry`, implemented in PR #286 |
| Field update with prior value retained | `set-field` — accepted, not built; a direct owner edit today |
| Reference between pages | `link` — accepted, not built; a direct owner edit today |
| Authority for an unattended occurrence | A revocable standing grant, enforced at the point of effect |
| Behaviour when an operation is unavailable | Degradation: the automation is lost; the conversation, the reasoning and a reviewable draft are not |
| Reasoning and composition | The skill, in the owner's own session |

The shipped `retro` ritual is the concrete demonstration. It collects the week's
owner pages, walks them as a conversation, writes the retrospective with
`create-page` and appends confirmed evidence with `append-entry`; its own
invariants state the division plainly — "The skill never writes a file. Every
effect is a named operation through the installed adapter." It distinguishes a
`written` result from a `proposed` one, refuses an ambiguous section rather than
guessing which objective was meant, and declines to promote a durable claim under
a scheduled grant because that grant covers one page family. Each of those is a
property of the operation layer and the grant. None of them is a property of an
identity above them, and no identity was needed to obtain them.

A role inserted into that chain would receive a packet, reason over it and
return a proposed patch to Maestro, which would route it to the same command
layer that performs the write today. It would add an identity, a packet type, a
dispatch hop and an authorization surface. It would not add a capability.

### The one honest argument for a role, and why it does not carry yet

The strongest case is accountability: a page without a named maintainer goes
stale, and owner pages have no maintainer.

The disanalogy is that the owner is present. Case and client pages go stale
because the person who knows the fact is not the person who opens the page. For
owner content, the author, the reader and the session operator are the same
person — and Proposal 006 admits exactly two shapes of content, pages about the
owner and pages about the work itself, neither of which has an absent data
subject to answer to. A ritual that runs in the owner's own session is already
the maintenance mechanism, and PR #286 shows one doing it. When that stops being
true — when occurrences run unattended often enough that nobody reads what they
wrote — the argument gets stronger, and this document is where it should be
re-argued.

## What a future proposal would have to supply

The option stays open on stated terms rather than being quietly abandoned. A
proposal creating an owner-scope role must deliver all of the following in one
document, with fixtures.

**All eight of Proposal 004's governed rules, answered explicitly:**

| Rule | What the proposal must show |
| --- | --- |
| 1 | A managed role definition and a signed local instance under Spec 027 |
| 2 | The registered parent that dispatches it, and the allowed edge that permits it |
| 3 | Its exact canonical input contract, in the signed Spec 023 envelope |
| 4 | Exact semantic tool-operation-resource grants — never broad `Read`, `Write`, `Bash` or `MCP` labels |
| 5 | A closed result schema: a bounded result or a proposed patch to its parent |
| 6 | That it never persists directly, delegates, browses a whole root or speaks to the user |
| 7 | A definition free of runtime-specific tool names and dependency installation |
| 8 | That it remains unavailable until native Claude and Codex conformance exists |

**Three additional items the rules presuppose and the current schema lacks:**

1. **A scope kind.** An `ownership_scope` value for owner scope, defined in the
   catalogue schema, with its authorization predicate in the shared core — not a
   reuse of the account or system scope. Proposal 003 requirement 1 already
   forbids the reuse.
2. **A packet contract.** A bounded owner packet: what it may carry, maximum
   sizes, denied sources, and the guarantee that it never carries workspace,
   client or third-party content. Proposal 006 enumerates five reader tiers over
   owner content — the owner session, Maestro, Walter as a stale-checked self
   proxy, an explicitly authorized attenuated excerpt or pointer for Case, Client
   Account and PA Expert agents, and no whole-root dump for anyone. A new role is
   a sixth reader; the proposal must say which tier it occupies, or argue for
   widening the set. `collect` already declares a purpose and a reader on every
   call, so the packet must be constructible within that discipline rather than
   beside it.
3. **Claude and Codex conformance evidence.** Shared fixtures plus both adapter
   results, per Proposal 004's promotion matrix. A green documentation harness is
   not conformance.

**And one question the eight rules do not ask, which this one must answer:**
what the role does that the command-layer operation set cannot. PR #286 raised
the bar for that answer rather than lowering it: the operations, the grants and a
working ritual now exist without a role, so the burden is to name a capability
they cannot reach. If the answer is "the same writes, with an identity in front
of them", the proposal should be declined again.

## Consequences

- The owner layer keeps no agent role. Proposal 006, Proposal 007, the segment
  shapes declared in PR #289 and the ritual shipped in PR #286 proceed unchanged
  on the command-layer route.
- The seven operations in Proposal 007's accepted set that remain unbuilt are the
  outstanding work in this layer, and none of them needs a role to be built.
- The question is settled with reasons rather than by omission, so each new
  owner-scope proposal need not relitigate it.
- The precedent generalizes: a new identity requires a capability argument, not
  a symmetry argument. Parity with the case and account layers is not a reason.
- If owner pages do begin to go stale in practice, that is the evidence a future
  proposal should carry, and this document names it as the trigger.
- Proposal 004's rulings and the closed catalogue remain authoritative and are
  not weakened by this document's willingness to reconsider.

## Explicit non-decisions

- no role is added to, or removed from, the agent catalogue;
- no ownership scope kind, packet type or allowed edge is created;
- no agent receives owner-scope access, a tool grant or a user channel;
- the depth, child and active-branch limits are unchanged;
- no deferred role from Proposal 004 is revived, narrowed or re-scoped;
- no operation is added to Proposal 007's accepted set, and none of the seven
  still unbuilt is reported available;
- no runtime is reported available by merging this document.
