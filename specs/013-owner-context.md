# Spec 013 - Owner context

Status: decision accepted; local facet registry, inspection surface,
cold-start interview contract and policy-enforcing refinement core implemented.
Walter consumes only a versioned digest projection of these facets. The
interaction evaluator and metadata-only provisional log are contract-tested,
but native observation and synthesis adapters remain unavailable; no Walter
invocation promotes or writes the canonical self.

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

The public runtime contract keeps Owner Context as the single authority.
`UserSelfSnapshot` is only a versioned digest projection. Observation records
contain minimized claim/provenance digests, evidence type, scope, confidence,
sensitivity and expiry/recheck metadata; they do not contain transcript,
prompt, client or generated-artifact bodies. Explicit instruction/correction
may become eligible under the facet policy, while an isolated pattern or
intent hypothesis remains a proposal. Promotion must compare-and-swap against
the current canonical snapshot digest and is never authored by Darwin or
Walter.

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
