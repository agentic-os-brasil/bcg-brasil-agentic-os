# Proposal 005 — Skill consolidation

**Status:** taxonomy and consolidation rules accepted as design guidance;
proposed product skills remain deferred until their authorities and evaluations
exist.

**Original contribution:** Marcelo Petrof Sanches.

**Architecture reconciliation:** BCG Brasil Agentic OS maintainers.

## Executive resolution

The useful principle is retained:

> An agent is the governed actor that receives authority and a bounded packet.
> A skill is a reusable method that helps an authorized actor perform work.

The original set of 19 draft skills is not promoted. Several entries duplicate
canonical methods, bypass the role graph or invent task, calendar, setup,
handoff and memory authorities. Files under `docs/proposals` are research
artifacts; they are not product skills and must not be advertised by the
catalog or runtime.

## Consolidation rules

1. **One authority.** A skill may compose a canonical capability but cannot
   duplicate its logic, persistence or policy.
2. **Governed caller.** Specialist orchestration follows
   `Maestro → owning agent → specialist`. A skill cannot create a new direct
   hub-to-specialist edge. Direct hub-to-reviewer, governance or errand dispatch
   is allowed only where the closed catalog and exact packet contract permit
   it; Walter still cannot pull tools.
3. **Method, not permission.** Loading a skill grants no filesystem, network,
   connector, scope or delegation authority.
4. **Runtime-neutral core.** Canonical instructions describe semantic inputs,
   outputs and constraints. Runtime adapters map those semantics to native
   tools and report unsupported operations honestly.
5. **Product contract.** A managed skill requires canonical location,
   `interaction-profile`, runtime metadata, generated index/catalog entries and
   evaluation fixtures. A standalone `SKILL.md` is insufficient.
6. **No speculative reliability.** Workflows that mutate multiple authorities
   require idempotency, receipts, partial-failure recovery and rollback before
   activation.

## Disposition matrix

| Proposed skill | Decision | Rationale / canonical direction |
| --- | --- | --- |
| `grill-me` | merge concept | Express as a sealed Walter review packet or a human interaction method; never a Walter tool |
| `wayfinder` | adopt later | Reusable reasoning method; evaluate as runtime-neutral content without authority |
| `investigate` | adopt later | Bounded diagnostic method for an authorized parent/specialist |
| `storyline` | adopt later | Reusable method that may support a future deck capability |
| `diagram` | defer | Needs a named deterministic renderer and artifact contract |
| `make-pdf` | defer | Needs a named deterministic renderer, distribution policy and artifact contract |
| `handoff` | reject as authority | Execution ledger/checkpoints own durable resumption; a presentation-only summary may compose them |
| `decision-log` | merge concept | Consolidate with the existing canonical decision-recording method; do not create a second writer |
| `record-learning` | defer | Requires the owner-private development authority absent today |
| `record-concept` | defer | Requires an accepted owner or practice canon and promotion boundary |
| `task` | defer | No authoritative task source or synchronization contract exists |
| `schedule` | defer | Calendar mutation, confirmation, receipt and rollback contracts are absent |
| `start-day` | defer | Cross-source orchestration depends on unavailable briefing/task authorities |
| `eod` | narrow later | May summarize one workspace through its owner; task reconciliation and cross-scope writes are unavailable |
| `newcase` | adopt later | May compose workspace initialization and managed ingestion after bounded-input evaluation |
| `meeting-to-work-items` | narrow later | May return proposed decisions/actions to one workspace; no task/calendar mutation |
| `consolidate` | reject as duplicate | Memory derivation and source hygiene must remain separate; use the existing canonical memory method where applicable |
| `retro` | defer | Requires an owner-private development contract and explicit source promotion |
| `setup` | reject as duplicate | Extend existing setup/update and workspace initialization flows instead of adding another authority |

`Adopt later` and `narrow later` are not runtime availability claims. They are
priorities for a future implementation PR with the full product-skill contract.

## Promotion checklist

A proposed skill can move into `bundles/base/skills/` only when one PR:

1. identifies the single existing authority it composes;
2. names allowed callers and their role/scope;
3. defines bounded inputs, outputs, denied data and interaction profile;
4. supplies runtime-neutral instructions and adapter metadata;
5. adds positive, negative and adversarial evaluations on supported runtimes;
6. updates generated `INDEX.md` and `catalog.json` surfaces through the
   repository compiler;
7. proves no duplicate source of truth remains; and
8. leaves unsupported integrations explicitly unavailable.

## What this proposal does not do

- It does not create or activate 19 skills.
- It does not modify Maestro, Walter or the accepted role graph.
- It does not choose task, calendar, email, chat or document connectors.
- It does not make `CLAUDE.md` a canonical runtime-neutral constitution.
- It does not treat a green documentation harness as product conformance.
