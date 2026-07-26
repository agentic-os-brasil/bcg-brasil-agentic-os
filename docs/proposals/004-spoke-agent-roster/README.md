# Proposal 004 — Domain Agents on the Core Role Graph

Status: draft (not yet submitted for review)
Author: Marcelo Petrof Sanches, with Maestro (Claude)
Date: 2026-07-26
Related: accepted specs `018` (maestro core agents), `016` (workspace-agent boundaries),
`027` (agent scaffolding); decision `ATLS`; Proposal 001 (`../001-context-and-brain-model/`);
Proposal 003 (people cross-project scope — a dependency of this proposal);
Anthropic's multi-agent research writeup and subagent-design guidance (see Sources).

## What this proposal does

Proposes **13 concrete domain agents** and maps each one onto the **accepted core role
graph** in Spec 018 — it is *not* a parallel topology. Spec 018 defines the skeleton
(the closed role set, the security/authorization model, delegation limits); this proposal
supplies the domain substance those roles need: what a client-keeper actually captures,
how a quant-analyst works, the review contract, and so on.

Where our earlier draft proposed 13 standalone agents, this version slots them into
018's roles: most become **`capability_specialist`** instances under a thin
**`workspace_agent`** or **`account_agent`**; the two reviewers merge into the managed
**`walter`**; the OS-evolution role is already **`darwin`**.

## Reconciliation with Spec 018

Spec 018's role graph (recap): `maestro` (hub, no tools) delegates to a small set of
roles; delegation is **sequential, one active branch, max depth 2, one active child per
agent**; only a `workspace_agent`/`account_agent` holds **persistent write** to its scope;
a `capability_specialist` receives a minimum work packet, has no persistent access, and
does not delegate.

Two consequences shaped this mapping:
- **The four "keepers" cannot be file-writers-as-specialists.** Persistent write belongs
  to the owning `workspace_agent`/`account_agent`. So a keeper stays a deep
  `capability_specialist` that **prepares the exact update and returns it; the owning
  agent commits the write.** The keeper keeps all its depth (integration logic, contracts,
  boundaries) — it just doesn't touch the filesystem.
- **Deep chains are illegal.** The hub sequences shallow delegations (hub → planner,
  close; hub → analyst, close; hub → walter), never one 5-deep chain.

## Concrete topology

```text
User
 └─ maestro · hub · no tools
     │
     ├─ workspace_agent · thin · per engagement (owns + commits workspace context)
     │    ├─ client-keeper    · capability_specialist · workspace scope
     │    ├─ work-logger      · capability_specialist · workspace scope
     │    ├─ case-onboarder   · capability_specialist · workspace scope
     │    ├─ quant-analyst    · capability_specialist · workspace scope
     │    ├─ quali-analyst    · capability_specialist · workspace scope
     │    └─ deck-builder     · capability_specialist · workspace scope
     │
     ├─ account_agent · thin · owner / cross-project (owns + commits owner context)
     │    ├─ career-keeper    · capability_specialist · account scope
     │    ├─ people-keeper    · capability_specialist · account scope   (see Proposal 003)
     │    ├─ briefing-analyst · capability_specialist · account scope
     │    ├─ work-planner     · capability_specialist · account scope
     │    └─ support-coach    · capability_specialist · account scope
     │
     ├─ walter · reviewer · leaf      (challenger + final-reviewer merged into it)
     └─ darwin · governance analyst · leaf   (already in the managed catalog)
```

`capability_specialist` is **one role**, not two — the "two boxes" in Spec 018's generic
diagram are the same role under two different parents/scopes. The named agents above
resolve exactly which parent and scope each instance has.

**One active child at a time:** the 6/5 edges are *allowed*, not simultaneous. The parent
dispatches one specialist, closes it, dispatches the next — sequential.

**Roles we do not populate** (they remain Spec 018 extension points, empty for now):
`practice_agent`, `subject_specialist`, `errand_helper`. The natural future occupant of
`practice_agent` is a **shared, governed `concepts/` canon** (reusable methods/playbooks) —
which is exactly the "personal vs. org-shared concepts" open question from Proposal 001.
While `concepts/` stays personal it lives under the account layer; if it graduates to an
org-shared methodology canon, it becomes a `practice_agent` (+ `subject_specialist`s).

## The two thin owner agents (new)

