# Skill promotion contract

This contract turns a proposed skill into an implementable, reviewable change. A
proposal is not a managed skill: files under `docs/proposals` are design artifacts and
must not be loaded by a runtime or advertised by the managed catalog.

## Lifecycle

Every proposed skill has exactly one disposition:

| State | Meaning | Runtime consequence |
| --- | --- | --- |
| `proposed` | Taxonomy or draft method under discussion | Not loadable; no catalog entry |
| `narrowed` | Useful method with scope or authority gaps explicitly recorded | Not loadable; implementation may be designed |
| `deferred` | Blocked on an authority, connector, renderer, or privacy contract | Not loadable; no availability claim |
| `adopted` | Approved design with a complete implementation plan | Still not loadable; promotion PR is required |
| `managed` | Canonical bundle, generated indexes, adapters, and evaluations are present | Loadable only through the closed catalog |
| `retired` | Previously managed skill removed or superseded | Must fail closed and retain a migration note |

The 19 entries in this proposal start as `proposed`. A later decision may move an entry
to `narrowed`, `deferred`, or `adopted`; no state transition is implied by the existence
of a `SKILL.md` in this directory.

## Required contract for `adopted` or `managed`

One promotion PR must define all of the following. Omitting a field keeps the skill out
of the managed catalog.

| Field | Required statement |
| --- | --- |
| Identity | Stable skill ID, display name, trigger, description, and canonical path |
| Authority | One existing owning agent/capability and its allowed callers; no direct edge invented by the skill |
| Profile | Supported `interaction-profile` values and the user-facing behavior for each |
| Input packet | Schema, size/secret limits, required references, and invalid-input behavior |
| Output | Schema, artifact references, receipts, and whether the output is advisory or mutating |
| Negative scope | Denied tools, connectors, files, data classes, delegation, and external disclosure |
| Persistence | Single source of truth, atomic write path, idempotency key, recovery behavior, and retention |
| Runtime adapters | Claude/Codex mapping, unsupported-operation behavior, and semantic parity fixture |
| Conformance | Positive, negative, adversarial, interruption/recovery, and metadata-only evidence tests |
| Distribution | Generated `catalog.json`/`INDEX.md`, bundle entry, versioning, and rollback path |

Loading a skill never grants authority. A skill can describe a method, but only the
closed agent catalog, authorization grants, and tool contracts can authorize execution.

## First implementation slice: `wayfinder`

`wayfinder` is the recommended first candidate because it can be purely advisory: it
turns a fuzzy question into a bounded issue tree without task, calendar, memory, network,
filesystem, or connector mutation.

The promotion PR for `wayfinder` must therefore prove:

1. input is a bounded natural-language problem plus optional owner-supplied constraints;
2. output is a structured issue tree with a stable correlation ID and no hidden writes;
3. unknown or sensitive references are rejected rather than fetched implicitly;
4. Claude and Codex receive equivalent semantic instructions and produce equivalent
   contract-shaped output, allowing runtime-specific wording differences;
5. retries with the same correlation ID do not create duplicate durable artifacts; and
6. unsupported tools are reported as unavailable, not simulated as completed.

The first slice should remain a proposal-level implementation fixture until the contract
is reviewed. It must not add a catalog entry or change release/distribution surfaces by
itself.

## Review gate

Reviewers should be able to answer “yes” to each question before approving promotion:

- Is there one authority and one persistence owner?
- Can a caller determine exactly what data and tools are denied?
- Can the operation be retried, interrupted, and recovered without duplication?
- Is evidence metadata-only where the output points to an artifact?
- Do Claude and Codex preserve the same semantics?
- Do generated bundle surfaces, rollback, and evaluations exist?

If any answer is “no” or “not yet,” the disposition remains `narrowed` or `deferred`.
