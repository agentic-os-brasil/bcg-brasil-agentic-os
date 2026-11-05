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

Compose only darwin-housekeeping-daily, darwin-deep-weekly and
darwin-structural-evolution-proposal through a closed maintenance-defined
constructor interface implemented by Darwin. Preserve Darwin's existing import
direction and one Worker/opaque grant/lease/terminal chain. A missing constructor
keeps those jobs unavailable. Scope comes from actual admitted home/workspace,
never a global maestro-system default.

The darwin-local-health-v1 cache is a strict maximum-64-KiB current.json under
home/state/maintenance/workspaces/<workspace-id>/darwin/managed-state. Its finite
body-free producer reuses actual canonical runtime/projection/configuration/
workspace/capability validators and bounded non-Darwin scheduler observations.
Bind actual implementation/configuration/external-source provenance, coverage
and aggregate findings. Legacy bin/bcgos readiness cannot establish modern
readiness. Native remains not_observed and unavailable coverage stays explicit.
Missing cache is absent, not healthy; initialize only through exact attended
review. Status/report and recurring runs cannot initialize.
The existing sealed review and actual trusted/local presentation expose a
bounded canonical projection: cache state, exact parent presence/SHA, external
source SHA and intended initialization, refresh or assessment effect. An opaque
source digest alone does not describe the effect being confirmed.

Currentness follows independently collected external provenance, never writable
stale flags, counters or timestamps. Exclude these three jobs' own caches,
reports, receipts, control/qualification state and occurrence/window/time from
that external fingerprint. Preserve all independent live source/process/control/
installation/lease checks. Track only the precisely permitted cache-parent to
replacement transition without admitting external drift.

Refresh uses the existing allowlisted reversible repair owner, bound to exact
old bytes/hash or absence, reviewed external source and desired bytes. Guard final
CAS/held-directory rename/fsync and read back exact bytes. Compensation restores
exact prior bytes or removes exactly its own new leaf through the same held
owner after matching new digest. Preserve bounded interrupted-recovery evidence;
failed compensation is recovery-required. No generic deletion or new transaction
framework is introduced. Windows unsupported publication refuses before writes.
Later-clock retry preserves the original job/occurrence/transition lineage or
verifies an explicit finalized successor in the same recovery owner. Reports
verify that exact closure; immutable proposals are reused rather than duplicated.
A further later unchanged replay cannot create a new qualification or proposal
from occurrence/time churn.
Preserve the original scheduled instant separately from actual request time;
they differ for delayed weekly wakes and catch-up. Recovered immutable proposal
bytes retain that scheduled provenance. Missing legacy schedule authority can
come only from its verified original artifact, never an inferred request time.

Keep actual reviewed lock order: domain, product control, installation, current
lease, publication. Expose only a narrow existing opaque-grant publication
operation; outside packages cannot mint grants. Hold that guard through every
active cache/diagnosis/proposal/domain-receipt and terminal publication. An
UpdatePrivateFile callback ending before rename is insufficient; extend the
owning primitive without changing historical API semantics.

Read external scheduler provenance through a narrow owning ReadReceipt with
existing strict decode/validation. Enumerate only closed non-Darwin catalog
directories, at most 128 entries/job and 256 total, and at most 64 KiB/receipt;
overflow refuses before body reads. Filtering the unbounded Receipts aggregate
afterward is not acceptable. Weekly state documents retain the 16-file,
12-KiB/240-line bounds and aggregate-only findings. Directory names do not prove
registered agents; keep state-document coverage unavailable until a concrete
admitted registration source is bound.

Daily publishes real reviewed initialization/refresh and finite diagnosis.
Weekly/monthly reuse immutable ProposalStore assessment/proposal ownership with
workspace/source/producer/effect provenance and verified exact readback. Keep
proposal_emitted, reviewed_no_change and succeeded truthful throughout domain
evidence, qualification and scheduler terminal verification. Nonempty genuine
observations may qualify bounded execution while findings remain unavailable.
Semantic identity excludes occurrence-derived proposal/window IDs; unchanged
replay preserves genuine qualification/artifact/receipt or remains unqualified.

Daily/weekly enrollment is separately reviewed exact opt-in with MaxCatchUp=1
and no backfill. Monthly permits attended bounded proposals only, rejects
recurrence and never applies code/policy/release/route/structural changes.
Acceptance requires actual observations, reviewed absent initialization, a
genuine upstream non-Darwin event, independently stale cache, reviewed refresh/
readback and unchanged replay through Service, SDK and rebuilt shipping stdio.
Include byte/absence compensation, interruption, alias/owner/mode/tamper/scope/
source/parent refusals and live grant/process/install/lease loss. Preserve
calendar/Wiki consumers and current source guards. Independent task gate and
four final front lenses remain required; native wake/clients/devices, hosted
CI, production trust/publication and pilot are separate.
