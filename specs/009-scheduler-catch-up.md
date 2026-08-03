# Spec 009 - Scheduler and presence-based catch-up

Status: runtime-neutral core and Darwin bounded worker implemented; macOS
LaunchAgent lifecycle is explicit-Canary-only, while Windows native task
creation remains unavailable until qualified.

## Objective

Run recurring Agentic OS maintenance with low friction without assuming that a corporate laptop is awake, online or authenticated at an exact wall-clock time.

## Architectural rule

The native scheduler accelerates execution but does not own consistency. The Agentic OS derives missed occurrences from durable local state and recovers them on the next authorized presence trigger.

Windows Task Scheduler, a macOS LaunchAgent, Claude lifecycle hooks, Codex lifecycle hooks and manual commands are interchangeable wake-up mechanisms. They invoke the same runtime-neutral planner and may not redefine whether work is complete.

## Responsibility boundary

| Layer | Responsibility |
|---|---|
| Owning subsystem | Defines the idempotent operation and its domain truth, such as a valid memory commit or atlas manifest. |
| Scheduler core | Calculates due occurrences, bounds catch-up, serializes execution and records metadata-safe attempts. |
| Native OS adapter | Wakes `bcgos` near the configured time without requiring administrator privileges. |
| Runtime adapter | Observes session presence and enqueues catch-up after startup. |
| Session Start | Reads status only; it never blocks startup or runs model work synchronously. |

The scheduler never stores prompts, memory bodies, client content or job outputs. A receipt is evidence of an attempt, not proof that the owning subsystem published valid state. Executors report success only after the owning subsystem's durable completion boundary.

## Durable state

Scheduler state is user-local and workspace-isolated:

```text
scheduler/
  workspaces/<workspace-id>/
    enrollment.json
    receipts/<job-id>/<attempt>-<scheduled>.json
    leases/<job-id>/<sha256-occurrence>.json
```

Enrollment establishes the no-backfill boundary: a fresh installation never invents work before the user enrolled the workspace. Receipts are append-only, metadata-safe and classified as `succeeded`, `failed`, `unavailable` or `suppressed`.

Only `succeeded` satisfies an occurrence. Failed, unavailable or suppressed
occurrences remain due so a later native wake-up, manual command or
session-presence trigger can recover them. Suppression records a failed
eligibility gate, not execution; it never advances the success anchor. Catch-up
is chronological and bounded per job to prevent an unattended backlog from
creating unbounded model cost.

The scheduler state schema is `schemas/scheduler-state.schema.json`. Exact approved application directories remain governed by Q-007.

## Initial jobs

The first job vocabulary is:

- `memory-checkpoint`: three-hour metadata-only workspace continuity receipt;
- `memory-light-dream`: three-hour light L1 synthesis, unavailable without a qualified synthesis adapter;
- `memory-deep-dream`: weekly deep L2/L3/lifetime consolidation, unavailable without a qualified synthesis and eligibility adapter;
- `wiki-reconcile`: reconciliation of source watermarks, outbox receipts and atlas manifests.
- `sharepoint-work-sync`: refresh of the explicitly enrolled organizational
  work-retrieval catalog through the approved Claude SharePoint adapter.
- `darwin-housekeeping-daily`: policy-gated daily/presence health work;
- `darwin-deep-weekly`: policy-gated weekly health/evolution review;
- `walter-self-review-weekly`: silent, bounded weekly Walter self-ingestion
  seam and the sole recurring self-refinement synthesis job;
- `darwin-structural-evolution-proposal`: monthly, proposal-only review of
  operational structure through Darwin's deterministic closed planner; it
  never applies code, policy or release changes.

Job IDs and cadence are runtime-neutral. Daily and weekly local windows, timezone behavior, retry/backoff and maximum catch-up are configuration, not hard-coded adapter behavior.

`memory-checkpoint` succeeds only after a versioned workspace watermark over
allowlisted durable scheduler metadata is fsynced, atomically activated and
followed by its terminal receipt. Interrupted publication preserves the last
known good pointer; no durable source remains unavailable. It never claims
memory synthesis. The two three-hour interval jobs
anchor first to enrollment and, after success, to that successful attempt;
both use `MaxCatchUp=1`. `memory-deep-dream` succeeds only after the complete
memory commit is active. Wiki work triggered by that commit follows the outbox
and publication boundary in Spec 008; a scheduler receipt cannot substitute
for either durable commit. Walter's weekly job supersedes the retired generic
`self-refinement-proposal` catalog placeholder; it is a silent, finite
self-ingestion/compaction pass, not a weekly user-facing proposal or
notification. Observation capture remains a separate evidence producer. The
monthly Darwin job emits a metadata-only proposal receipt and remains
unavailable until its deterministic local executor is qualified and attended
authority is present.

`sharepoint-work-sync` succeeds only after Spec 037 publishes a new or
idempotently unchanged active catalog manifest. If SharePoint collection is
forbidden or unavailable in the active runtime, the occurrence remains due.

## Presence recovery

Session Start may perform one bounded, read-only status check. If work is due, the runtime adapter enqueues execution after startup and immediately returns control to the user. It may not invoke a model, compile a wiki or wait for completion inside Session Start.

