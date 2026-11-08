---
name: interaction-profile
description: Resolve the active Maestro interaction profile before selecting language, explanation depth and optional technical suggestions in a professional workflow. Called by other skills as a preflight step; also for "set my interaction profile", "switch to advanced mode".
---

# Interaction Profile

Before owner-state file operations, follow [admitted owner-state paths](resources/owner-paths.md). Owner `brain/` paths are logical; confirmed case/workspace-local paths retain their scope.


Resolve the admitted owner brain first, then read `owner/style.json` under its `brain_root` (preferred key: `interaction_profile`). If that admitted file or key is absent, fall back to `owner/identity.json` under the same root. An unavailable binding uses the standard profile without probing a product-relative directory. Do not infer the profile from a role, project, client, memory or the user's current wording.

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
