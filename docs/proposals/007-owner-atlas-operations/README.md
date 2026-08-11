# Proposal 007 — Owner atlas operations

**Status:** request for decision. Specifies an operation set; ships no segment and no skill.

**Original contribution:** Marcelo Petrof Sanches.

**Depends on:** Proposal 006 (owner scope contract).

**Unblocks:** Proposals 008–011, 018–023, 029–030 and 032–033.

## The gap this closes

Spec 014 created the human atlas as a navigation scaffold and was explicit about
the boundary: it "provides no automatic writing", and "agent/ritual writers
require separate decisions and tests."

That decision has held. The consequence is that no ritual can maintain the atlas:
every skill that would keep a daily log, a retrospective or a learning current is
blocked on the same missing piece. Proposal 005 deferred `retro`,
`record-learning` and `record-concept`; none of them can be adopted while the
atlas is write-only by hand.

This proposal supplies the separate decision Spec 014 asked for, for owner scope
only.

## Precedent — this is the memory engine's shape

Canonical memory already solved the same problem. The shipped `dream-memory`
skill states the rule directly: *"Never write, summarize or promote memory files
directly from the skill"*, and *"Do not emulate dreaming by editing local memory
files."* The skill invokes one deterministic operation through the installed
adapter; the engine performs the write.

Owner atlas operations adopt that contract without variation. A skill never
touches a file. If the adapter or command is unavailable, the skill reports the
capability unavailable and stops — it does not fall back to editing Markdown.

## Operation set

Five named operations. No free-form file write exists at any layer.

| Operation | Behaviour | Idempotency |
| --- | --- | --- |
| `collect` | Return a bounded projection of named pages or a segment index | Read-only |
| `create-page` | Create a page from its segment template if absent | Second call is a no-op, never an overwrite |
| `append-entry` | Append a timestamped entry under a named section | Identical entry within the same period is not duplicated |
| `set-field` | Replace a declared field value | Prior value retained with its provenance |
| `link` | Add a reference from one page to another | Duplicate link is a no-op |

Every operation declares its segment. An operation naming a segment outside
owner scope is rejected by the authorization core, not by convention.

## Invariants

- **Non-destructive.** No operation deletes a page, truncates a section or
  replaces text the owner wrote by hand. `set-field` touches only declared
  fields and preserves what it replaced.
- **Idempotent.** Running the same ritual twice on the same day produces one
  page and one entry set. This is the property that makes scheduled and manual
  invocation safe to mix, matching the canonical memory engine.
- **Provenance on every write.** Each write records the invoking operation, the
  session and the timestamp. A page can always answer how a line got there.
- **Bounded reads.** `collect` returns a declared projection, never a whole root.
  Segment bodies are not returned wholesale to a caller.
- **Fail closed.** An unavailable adapter, an unknown segment or a template
  mismatch stops the operation and reports it. Partial writes are not committed.
- **Owner scope only.** Workspace and client roots are unreachable from this
  operation set. Proposal 011 addresses those separately and does not inherit
  these grants.

## Why this does not widen any role

No agent gains a capability here. The operation set is reachable only from the
command layer through the installed adapter, on behalf of a user-invoked skill.

- Maestro remains a hub with no tools and no direct read of owner facets.
- No spoke role receives a persistence grant, a new packet type or owner access.
- Proposal 004's rule that a role must never persist directly is unaffected,
  because no role persists — the command layer does.

## Consequences

- Spec 014's writer gap is closed for owner scope, on the terms Spec 014 set.
- Rituals become safely repeatable, so scheduled and manual invocation converge
  on the same result.
- Proposal 011 may later extend an equivalent operation set to workspace scope,
  reusing this shape but requiring its own authorization review.

## Explicit non-decisions

- no atlas segment, template or page is created by this proposal;
- no skill is registered, and no ritual is scheduled;
- no workspace, client account or managed root becomes writable;
- no memory layer, capture path or eligibility policy is changed;
- no runtime is reported available by merging this document.
