# Changelog

All notable changes will be documented here.

## Unreleased

### Added

- [PR #150] Bounded Darwin daily/weekly maintenance and state-hygiene
  contracts, local repair and quarantine handling, occurrence-bound receipts,
  and fail-closed catalog updates. The repository contains local contract
  evidence; native runtime and scheduler qualification remain pending.
- [PR #150 documentation follow-up] Clarified the separate lifecycle evidence
  classes configured, local contract-tested, adapter-observed and
  native-qualified, alongside the delivery gates release-ready and pilot-ready;
  recorded the current evidence boundary (`as_of: 2026-08-02`, base commit
  `03fe7a0bdcb12bf6fbab693fa8e5fca418b160b3`, no reproducible in-repo runtime
  version artifact or fresh Claude/Codex native-session receipt, and no signed
  or clean-device pilot evidence).
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