Installed Maestro lifecycle hooks resolve the exact workspace-local
`.bcgos/maestro-orchestration-state.json` pointer before emitting this signal.
The pointer cannot be absolute, escape the initialized workspace or traverse a
symlink; an existing store must be a bounded, strict JSON durable snapshot.
`SessionStart` starts the same `maintenance wake --trigger presence` boundary
as a best-effort child process and does not wait for it. Repeated starts remain
idempotent at the scheduler occurrence/lease boundary. Failure to start the
best-effort wake does not turn configuration into execution evidence, block
context output or invoke a model.

No lifecycle event may wait for a scheduler or worker lock. The eventual worker
owns serialized execution; hooks read a last committed snapshot or emit a
best-effort idempotent signal as defined in Spec 019. Signals carry a bounded
typed command with an explicit deadline; an explicit authority must bind that
command to a due occurrence before execution. A busy lease returns immediately
as a nonterminal result and does not become an inline retry loop. Occurrence
leases use opaque fencing tokens and hold the OS guard across side effects plus
terminal publication, so retries with different command IDs cannot overlap and
a stale worker cannot release or finalize over its successor. Terminal command
receipts carry an opaque occurrence digest so a later command ID cannot replay
already completed work.

If no approved model or eligibility adapter is available, the executor records `unavailable`; it never substitutes a provider or marks the occurrence successful. Deterministic jobs may run unattended only when their own policy permits it.

## Native adapter contract

- Windows uses a per-user Task Scheduler task and requires no administrator privileges.
- macOS uses a per-user LaunchAgent.
- Both may combine a calendar trigger with run-at-logon or run-at-load behavior.
- A sleeping, powered-off or offline machine is an expected missed wake-up, not data corruption.
- Both adapters invoke the same core and expose equivalent status, pause and manual-run semantics.
- Security denial and revocation work is never paused behind an ordinary enrichment schedule.

The macOS adapter owns only the attended per-user lifecycle and a periodic
15-minute presence pulse. The pulse only checks due work, eligibility and the
depth-one worker; it is not the job cadence. Idle eligibility is explicit and
fail-closed: `unknown` is not `idle`. Active or unknown pulses emit at most one
suppression set per cooldown window, and suppression never satisfies due work.
Recent failed or unavailable attempts also enter a per-job/occurrence cooldown;
the next pulse records `failure_cooldown` suppression without dispatch, and a
retry becomes eligible after expiry.
Enrollment persists the validated IANA timezone, workspace and
activated job digests; fixture homes remain filesystem-only. Windows continues
to fail closed rather than claim native parity. Neither adapter decides whether
an occurrence succeeded.

`bcgos maintenance canary install-macos` requires the exact initialized
workspace path and validates it against local workspace metadata. The rendered
plist stores only the opaque workspace ID, never the workspace path or customer
content, and invokes the exact regular running `bcgos` executable through the
fixed bounded command `maintenance wake --trigger presence`. Executable and
plist symlinks fail closed. Plist creation/removal is atomic and idempotent;
native `launchctl` inspection or mutation occurs only with explicit
`--launchctl`, only in the current user's `gui/<uid>` domain, and never requests
administrator authority. `RunAtLoad` plus the bounded interval accelerate
presence recovery but do not activate Walter, memory dreaming or any other
model-backed maintenance capability.

The presence planner receives only the jobs explicitly activated in the exact
workspace enrollment. Catalog entries that are unavailable or not activated
remain visible in capability/status reporting, but are not converted into due
occurrences and do not emit repeated `unavailable` receipts on RunAtLoad or
interval wakes. Adding a handler to the binary is not activation; an attended,
validated enrollment update is required before that job can enter the plan.

## Executable core

`internal/scheduler` currently implements:

- daily, weekly, monthly and fixed-interval occurrence planning in an injected local timezone;
- enrollment-based no-backfill behavior;
- chronological, per-job bounded catch-up;
- successful-receipt deduplication;
- retry eligibility after failed, unavailable or suppressed attempts;
- append-only user-local enrollment and receipt persistence;
- deterministic ordering across multiple jobs.
- cancellation before the next catch-up occurrence, leaving the remainder due;

It deliberately does not install OS tasks, choose schedules, invoke memory dreaming, compile the wiki or make unattended model calls.

## Test expectations

- no occurrence before enrollment;
- due only after the configured local window;
- missed daily and weekly occurrences recover on later presence;
- successful occurrences do not execute twice;
- failed and unavailable attempts remain recoverable;
- catch-up remains bounded and chronological;
- invalid IDs, cadence and state fail closed;
- event/continuous signals require an event identifier and bounded deadline;
- monthly structural evolution remains proposal-only and unavailable without
  native evidence;
- workspace state remains isolated and contains metadata only;
- Windows and macOS adapters pass the same conformance fixtures before pilot use.

## Deferred decisions

- default daily and weekly windows and timezone-change behavior;
- whether approved model calls may run without the user present;
- retry/backoff, runtime and cost limits;
- user notification, pause and manual-run UX;
- receipt retention and privacy-safe telemetry;
- Windows native adapter installation and removal flow.
