# Spec 062 - Product maintenance service

Status: #505 maintenance is landed. Issue #506 composes three source-owned wiki jobs and three read-only queries on that same lifecycle, for eleven supported job IDs in the eighteen-entry catalog. Source/local/hosted/client qualification remains separately recorded. Native Windows private wiki and background wake remain unavailable. See the [operator workflow](../docs/product-mcp-maintenance.md).

## Outcome

An admitted product workspace can inspect maintenance without writing state,
review and qualify a named deterministic job, enroll exactly that job, execute
due work, inspect its domain result, and pause, resume or revoke it. Code,
Desktop and the local operator use one canonical maintenance service. Existing
Worker, scheduler leases/receipts and domain stores remain authoritative.

Spec036 owns the eighteen job contracts. This delivery connects checkpoint,
light/deep dreaming, retention diagnosis, skills indexing, runtime health,
capability diagnosis, drift diagnosis and the three source-owned wiki jobs described in [Spec 008](008-wiki-update-okf.md). Other catalog entries retain explicit
unavailable reasons until their complete owning journeys are constructed.
Catalog presence, a handler declaration and installation grant no activation.

## Identity and enrollment

The service derives its private home, registered workspace identity and roots
from admitted product state. Model arguments select only closed operations and
catalog job IDs. They cannot provide paths, executables, qualification digests,
signers, approval flags, a healthy observation or a replacement handler.

Product scheduler and maintenance roots are `home/state/scheduler` and
`home/state/maintenance`. Their canonical stores retain their workspace trees;
checkpoint versions remain under the maintenance store. These are new product
bindings, not permission to migrate or delete older Canary-library state.

Qualification and recurring enrollment are separate. An attended, exactly
reviewed one-shot qualification executes the actual bound handler and verifies
its durable domain result and receipt. Record workspace/job, current executable
and relevant configuration identity, adapter/platform, observed result identity
and verification time. A constant job hash, forged receipt, empty input,
source-free checkpoint or successful no-op cannot produce qualification.
Changes to the bound implementation or configuration invalidate qualification.
Engineering evidence never promotes the shipped global catalog or native state.

Index and retention evidence must bind the actual executing implementation and
configuration, so an update can be requalified with otherwise unchanged genuine
inputs. Historical intact evidence may be verified as old and unqualified;
never synthesize current provenance or require deleting a valid prior result.
Clock-only, timestamp-only and nonce-only changes cannot qualify a no-op.

After qualification, trusted owner review binds the exact job, timezone,
cadence, write set, qualification and expected enrollment revision. Decline
creates no enrollment. Each job has its own monotonic revision, no-backfill
boundary and active/paused/revoked state; revoke retains a denial tombstone.
Every dispatch requires the exact active revision, including default-enabled
jobs. Pause preserves pending work, resume allows at most the existing bounded
catch-up, and reenrollment cannot invent work before its new boundary.
Status and review perform no directory/key creation, repair or qualification.

Existing control records supply their own required stored schema/workspace/job
binding; caller defaults apply only to absent-file initialization. Read mutable
control authority through a held descriptor that verifies owner, private mode,
regular bounded contents and a single link. Unsafe aliases or damaged records
refuse without repairing bytes, permissions or directory entries.

## Effects and revocation

Bind a worker-issued grant to the exact command, occurrence, workspace,
enrollment revision and live lease. Validate before effects and fence each
visible domain publication and terminal success with the same current-revision
and lease guard. A check followed by an unguarded rename is insufficient.
Revocation may proceed while synthesis is running; after its denial commits,
that attempt cannot publish a new active artifact or success. Orphaned private
staging is not active output. Preserve the domain's last known good state,
capture visibility barrier, timeout quarantine and stale-attempt refusal.

Use narrow canonical publication seams in memory and checkpoint storage rather
than a parallel transaction store. Establish and test one lock order. A domain
commit followed by receipt failure remains partial evidence, never an invented
success; replay reads current domain truth and preserves idempotency.

The three-hour memory jobs retain MaxCatchUp=1. Scheduler occurrence identity
uses ScheduledFor; light dreaming intentionally processes eligible captures for
the execution day at RequestedAt, matching the existing public handler. Record
both times; a delayed wake must not claim an earlier day's synthesis. Checkpoint
needs real durable non-checkpoint scheduler metadata, so bootstrap through a
trusted capture and qualified light dream before qualifying checkpoint.

## Domain ownership

Reuse the current embedded memory policy, budgets, attestor and canonical
home/brain/memory workspace store. Daily work produces only L1. Weekly work
uses the existing deterministic weekly synthesizer and real L3 ancestry. Exact
owner review still governs an eligible lifetime candidate; an unattended pass
may retain L2/L3 while reporting lifetime confirmation unavailable. Enrollment
does not grant standing approval for future lifetime text. Reconcile stale
model_adapter declarations with deterministic Specs006/009/036 without enabling
the job by default or borrowing a model/credential.

