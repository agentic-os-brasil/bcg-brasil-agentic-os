# Spec 030 - Product skills v1

Status: accepted for initial implementation.

## Objective

Turn the first useful methods from Proposal 005 into managed, runtime-neutral
skills without granting new authority or loading an unbounded operating manual
into Session Context.

## Wave 1

| Skill | Type | Input | Output | Authority |
| --- | --- | --- | --- | --- |
| `wayfinder` | atomic method | question and constraints | issue tree and first branch | none |
| `investigate` | atomic method | observed symptom and evidence pointers | ranked hypotheses and verification plan | none |
| `storyline` | atomic method | audience, objective and claims | governing thought and argument arc | none |
| `grill-me` | interactive method | user's draft or plan | one question at a time and closing synthesis | none |

Every Wave 1 skill is advisory. It must not read files, retrieve context,
delegate, mutate state, invoke a connector, create an artifact, write a task or
disclose content externally. A missing input produces a bounded clarification or
an explicitly unavailable result; it must not infer facts.

## Shared contract

- Canonical instructions live under `bundles/base/skills/<id>/SKILL.md` with
  the adjacent runtime metadata required by the managed skills index.
- Each skill resolves the canonical `interaction-profile` only to vary
  explanation depth; the profile never grants authority.
- Session Context exposes the catalog pointer only. A runtime reads one skill
  on demand after explicit user intent.
- Claude and Codex share the same semantic input/output and denial behavior.
  Native automatic loading remains unavailable until its adapter conformance
  exists.
- Each skill returns only its advisory result. It does not create a receipt,
  execution item or durable record because no state changes. A future mutating
  capability must use the execution ledger and metadata-only evidence contract.

## Composition

An atomic skill owns one reusable method. An orchestration skill may sequence
atomic skills and already-authorized capabilities, but it cannot invent a
caller, role, tool, persistence owner or connector. Before an orchestration
skill becomes managed it must name its complete input/output packet,
confirmation boundary, idempotency behavior, partial-failure recovery and
evaluation fixtures.

## Wave 2

`extract-work-items` is an atomic advisory method over user-supplied notes.
`meeting-to-work-items` is the first managed orchestrator: it composes that
method and a human correction step, then returns confirmed proposals only.
Neither skill may write a task, decision, atlas entry, memory item or external
message. A future writer receives only the confirmed subset and needs its own
authority, confirmation, idempotency, receipt and recovery contract.

## Wave 3

`start-day` and `eod` are advisory orchestration skills over current-request
inputs. They may compose Wave 1/2 methods but cannot read calendar, mail,
tasks, workspace history or memory, and cannot create a daily log. Their future
integration paths remain deferred under the external-source authority contract.

## Deferred work

`diagram` and `make-pdf` wait for deterministic artifact contracts. `task`,
`schedule`, `start-day` and `eod` wait for governed external or multi-source
authority; `meeting-to-work-items` remains advisory until its mutation path has
that authority. `handoff`, `decision-log`, `consolidate`
and `setup` must compose existing canonical capabilities rather than create new
writers. `newcase`, `record-learning`, `record-concept` and `retro` require
their workspace or owner-private contracts.

## Portfolio decomposition

| Proposed item | Product shape | Earliest safe implementation |
| --- | --- | --- |
| `diagram`, `make-pdf` | atomic deterministic artifact wrappers | after renderer, artifact and distribution contracts |
| `newcase` | orchestrator over workspace initialization and managed ingestion | after bounded workspace-agent input/output evaluation |
| `meeting-to-work-items` | orchestrator over an advisory extractor, decision proposal and task proposal | begin read-only; mutations wait for task authority |
| `start-day` | multi-source briefing orchestrator | after explicit calendar/mail/task source grants |
| `eod` | one-workspace closeout orchestrator | begin with advisory recap; external reconciliation waits |
| `retro` | owner-private reflective orchestrator | after owner-private development contract |
| `task`, `schedule` | mutating connector capabilities, not first-wave skills | after confirmation, idempotency, receipt and rollback contracts |
| `handoff` | presentation over the execution ledger | no independent state writer |
| `decision-log` | front-end to the canonical decision writer | no second decision format |
| `consolidate` | proposal/review layer over canonical memory hygiene | separate scans from approved writes |
| `setup` | composition of setup/update and workspace initialization | no competing bootstrap authority |
| `record-learning`, `record-concept` | owner-private writers | after private-scope and promotion contracts |

Every later wave starts as a bounded read-only or advisory slice when possible.
Only a demonstrated authority contract can turn it into a mutating capability.
