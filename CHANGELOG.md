# Changelog

All notable changes will be documented here.

## Unreleased

### Current evidence snapshot (2026-08-12)

- Source baseline is `77d3728` (`origin/main` after onboarding restoration + MarkItDown
  install + agent naming — PR #341 — plus walter skill entry-point PR #339, profile-outputs
  schema shape PR #337, and schema registry PR #336). `validate`, `wiki validate` and
  `wiki verify` all pass. The managed atlas bundle is current (watermark unchanged —
  allowlist sources have not changed since 2026-08-06). No hosted CI evidence; workflows
  remain disabled.
- Skills bundle: 38 user-facing skills in the base bundle. All pass `validate --full` on
  the skill metadata gate.
- One PR open: #340 (capabilities: context_injection state reconcile — canary DEF-04).
- Latest eval-release: 94 pass, 0 fail, 0 skip on `dist/Maestro-v0.1.4.zip`
  (SHA256: `82b0be9a267938c3df8bcb2f84bc56bb11c677bced776a39f0b88d86545200ce`).

### Added (2026-08-11 wave)

- [PR #292] `start-day` and `eod` daily-ritual skills: bounded orientation to open threads
  and time remaining at day open; reviewable closure packet and tomorrow's first priority at
  day close.
- [PR #293] Four owner-knowledge skills: `craft-update` (document a personal method or
  working preference deliberately), `learnings-bridge` (promote daily candidates into durable
  knowledge), `feedback-capture` (record received feedback against objectives), and
  `upward-feedback` (prepare considered feedback for a senior colleague).
- [PR #288] Three advisory skills: `wayfinder` (structure an open problem into a first move),
  `investigate` (find the root cause of a wrong or surprising result), and `deck-drill`
  (rehearse a deck against the questions the room will actually ask).
- [PR #297] AtlasOps `set-field` and `link` atomic write operations: named-target scoping
  prevents silent corruption; standing grants enforced at the point of effect with a session-
  boundary proof.
- [PR #295] Optional runtime packs transport: the release pipeline can ship packs (e.g.,
  MarkItDown) alongside a release artifact; base install is unchanged when no pack is
  selected.
- [PR #299] Windows harness gate: batch `gofmt` invocations to stay below the CreateProcess
  32 KB argument limit so the repository gate runs on Windows without a helper script.
- [PR #300] Windows symlink test fixtures: skip symlink-dependent tests with `t.Skip` on
  platforms that require elevation, eliminating false failures in unprivileged CI.
- [PR #301] Windows FileMode bits: `GOOS` guard prevents treating synthetic Unix mode bits as
  authoritative on Windows where those bits carry no meaning.
- [PR #302] ZIP distribution and surgical delete: Go-native ZIP generation and a clean delete
  path for the release toolchain; eval harness extended.
- [PR #303] Skill prefix normalization and bundle hygiene: renamed skills to the `bcg-`
  prefix convention and removed a personal-owner rule that does not belong in the shared
  bundle.
- [PRs #248–#291, #294, #296, #298] Maestro and platform-portability wave: macOS ad-hoc
  portable activation, Windows portable direct-activation, platform-portable ZIP generation,
  runtime-first Maestro workflow, memory-bootstrap routines, permissive beta routing, low-
  friction capabilities, BCGOS operator session-start, owner atlas vertical and segment
  documentation, owner scope contract (doc #006), Windows ownership fix, and Windows platform
  findings report.
- [PR #305] Maestro user-template pointer discipline: `CLAUDE.md` and `WELCOME.md` now defer
  to `README-INSTALL.md` as the single canonical source for the update ritual, eliminating a
  3-way contradiction that could produce divergent installation instructions across files.

### Added (2026-08-12 additions)

- [PR #304] Skills quality pass: content depth, leakage removal, and contract strings added
  across multiple base skills; `quantitative-analysis` wording tightened to prevent illustrative
  placeholder numbers.
- [PR #308] `generate-portable-zip` development skill: single-entry-point operator wrapper
  that orchestrates the three-phase portable ZIP pipeline (candidate release tree, platform
  bootstrappers, ZIP export with provenance). Development tooling only — not distributed in
  the user bundle.
- [PR #310] Eval-quality fixes: resolves A5, B7 and B1 gaps in the ZIP quality evaluation
  harness.
- [PR #312] Onboarding document ingestion surface: `maestro-onboarding` Step 4 menu now
  includes document ingestion (CV, PDF, DOCX) as a fifth explicit capability bullet.

### Evidence snapshot (2026-08-06)

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
