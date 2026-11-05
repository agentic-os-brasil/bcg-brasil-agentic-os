# Complete verified native Windows core MCP admission

Status: implementation of existing private-state/confinement contracts released
for a bounded core pilot. No Windows availability is claimed before native proof.
User target: Mac and Windows real-client setup by end of day2026-11-05,
America/Sao_Paulo. Preserve the full-product mission and explicit unmet gates.

Base:298a9f4e3045b04686980c9b9a52547cde1f17ea,
tree0e3311784526457218598427f41e60ca3f17e27e.
Topic:codex/maestro-windows-core.
Integration:codex/integration/maestro-windows-core, created before source.
Immutable before:maestro-pilot-ci-baseline at the same source/tree, read-only.

## Outcome and existing authority

A real ordinary Windows user can admit a fresh private HOME and workspace,
open the confined work ledger, create/checkpoint/pause/restart/resume work and
complete a trusted owner/memory review with authentic persisted readback.
The implementation must establish actual OS facts, not replace existing
unavailable returns with declarations. Existing schemas, owner policies,
confirmation, publication grants, source/process guards and denials remain.

This implements the already required Windows equivalents of Specs002/004/006/
013/020/060 and their canonical owners. Reuse the finite owner/DACL policy
already present in agentorchestration: actual current-process SID owner, valid
non-null descriptor/DACL, effective allowed principals limited to that owner,
LocalSystem and Administrators, unsupported ACE forms refused. Apply policy to
held handles, not a pathname inspected separately. Windows mode bits are not
private ownership authority. No new enterprise identity, trust root, approval
mechanism or wire contract is authorized.

## Complete owning closure

Use existing x/sys/windows and NT RootDirectory/no-reparse primitives, native
locks and held-file identity/link checks. Keep the Windows foundation finite,
below the domain owners and limited to actual required operations. Do not
introduce a generic filesystem framework. Preserve current Unix APIs/behavior.

- Scheduler Windows private primitives: verified directory/leaf custody,
  exclusive creation/publication, exact replacement and restore-to-absence,
  actual durability and truthful partial recovery. Hold native directory/file
  identity through the final operation; no path-racy MoveFileEx fallback.
- Datahome: real private owner/ACL admission, exclusive fresh initialization,
  empty-home/locator recheck and authority/origin publication last. Never repair
  an existing foreign/populated home or silently alter its ACL/custody.
- Execution: actual confined ledger backend with held root/relative operations;
  preserve the existing store/wire/locking and work lifecycle.
- Memory: confined approved-capture IO/readback/correction/deletion and current
  visibility/denial/publication fences. No historical content resurrection.
- Ownerctx: private directories/files/locks and exact-parent durable publication
  for the existing onboarding/refinement journey and source provenance.

The Windows source lead exclusively owns these packages' Windows implementations,
their directly necessary platform-dispatch seams and focused tests. It may read
existing agentorchestration/releaseprovider primitives; preserve their owners.
Root owns canonical docs/specs/schema/CI/SDD, Git and all integration operations.
Other active source writers own setup/clients, CI fixtures/workflow and pilot-kit
paths; do not edit/revert those. Ask root before any ownership overlap.

Private wiki, Darwin repair, converters/providers, native agents/hooks and
stable-carrier Windows inspection remain explicitly unavailable unless separately
implemented and proven. Do not advertise their availability through this core
delivery. Existing unversioned developer-rehearsal semantics may be tested as
such; no versioned release is stripped, existing custody cleared or test-native
authority shipped to participants.

## Native acceptance and publication gate

First capture meaningful public refusal REDs for the missing Windows path. Then
prove ordinary binary init/inspect/serve and the full core work/owner/memory
journey through SDK and rebuilt shipping stdio, including restart, decline,
correction/deletion and no resurrection. Add real native junction/reparse,
ancestor/source/target replacement, hardlink/nonregular, wrong/null/unknown ACL,
foreign-owner, no-clobber, contention and publication-loss cases. Preserve exact
bytes outside the admitted root and all current-domain/terminal fences.

Proof must include an actual non-elevated ordinary Windows user. An elevated
hosted matrix or PE cross-build is not ordinary-user qualification. A bounded
ephemeral CI-local standard test account may supply that witness with explicit
kernel token assertions, isolated synthetic roots, no credential output and
cleanup; no enterprise user or colleague device is modified automatically.
Provide the minimal native job/test-driver proposal to root for CI ownership.

Only directly relevant local/cross-compile checks run locally; no full suites,
dependency/runtime acquisition, models, real clients/auth or host installation.
Coordinate the heavy Go/native-proof slot. Preserve failed commands and exact
before/final source, actual OS facts and native output. Independent Gamma and
subagent review plus eligible CI/integration precede a kit recommendation.
Unavailable or unobserved remains so if any required proof does not pass today.
