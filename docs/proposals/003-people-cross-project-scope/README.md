# Proposal 003 — Internal people across projects

**Status:** central need accepted; cross-project owner record deferred until a
dedicated owner-private contract exists.

**Original contribution:** Marcelo Petrof Sanches.

**Architecture reconciliation:** BCG Brasil Agentic OS maintainers.

## Executive resolution

The same internal colleague may appear in several engagements, so recreating a
complete relationship record in each workspace is wasteful. That observation is
valid. The proposed solution is not: the `account_agent` is client-scoped, not a
user-global owner agent, and cannot own cross-client colleague context.

The current contract therefore remains unchanged:

- workspace-specific people and interactions stay authoritative under
  `<workspace>/brain/people/`;
- client/account context receives only explicit `account_safe` promotions from
  workspaces under Spec 024;
- no agent may aggregate colleague records across workspaces today; and
- a future owner-private people record requires its own accepted specification,
  storage boundary and reader policy before implementation.

This resolution preserves the useful insight without weakening workspace
isolation or silently redefining the account layer.

## Scope matrix

| Information | Authority today | Allowed movement |
| --- | --- | --- |
| Client stakeholder context | Exact workspace | Explicit account-safe promotion to the same client account |
| Engagement-specific interaction with an internal colleague | Exact workspace | No automatic movement |
| Curated client/account fact supplied by an internal colleague | Exact workspace, then account after approval | Spec 024 promotion only |
| Cross-client colleague identity or collaboration preference | Unavailable | Requires a future owner-private contract |
| Organization-wide people directory | Unavailable | Requires separate organization governance |

An internal colleague being employed by the same organization does not make
facts about them safe to move between clients. A workspace record may contain
only professional context necessary for that engagement, with source,
sensitivity and correction information.

## Requirements for a future owner-private people contract

The feature may advance only when one proposal defines and tests all of the
following:

1. **Independent owner scope.** The storage and authorization domain must be
   distinct from workspace, client account and managed bundle roots. It must not
   reuse `account_agent` as a global owner.
2. **Minimal record.** The schema must enumerate allowed fields. Client names,
   engagement facts, feedback from third parties, inferred traits, diagnoses
   and behavioural scoring are denied by default.
3. **Explicit promotion.** A user-confirmed, redacted operation may copy a
   minimal fact into the owner record. There is no cross-bundle link, workspace
   enumeration or automatic memory aggregation.
4. **Purpose and readers.** Every field has a professional purpose, sensitivity
   class and closed reader set. Runtime packets contain the minimum required
   fields, not the record body by default.
5. **Human rights over the record.** Source, consent or other legitimate basis,
   correction, deletion, retention, revocation and audit behavior are defined
   before persistence.
6. **No reverse leakage.** Owner context is never copied into a workspace or
   account merely because the same colleague appears there.
7. **Runtime-neutral enforcement.** Claude and Codex adapters must use the same
   authorization core or report the feature unavailable.

## Consequences

- Spec 014 is not superseded and its bootstrap remains stable.
- Spec 016 keeps `account_agent` bound to one client account.
- Proposal 004 cannot create owner-level people, career, planning or wellbeing
  agents on top of the account role.
- Duplicate workspace records may be reduced through better templates and
  human navigation, but not by crossing security boundaries.
- The future feature is deliberately reported as unavailable rather than
  simulated through prompt instructions.

## Not in scope

- changing the account-promotion protocol;
- an HR, staffing or organization directory;
- automated profiling or relationship scoring;
- a task, calendar, email or chat aggregation layer.
