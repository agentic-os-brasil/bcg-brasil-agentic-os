# Spec 033 — Deterministic agent activation and PA expert advisory

## Status

Accepted for an executable shadow slice. Native runtime dispatch remains
unavailable until separately qualified.

## Purpose

Maestro needs a repeatable way to decide whether a decision episode should go
straight to its accountable agent, consult one PA expert, or run a
governed multi-agent loop. The decision cannot depend only on prompt wording.
Darwin must be able to observe the chosen route and later recommend calibration
without changing policy during an episode.

The initial posture is `balanced`.
An omitted posture normalizes to `balanced`; changing it is an explicit
episode-level decision.

## Closed decision envelope

Every route is computed from an `IntentEnvelope` with:

- one opaque episode ID;
- accountable owner: `client_account_agent` or `case_agent`;
- posture: `direct`, `balanced` or `deliberative`;
- consequence: `low`, `medium` or `high`;
- reversibility: `reversible`, `limited` or `irreversible`;
- sensitivity: `public`, `internal`, `confidential` or `restricted`;
- knowledge need: `none`, `functional`, `industry` or `both`;
- boolean ambiguity, cross-scope, external-effect and privileged-action flags;
- optional exact PA expert IDs proposed by planning.

Unknown fields and values fail closed. Narrative task text is deliberately not
part of the authority-bearing envelope. A planner may propose exact expert IDs
but cannot propose a route or reduce the route, budgets, sensitivity or
confirmation requirement. A proposed expert is eligible only when its exact
registry entry is `published`; `draft` and `retired` suggestions fall back to a
compatible published expert or fail closed.

## Deterministic policy

Policy version `pae-v1` produces exactly one shadow route:

- `D0_DIRECT`: accountable agent only;
- `D1_TARGETED`: accountable agent plus one exact PA expert when knowledge is
  needed, otherwise one exact Walter review;
- `D2_GOVERNED`: Maestro coordinates a bounded loop with at most two exact
  PA experts and a required assurance receipt;
- `BLOCKED`: no execution until the rejected condition changes or a human
  confirms through a future explicit confirmation contract.

Hard policy:

1. Privileged action and restricted external effect are blocked.
2. High consequence, irreversible change, cross-scope activity or any external
   effect requires D2.
3. In `balanced`, a functional or industry knowledge need, ambiguity or medium
   consequence requires at least D1. `both` requires D2.
4. `direct` may collapse a medium/ambiguous but otherwise safe episode to D0;
   it never bypasses hard D2 or block rules and never suppresses an explicit
   knowledge need.
5. `deliberative` raises medium, ambiguity or any knowledge need to D2.
6. A route that requires a PA expert fails closed when an exact compatible,
   published expert version is unavailable. There is no silent fallback.

Candidate PA experts are filtered by required kind, sorted by immutable ID and
selected deterministically. Exact proposed IDs are considered first only when
published and compatible. The route records the policy version, normalized
input digest, expert version and canon digest, reason codes, budgets and its own
digest.

Initial budgets:

| Route | PA experts | Calls | Token units | Duration |
|---|---:|---:|---:|---:|
| D0 | 0 | 1 | 4,000 | 10 min |
| D1 | 1 | 3 | 10,000 | 20 min |
| D2 | 2 max | 6 | 24,000 | 45 min |

These are planned ceilings, not usage targets. The working hypotheses of
70% D0, 20–25% D1 and 5–10% D2 are shadow-evaluation metadata only. They never
alter a route and are not quotas.

In this slice the envelope is caller-asserted and the plan reports
`authority_state: caller_asserted_shadow` and
`may_authorize_dispatch: false`. Budgets and stop conditions are evaluation
targets, not runtime counters. Authenticated source classification, Execution
Ledger counters and native receipts are required before a plan may authorize
dispatch.

## PA expert advisory boundary

A PA expert is a centrally versioned Helix Brasil agent of kind `FPA` or `IPA`.
It contributes the best maintained functional or industry perspective but has
no client or case scope.

The advisory request is a bounded shadow-assessment packet. It may contain only
an opaque request ID, episode and route digests, exact expert identity/version/
canon digest, a public or internal question code, bounded public/internal facts
and requested output sections. It must not contain:

