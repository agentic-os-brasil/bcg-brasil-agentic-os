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
x-maestro-profile-version: "1"
x-maestro-stable-id: managed/darwin-lifecycle-cadence
x-maestro-scope: managed
x-maestro-source-fingerprint: 600f153de5f818d6e27b41c172d968024faa678fcea22f33d3c0dd3f79b892c7
x-maestro-freshness: fresh
x-maestro-status: active
x-maestro-generator-version: maestro-managed-wiki/0.2
x-maestro-policy-version: managed-product/1
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
`yoda-self-review-weekly` and `darwin-structural-evolution-proposal`.
Darwin housekeeping and the operational portion of deep review may execute the
same allowlisted reversible repair after validation. The Yoda scheduler seam
and runtime-neutral review core exist, but its handler remains unavailable
until an approved model adapter, authority and scheduled-input integration are
installed.
The monthly Darwin job uses the existing deterministic closed planner, with no
model-backed executor. Without a concrete product binding it remains unavailable;
catalog presence alone supplies no authority. The DHRP binding below permits an
exactly reviewed attended qualification to publish a bounded assessment/proposal
for its authoritative monthly occurrence. It rejects recurring enrollment and
never runs unattended. Proposal approval and application are separate operations;
Darwin cannot mutate code, policy, release state or capability manifests from
this job.

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

## Product local-health binding (DHRP)

The three closed Darwin jobs compose a maintenance-owned constructor implemented
by `internal/darwin`. A missing constructor or unavailable platform retains
unavailable status. Scope derives from the admitted home/workspace, actual
producer/configuration and existing signed installed-source guards. Model input
cannot supply a root, collector, healthy finding, repair command or grant.

Read-only status exposes absent, current, stale, corrupt, foreign, unsafe and
unavailable cache states with bounded metadata. It creates no directory, lock,
cache, enrollment or proposal. Review binds the exact diagnosed external inputs,
cache-parent presence/hash and proposed effect. Genuine trusted confirmation is
required for any admitted mutation; an altered projection or changed input
requires fresh review.

The finite collector reuses actual workspace/configuration/process/projection/
capability owners and bounded non-Darwin scheduler observations. Enumerate only
catalog directories, at most128 entries/job and256 total before body reads,
with strict held-private receipt reads limited to64KiB. Exclude all Darwin job
outputs/control/receipts and time/occurrence churn from the external fingerprint.
Native observation and registered state-document coverage remain unavailable
until independently bound and qualified.

The cache is a strict maximum64KiB `darwin-local-health-v1` current.json under
home/state/maintenance/workspaces/<workspace-id>/darwin/managed-state. Attended
first qualification may create it exclusively. An occupied leaf is preserved;
readback must match exact newly published bytes. A failed post-publication check
retains the actual bytes and recovery journal as partial/recovery-required.

Automatic replacement and byte/absence compensation remain unavailable under
#462/#488: held-parent/CAS observations do not establish physical preservation
against an uncooperative writer. Daily stale-cache review exposes
`manual_refresh_required`; apply refuses before control or effect publication.
The owner must preserve the state for explicit reconciliation. No command,
archive/delete action, alternate service or waiver is supplied by this product
operation. Existing scheduler/private-kernel owners remain unchanged.

Weekly work can assess current external evidence while preserving an absent or
stale cache; attended first creation is permitted when absent. Weekly/monthly
publish immutable bounded assessment/proposal artifacts through ProposalStore,
with verified workspace/producer/configuration/source/occurrence provenance.
Monthly remains attended-only, cannot enroll and applies no structural change.
Daily/weekly enrollment is a separate exact opt-in with MaxCatchUp=1 and no
backfill. A diagnostic qualification attests actual execution, producer and
configuration; every later attempt independently binds its new external inputs.

Reuse one existing Worker/opaque grant/lease/terminal chain. Final active
publication rechecks current process, control revision, actual installed source,
exact per-attempt diagnosis and lease. Interrupted cache/proposal publication
retains original occurrence and scheduled instant separately from request time;
retry verifies/reuses the exact durable artifact rather than relabeling it.
Semantic no-change cannot manufacture a qualification or proposal from time.

The current Darwin constructor/publication remains unavailable on Windows.
The existing native private kernel is preserved; source compilation is not
native Darwin qualification. Focused owning and real SDK/rebuilt-stdio evidence,
four exact-source reviews and affected hosted checks are separate gates.
Automatic refresh/compensation acceptance, native Windows, real Code/Desktop
loading, wake, production trust/distribution and colleague acceptance stay open.