- **`workspace_agent`** — the owner, gatekeeper and committer for one engagement's
  context. Holds the `workspace_id` scope, prepares work packets for its specialists,
  commits their prepared updates, and enforces the boundary (out-of-scope access denied).
  Instantiated per workspace via scaffolding (Spec 027); little domain logic of its own.
- **`account_agent`** — the same pattern for the owner's cross-project layer (profile,
  development, learnings, concepts, internal people). Holds only curated, promoted
  context — never raw workspace data (Spec 016).

## The 13 domain agents

`brain_access` marks read/edit against the atlas. For a keeper it now means "prepares the
edit; the owning agent commits" rather than a direct write.

### Under `workspace_agent` (workspace scope)
- [`client-keeper`](agents/client-keeper.md) — client context: people, org, sensitivities, commercials. `reader + editor`
- [`work-logger`](agents/work-logger.md) — daily logs, backlog, project status, decisions, Current-truth facts. `reader + editor`
- [`case-onboarder`](agents/case-onboarder.md) — bootstraps a workspace from a proposal deck (pairs with `bcgos init`). `reader + editor`
- [`quant-analyst`](agents/quant-analyst.md) — quantitative analysis: models, data pulls, Excel/Python. `reader`
- [`quali-analyst`](agents/quali-analyst.md) — qualitative synthesis and hypothesis/issue-tree structuring. `reader`
- [`deck-builder`](agents/deck-builder.md) — storyline + slide exhibits. `reader`

### Under `account_agent` (account / cross-project scope)
- [`career-keeper`](agents/career-keeper.md) — development record: objectives, retros, feedback, career-spanning learnings, concepts. `reader + editor`
- [`people-keeper`](agents/people-keeper.md) — internal-colleague context (cross-project — see Proposal 003). `reader + editor`
- [`briefing-analyst`](agents/briefing-analyst.md) — digests the day from email, calendar, transcripts. `none`
- [`work-planner`](agents/work-planner.md) — planning at task / short / long horizon. `reader`
- [`support-coach`](agents/support-coach.md) — professional reflection and wellbeing support ("a professional is still human"). `reader + editor`

### Merged into `walter` (reviewer)
- **`challenger`** (pre-work: bias/anchoring + is-there-a-cheaper-way) and
  **`final-reviewer`** (pre-send: does it conform to the brief) become the **two review
  modes** of the managed `walter`, keeping our output contract (verdict + ≤3 load-bearing
  objections, each with a concrete fix). This is a merge, not a drop — our review content
  is the substance `walter` runs. Their standalone files
  ([`challenger`](agents/challenger.md), [`final-reviewer`](agents/final-reviewer.md))
  remain here as the source of that contribution.

## Anatomy of an agent definition

Aligns with Spec 027's managed template shape (`AGENT.md`), plus the frontmatter fields
from Anthropic's subagent guidance:

**Frontmatter:** `name` · `description` (the router — trigger scenarios + what to pass when
delegating) · `tools` (minimal) · `brain_access` · `role` + `scope` + `parent` (from the
018 catalog) · `color` (UI-only).

**Body:** identity line (with an explicit "you are NOT …" boundary) · inputs it receives in
its work packet · method/workflow · a fixed **output contract** ending in an "obstacles
encountered" item · rules (confidentiality, "never speak to the user directly", and — for
keepers — "prepare the edit, the owning agent commits").

## Design principles (from Anthropic's subagent guidance)

1. **Example-driven descriptions** that shape both *when* an agent runs and *what it's told*.
2. **Structured output** as a natural stopping point.
3. **Obstacle reporting** in every output contract.
4. **Minimal tool access** — an explicit, scoped tool list, never the full toolbox.

## Explicitly not decided by this proposal

- The Hub's (`maestro`) own constitution — governed by Spec 018 and a later follow-up.
- The `people/` relocation itself — proposed separately in **Proposal 003** so it gets a
  clean decision as a change to the accepted brain model; this proposal only assumes it.
- Skill↔agent wiring — Proposal 005.
- Exact MCP connector names; model selection (a runtime-adapter concern, kept out of the
  canonical definition per `PORT`); `color`.

## Sources

- [Engineering at Anthropic — How we built our multi-agent research system](https://www.anthropic.com/engineering/multi-agent-research-system)
- [Claude by Anthropic — When to use multi-agent systems (and when not to)](https://claude.com/blog/building-multi-agent-systems-when-and-how-to-use-them)
- Anthropic, "Introduction to Subagents" (Skilljar course).
