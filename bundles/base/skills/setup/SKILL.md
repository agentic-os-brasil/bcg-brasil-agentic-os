---
name: setup
description: Guide a new Maestro user through a safe setup plan by identifying the desired outcome, prerequisites and existing managed setup routes without making changes implicitly. Use for “set me up”, “onboard me”, “configure Maestro” or “what do I need to start?”.
---

# Setup

Resolve the canonical `interaction-profile` before responding. It controls
detail only; it never grants authentication, integration or workspace authority.

## Orchestration contract

- Accept only the desired setup outcome and prerequisites supplied in the
  current request.
- Ask whether the user needs product setup, workspace initialization or both,
  then identify `maestro-setup-update` or `workspace-agent-setup` as the next
  separately invoked route.
- Return the stated prerequisites, safe next action, expected confirmation and
  unavailable dependencies; never inspect runtime state.
- Do not invoke either named skill, run `bcgos`, install software, authenticate,
  connect integrations, create a profile/workspace or claim setup progress.

## Completion

Return the guided plan. The separately invoked deterministic setup/update route
retains its own confirmation and rollback controls.
