# Changelog

All notable changes will be documented here.

## Unreleased

### Current evidence snapshot (2026-08-11)

- Source baseline is `73c05e5` (`origin/main` after a large continuous wave: skills (#292,
  #293, #288), atlasops (#297), runtime packs (#295), Windows harness fixes (#299, #300,
  #301), ZIP distribution (#302, #303), platform-portability (#279, #282, #283, #284, #285,
  #287, #291, #296), owner-atlas docs (#286, #289, #290, #294), and Maestro runtime/installer
  wave (#248–#276, #278, #280). `validate`, `wiki validate` and `wiki verify` all pass. The
  managed atlas bundle is current (watermark unchanged — allowlist sources have not changed
  since 2026-08-06). No hosted CI evidence; workflows remain disabled.
- Skills bundle grows from 16 to 34; all new skills pass `validate --full` on the skill
  metadata gate. No native-qualified runtime evidence is claimed beyond the local gate.
- Windows compatibility: three targeted fixes (#299, #300, #301) land the harness gate on
  Windows without requiring elevation or elevated symlink permissions. macOS ad-hoc portable
  activation and platform-portable ZIP generation now exist but are not notarized or clean-
  device tested.

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
