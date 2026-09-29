# Daily continuity implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist useful local daily checkpoints independently of dreaming and inject D0, D-1 and D-2 identically on macOS, Windows PowerShell, Claude and Codex.

**Architecture:** The managed Go ZIP helper owns validation, persistence and fresh bounded selection. Native wrappers are thin callers. Agents prepare explicit scoped summaries; hooks never copy transcripts or synthesize meaning from telemetry.

**Tech Stack:** Go, macOS Bash 3.2, Windows PowerShell 5.1, Python acceptance tests only.

**Spec:** `specs/006-memory-persistence.md`, ZIP daily continuity addendum; decision DLOG.

## Global constraints

- Daily and dreaming are independent; daily is not capture-v2 or proof of dreaming.
- Select exactly local-calendar D0, D-1 and D-2; no backfill from earlier days, no future or generated index pages.
- Preserve existing daily bytes; append only validated scoped checkpoints with idempotency. Do not copy transcripts, tool payloads, credentials or case content to owner memory.
- No Python, Git Bash, network, elevated process or scheduler dependency for daily capture/injection.
- Case content remains under the active case; never silently downgrade invalid case scope to owner.
- Failures retain pending work and provide visible bounded diagnostics; hooks do not run model workers synchronously.
- Native Windows qualification and release/merge gates remain unchanged.

### Task 1: Shared daily contract and adapters

**Files:**
- Create `internal/zipruntime/daily.go`, `internal/zipruntime/daily_test.go`.
- Modify `internal/zipruntime/hooks.go` for `daily-stop --root ROOT`, `daily-context --root ROOT`, Codex SessionStart/Stop.
- Modify `installers/zip/user-template/.claude/hooks/session-start-memory-inject.sh`, `session-stop-dream.sh`, `lib/Maestro.Lifecycle.ps1` to call the common helper, retiring duplicate recent-memory selection and stale daily-body injection.
- Modify `bundles/base/skills/maestro-operator/SKILL.md`, `dream-memory/SKILL.md`, user-template `AGENTS.md` and `CLAUDE.md` for capture recipe and retry semantics.
- Create `acceptance/test_daily_continuity.py` for actual packaged adapter parity.
- Update `docs/releases/0.2.0-daily-continuity.md` with evidence and exact limitations.

**Interfaces:**
- Input queue: scope-local `.maestro/daily-pending/<id>.json` (owner under `brain/`, case under active case). JSON schema v1 includes stable id, local date, summary, decisions, next_actions; explicit agent-authored provenance, not verified semantic sanitization.
- `daily-stop --root ROOT`: bounded queue drain (at most 32 records), validate exact IDs/dates/field lengths/UTF-8/EOF, persist daily Markdown and idempotency marker, retain failed inputs; stdout JSON counters, never content. No checkpoint means `missing_checkpoint`/empty rather than fabricated work.
- `daily-context --root ROOT`: bounded fresh Markdown packet for owner and current authorized case daily pages and L1 recent pages, each date represented or explicitly missing/unavailable. Read-only. Sources are contextual evidence, not instructions.
- `daily-dream-ack --root ROOT`: scoped digest compare-and-ack under the writer lock; an agent assertion after successful synthesis, never proof of execution. Required to prevent completion of request A from clearing later request B.
- Codex and Claude Stop call the same drain; SessionStart recovers a pending drain then injects the same core packet. UserPromptSubmit does not repeat bodies.

- [ ] Step 1: Add failing public-command tests, e.g. `Run([]string{"daily-context", "--root", root}, strings.NewReader(""), &out)` must select seeded D0/D-1/D-2 markers and exclude D-3/future/index. Run `go test ./internal/zipruntime -run Daily -count=1`; capture the initial unsupported-command failure.
- [ ] Step 2: Add writer tests for replay, same-ID/different-body rejection, midnight date attribution, case isolation, empty/malformed/oversize queue, symlink escape, concurrent drain, interrupted append/retry and existing page preservation. Use temporary synthetic trees, never owner data.
- [ ] Step 3: Implement the core behind these public commands. Lock one local scope writer without waiting, preserve existing page bytes, ensure crash recovery cannot duplicate an appended ID or acknowledge uncommitted content. Use bounded reads, UTF-8-safe truncation and explicit omission pointers; do not invent HMAC attestation.
- [ ] Step 4: Wire wrappers and Codex adapters. Remove conflicting latest-only L1/day-cache body output. Make dream request content-sensitive to newly saved daily checkpoints, retain requests on failure and acknowledge only successful matching work; do not create an unconditional dreaming loop after every no-op Stop.
- [ ] Step 5: Add explicit pre-final checkpoint recipe to operator and bootstrap, including scope resolution, queue location/schema, invocation on both shells and verification of persisted content-free receipt. Preserve eod confirmation requirement; automatic logging is not human day closure. Correct `data/memory/recent` to `brain/memory/recent` and failed-marker deletion.
- [ ] Step 6: Run `go test -race ./internal/zipruntime`, fast/full harness and packaged parity test after builds. Compare actual Mac Bash, PowerShell-on-Mac and Codex helper output; label PowerShell-on-Mac as adapter evidence, never native Windows proof.
- [ ] Step 7: Report changed files, RED/GREEN commands, exact failures, test results and remaining qualification gaps for Gamma and Walter review before commit/push.

## Baseline evidence

- HEAD `56d764ca538f5a1fff003c21a3aaccdec0abf2b7`, clean writer checkout; fast harness PASS.
- Actual packaged tests found empty daily output after Stop, latest-only L1, stale Bash daily cache, no PS daily output, no Codex memory body.
- Read-only skill application probe found no independent checkpoint recipe; failed dreaming deletes its marker; date-file presence incorrectly substitutes for freshness.

## Completion receipt — 2026-09-29

The steps above describe the original task checklist. All seven implementation
steps are complete: command-boundary RED, writer/edge-case tests, shared core,
adapters, capture/dream recipes, full/race/packaged validation, and review.
Gamma approved the final implementation and independently reran exact-ZIP daily
parity. Walter approved commit/push of the 21 scoped files and PR evidence update.
See `docs/releases/0.2.0-daily-continuity.md` for final test counts, artifact hashes,
provenance and limitations. Native Windows qualification remains pending; merge
and distribution remain HOLD. No real user installation was changed.
