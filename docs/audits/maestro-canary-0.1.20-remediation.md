# Maestro v0.1.20 Canary remediation

Status: locally implemented and validated; not yet merged or released.

This record classifies the v0.1.20 acceptance findings against the canonical
contracts. It contains no owner identity, workspace path, prompt, client data or
device identifier.

## Executive disposition

| ID | Finding | Classification | Disposition | Proof required |
| --- | --- | --- | --- | --- |
| C20-01 | `agent personalize draft` rejects the first managed identity as off-sequence when the caller copies `agent_id` from `agent identity` | product defect | Accept an omitted ID or the exact canonical managed ID; reject mismatches and future-agent batching. | package and CLI tests plus a fresh three-step draft/review/confirm run |
| C20-02 | `status <workspace-id>` is interpreted as a relative path | product/UX defect | Persist a private ID-to-path binding during init and revalidate the bound manifest on every ID lookup. Never scan the filesystem. | path and ID parity; missing, tampered, permission-relaxed and symlink/reparse-point bindings fail closed |
| C20-03 | Darwin appears unloaded when status observes only the plist | diagnostic defect | Reuse the durable native enrollment for read-only launchctl observation. Keep explicit authority for pause, resume and uninstall. | loaded/enabled/native-qualified status from exact binding; mutation tests remain explicit |
| C20-04 | Fallback ingestion adapter descoped | release capability gap | Select an opensource, ZIP-embeddable replacement, then produce separately versioned, verified Windows/macOS packs. | offline sanitized fixtures, size/first-use/proxy evidence, signed manifest and rollback |
| C20-05 | Client Account, Case and Walter do not execute as native delegated agents | architecture promotion gap | Keep shadow/runtime-neutral contracts unavailable until the native Claude adapter invokes the durable Pilot boundary. | accepted promotion decision, native conformance, restart/replay/privacy evidence |
| C20-06 | Claude lifecycle is observed but not fully promoted | qualification gap | Run the attended qualification protocol for the exact release/runtime/platform tuple. | fresh-session native event evidence and promotion record |
| C20-07 | Codex is detected but its adapter is not projected | product-scope decision | Keep Claude-first for the current Canary; decide whether Codex parity is a pilot gate before claiming dual-runtime readiness. | explicit pilot requirement and, if required, installed/qualified Codex projection |
| C20-08 | Model-backed weekly jobs remain unavailable | capability/authority gap | Select approved model execution, retention and unattended authority before activation. | approved authority, bounded inputs, receipts, failure recovery and native schedule evidence |

## Corrective-release evidence

The corrective slice was developed from `origin/main` at `a0a3cad`. The
following evidence is local engineering evidence for the source tree; it is
not evidence that a new installer was built, signed, distributed or accepted
on a clean device.

| Contract | Evidence | Result |
| --- | --- | --- |
| Managed identity accepts the canonical first selection and rejects a mismatched managed ID | `go test ./internal/agentidentity`; CLI integration tests | pass |
| Workspace path and exact workspace ID resolve to the same inspected manifest | `go test ./internal/workspace`; CLI `status`, `doctor` and `maestro status` tests | pass |
| Missing, tampered, permission-relaxed or symlink/reparse-point workspace bindings fail closed | workspace and shared private-path adversarial tests | pass |
| Native maintenance status observes an already enrolled current-user LaunchAgent without granting mutation authority | maintenance status tests; read-only status against the installed v0.1.20 enrollment | pass |
| An oversized SessionStart packet preserves the Maestro directive and onboarding skill while omitting the JSON envelope whole | session hook and CLI tests | pass |
| Repository contracts remain coherent | `go run ./dev/harness validate --full` | pass |
| Formatting has no whitespace errors | `git diff --check` | pass |

