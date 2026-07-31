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
and `Error` fields are explicitly dropped by the adapter.

## Scorecards

The weekly operational report is a deterministic view over one versioned
window. It covers health/freshness/recovery, route outcomes, latency, budget,
manual overrides, PA coverage and capability gaps. Recommendation codes are
closed and advisory; `may_mutate_policy` is always `false`.

The monthly structural report compares versioned windows and alternative
scenarios, then reports the proposal funnel and post-change evaluation. Darwin
may author a proposal, but acceptance belongs to `human_maintainer` and
evaluation belongs to `independent_evaluator`. A Darwin self-evaluation is
invalid by contract.

Reports include an `input_sha256` over canonical, sorted evidence records so a
reviewer can reproduce the same result from the same local evidence set.

## Operating boundary

Evidence remains local. A later bridge may export only a separately approved
typed report or issue metadata; this package has no network or federation
dependency and does not change the central curator contract. Thresholds remain
hypotheses to measure, not values to activate.
