# Spec 036 - Universal maintenance plane

Status: catalog, bounded command/receipt surface and safe CLI are implemented;
owning executors and native scheduler installation remain unavailable pending
runtime evidence.

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
Workers own the lease and explicit timeout. The monthly Darwin structural job
can emit a reviewable proposal receipt only; approval and application are
separate operations.

The jobs are deliberately split by success boundary:

- deterministic checks may eventually run unattended once their local contract
  is qualified;
- local adapters (capture, owner observation and update diagnosis) remain
  policy-gated;
- model-backed weekly dreaming and self refinement are never silently enabled;
- managed-scope jobs cannot write owner or private workspace state.

## Wake and catch-up

`bcgos maintenance wake --trigger presence|daily|weekly|monthly|event` is a bounded,
read-only probe today. It returns `state: unavailable` and emits no scheduler
receipt when no executor is installed. This makes native adapters safe to
install early: a wake-up cannot masquerade as completed memory or wiki work.

macOS and Windows templates live under `adapters/` and are intentionally not
part of the immutable base distribution. They are disabled reference artifacts,
not installable automation. A future installer may render and enable one only
after the owning executor, enrollment and receipt contract are qualified. A
launch agent or Task Scheduler task is never the source of truth; the owning
subsystem's durable commit/manifest is.

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
