# Changelog

All notable changes will be documented here.

## 0.1.11 — 2026-08-16

Release wave since 0.1.10 (nineteen merged PRs on `main`):

- [PR #379] Cross-case write guard extended to Windows-style paths (POSIX path
  handling covered; drive-letter/backslash/mixed MSYS variants still surface in
  `eval-release.sh` Phase 14 as known Windows edge cases pending follow-up).
- [PR #380] Lifetime eligibility policy scaffolded so permanent memory can activate.
- [PR #381] Hooks stop telling owners to type commands that cannot work.
- [PR #382] Tech-core rollup points at the real index location.
- [PR #383] Three slash commands point at the real skill path.
- [PR #384] Hooks stop injecting pointers to files that are never created.
- [PR #385] Onboarding suggests `/bcg-case-kickoff` (the id that actually ships).
- [PR #386] Scaffold creates the owner atlas tree the daily skills depend on.
- [PR #387] Repair and update ritual reconciled with README-INSTALL.md.
- [PR #388] Skill catalog normalized to Portuguese.
- [PR #389] Owner atlas skills get the page shapes they were missing.
- [PR #391] Onboarding structured questions with progress counter + single closing
  confirmation (`AskUserQuestion` contract, tone rules, Bloco A/B).
- [PR #392] Onboarding persists the name at turn 2 and gates re-entry on
  `onboarding.json.status`.
- [PR #393] Hooks stop injecting empty identity on Windows paths and stop
  destroying dream-memory requests on skip.
- [PR #394] `doctor` detects installations where the hooks never ran (Windows
  without bash).
- [PR #395] Knowledge pills corrected to the 4-tier memory model.
- [PR #396] Wave record stops stating a stale skill count; points at the live
  `INDEX.md`.
- [PR #397] `identity.schema.json` reflects that `focus` is populated in every
  guided track; `track` enum includes `imported-brain`.

Bundle: `bundles/base/manifest.json` `bundle_version` propagated to `0.1.11`
via `dev/sync-bundle-version.sh`.

## 0.1.10 — 2026-08-15

Release wave since 0.1.9 (six merged PRs on `main`):

- [PR #370] `doctor` now detects pre-fix owner control-tree drift so the operator
  sees the actual divergence before any repair.
- [PR #374] Spec 053 — knowledge folder manifest for delta analysis: per-folder,
  per-analyzer manifest that lets recurring scans skip unchanged content under
  unchanged analyzer identity.
- [PR #373] Onboarding humanized into a 3-turn warm flow with a pre-check that
  refuses to advance on an unknown workspace state.
- [PR #375] Persona rename Walter → Mestre Yoda across contracts, package and
  runtime surfaces; behaviour unchanged, identifiers stabilized.
- [PR #376] `maestro-knowledge-pill` skill plus 50 pills (batch 041–090) and the
  personas canon bundle refresh.
- [PR #377] Spec 054 — anonymized telemetry export with a receiver-side verifier:
  contract-only, closed-union records, per-day salt, 90-day rolling retention.

Bundle: `bundles/base/manifest.json` `bundle_version` propagated to `0.1.10`
via `dev/sync-bundle-version.sh`.

## Unreleased

### Current evidence snapshot (2026-08-12)

- Source baseline is `ed9db1a` (`origin/main` — lightweight UserPromptSubmit context-inject
  hook PR #342 + README/CHANGELOG/VERSION fix PR #343 + onboarding restoration PR #341;
  bundle version bumped to 0.1.5). `validate`, `wiki validate` and `wiki verify` all pass.
  The managed atlas bundle is current (watermark unchanged — allowlist sources have not
  changed since 2026-08-06). No hosted CI evidence; workflows remain disabled.
- Skills bundle: 39 user-facing skills in the base bundle. All pass `validate --full` on
  the skill metadata gate.
- No PRs open.
- Latest eval-release: 94 pass, 0 fail, 0 skip on `dist/Maestro-v0.1.5.zip`
  (SHA256: `fa28d79d99906f8409589a1fddcede60f66760d046808b6bc23b5b6808901a9e`).

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
  timeout and a timing-sensitive Yoda lease; focused reruns passed, so the
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
