---
name: dream-memory
description: Run or inspect professional memory consolidation through the BCG Brasil Agentic OS memory engine. Use for session or daily closure, weekly deep dreaming, memory status, lifetime promotion explanations, missed-cycle catch-up, or requests such as "consolide a memória", "fecha o dia", "fecha a semana" and "dreaming".
---

# Dream Memory

Use the installed runtime adapter for the canonical memory engine. Never write, summarize or promote memory files directly from the skill.

## Interaction profile

Resolve `interaction-profile` before presenting a human-facing result. The
memory operation, policy, budgets and safety behavior never vary by profile;
only the explanation and optional detail do.

- `standard`: state the result, what changed and one safe next action.
- `advanced`: add the relevant cycle rationale, diagnostics and drill-down
  pointers when useful.
- `power`: add explicit source fingerprints, commit/manifests, layer budgets
  and operational trade-offs on request or when they materially affect a
  decision.

## Choose the cycle

- Use **daily light** for session or day closure. It may capture sanitized signals and update L1 only.
- Use **weekly deep** for week closure or an overdue weekly cycle. It may update L2 and L3 and promote eligible lifetime memory.
- Use **status** when the user asks what is remembered, why a promotion occurred or whether a cycle was missed.

## Workflow

1. Resolve the active workspace identity and local memory root through the installed runtime adapter.
2. Confirm the adapter reports the memory capability as supported and the effective per-layer budgets as configured.
3. For capture, persist only signals already classified as sanitized by the adapter. Never pass raw credentials, client files or unrestricted prompt history.
4. Invoke exactly one canonical cycle through the adapter. Hooks, schedules and manual requests all call the same idempotent engine operation.
5. For weekly lifetime promotion, require a named eligibility policy. If it is missing, stop: lifetime activation must fail closed.
6. Return the cycle, period, source fingerprint, activated layers, lifetime eligibility reason and any skipped or missing layers.
7. If the adapter or command is unavailable, report the capability as unavailable. Do not emulate dreaming by editing local memory files.

## Invariants

- Daily dreaming cannot write L2, L3 or lifetime.
- Weekly deep dreaming stages all outputs and exposes them through one committed manifest.
- Empty, invalid or interrupted pre-commit synthesis changes nothing visible.
- Readers use only the newest fully valid commit, so partial L2/L3/lifetime state is never injected.
- Existing but wholly invalid commit history is reported as corrupt, never as empty memory.
- One workspace-wide activation lock prevents daily and cross-week cycles from racing over shared memory.
- Lifetime updates require provenance, eligibility, version history and no in-place overwrite.
- Context is assembled as `lifetime -> L3 -> L2 -> L1`, with independent budgets and drill-down pointers.
- Source captures remain append-only and workspace-isolated.

## Current delivery boundary

The managed bundle contains this canonical skill and the executable Go engine contract. Runtime adapters and the user-facing `bcgos memory` command are separate capabilities; until one is installed, this skill must report dreaming as unavailable rather than claim execution.
