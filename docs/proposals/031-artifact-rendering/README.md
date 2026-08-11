# Proposal 031 — Artifact rendering

**Status:** blocked on an external prerequisite; filed to record the
requirement. This document requests no skill and proposes no renderer.

**Original contribution:** Marcelo Petrof Sanches.

**Depends on:** nothing in the owner and workspace atlas series. The blocker is
outside it.

**Unblocks:** nothing until a renderer proposal lands.

## Reading the proposals this document cites

Proposals 004 and 005 were accepted and then removed from the working tree. They
are still readable from history:

```sh
git show 760abd8:docs/proposals/004-spoke-agent-roster/README.md
git show 760abd8:docs/proposals/005-skill-consolidation/README.md
```

One caution. 005 survives in two forms: the reconciled document quoted below is
the one at `760abd8`, while `2fe2a50` carries an earlier unreconciled draft of
the same proposal on another branch and is not the text cited here.

## The verdict being recorded

Proposal 005 reviewed nineteen draft skills and deferred two of them for the
same missing piece:

| Skill | Decision | Stated blocker |
| --- | --- | --- |
| `diagram` | defer | "Needs a named deterministic renderer and artifact contract" |
| `make-pdf` | defer | "Needs a named deterministic renderer, distribution policy and artifact contract" |

That verdict was correct and remains correct. Both skills produce a file rather
than an answer, and the system has no contract for what a produced file is,
where it lives or how it is reproduced.

The reason for writing this down is narrow. Both skills are obvious, both get
proposed again, and the deferral has already been re-derived once. This document
exists so the requirement is not rediscovered from scratch a third time.

## What the two prerequisites would have to specify

### 1. A deterministic renderer

| Requirement | What it means concretely |
| --- | --- |
| Named | One renderer, identified in the proposal, not "whatever the runtime provides" |
| Deterministic | The same input produces the same output bytes, or a declared and bounded set of differences |
| Reproducible | The output can be regenerated later from the recorded input, without the session that made it |
| Versioned | The renderer version is recorded with the artifact, because a renderer upgrade changes output |
| Failure-visible | An unavailable or failing renderer produces a reported failure, never a degraded substitute |

Determinism is the load-bearing property. Without it there is no way to tell a
regenerated artifact from a different one, and no way to review a change to a
deliverable.

### 2. An artifact contract

| Requirement | What it means concretely |
| --- | --- |
| Declared type | The artifact kind is enumerated, not inferred from a file extension |
| Storage location | A defined path inside a declared scope, subject to that scope's authorization. An artifact is scoped content like any other page |
| Lifecycle | Creation, regeneration, supersession, retention and deletion behaviour, defined before the first write |
| Provenance | The input, the renderer, the version and the invocation are recorded with the artifact |
| Persistence route | The write is performed by the command layer through a named operation, as Proposal 007 established for the owner root and PR #286 implemented there. No skill and no role writes a file |
| Distribution policy | For `make-pdf` specifically: whether, where and to whom an artifact may be sent, since a document that renders is a document that circulates |

The persistence route is the one row with a working precedent. The owner root now
has named operations, revocable standing grants and terminal states that separate
a write from a proposal; an artifact operation would be a member of that family,
declared in Proposal 007's set rather than beside it. No comparable set is
accepted for any other root, so a proposal that wants to write an artifact
outside owner scope has to establish the operation surface as well as the
renderer.

### 3. No ad-hoc installation at runtime

Proposal 004's governed rule 7 requires a role definition to "keep
runtime-specific tool names and dependency installation out of the canonical
definition". A renderer that installs a package when it first runs violates that
rule for every role that would reach it, and defeats determinism at the same
time: the output then depends on what got installed and when.

A renderer must therefore be present or absent, declared in the runtime's
capability surface. If it is absent, the capability is reported unavailable.
Installing it is a separate, explicit act — never a side effect of asking for a
diagram.

### 4. Runtime-neutral behaviour

Proposal 004's governed rule 8 holds a capability unavailable until native
Claude and Codex conformance exists, and Proposal 005's consolidation rules
require runtime adapters to "map those semantics to native tools and report
unsupported operations honestly".

Applied here: the two skills describe a semantic artifact request. Each adapter
either produces the declared artifact through the named renderer, or reports the
capability unavailable. Producing a Markdown approximation, an inline
description or a hand-built HTML file in place of the artifact is the failure
mode this rule exists to prevent — it satisfies the request visually while
breaking determinism, provenance and the storage contract at once.

## Recommendation

`diagram` and `make-pdf` stay deferred until a renderer proposal lands and
satisfies sections 1 through 4. They should not be promoted piecemeal, and
neither should be adopted as an exception on the grounds that its output is
"just a picture" or "just a document" — the artifact contract is the part that
is missing, and it is missing equally for both.

When the renderer proposal does land, each skill still needs its own document
with Proposal 005's full promotion checklist, including evaluations. This
document does not pre-approve either of them; it records what would have to be
true first.

## Consequences

- The deferral has a written basis, so a future proposer starts from four
  requirements rather than from the original idea.
- A renderer proposal has a defined acceptance target and can be evaluated on
  determinism, contract, installation behaviour and runtime neutrality.
- Any nearby capability that produces a file — a deck artifact, an exported
  table, a rendered chart — inherits the same four requirements. The blocker is
  the artifact contract, not the diagram.
- Until then, a request for a rendered artifact is answered as unavailable, with
  the reason, rather than approximated. What is lost is the file, not the work:
  the reasoning, the structure and a reviewable draft in the conversation remain
  available, and must not be presented as the artifact that was asked for.

## Explicit non-decisions

- no renderer is named, chosen, adopted or made available;
- no artifact type, storage location, lifecycle or distribution policy is
  defined;
- `diagram` and `make-pdf` are not promoted, narrowed or re-scoped;
- no skill is registered, and no catalogue or index surface is changed;
- no operation is added to Proposal 007's accepted set;
- no role, grant, packet type or operation is created or modified;
- no runtime is reported available by merging this document.
