---
name: evolve-memory
description: Develop or change memory persistence, L1/L2/L3 rollups, dreaming, lifetime promotion, provenance, retention or context injection in the BCG Brasil Agentic OS. Use for any behavioral or architectural change under internal/memory, bundles/base/memory, memory schemas or future bcgos memory commands and runtime adapters.
---

# Evolve Memory

Preserve the accepted memory contract while developing one observable behavior at a time.

## Workflow

1. Read `specs/006-memory-persistence.md`, decision `MEMO` in `docs/decisions/decision-log.md`, `specs/002-data-boundaries.md` and `specs/004-runtime-portability.md`.
2. Run `go run ./dev/harness validate` for a baseline.
3. Identify the affected invariant: storage boundary, layer promotion, dreaming safety, lifetime curation, provenance, injection or runtime parity.
4. If the change alters a durable contract, use `$record-decision` before implementation and update Spec 006.
5. Write the smallest contract test and confirm it fails for the expected reason.
6. Implement the smallest runtime-neutral change. Keep provider, scheduler and native hook mechanics behind adapters.
7. Re-run the targeted test, then `go run ./dev/harness validate`.
8. Before handoff, run `go run ./dev/harness validate --full` and report test-first evidence separately from implemented and still-pending behavior.

## Non-negotiable invariants

- Keep user memory, logs, credentials and client work outside managed core and release artifacts.
- Keep workspace memory isolated and updates non-destructive.
- Preserve L1 sources; generate L2 and L3 through deterministic selection, source fingerprints, staging, validation and atomic activation.
- Preserve the last known-good output when synthesis is empty, invalid or interrupted.
- Require provenance and drill-down pointers for generated layers.
- Keep lifetime promotion owned by the governed weekly deep cycle; require eligibility, provenance, version history and no in-place overwrite.
- Inject bounded context in the canonical order `lifetime -> L3 -> L2 -> L1`.
- Skip missing generated layers with a diagnostic; never silently inject unbounded raw history.
- Preserve equivalent observable contracts in Claude and Codex even when native triggers differ.

## Current boundary

The runtime-neutral core now implements capture, daily and weekly rollups, governed lifetime activation, version history and bounded context assembly. The CLI connects capture, status and context; dreaming reports unavailable until an adapter exists. Synthesis and eligibility adapters, scheduling, stale-lock recovery and executable dreaming remain pending. Do not describe those adapter capabilities as implemented until their contract and conformance tests pass.
