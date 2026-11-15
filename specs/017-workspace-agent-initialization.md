# Spec 017 - Case agent (workspace-first) initialization and context

Status: accepted architecture; concrete data-free agent stub, guided interview,
versioned brief, research-plan approval, sourced-evidence persistence, attested
public economic snapshots and managed setup skill implemented. Web execution
depends on an approved runtime tool; hard runtime isolation adapters remain
unavailable.

## Objective

Create a useful Case Agent without turning its case workspace state into an untraceable
context blob. Local first-case creation needs a reviewed interview and an
explicitly saved brief; authorized external research and an attested public
economic snapshot are separate optional work. Substantive case context remains
workspace-scoped, revisioned and attributable.

## Creation flow

Existing setup registers the workspace boundary first and offers the canonical
`case-agent-setup` method. The MCP first-case profile below uses the six-question
local first-value interview and separate explicit Save. Research is optional,
never a prerequisite for that first brief. The retained `workspace-agent` CLI is
a compatibility surface for the same `case_agent` role; its older
research-oriented interview can collect the following mandate details:

1. client/account and project label;
2. objective, deliverables, time horizon and decision to support;
3. user roles, stakeholders and information classification;
4. known facts, working hypotheses, constraints and available local material;
5. desired research questions and permitted external-disclosure level;
6. success signals, risks and preferred refresh cadence.

The user reviews the resulting mandate and explicitly confirms any external
research plan before the first query is sent. Setup remains resumable; a
workspace may exist without completed research.

## Governed MCP first-case profile (#545)

This profile connects the existing workspace gatekeeper and local agent scaffold
to canonical authored-brain persistence and the existing execution ledger. It
is a first-case delivery, not a broader account narrative, generic agent-hiring
API or runtime dispatcher. Use the current registered workspace and admitted
owner home; model input cannot choose roots, roles, parent paths, signable bytes
or integrity-key material. The existing owner-local HMAC authenticates instance
integrity only, never platform/publisher/release authority.

Noncreating setup inspection precedes reviewed prepare. Client mode reviews the
exact bounded account owner/mandate and canonical account/case relation, then
selects the exact pending pair. Direct internal mode keeps `ScopeKind=workspace`
and an empty account relation, with no fictional internal account or pair
markers. New case scope IDs default to the opaque workspace ID; an existing
exact binding and its legacy IDs/destination are preserved. A second case in an
already-bound workspace is an immutable conflict for this unit.

Preview covers the full resolved tuple: personalization, managed template and
catalog identities, registration metadata, destinations, current setup/marker
revisions and immutable account relation. Native review occurs outside every
lock. Apply reacquires the canonical custody in one acyclic owner order and
recomputes that same tuple; any changed resolved field requires a new review.
Only timestamps/authenticators may be generated afterward. A missing local
integrity key may be created after approval only when no instances exist;
missing/invalid keys with existing registrations, aliases or tampering refuse
without re-signing or repair.

After the six minimum questions, show the complete brief and one-to-three-action
plan. Saving is a separate explicit request through the existing reviewed brain
owner, with complete frontmatter and read-back at the actual page revision.
Client defaults use `accounts/<account-id>/cases/<workspace-id>/<workspace-id>.md`;
internal defaults use `projects/<workspace-id>.md`, relative to the registered
workspace brain. The actual returned existing destination always wins. No
arbitrary deliverables path or legacy multi-write first-value helper is exposed.

Reviewed finish checks current setup and saved-page revisions. When work was
requested, the existing execution owner atomically creates or recovers one item
from the stable server-derived setup identity, rejecting changed accepted work
or artifacts even after a crash before the setup link is published. Finish
reconciles normal revision/attempt-fenced start/checkpoint/pause; title search or
an unrelated execution create is not recovery. With no requested work, finish
needs no checkpoint and creates none.

Client active selection commits at durable `.active` publication, followed by
conditional removal of only the exact reviewed `.pending`. The common case path
policy admits a valid first pending pair when active is absent; corrupt/aliased
active state never becomes fresh state. Existing unrelated valid active-case
reads remain admitted while conflicting setup transitions refuse. Return
`active_cleanup_pending` if active committed but cleanup remains; retry only
while selection, setup identity and pending snapshot still match. Never restore
an older active marker or delete a newer pending marker as compensation.
Internal progress instead uses the revisioned gatekeeper. Interrupted stages
report exact completed/pending effects; this is not a cross-owner transaction.

The current MCP schemas define accepted fields and bounds for
`case.setup.status`, `case.setup.prepare`, `case.setup.finish`. Missing operations
or native review remain unavailable/pending; neither a chat yes nor a model
consent field substitutes. Read/setup/brief/checkpoint evidence must be
reported separately from actual native-client and hosted-platform qualification.

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

A Case Agent can request a filtered snapshot and records the snapshot
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
The current CLI retains the `workspace-agent` command as the compatibility CLI
for canonical `case_agent`. The following is retained source/recovery reference,
not the MCP participant workflow or a fallback when native review is unavailable:

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

The canonical `case-agent-setup` skill conducts the conversation and may use a
runtime web-search/browser tool only after approval and when an
enforceable workspace/pre-action guard is present. It reports unavailable
otherwise. The CLI does not embed a search provider or credential and does not
claim OS-level filesystem isolation.

The retired `workspace-agent-setup` skill ID remains distributed only as an
explicit migration alias. It redirects callers to `case-agent-setup`, never
creates a `workspace_agent` or `practice_agent`, and must not appear in newly
persisted role identities.
