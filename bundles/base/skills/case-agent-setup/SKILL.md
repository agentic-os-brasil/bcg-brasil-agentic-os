---
name: case-agent-setup
description: Create or resume one Case Agent through a reviewed interview, approved public research, sourced evidence and an optional public economic snapshot. Use whenever a new client project case is initialized or its briefing needs refresh.
---

# Case Agent Setup

Build useful project context while preserving the case workspace as a
confidentiality boundary. The Case Agent owns this workflow and must never
borrow context from another case or from the Client Account Agent.

## Command surface

`$case-agent-setup` is the canonical user-facing skill. The current CLI still
exposes the legacy `bcgos workspace-agent` command as a compatibility surface
for this same `case_agent` role. Do not present `workspace-agent` as a current
agent type, and do not claim a `bcgos case-agent` executable until a runtime
adapter implements it. If the required adapter or guard is absent, report the
capability as `unavailable`.

## Interaction profile

Resolve the canonical `interaction-profile` before starting. It controls how
much technical detail is shown during setup, but never changes approval,
classification, provenance or case isolation requirements.

## First useful result

1. Resolve the active case workspace with `bcgos status`. Stop if it is missing,
   ambiguous or different from the workspace shown to the user.
2. Start `bcgos workspace-agent value start <workspace-path>` before the first
   question. Keep its run ID inside the workflow; do not expose it to a
   non-technical user. This invocation is the CLI compatibility surface, not a
   second agent identity.
3. Run `bcgos workspace-agent interview <workspace-path>` conversationally.
   The compatibility command operates the canonical `case_agent` role.
   Ask the six minimum questions: decision/horizon; audience/constraints;
   useful result; authorized material; balanced hypotheses; and next step.
   For authorized material, explicitly ask the owner to point to the minimum
   local folders and SharePoint project roots needed for this case. Keep those
   roots as reviewed source pointers in the brief; do not enumerate, read,
   copy or ingest them during setup.
   Client Account context and PA Expert advice are separate mediated routes,
   never implicit setup inputs.
   Show the consolidated brief and one-to-three-action plan before writing.
4. Persist the reviewed result with `bcgos workspace-agent value submit --run
   <id> --stdin <workspace-path>`. It creates the classified decision brief,
   compact handoff and local first-value receipt.
5. On a correction, record only its kind with `bcgos workspace-agent value
   intervention --run <id> --kind <brief_correction|plan_correction|artifact_revision>
   <workspace-path>`.
6. Later, use `bcgos workspace-agent value status <workspace-path>` to resume
   from the handoff. Do not repeat the interview or inject its transcript.

The first-value flow does not browse, ingest documents, query a wiki, dream
memory, refresh economics or create an agent. A SharePoint root named in the
brief is not a connection or collection grant. External research and source
processing remain separate approved flows below.

## Optional public research
1. Propose a minimized public research plan. Use only hostname allowlist entries
   and never include confidential project names, stakeholder names, unpublished
   strategy or client-provided facts in query themes.
2. Persist the proposal with
   `bcgos workspace-agent research plan --stdin <workspace-path>`. Display the
   exact purpose, query themes and source allowlist. Continue only after the
   user explicitly approves it.
3. Record approval with
   `bcgos workspace-agent research approve --plan <id> --approved-by <owner> --confirm <workspace-path>`.
4. Execute external research only when the runtime exposes both an approved
   web-search/browser tool and an enforceable workspace/pre-action guard.
   Otherwise report the research capability as unavailable; never substitute
   an unapproved API, credential or provider, and never claim hard isolation.
5. Immediately before each external query, consume one immutable budget slot
   with `bcgos workspace-agent research query --stdin <workspace-path>`. Stop
   when the approved budget is exhausted.
6. For every retained claim, call
   `bcgos workspace-agent research record --stdin <workspace-path>` with the
   plan ID, exact approved query, HTTPS source URL, retrieval time, claim,
   evidence strength, validity date and public classification. Reject expired
   plans/evidence and queries or sources outside the approved plan.
7. A public macroeconomic snapshot may be imported with
   `bcgos workspace-agent economic import --stdin --attested-public
   --attested-by <owner> --confirm-no-workspace-derivation`, then attached by
   snapshot ID. Every claim must be public and reference a declared source.
   The human attestation is a governance boundary, not automated content
   detection; never use workspace queries, metadata or client-derived synthesis.
8. Return the workspace, briefing version, approved plan, sourced findings,
    economic snapshot version, freshness gaps and unavailable capabilities.

## Refresh

Refresh only when requested, when a decision date approaches, when a source is
stale or when a material event creates an evidence gap. A new query outside the
approved themes or domains requires a new plan and approval.

## Invariants

- The compact state contains pointers and operating signals, not research
  bodies or transcripts.
- Briefs, plans, approvals and evidence are immutable, case-scoped
  artifacts with provenance.
- No cross-case lookup or automatic promotion to Client Account context occurs.
- A project SharePoint pointer stays case-scoped. Cross-project prior-work
  retrieval uses the separate explicitly enrolled index and is never inferred
  from this brief.
- Attested public economic snapshots are stored outside every workspace and can
  only be attached by immutable ID. Each claim points to declared public
  sources and records who attested that no workspace material was used.
- Runtime filesystem isolation remains fail-closed: if the runtime cannot
  enforce the declared workspace root, state that limitation explicitly and do
  not claim hard isolation.
