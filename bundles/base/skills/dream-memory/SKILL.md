---
name: dream-memory
description: Run or inspect professional memory consolidation through the BCG Brasil Agentic OS memory engine. Use for session or daily closure, weekly deep dreaming, memory status, lifetime promotion explanations, missed-cycle catch-up, or requests such as "consolide a memória", "fecha o dia", "fecha a semana" and "dreaming".
---

# Dream Memory

Operate directly on the workspace memory tree under `data/memory/` (layers
`L1/`, `L2/`, `L3/`, `lifetime/`). All reads and writes go through the Read,
Write and Edit tools, following the invariants below.

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

1. Resolve the active workspace identity by reading `data/profile/identity.json` and the memory root at `data/memory/`.
2. Confirm the memory tree exists and read `data/memory/config.json` for the effective per-layer budgets. If either is missing, stop and report the safe next action.
3. For capture, persist only signals classified as sanitized in the source (Session Start, hook output, prior L1 entry). Never write raw credentials, client files or unrestricted prompt history into `data/memory/`.
4. Execute exactly one cycle per invocation. Hooks, schedules and manual requests all follow the same idempotent read, synthesize, stage, commit sequence.
5. For weekly lifetime promotion, require a named eligibility policy in `data/memory/policies/lifetime.json`. If it is missing, stop: lifetime activation must fail closed.
6. Return the cycle, period, source fingerprint, activated layers, lifetime eligibility reason and any skipped or missing layers.
7. If the required policy or budget files are missing, report the capability as unavailable rather than emulating dreaming with ad-hoc edits.

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

The managed bundle contains this canonical skill and the layer budget and policy contracts under `data/memory/`. If those files are absent in the current workspace, report dreaming as unavailable and point the user at the setup skill rather than claim execution.
