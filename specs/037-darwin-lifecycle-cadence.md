# Spec 037 - Darwin lifecycle and bounded cadence

Status: runtime-neutral command, cadence, lease and local worker contracts
implemented; macOS adapter is explicit-Canary-only and native qualification
remains pending; Windows native creation is unavailable.

## Objective

Allow Darwin to improve Maestro continuously without turning Claude/Codex hooks
into workers or allowing unattended structural changes. Event signals, daily
health, weekly operations and monthly structural evolution all use one bounded
worker contract.

## Lifecycle boundary

Lifecycle hooks emit only a typed signal. They do not acquire a worker lease,
wait for another process, call a model, use the network, read source bodies or
apply a proposal. A worker validates the command, requires an explicit
authoritative occurrence, checks the Darwin-owned job/trigger matrix, acquires
a non-blocking lease and runs within the explicit command deadline. Syntax is
never execution authority.

Busy leases return an ephemeral metadata-only `busy` result immediately; they
do not occupy the command's durable terminal receipt. Leases are keyed by the
canonical work occurrence rather than command attempt, use unguessable fencing
tokens and hold an OS guard across side effects and terminal receipt
publication. An expired lease is recoverable after its prior OS ownership is
released (including process exit/crash); a live stalled owner continues to
return `busy` rather than permit overlapping mutation. Failed and unavailable
attempts remain due through the scheduler enrollment and catch-up contract.

## Cadence boundary

The runtime-neutral scheduler supports daily, weekly and explicit monthly
occurrences. Continuous lifecycle events map only to catalog jobs with the
`event` trigger and require an event ID. No cadence or executor receives a
silent default.

The base catalog separates `darwin-housekeeping-daily`, `darwin-deep-weekly`,
`walter-self-review-weekly` and `darwin-structural-evolution-proposal`. The
Walter handler remains unavailable until its runtime-neutral integration lands.
The monthly Darwin job is unavailable, disabled and never unattended. The worker can emit a bounded proposal
receipt only after a concrete runtime-qualified catalog, explicit activation
and attended monthly authority bind the exact occurrence; the shipped
catalog-only/unavailable state cannot authorize it. Approval and application
are separate transactions. Darwin cannot mutate code, policy, release state or
capability manifests from a scheduled run.

The Darwin worker does not implement the scheduler's raw `Executor` interface.
Native adapters must construct the qualified authority and bounded command;
passing an occurrence directly cannot invoke Darwin tools.

## Evidence boundary

The command/receipt/lease schemas, conformance fixture and adversarial tests
prove local contracts only. Receipt attempts are immutable and carry an opaque
occurrence digest; successful or proposal-emitted attempts suppress every retry
for that occurrence even when the command ID changes. Claude and Codex remain
unavailable. macOS is
`adapter_installed_native_qualification_pending` after explicit Canary install;
Windows is `unavailable_native_qualification_pending`. Neither state is native
qualification, and no wake receipt alone promotes a capability.
