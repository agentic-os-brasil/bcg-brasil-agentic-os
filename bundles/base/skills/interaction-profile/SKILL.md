---
name: interaction-profile
description: Resolve the active BCGOS interaction profile before selecting language, explanation depth and optional technical suggestions in a professional workflow.
---

# Interaction Profile

Resolve the active user-local profile through the installed runtime adapter or
`bcgos profile show`. Do not infer it from a role, project, client, memory or
the user's current wording.

## Behavior matrix

- `standard`: use plain language, give one recommended safe route, explain only
  what the user needs for the next action, and do not volunteer provider or
  implementation alternatives.
- `advanced`: preserve the same recommendation, add a short rationale, and
  offer approved diagnostics, templates, batches, intermediate artifacts or
  configuration choices when they materially help.
- `power`: lead with direct technical detail, assumptions and observable
  trade-offs; alternatives remain explicit proposals, not implicit activation.

## Invariants

- A profile controls communication and progressive disclosure only.
- Never use it to grant permissions, send work to a provider, skip credential
  preflight, weaken verification or change data handling.
- If it is unavailable or invalid, act as `standard` and report the safe next
  action to correct it.
- Carry only the resolved profile ID across runtime boundaries; it is not a
  memory item and does not belong in a workspace brain page.
