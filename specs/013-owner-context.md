# Spec 013 - Owner context

Status: decision accepted; local facet registry, inspection surface,
cold-start interview contract, policy-enforcing refinement core and
metadata-only interaction evaluator implemented. Assessment extraction and
semantic observation synthesis remain unavailable.

## Objective

Give a future Session Start a stable, human-correctable source for the owner's
professional SELF and current operating state without turning identity into
memory or creating an implicit task system.

## Local surface

Owner context lives only in user-local BCGOS application storage:

```text
owner/
  registry.json
  self/
    professional-role.md
    communication-style.md
    voice.md
    preferences.md
    decision-rules.md
    working-boundaries.md
    psychological-profile.md
  operating/
    work-state.md
  observations/
    observations.jsonl
  self/projections/
    self-<canonical-digest-prefix>.json
  sources/
    assessments/
```

`registry.json` contains pointers, sensitivity, allowed readers and refinement
policy only. The Markdown files are human-authored and are never copied to a
workspace, managed bundle, Git, memory layer or shared atlas.

`psychological-profile.md` is optional and sensitive. It is intended only for
professional self-understanding and for the Walter role where the owner has
authorized that purpose. It is not a diagnosis, a fixed label, an employment
selection tool or a source of inferences about other people.

The managed cold-start interview asks only for the non-sensitive professional
facets. It returns a runtime-neutral question contract; a Claude or Codex
adapter must show answers to the owner before proposing any write. The owner
may later choose to import an assessment report through an approved local
adapter with explicit consent. Raw reports stay local under `sources/` and are
never automatically injected; any professional synthesis requires provenance
and confirmation.

## Refinement policy

Every self change must be explainable, versioned by its future owning adapter
and reversible. Facets are declared now with one of three policies:

- `automatic_with_audit`: voice, communication style and preferences may be
  refined from repeated approved work only when a future adapter records the
  evidence, change and reversal path.
- `proposal_only`: professional role and decision rules may receive a proposal
  but require owner action.
- `confirmation_required`: boundaries and psychological profile may never be
  changed silently.

The current CLI implements the local enforcement core: a producer submits a
proposed facet body with an evidence summary; an eligible facet applies
automatically only when that producer presents an owner-authorized local
capability. All other proposals require `--confirm`. Before every application,
the core journals the protected before-version and audit record; every reversal
checks that the facet has not changed since that audit, journals its own event,
and refuses to erase newer work. The core does not observe work or synthesize a
proposal itself: lifecycle and model adapters remain separate producers and are
reported as unavailable.

## Runtime behavior

`bcgos owner init` creates non-overwriting templates. `bcgos owner status`
returns pointers, policy and availability, never document bodies. `bcgos owner
interview` exposes the cold-start questions without persisting an answer.
`bcgos owner refine submit --facet <facet> --evidence <summary> --stdin`
accepts a proposed body through standard input, applies only an eligible
policy, and returns an opaque receipt. `apply --confirm <proposal-id>` and
`revert --confirm <audit-id>` protect guarded application and every reversal. A later Session Context Packet
may read bounded content only after an adapter resolves purpose, owner and
policy. Tasks remain an explicit unavailable pointer until a task system
contract is accepted.

### Self projection and evidence-bound learning

The canonical facet files and registry are the one Owner Context authority.
`UserSelfSnapshot` is a versioned, stale-checked projection for a bounded
Walter packet, never a second database. Precedence is current explicit
instruction, explicit correction, canon, relevant observations, then a Walter
intent hypothesis. An explicit correction supersedes earlier claims and
invalidates proposals whose canonical-source digest is stale.

Maestro evaluates every interaction. It persists only a material,
owner-attested signal under the local owner boundary in the append-only observation log; routine
loops, hypotheses, client documents and generated output are not persisted as
self evidence. Observation metadata carries signal class, minimal normalized
claim, evidence type, provenance digest, independent episode, scope,
confidence, sensitivity and expiry. Scope is one of global, workspace,
account or case; promotion to global requires explicit owner declassification.
The lifecycle is `captured -> eligible -> corroborated -> proposed -> promoted`,
with `rejected`, `contradicted`, `expired` and `redacted` terminal paths.

Communication style, voice and preferences may receive audited automatic
promotion only after explicit confirmation. Professional role and decision
rules remain proposal-only. Boundaries, psychological profile and claims about
intrinsic user motivation require explicit confirmation. Repetition means
independent episodes, not multiple messages in one chat. Darwin may report
metadata-only duplicate, age, conflict and drift signals; it cannot write or
replace canonical self content. Local controls expose snapshot inspection and
export, observation rejection/redaction, facet revert and snapshot deletion.
`bcgos owner self reset --confirm` redacts provisional observations through
tombstones and removes derived projections; it refuses to hide promoted
canonical facets, which must use the audited facet revert path.

### Owner-local prompt history

Prompt retention is a separate product surface from self learning. When the
owner enables it, `owner/prompt-history/entries.jsonl` stores only raw user
prompts. Each entry binds owner identity, timestamp, language, source/session,
SHA-256 and one of the global, workspace, account or case scopes. The store is
private, symlink-checked and bounded by configurable entry count, bytes and
age. It is never copied to managed bundles, telemetry, receipts, ledgers,
federation or release artifacts.

`bcgos owner prompt-history` exposes configuration, metadata inspection,
explicit export, per-entry deletion and confirmed reset. Walter selection is
bounded by count, bytes, age and relevant scope, and uses stable lexical
relevance against the current prompt or explicit keys; recent irrelevant
history cannot outrank an older relevant prompt. The root is single-owner
bound and mutating operations use a symlink-safe cross-process lease lock.
Maestro first places the current prompt before history, preserves its original
as source of truth, creates a digest-bound working representation, then
translates or normalizes selected history into the configured working language
and marks it as quoted data. Packet ceilings are eight prompts and 32 KiB even
when store retention is larger. Each original and working representation is
independently capped at 32 KiB, and the combined current plus selected
original/working bytes must also fit the 32 KiB packet ceiling. Translator
expansion fails closed, and a facet larger than the snapshot projection bound
is rejected rather than truncated. Prompt bodies exist only in the ephemeral
sealed review packet. A translation adapter is required when languages differ;
absence fails closed for that normalization stage without changing the user
request.
