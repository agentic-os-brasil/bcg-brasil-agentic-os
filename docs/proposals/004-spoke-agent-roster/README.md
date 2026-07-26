# Proposal 004 — Domain methods on the governed role graph

**Status:** domain methods retained as a design inventory; named agent
registration deferred until each method has a scoped parent, packet and grant
contract.

**Original contribution:** Marcelo Petrof Sanches.

**Architecture reconciliation:** BCG Brasil Agentic OS maintainers.

## Executive resolution

The original roster contains useful professional methods, but a descriptive
persona is not yet a governed agent. Spec 018 defines a closed role graph and
Spec 027 binds local instances to signed, scope-specific manifests. This
proposal does not add a second graph, grant tools, change Walter, create an
owner-global agent or activate any runtime capability.

For now, the roster is classified into:

- **adopt as a bounded method:** suitable for a future specialist below an
  existing workspace or practice parent;
- **fold into an existing canonical role:** behavior belongs in the owner
  agent or Walter rather than a new identity;
- **defer:** requires an authority or data boundary the product does not yet
  have; or
- **reject:** conflicts with a non-negotiable role or privacy contract.

## Governed rules

Any future named specialist must:

1. use the managed `capability_specialist` or `subject_specialist` definition
   and a signed local instance from Spec 027;
2. be dispatched only by its registered workspace, account or practice parent;
3. receive its role's exact canonical input contract, delivered in the signed
   Spec 023 `WorkPacket` envelope;
4. use exact semantic tool-operation-resource grants enforced by the shared
   controller, never broad `Read`, `Write`, `Bash` or `MCP` labels;
5. return a bounded result or proposed patch to its parent;
6. never persist directly, delegate, browse a whole root or speak to the user;
7. keep runtime-specific tool names and dependency installation out of the
   canonical definition; and
8. remain unavailable until native Claude and Codex conformance exists.

The parent remains accountable for validation and persistent writes. “Thin
owner agent” does not mean a specialist may become the real authority behind
it.

## Disposition of the original roster

| Original name | Disposition | Governed home and constraint |
| --- | --- | --- |
| `client-keeper` | fold | Workspace agent behavior; may use a bounded extraction method, but the workspace owner validates and writes |
| `work-logger` | fold/defer | Workspace agent may prepare human-log updates; backlog and external synchronization remain unavailable |
| `case-onboarder` | adopt later | Workspace capability specialist over explicit proposal-deck pointers; must use managed ingestion and return a plan |
| `quant-analyst` | adopt later | Workspace capability specialist with exact data/artifact grants and no implicit connector or shell authority |
| `quali-analyst` | adopt later | Workspace capability specialist receiving only selected artifacts and hypotheses |
| `deck-builder` | adopt later | Workspace capability specialist returning a deck artifact/plan; no ad-hoc package installation |
| `career-keeper` | defer | Requires an owner-private professional-development scope, not an account scope |
| `people-keeper` | defer | Blocked by Proposal 003's future owner-private people contract |
| `briefing-analyst` | defer | Email, calendar, chat and cross-workspace aggregation require explicit source grants and privacy contracts |
| `work-planner` | defer | Cannot create a task authority or browse workspace backlogs from an account role |
| `support-coach` | defer | Requires a purpose-limited owner context contract and must not infer health or expose client material |
| `challenger` | fold | Walter may challenge only the sealed packet supplied by Maestro; no tools or owner-root browsing |
| `final-reviewer` | fold | Existing Walter final review contract; no separate instance |

This table preserves methods worth developing while preventing names from
becoming unauthenticated authorities.

## Privacy and portability matrix required for promotion

Before any `adopt later` row becomes managed content, its implementation PR
must include:

| Contract | Required evidence |
| --- | --- |
| Parent and scope | Registered parent ID, exact scope kind/ID and allowed role edge |
| Input | Packet schema, maximum sizes, denied sources and minimum pointers |
| Tools | Semantic capability, exact operation, canonical resource prefix and denied operations |
| Output | Closed result schema, content classification and parent validation step |
| Persistence | Owning parent, atomic write path, receipt and recovery behavior |
| External disclosure | Connector, purpose, redaction and explicit authorization, or `none` |
| Lifecycle | Retention, correction, revocation and runtime availability rule |
| Conformance | Shared fixtures plus Claude/Codex adapter result |

## Walter boundary

This proposal does not add a “pre-work Walter mode.” Walter remains a leaf with
no tools, delegation or direct user channel. Maestro may send an existing
sealed review packet before or after a work branch, sequentially, if the
canonical review contract accepts that packet type. A new packet type or
authority requires a separate durable decision and conformance tests.

## Explicit non-decisions

- no new role is added to the catalog;
- no specialist instance is registered by merging this document;
- no owner-private, task, calendar, email or chat authority is created;
- no direct hub-to-specialist edge is allowed;
- no runtime is reported available.
