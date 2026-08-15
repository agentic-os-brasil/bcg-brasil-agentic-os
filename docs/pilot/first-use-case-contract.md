# First-use case contract — Q-011

**Status:** proposed — requires explicit product-owner approval before any
technical shadow or user pilot.

## Candidate

Use one bounded **case-agent decision brief** as the first value loop:

> Given a reviewed case brief, the Case Agent produces a balanced decision
> brief, a one-to-three-action plan and a resumable handoff. It may request one
> exact PA Expert advisory when the deterministic activation policy selects D1;
> it never browses, ingests or exports material without an approved contract.

This candidate is intentionally narrow. It exercises the local workspace,
execution ledger, bounded Case Agent, optional PA Expert route, Yoda review
when required and metadata-only canary receipts without requiring Docling,
dreaming, wiki navigation, Codex parity or autonomous side effects.

## Target persona and shadow cohort

The target audience is classic and technical consulting. The first technical
shadow therefore includes **one classic consultant and one technical
consultant**, each working on a case with a clear decision, an explicit
horizon and a small set of authorised materials. Both users must use a
non-client fixture or an approved low-sensitivity workspace.

## Acceptance metric

For each shadow user, at least 4 of 5 attempts must reach a reviewed decision
brief and resumable handoff in **10 minutes or less**, with:

- no cross-workspace read or unapproved external query;
- no client/work content in canary or federation receipts;
- a deterministic route receipt (`D0`, `D1` or `D2`) for every attempt;
- successful resume after one deliberate interruption; and
- explicit human confirmation before any high-stakes completion.

## Stop criteria

Stop the shadow immediately for any boundary breach, invented authority,
missing route/receipt, unbounded tool request, lost workspace state, sensitive
content in telemetry, or severity-1/2 incident. Do not expand the cohort while
an acceptance metric is unavailable or a stop criterion is unresolved.

## Required evidence before approval

1. Product owner confirms the use case, persona and metric above (or records a
   replacement decision in the decision log).
2. Claude-first native lifecycle evidence exists on the target platform(s),
   including SessionStart, context injection and pre-action guard.
3. The exact pilot release and runtime capability states are recorded; local
   adapter-command receipts do not substitute for native evidence.
4. A two-user technical shadow **plan** has a named operator, support path and
   rollback plan. The observed two-user evidence is collected only after
   approval and before promotion to the controlled-pilot tier.

Until these items exist, Q-011 remains **proposed**, the maturity tier remains
`contract-validated`, and no user-facing pilot claim may be made.
