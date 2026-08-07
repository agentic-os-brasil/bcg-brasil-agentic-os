# Changelog

All notable changes will be documented here.

## Unreleased

### Current evidence snapshot (2026-08-06)

- Source baseline is `b3d85edeac16816ccca8b69cf887a7d674786710`
  (`origin/main`, including PRs #197, #195, #196 and #198). The local
  `validate`, `wiki validate` and `wiki verify` gates pass. `validate --full`
  reaches contracts, formatting and `go vet`, but the sandbox full-test run is
  not green because of an IPv6 listener restriction, a transient MarkItDown
  timeout and a timing-sensitive Walter lease; focused reruns passed, so the
  full gate remains environmentally inconclusive. Hosted CI has no evidence
  because the repository workflows are disabled and billing/hosted status is
  not inferred locally.
- The repository `VERSION` is `0.0.0`, a factory-dev marker. Release tooling
  requires an explicit semantic version; `0.1.0` remains a product target, not
  a published release. No native-qualified, signed/notarized, Windows-device,
  clean-device or pilot-ready evidence is claimed.

### Added

- [PR #197] Installer workspace-flow analysis, explicit approval and rollback
  receipts for update, external import and workspace migration journeys. The
  installer presents unavailable migration authority as a blocker rather than
  treating UI/fixtures as native qualification.
- [PR #196 / #195] Transactional external workspace import and separate,
  versioned managed-workspace migration contracts. Import is bounded and
  receipt-bound; public native migration remains unavailable pending trusted
  post-bootstrap authority and explicit target selection.
- [PR #198] Local-beta installer directions that keep unsigned rehearsal
  artifacts as engineering evidence only; organization signing, notarization,
  Windows trust and clean-device acceptance remain release gates.
- [PR #150] Bounded Darwin daily/weekly maintenance and state-hygiene
  contracts, local repair and quarantine handling, occurrence-bound receipts,
  and fail-closed catalog updates. The repository contains local contract
  evidence; native runtime and scheduler qualification remain pending.
- [PR #150 documentation follow-up; historical snapshot superseded by the
  2026-08-06 entry above] Clarified the separate lifecycle evidence
  classes configured, local contract-tested, adapter-observed and
  native-qualified, alongside the delivery gates release-ready and pilot-ready;
  recorded the then-current evidence boundary (`as_of: 2026-08-02`, base commit
  `61c66dfecbdc21bff6137398243236232fd14988`, no reproducible in-repo runtime
  version artifact or fresh Claude/Codex native-session receipt, and no signed
  or clean-device pilot evidence).
- Contributor-facing development-harness guide and evidence-layer map, with
  README/onboarding links and explicit bare-repository/worktree diagnosis.
- Managed wiki status clarified to distinguish the implemented compiler slice
  from the still-pending full profile/security lifecycle and private atlas.
- Metadata-only core receipt contract for agent tool-call lifecycle breadcrumbs, using closed runtime/tool registries and no prompts, arguments or outputs; native adapter emission remains pending.
- Initial workspace-scoped execution ledger contract and `bcgos work create|start|inspect|export|delete` commands, with immutable contract digests, revision-checked starts and metadata-only transition history.
- Resumable execution handoff through bounded checkpoints, metadata-only mutation receipts, pause, compact next-action projection and explicit resume with a new fenced attempt.
- Core-witnessed artifact and command receipts with no-shell execution, immutable evidence history and completion-time revalidation of the done contract.
- Opaque active-execution Session Context pointer with fail-closed ambiguity and a two-session fenced handoff proof.
- Initial repository foundation.
- CLI-first private distribution decision.
- Collaboration contract, roadmap and initial specs.
- Reserved structure for CLI, bundles, adapters, schemas, migrations and installers.
- Claude-first, Codex-compatible runtime portability contract.
- Development-only Go harness with decision-log and skill integrity checks.
- Unit tests for four-letter decision codes, required fields, skill metadata and repository root discovery.
- Canonical `develop-change` and `record-decision` development skills.
- Windows, macOS and Linux validation workflow.