- client, account, case, workspace, stakeholder or person identifiers;
- raw excerpts, prompts or attachments;
- workspace/account/case pointers;
- confidential or restricted claims;
- persistent client correlation keys.

Facts use closed codes rather than free narrative. The validator fails closed
on forbidden fields, non-allowlisted classifications or scoped pointer schemes.
Confidential and restricted source episodes cannot enter the assessment. A
successful shadow assessment produces a content-free receipt with request
digest and policy version, `may_export: false`. A future authenticated
downgrade/export capability is required before a packet may leave its scope.

The response is schema-bound and includes the exact request digest, expert
identity/version/canon digest, bounded findings, assumptions, challenges and
application cautions. It cannot grant permissions or change the route.

## Enforced shadow composition

Shadow composition is valid only when unverified breadcrumb receipts exactly
satisfy its plan:

- D0: accountable-agent receipt;
- D1: accountable-agent receipt and one advisory receipt from the selected
  PA expert, or the exact Walter review receipt selected by policy;
- D2: accountable-agent receipt, one advisory receipt per selected PA expert and
  one Walter assurance receipt;
- BLOCKED: never evaluable as completed.

Missing, extra, duplicated, stale or digest-mismatched breadcrumbs fail closed.
The CLI returns `shadow_evaluated`, never `complete`. This makes intended
participation testable without claiming that execution occurred.
The CLI recomputes the plan from the original closed envelope and the current
signed local Helix registry before advisory export or completion. Advisory
export also binds the request to the digest of the exact episode ID. A caller
cannot modify a plan and merely recompute its public digest.

## Hiring and lifecycle

Managed scaffolding supports:

- `client_account_agent`: Partner-like account intelligence and relationship
  context, owned by Maestro;
- `case_agent`: exact `case`-scoped project execution, related immutably to its
  exact Client Account Agent and owned sequentially by Maestro;
- `pa_expert`: FPA/IPA advisory agent, owned by Maestro and bound to an exact
  Helix canon version and digest.

Scaffolding is idempotent and immutable for a given agent ID. It creates signed
local registration and data-free definition/state files, but leaves runtime
state `unavailable`. A `pa_expert` scaffold is always `draft` and is never
inferred to be published. It additionally requires expert kind, semantic
version, curator and verified canon. A separate authenticated, centrally
distributed Helix lifecycle must publish it before routing can select it.
Updating canon or version requires a new immutable registration/version
workflow; it is not an in-place prompt edit.

The legacy role names remain accepted only as compatibility input aliases:
`account_agent` resolves to `client_account_agent`, and `workspace_agent`
resolves to `case_agent`. They are not canonical graph nodes, cannot introduce
new delegation edges and are never written into a new signed registration.

## Orchestration topology

Case and Client Account Agents are both Maestro roots with an immutable signed
case-to-account relation. This avoids pretending that an account-scoped parent
can delegate a child that inherits a different case scope.

PA expert advice crosses from case/account scope to centrally managed practice
scope. It therefore never runs as a child that inherits client scope. Maestro
coordinates it sequentially:

1. close or checkpoint the accountable agent episode;
2. a future qualified native adapter exports a validated advisory packet;
3. open the exact PA expert with a fresh bounded packet;
4. close the PA expert and record its advisory receipt;
5. reopen the accountable agent with a new packet containing only the validated
   advisory response.

Only one branch is active. Raw context never transits between scopes.

## Darwin calibration

Shadow evaluation records only closed routing metadata and content-free
receipts. Darwin may compare route mix, latency, budget exhaustion, missing
expert coverage and human overrides over a defined window. Darwin proposes a
new versioned policy; it cannot mutate thresholds or posture directly.

`bcgos agent monitor --stdin` produces a deterministic content-free report for
one explicit calibration window, policy version and posture. It rejects mixed
windows, mixed postures and duplicate plan observations, labels evidence
`caller_asserted_shadow`, publishes recommendation codes only and always
reports `may_mutate_policy: false`.

Promotion from shadow to native dispatch requires:

- deterministic parity fixtures;
- authenticated runtime provenance for calls;
- privacy review of the declassification adapter;
- explicit acceptance of a new policy/runtime decision.
