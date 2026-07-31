# Spec 037 - Darwin lifecycle and bounded cadence

Status: runtime-neutral command, cadence and lease contracts implemented;
native runtime and scheduler activation remain unavailable.

## Objective

Allow Darwin to improve Maestro continuously without turning Claude/Codex hooks
into workers or allowing unattended structural changes. Event signals, daily
health, weekly operations and monthly structural evolution all use one bounded
worker contract.

## Lifecycle boundary

Lifecycle hooks emit only a typed signal. They do not acquire a worker lease,
wait for another process, call a model, use the network, read source bodies or
apply a proposal. A worker validates the command, acquires a non-blocking lease
and runs within the explicit command deadline.

Busy leases return a metadata-only `busy` receipt immediately. Expired leases
are recoverable. Failed and unavailable work remains due through the scheduler
enrollment and catch-up contract.

## Cadence boundary

The runtime-neutral scheduler supports daily, weekly and explicit monthly
occurrences. Continuous lifecycle events map only to catalog jobs with the
`event` trigger and require an event ID. No cadence or executor receives a
silent default.

The base catalog includes `darwin-structural-evolution-proposal` as unavailable,
disabled and never unattended. It may emit a bounded proposal receipt, but
approval and application are separate transactions. Darwin cannot mutate code,
policy, release state or capability manifests from a scheduled run.

## Evidence boundary

The command/receipt schemas, conformance fixture and unit tests prove local
contracts only. Claude, Codex and native macOS/Windows schedulers remain
`unavailable` or `template_only` until a qualifying native observation and
executor success boundary are recorded. Nothing in this spec promotes a
capability.
