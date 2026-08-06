---
name: workspace-agent-setup
description: Migration alias for the retired workspace-agent setup name. Use only to move callers to the canonical case-agent-setup skill; it does not define a current agent type.
---

# Legacy migration alias: `workspace-agent-setup`

`case-agent-setup` is the canonical user-facing skill for setting up a project
Case Agent. This ID is retained only so existing installed projections can
migrate without deleting historical fixtures.

When this alias is invoked:

1. Tell the user to switch to `$case-agent-setup`.
2. Resolve the canonical `interaction-profile` before continuing, without
   changing approval, classification or scope.
3. Preserve the existing case and approval scope; do not create a workspace
   agent or a practice agent.
4. Continue only through the canonical skill and report any missing runtime
   adapter as `unavailable`.

The current `bcgos workspace-agent` CLI is also a compatibility surface for
the canonical `case_agent` role. It is not evidence that a `workspace_agent`
entity exists, and this alias must never be used to broaden context or
authority.
