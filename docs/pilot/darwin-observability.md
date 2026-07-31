# Darwin observability and selection evidence

This contract is the minimum local evidence layer for the canary. It does not
expand federated learning, add a telemetry server or promote native runtime
dispatch.

## What is emitted

`schemas/darwin-evidence.schema.json` defines a closed metadata-only union:

- `health` records freshness and recovery of Darwin, weekly federation and
  activation-monitor jobs;
- `selection` records route, outcome, latency, planned/used budget, human
  override, PA Expert coverage and closed capability gaps;
- `proposal`, `acceptance` and `evaluation` records make the evolution loop
  auditable without carrying proposal text or work content; and
- `alternative` records compare baseline and candidate scenarios without
  finalizing a depth threshold.

The contract excludes prompts, outputs, documents, paths, people, clients,
workspaces, raw errors, URLs and arbitrary attributes. Scheduler `WorkspaceID`
and `Error` fields are explicitly dropped by the adapter. Every record carries
only an opaque `scope_sha256`; reports reject mixed scopes even when two
workspaces happen to use the same window name. Local window labels are
one-way projected to `win-<digest>` identifiers before they enter evidence.

This first schema version is explicitly `caller_asserted_shadow`. Typed local
adapters make the payload closed, but they do not authenticate native runtime
provenance. Reports therefore always emit `review_evidence_authority` and
cannot qualify or activate a runtime capability.

## Scorecards

The weekly operational report is a deterministic view over one versioned
window. It covers health/freshness/recovery, route outcomes, latency, budget,
manual overrides, PA coverage and capability gaps. Recommendation codes are
closed and advisory; `may_mutate_policy` is always `false`.

Freshness uses a fixed local rule: up to five minutes late is `current`, up to
one hour is `aging`, and later is `missed`. A successful missed occurrence is
`recovered`; failed, partial, blocked and unavailable occurrences cannot be
reported as recovered. Activation observations with a missing composition
receipt become `partial` and always carry `receipt_coverage`.

The monthly structural report compares versioned windows and alternative
scenarios, then reports the proposal funnel and post-change evaluation. Darwin
may author a proposal, but acceptance belongs to `human_maintainer` and
evaluation belongs to `independent_evaluator`. A Darwin self-evaluation is
invalid by contract. Windows are canonical, non-overlapping, half-open
`[start,end)` intervals. Every record must fall inside its named window.
Acceptances and evaluations are counted only when they link uniquely to the
same proposal digest; implementation requires an accepted decision, and an
evaluation requires an accepted implemented proposal plus valid baseline and
post-change windows.

Alternative observations are paired by opaque cohort digest. Every cohort must
contain the same alternatives and include a baseline, preventing comparisons
between unmatched task populations.

Reports include an `input_sha256` over canonical, sorted evidence records so a
reviewer can reproduce the same result from the same local evidence set and
canonical window definitions. Inputs are bounded to 10,000 evidence records
and 64 windows. Each record ID binds the complete closed record, so the same
payload cannot be replayed under multiple caller-chosen identifiers. Selection
usage must retain the exact D0/D1/D2 policy ceilings; this shadow layer cannot
invent a larger budget.

## Operating boundary

Evidence remains local. A later bridge may export only a separately approved
typed report or issue metadata; this package has no network or federation
dependency and does not change the central curator contract. Thresholds remain
hypotheses to measure, not values to activate.
