# Spec 021 - Maestro canary observability

Status: local report, receipt/report schemas and privacy tests implemented.

## Objective

Measure whether the Maestro canary creates practical value and where it fails,
without turning observability into a new federated-learning path or collecting
workspace/client content.

## Scope

The canary emits metadata-only receipts for these closed events:

- first value reached, represented by a duration bucket;
- long-running goal resumption outcome;
- installation, update and rollback outcome;
- a manual-intervention count bucket;
- a capability failure, represented only by a closed capability ID and outcome.

The local report aggregates those receipts into one operational dashboard. It
shows totals and buckets, not raw timeline detail, exception strings, user
identity, workspace identity, client name, file path, prompt, transcript or
artifact body.

## Boundary

Canary observability is local and has no bridge, federation batch, Darwin
curator, GitHub, skill export or automatic network path. It may later become
an input to a separately approved pilot-analysis workflow, but this contract
does not create that path.

The existing scheduler receipt cannot be reused as the canary wire artifact:
its optional error string is intentionally useful for local scheduling but is
not safe for telemetry. The canary has a closed receipt vocabulary instead.

## Receipts and report

Receipts are append-only user-local JSON under the product data root. Every
receipt contains only schema version, timestamp, event kind, outcome and
allowed buckets/enums. Unknown fields and unapproved enum values fail closed.
`bcgos canary record --stdin` is the bounded adapter-facing ingestion surface;
`bcgos canary report` renders the aggregate report. Native Claude/Codex
lifecycle hooks are not implemented by this specification and must not be
claimed as active collection.

The report contains:

- first-value duration distribution;
- resume success/failed/blocked counts;
- installation/update/rollback outcome counts;
- manual-intervention bucket distribution; and
- capability-failure counts by approved capability.

## Acceptance criteria

1. A report can be generated from valid local receipts.
2. No receipt or report field accepts arbitrary text, maps, workspace IDs or
   client data.
3. Strict parsing rejects unknown/content-bearing fields.
4. Canary strings injected into invalid input do not appear in persisted data
   or a valid report.
5. Federation code and outbound federation schemas remain unchanged by this
   capability.
