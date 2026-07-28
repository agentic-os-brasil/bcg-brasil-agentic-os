# Spec 017 - Case agent (workspace-first) initialization and context

Status: accepted architecture; concrete data-free agent stub, guided interview,
versioned brief, research-plan approval, sourced-evidence persistence, attested
public economic snapshots and managed setup skill implemented. Web execution
depends on an approved runtime tool; hard runtime isolation adapters remain
unavailable.

## Objective

Create a useful Case Agent without turning its workspace state into an untraceable
context blob. Creation combines a user interview, authorized external research
and a small attested public economic snapshot; all substantive context remains
workspace-scoped, versioned and attributable.

## Creation flow

`bcgos init` registers the workspace boundary first. It then offers a guided
Case Agent setup (the `workspace-agent` command remains a compatibility
surface) that collects only the information needed to establish
the mandate:

1. client/account and project label;
2. objective, deliverables, time horizon and decision to support;
3. user roles, stakeholders and information classification;
4. known facts, working hypotheses, constraints and available local material;
5. desired research questions and permitted external-disclosure level;
6. success signals, risks and preferred refresh cadence.

The user reviews the resulting mandate and explicitly confirms any external
research plan before the first query is sent. Setup remains resumable; a
workspace may exist without completed research.

## External research guardrail

External research is a disclosure boundary, not a background convenience. The
agent must:

- show the proposed query themes, allowed sources and disclosure risk before
  searching;
- use the least revealing terms that answer the approved research question;
- avoid unpublished project names, confidential facts, stakeholder names and
  client strategy unless the user explicitly authorizes those exact terms;
- prefer an approved source allowlist appropriate to the question, including
  primary sources for company, regulation and macroeconomic facts;
- record each external query, its approval, source URL, retrieval time,
  validity date, extracted claim, evidence strength and classification;
- label uncertainty, disagreement and freshness rather than presenting a
  research summary as established fact.

The initial dossier may include market/client context, facts, key trends,
upside and downside hypotheses, open questions and a public macroeconomic
snapshot. Bullish and bearish theses are structured hypotheses: each declares
its evidence, assumptions, counter-evidence and signals that would change it.
They are never represented as investment advice or as facts about a client.

## Three context layers

### 1. Agent state: compact operational control plane

The agent reads a small state at the start of work. It contains only:

- workspace identity, mandate version and active lifecycle state;
- current objective, active workstream and next decision or deliverable;
- material constraints, approvals and open risks;
- pointers to the current dossier, handoff and approved research plan;
- freshness/refresh signals and the last successful work summary.

It must not contain raw documents, transcripts, broad research summaries,
embeddings, unreviewed facts or accumulated discussion history.

### 2. Workspace dossier: versioned evidence and reasoning

The workspace dossier is the source for interview notes, research results,
claims, sources, bullish/bearish hypotheses, stakeholder context and decision
history. Every entry has provenance, retrieval/creation date, classification,
authoring agent or user and review/freshness state. It remains inside the
workspace authorization boundary.

### 3. Attested public economic rollup: reusable but isolated

The OS may maintain a separate economic rollup sourced from independently
collected public material. Each version records an explicit human attestation
that no workspace-derived material was used, plus public classification and
source provenance for every retained claim. This is a governance boundary, not
automated content detection. It must never receive client data,
workspace-derived queries, metadata, synthesis or automatic write-back.

A workspace agent can request a filtered snapshot and records the snapshot
version it used. The snapshot does not grant access to any other workspace.

## Refresh and promotion

Research is refreshed from explicit triggers: approaching a decision date,
user request, source expiry, material news or an identified evidence gap. The
agent proposes a new external research plan when the prior approval does not
cover the query.

Only reviewed, non-confidential facts can be promoted periodically to the
Client Account Agent layer under Spec 016. Neither interview data nor research
findings flow upward automatically.

## Acceptance criteria for implementation

1. Workspace setup can be completed, paused and resumed without losing the
   authorization boundary.
2. No external query executes without recorded user approval and an approved
   scope.
3. The startup state stays within the compact control-plane schema and points
   to, rather than duplicates, the dossier.
4. Every dossier claim can show source, date, classification and confidence.
5. Bullish/bearish hypotheses expose assumptions, counter-evidence and
   invalidation signals.
6. Public economic rollups can be versioned and injected into a workspace,
   but cannot read from or write to any workspace.
7. Claude and Codex adapters provide equivalent approval, provenance and
   workspace-isolation behavior or declare setup unsupported.

## Initial executable contract

`bcgos init` creates the compact Case Agent control plane and atomically
materializes its governed local stub from the managed `case_agent` template.
The current CLI retains the `workspace-agent` command as a compatibility
surface and supports:

```text
bcgos workspace-agent interview [workspace-path]
bcgos workspace-agent brief submit --stdin [workspace-path]
bcgos workspace-agent research plan --stdin [workspace-path]
bcgos workspace-agent research approve --plan <id> --approved-by <owner> --confirm [workspace-path]
bcgos workspace-agent research query --stdin [workspace-path]
bcgos workspace-agent research record --stdin [workspace-path]
bcgos workspace-agent economic import --stdin --attested-public --attested-by <owner> --confirm-no-workspace-derivation
bcgos workspace-agent economic attach --snapshot <id> [workspace-path]
```

Briefs, proposed and approved plans, and evidence are immutable artifacts. The
current brief and economic snapshot are replaceable pointers, so the compact
state does not absorb their bodies. Research evidence fails closed unless its
plan is approved and unexpired, its query consumed an immutable budget slot,
its exact query is part of the approved themes,
its URL belongs to the approved hostname allowlist and it is classified public.
Economic snapshots require an immutable independent-public-source attestation
and per-claim source provenance. They live outside workspace roots; a workspace
stores only their immutable ID.

The canonical `workspace-agent-setup` skill conducts the conversation and may
use a runtime web-search/browser tool only after approval and when an
enforceable workspace/pre-action guard is present. It reports unavailable
otherwise. The CLI does not embed a search provider or credential and does not
claim OS-level filesystem isolation.