Retention diagnosis inspects current canonical visibility/retention state and
never deletes sources. Skills indexing derives from verified installed assets
and writes only the external runtime index. Runtime health, capability and
drift use bounded actual observations; missing native or installed evidence is
reported as missing/unavailable, never synthesized healthy. Each job's success
requires its named durable domain output and a separate maintenance receipt.

## Product interface

Expose closed maintenance status, qualification, enrollment, due-run, pause,
resume and revoke operations through the existing maestro_query/maestro_apply
tools. Trusted elicitation reviews exact intent and revalidates after the
response. Reuse modern/legacy confirmation support; a client without trusted
review receives the existing local stdin review/apply fallback. Model output
contains bounded metadata and domain references, not private captures or full
profiles. The installed method routes these intents to the same service.

Ordinary run dispatches only active due jobs and never installs OS tasks,
downloads dependencies or calls a provider/model. Native lifecycle ownership
remains a separately attended operation. Foreground proof does not establish
background wake behavior or client qualification.

## Acceptance

Public SDK and rebuilt stdio must demonstrate: read-only fresh status; declined
review without state; real owned capture to qualified L1 and receipt; exactly
one enrolled job with all others inactive; separately qualified checkpoint and
readback; due-run/restart replay without duplicate output; stale/cross-workspace
qualification and revisions refused; pause/resume bounds; revocation during
blocked synthesis preventing active output/success; changed build/config
qualification refusal; real weekly L2/L3 without unreviewed lifetime; and each
diagnostic/index job's real durable result. Use focused tests and existing
private filesystem primitives. No full local suite, fake positive qualification
or native/signing/pilot claim is permitted.

## Runtime index and diagnostic execution contract

The external per-workspace index root is
`home/brain/runtime/workspaces/<workspace-id>/skills-index`. Publish immutable
generations containing catalog.json, INDEX.md and typed source/projection/hash
metadata, then replace one canonical current pointer under the owning guard.
Derive the exact installed subset and runtime pointers from integrity-checked
RoutingInputs/Inspect; a ProductRoot or arbitrary directory enumeration is not
installed-source authority. Preserve the factory compiler/generated assets.
Spec012 owns the compact content and routing contract.

Runtime selection comes from actual admitted workspace projection metadata,
validated through its canonical reader. CLI version is a trusted native
constructor input from the actual caller; unknown version stays unknown.
Use closed canonical collectors, not callbacks or model booleans declaring a
healthy observation. Preserve catalog/parser compatibility while isolating
declarative catalog types below runtime embedding to avoid dependency cycles.

A diagnostic job may qualify its actual bounded execution and durable report
while its finding remains missing or unavailable. Require nonempty genuine
observations and verified readback; timestamp-only, synthetic-empty and no-op
reports cannot qualify. This never promotes the reported runtime/capability to
observed, healthy or native-qualified. Retention qualification similarly needs
real capture/denial/commit observations and performs no deletion or repair.

Deep-dream intent/enrollment is not lifetime-text approval. Reuse the existing
exact per-candidate owner review; without that trusted review retain L2/L3 and
report lifetime confirmation unavailable. The existing attended memory
consolidation path remains available; do not create a second approval authority
or effectful MRTR retry mechanism merely to qualify the recurring job.

### Attended signed-source diagnosis (MPDU)

The existing update-check job can execute only as an explicitly reviewed attended
check of the installed owner's enrolled signed source. It remains unattended-never
and cannot enroll as recurring work or activate an update. Declare its actual
installation-source custody, bounded candidate staging/plan and local-diagnosis
effects in the closed catalog and exact trusted review; local_diagnostics alone
cannot authorize source-state mutation. Model input selects no root, source/key,
executable, clock, native verifier or approval authority.

Revalidate the captured signed input, source tuple, current installation and live
grant before effects. Confirmed authentic higher metadata retains its sequence
and recalls even when expired or payload-incomplete, as specified by MPDU. Report
that actual observation and unavailable eligibility separately. A genuine check
or diagnosis is not a claim that the current release, provider or native client
is healthy; synthetic/timestamp-only reports cannot qualify execution.

Retain one existing Worker/control/lease/terminal chain and one installation-state
serialization owner. Source custody stays outside Previous and cannot be reset
by a report, compensation or managed rollback. A failed/stale/revoked publication
must retain truthful partial/recovery evidence. The trusted installer provides
the independent repair path when the MCP's qualified source launch is blocked.

For update-check, advancing the source observation time can legitimately change
the exact-state-bound pending plan ID without changing the authenticated
source/candidate diagnosis. Exclude that derived plan ID only from diagnostic
semantic identity; retain the full exact plan/state checks for activation.
Reviewed-no-change preserves existing genuine qualification and its actual
artifact/receipt, or remains unqualified, and clears only its own one-shot under
the live guard. It cannot mint a new qualification from that clock-only churn.
Reports retain the stored plan identity as historical when stale; a separate
bounded current-pending projection requires read-only canonical verification.
