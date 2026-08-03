# Spec 036 - Universal maintenance plane

Status: catalog, bounded command/receipt surface, metadata-only memory checkpoint, deterministic L1 light dream, Darwin deterministic worker and
explicit Canary lifecycle are implemented; Walter/model-backed work and native
Windows task creation remain unavailable pending runtime evidence. macOS native
qualification is environment-dependent and is never inferred from a plist.

## Intent

The base bundle prebuilds the recurring work that should keep a professional
Agentic OS healthy: memory capture and rollups, wiki synchronization and
reconciliation, skills/runtime indexes, capability and drift checks, self
observation proposals, and update diagnosis. This is a catalog of contracts,
not a promise that every laptop can run every job immediately.

## Catalog boundary

`bundles/base/runtime/maintenance.json` is platform-neutral and contains no
provider, path, credential, schedule window, client content or raw OS task.
Every initial job is explicitly `unavailable` until its owning subsystem and
runtime adapter establish executable evidence. The catalog is embedded in the
base bundle and exposed by `bcgos maintenance catalog` and `bcgos maintenance
status`.

Lifecycle hooks only emit bounded typed wake signals. They do not acquire a
worker lease, wait for a command, call a model or apply maintenance inline.
Workers require a concrete runtime-qualified catalog/policy authority and an
exact scheduler-emitted occurrence; the shipped `catalog_only`/`unavailable`
catalog fails closed. Workers own the occurrence-keyed fenced lease and
explicit timeout, and the OS guard spans both side effects and terminal
publication. The monthly Darwin structural job can emit a reviewable proposal
receipt only after explicit activation plus attended qualification; approval
and application are separate operations.
The Darwin worker deliberately has no raw occurrence execution method, so
`scheduler.RunDue` cannot bypass command authority and fencing.

The jobs are deliberately split by success boundary:

- the workspace-scoped `memory-checkpoint` may atomically advance only a
  versioned watermark over allowlisted durable scheduler metadata every three
  hours, then close its own receipt boundary; it preserves last-known-good,
  rejects source-free success and does not read or synthesize memory;
- `memory-light-dream` has a separate three-hour due contract and a qualified
  deterministic L1-only handler over already-sanitized captures;
  `memory-deep-dream` remains weekly, model-backed and unavailable until deep
  synthesis plus lifetime eligibility are qualified;
- deterministic checks may eventually run unattended once their local contract
  is qualified;
- local adapters, including Darwin housekeeping, remain policy-gated; Darwin
  may apply only an allowlisted, reversible repair inside managed state after
  deterministic assessment, guard authorization and post-repair validation;
- model-backed weekly memory dreaming and Walter self review are never silently enabled;
- managed-scope jobs cannot write owner or private workspace state.

## Wake and catch-up

`bcgos maintenance wake --trigger presence|daily|weekly|monthly|event` is a
bounded worker invocation. Without persisted Canary enrollment it fails closed
and emits no receipt. With enrollment, Darwin daily housekeeping may apply its
smallest allowlisted reversible repair after validation, while weekly deep
review may apply the same operational repair and emit proposals only for
remaining structural findings. Both require their exact activation,
qualification digest, lease, deadline and occurrence fence. Walter and monthly
structural work remain due/unavailable and never become successful from a wake
receipt alone. Walter is the sole recurring self-refinement synthesis surface;
the retired generic placeholder is not a second job.

macOS and Windows surfaces live under `adapters/` and are not part of the
immutable base distribution. macOS has an explicit per-user Canary installer
and attended LaunchAgent lifecycle; Windows remains an honest unavailable
native adapter in this PR. A launch agent or Task Scheduler task is never the
source of truth; the owning subsystem's durable commit/manifest is.

The macOS LaunchAgent emits a 15-minute pulse only. The scheduler derives due
work and the depth-one worker applies activation, qualification, explicit idle
and cooldown gates before one handler dispatch. Its `auto` idle mode reads the
bounded native HID idle counter and requires five minutes without activity.
Unknown, malformed or unsupported activity state fails
closed and is not treated as idle. `suppressed` is distinct from failed,
unavailable and succeeded: it records metadata-safe eligibility denial, stays
due and cannot move a three-hour success anchor. Repeated suppressions are
cooldown-bounded to avoid a pulse receipt loop.
Failed and unavailable attempts use a separate per-job/occurrence cooldown;
their `failure_cooldown` suppression stays due and retry resumes after expiry.

## Automatic improvement scope

The prebuilt plane covers the maximum safe universal surface. It does not
auto-commit, auto-push, publish private content, call an unapproved model,
change agent policy, or ingest external data without explicit provider and
consent contracts. Those capabilities can be added behind the same catalog
without changing native scheduler semantics.

## Evidence rule

The conformance fixture in `adapters/conformance/maintenance.json` distinguishes
catalog presence, adapter templates and native evidence. No capability manifest
promotion is allowed from a fixture, a local configuration, or an adapter file
alone.
