---
type: Runtime Contract
title: Darwin lifecycle and cadence
description: Bounded Darwin maintenance, state hygiene and runtime qualification boundaries.
resource: repo://specs/037-darwin-lifecycle-cadence.md
tags:
    - darwin
    - maintenance
    - runtime
    - qualification
sources:
    - id: darwin-lifecycle-cadence
      resource: repo://specs/037-darwin-lifecycle-cadence.md
      title: Darwin lifecycle and cadence
status: stable
x-bcgos-profile-version: "1"
x-bcgos-stable-id: managed/darwin-lifecycle-cadence
x-bcgos-scope: managed
x-bcgos-source-fingerprint: eaaee7072b9aa1d0faf0279c3b9ae26e9200e7cd0bb6d2193e55b9995e8ccd5f
x-bcgos-freshness: fresh
x-bcgos-status: active
x-bcgos-generator-version: maestro-managed-wiki/0.2
x-bcgos-policy-version: managed-product/1
---

# Source snapshot

This managed concept is generated from the reviewed repository source `specs/037-darwin-lifecycle-cadence.md`. The source remains authoritative.

## Related

- [Wiki and atlas entrypoint](/concepts/wiki-entrypoint.md)
- [Model-backed maintenance activation](/concepts/model-backed-maintenance-activation.md)

## Source content

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
`walter-self-review-weekly` and `darwin-structural-evolution-proposal`.
Darwin housekeeping and the operational portion of deep review may execute the
same allowlisted reversible repair after validation. The Walter scheduler seam
and runtime-neutral review core exist, but its handler remains unavailable
until an approved model adapter, authority and scheduled-input integration are
installed.
The monthly Darwin job uses the existing deterministic closed planner; it is
not a model-backed executor. It remains unavailable, disabled and never
unattended. The worker can emit a bounded proposal
receipt only after a concrete runtime-qualified catalog, explicit activation
and attended monthly authority bind the exact occurrence; the shipped
catalog-only/unavailable state cannot authorize it. Approval and application
are separate transactions. Darwin cannot mutate code, policy, release state or
capability manifests from a scheduled run.

The weekly deep review also evaluates only registered, Darwin-owned
`<agent-id>/states.md` control-plane documents beneath its local maintenance
root. It retains no path or body: it counts only documents that exceed the
closed byte/line bounds and emits a proposal for concision. It never reads a
client workspace or dossier, rewrites a `states.md`, or treats a missing state
document as drift. Semantic condensation remains an attended, separately
authorized runtime activity.

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