An isolated black-box run built the corrective CLI, initialized a new temporary
workspace and then exercised all three status surfaces by workspace ID. Both
top-level status and doctor re-inspected the same ready workspace; Maestro
status reached the expected `action_required` onboarding state rather than
misinterpreting the ID as a path. The same run completed
`agent personalize draft -> review -> confirm` with `agent_id: maestro` and
returned `state: applied`; a mismatched managed ID was rejected.

The installed v0.1.20 maintenance enrollment reported
`active_loaded_enabled`, with `loaded`, `enabled` and `native_qualified` all
true. Therefore the report's unloaded-LaunchAgent conclusion was a status
observation defect, not proof that installation failed. A null execution
receipt with no due work still does not prove that a scheduled job executed.

Formal clean-device acceptance was not run. Its contract requires an
organization-signed bootstrapper and release, an authenticated provider,
immutable release identity, an approved signer and the complete
`none -> A -> B -> A` install/update/rollback sequence. An unsigned local
candidate cannot satisfy those preconditions and must not be used to create a
passing corporate acceptance receipt.

## Required repeat Canary

The next build must repeat the three affected findings before running the full
12-task script:

1. Complete `agent personalize draft -> review -> confirm` starting with the
   canonical `maestro` identity returned by `agent identity`.
2. Run `status`, `doctor` and `maestro status` once by path and once by the
   exact workspace ID; compare workspace ID, state and brain readability.
3. Read maintenance status without a mutation flag and verify it reflects the
   enrolled LaunchAgent. Then prove that pause/resume/uninstall still require
   explicit attended authority.
4. Start a session with enough bounded context to exceed the packet budget and
   confirm that the onboarding directive and selected skill remain visible
   while no partial JSON is emitted.
5. Run all 12 Canary tasks. Keep ingestion, native agent orchestration and
   model-backed jobs marked unavailable unless their separate qualification
   contracts have actually completed.

## Delivery order

### Wave 1 — corrective release

1. Ship C20-01, C20-02 and C20-03 together.
2. Run focused unit/integration tests and the full repository harness.
3. Build a new installer from the merged source; do not reuse the v0.1.20
   artifact.
4. Repeat only the affected Canary tasks first, then the complete 12-task run.

Done means the formal identity flow completes, all three status surfaces agree
for path and ID, and a native enrollment reports its actual loaded state without
granting mutation authority.

### Wave 2 — ingestion pack qualification

1. Close Q-037: size, models/extras, prefetch, proxy, offline and rollback.
2. Build per-platform packs with pinned Python and adapter bytes.
3. Bind the pack manifest to an approved verifier supplied by the managed
   installer.
4. Prove offline DOCX/XLSX/HTML/text conversion on sanitized Windows and macOS
   fixtures and tamper rejection.

Done means the `/ingest-content` skill produces a bounded derived artifact and
provenance receipt from an installed verified pack. Source-only adapter tests
are not enough.

### Wave 3 — native agent orchestration

1. Accept the promotion decision required by Spec 033.
2. Wire Claude first to the durable `agentdispatch.Pilot` recovery root.
3. Prove the exact user → Maestro → optional Client Account → Case → optional
   Client Account return → optional Walter → Maestro route.
4. Test restart, replay, cross-scope denial, single-active-branch and done
   contracts in a native attended session.
5. Repeat for Codex only if Codex parity is an accepted pilot requirement.

Done means native receipts prove the same controller decisions as the
runtime-neutral fixtures. A model response that merely narrates the roles is
not orchestration evidence.

### Wave 4 — Darwin model-backed evolution

1. Approve model/provider and unattended execution authority.
2. Qualify weekly Walter self review, deep memory synthesis and structural
   evolution separately.
3. Keep proposals bounded and owner-reviewable; no job mutates policy directly.

Done means each activated job has a due occurrence, bounded input, terminal
receipt, retry/recovery evidence and an explicit owner-facing review boundary.

## Remaining pilot claims

After Wave 1, the product may claim improved core Canary behavior, but it must
still describe ingestion, native multi-agent orchestration and model-backed
maintenance as unavailable. Signed distribution and clean-device acceptance
remain separate release gates.
