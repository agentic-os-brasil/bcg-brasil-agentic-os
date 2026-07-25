---
name: workspace-agent-setup
description: Create or resume one workspace agent through a reviewed interview, approved public research, sourced evidence and an optional public economic snapshot. Use whenever a new client project workspace is initialized or its briefing needs refresh.
---

# Workspace Agent Setup

Build useful project context while preserving the workspace as a confidentiality
boundary. The workspace agent owns this workflow and must never borrow context
from another workspace.

## Interaction profile

Resolve the canonical `interaction-profile` before starting. It controls how
much technical detail is shown during setup, but never changes approval,
classification, provenance or workspace-isolation requirements.

## Workflow

1. Resolve the active workspace with `bcgos status`. Stop if it is missing,
   ambiguous or different from the workspace shown to the user.
2. Run `bcgos workspace-agent interview <workspace-path>` and conduct the
   questions conversationally. Show the consolidated briefing to the user
   before persisting it.
3. Persist the reviewed JSON briefing with
   `bcgos workspace-agent brief submit --stdin <workspace-path>`. Keep bullish
   and bearish hypotheses distinct from facts; each thesis needs evidence,
   assumptions, counter-evidence and invalidation signals.
4. Propose a minimized public research plan. Use only hostname allowlist entries
   and never include confidential project names, stakeholder names, unpublished
   strategy or client-provided facts in query themes.
5. Persist the proposal with
   `bcgos workspace-agent research plan --stdin <workspace-path>`. Display the
   exact purpose, query themes and source allowlist. Continue only after the
   user explicitly approves it.
6. Record approval with
   `bcgos workspace-agent research approve --plan <id> --approved-by <owner> --confirm <workspace-path>`.
7. Execute external research only when the runtime exposes both an approved
   web-search/browser tool and an enforceable workspace/pre-action guard.
   Otherwise report the research capability as unavailable; never substitute
   an unapproved API, credential or provider, and never claim hard isolation.
8. Immediately before each external query, consume one immutable budget slot
   with `bcgos workspace-agent research query --stdin <workspace-path>`. Stop
   when the approved budget is exhausted.
9. For every retained claim, call
   `bcgos workspace-agent research record --stdin <workspace-path>` with the
   plan ID, exact approved query, HTTPS source URL, retrieval time, claim,
   evidence strength, validity date and public classification. Reject expired
   plans/evidence and queries or sources outside the approved plan.
10. A public macroeconomic snapshot may be imported with
   `bcgos workspace-agent economic import --stdin --attested-public
   --attested-by <owner> --confirm-no-workspace-derivation`, then attached by
   snapshot ID. Every claim must be public and reference a declared source.
   The human attestation is a governance boundary, not automated content
   detection; never use workspace queries, metadata or client-derived synthesis.
11. Return the workspace, briefing version, approved plan, sourced findings,
    economic snapshot version, freshness gaps and unavailable capabilities.

## Refresh

Refresh only when requested, when a decision date approaches, when a source is
stale or when a material event creates an evidence gap. A new query outside the
approved themes or domains requires a new plan and approval.

## Invariants

- The compact state contains pointers and operating signals, not research
  bodies or transcripts.
- Briefs, plans, approvals and evidence are immutable, workspace-scoped
  artifacts with provenance.
- No cross-workspace lookup or automatic promotion to account context occurs.
- Attested public economic snapshots are stored outside every workspace and can
  only be attached by immutable ID. Each claim points to declared public
  sources and records who attested that no workspace material was used.
- Runtime filesystem isolation remains fail-closed: if the runtime cannot
  enforce the declared workspace root, state that limitation explicitly and do
  not claim hard isolation.
