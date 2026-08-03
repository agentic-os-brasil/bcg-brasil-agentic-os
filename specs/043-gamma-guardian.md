# Spec 043 — Gamma Guardian longitudinal quality agent

Gamma Guardian is Maestro's longitudinal code-quality evaluator. It is a
direct Maestro spoke, outside the Case Agent hierarchy, and does not inherit
case context. Its identity and rubric persist across cases while each request
binds exactly one authorized workspace and source head.

## Contract

- Canonical ID: `gamma-guardian`.
- Canonical role: `quality_guardian`.
- Ownership scope: `quality_longitudinal`.
- Input contract: `bounded_quality_packet`.
- Runtime grant scope: one `workspace` and one immutable lower-case Git object
  ID (SHA-1 or SHA-256) per request. The source head is mandatory and included
  in the integrity-bound plan and dispatch binding digest; a branch,
  tag, missing or divergent revision fails closed. Gamma never receives a
  `case` scope or inherited Case context.
- Tool access: scoped inspection only; no merge, publish, release or routing
  authority.
- Delegation: Maestro → Gamma Guardian, depth one, one active spoke, no child
  agents, no recursive delegation and no parallel branches.

Gamma evaluates Clean Code, Architecture/System Design, Testing,
Security/Reliability and Documentation/SDD independently. Each dimension
returns a bounded score, signal, severity and evidence reference. The overall
signal is one of `GREEN`, `YELLOW`, `RED`, `UNAVAILABLE` or `BLOCKED`.

## Evidence and privacy

Gamma fails closed on invalid identity, scope, grants, source head, SDD
artifacts or unavailable runtime evidence. Receipts contain metadata-only
hashes, counts, statuses, scores, severities and evidence IDs. Prompts, client
payloads, credentials, secrets, full tool output and unnecessary paths never
enter the agent identity or receipts.

A local `GREEN` is contract evidence only. Native Claude/Codex or CI
qualification requires an independent runtime-owned observation and cannot be
inferred from configuration or unit tests.

## Acceptance

1. The managed catalog exposes `gamma-guardian` as a direct Maestro spoke.
2. The planner routes a closed `code_quality` intent to Gamma without a Case
   binding or Case context.
3. A missing Gamma registration or malformed quality packet fails closed.
4. The quality-loop state returns to Maestro after one Gamma result.
