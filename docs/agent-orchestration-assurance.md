# Maestro agent-orchestration assurance

This matrix separates enforced runtime-neutral contracts from evidence that a
native Claude or Codex integration has actually invoked them. A local Go test
does not promote a product capability from `unavailable`.

| Implemented contract | Local evidence | Native evidence required | Current state |
| --- | --- | --- | --- |
| Agent identity is path-safe, unique and capability-bound | `agentcatalog`, `agentorchestration` and scaffold tests reject malformed, duplicate and forged identities | Installed adapter resolves private capabilities and sends authenticated native events | unavailable |
| Only registered targets and role edges may start work | Catalog validation and controller conformance reject unknown targets and disallowed edges | Native dispatch never bypasses the controller | unavailable |
| Scope and tool/resource grants are exact and default-deny | Controller tests reject cross-scope pointers, forged scope, unknown tool operations and unbound prefixes | Native tool events carry the same canonical resource and scope values | unavailable |
| One direct spoke is active at a time; depth is one and children are forbidden | Shared state and controller tests reject parallel branches, child packets and nested delegation | Adapter uses one durable shared state store across restarts/processes | unavailable |
| Delegation state can recover safely | `Pilot` can persist owner-local packet state and nonce replay state atomically; restart tests authenticate and complete a prior return, while tamper and unavailable paths fail closed | Native adapter supplies the same recovery root and proves restart/partition conformance | runtime-neutral available; native unavailable |

| Packets and completion authority are bounded | Dispatcher and pilot tests verify signed packets, scope inheritance, target-authenticated execution envelopes, nonce replay rejection and finish authority | Adapter delivers only authenticated packets/envelopes without exposing capabilities | unavailable |
| High-leverage output receives Walter's calm advisory review | The typed Maestro plan resolves `walter_required` independently from Account consultation; low-leverage skips carry a reason and evidence, while Walter verdicts require concrete refinements for load-bearing gaps and reserve `hold` for exceptional blockers | Native adapter emits the sealed packet and observes Walter's typed verdict in the same governed session | unavailable |
| Direct skill selection stays with the active owner | Dispatcher tests require a signed active root packet, matching agent capability and no active child | Native agent execution proves the same root/capability binding | unavailable |
| Claude/Codex semantic parity | Shared controller fixtures execute both event vocabularies and denial cases | Native session conformance from installed Claude and Codex adapters | unavailable |

The pilot exposes the restart boundary programmatically as `Pilot.Recovery()`.
The process-local constructor remains `unavailable`; the durable constructor
requires an explicit owner-local recovery root and returns `available` only
after every stored packet and nonce state authenticates. A `delegated` receipt
without that durable boundary is never a claim that its packet can be completed
after process restart.

## Durable breadcrumbs and done authority

The shared orchestration state keeps a bounded, hash-linked tail of
metadata-only breadcrumbs for every authenticated branch/tool event. It is
outside model context and contains no prompt, argument, output, error, client
body, absolute path or credential. The tail is capped at 64 entries and is a
recovery/diagnostic projection; detailed execution tool history remains in the
explicit execution ledger export.

Every schema-v2 work packet carries a signed `DoneContract`. Producer agents
use the closed `authenticated_return` policy with exact required evidence
pointers and a minimum evidence count; Walter uses the closed
`typed_walter_verdict` policy. The target and Maestro both validate the
contract, and public receipts pin its digest. A prose return, missing evidence
or generic Walter return cannot promote a dispatch to `completed`.

This is deterministic runtime-neutral enforcement, including an explicit
private recovery store for packet bodies and replay state. It is not proof of
native hook delivery. Native Claude/Codex qualification remains unavailable
until an attended adapter conformance run observes the same events and
receipts.

## Deterministic adapter bridge

`bcgos maestro dispatch --stdin` accepts an optional bounded `agent_events`
array. Each event contains only an agent identity, a closed decision and (when
the agent returns work) a content digest. Maestro applies the events through
the same `ChainState.Advance` transition authority used by the planner tests,
persisting only the metadata-only chain and typed receipts. An account-assisted
Case therefore has one enforced order: Client Account framing → Case → Client
Account validation → Walter (when required). A direct low-leverage Case can
finish without either consultation, while a material Case cannot skip Walter.

An absent event array remains `dispatch_boundary_model_unavailable`; the CLI
never fabricates an agent invocation. The bridge is deterministic contract
evidence and a handoff seam for Claude/Codex adapters. It does not by itself
promote `agent_orchestration` to `native-qualified`; that still requires a
fresh installed-runtime session proving that the native adapter emitted and
authenticated the corresponding events.

## Abuse cases covered locally

- forged or unknown capability;
- malformed or unregistered agent identity and target;
- role-edge, depth, scope and resource-prefix escalation;
- Maestro attempting direct tool use;
- parallel branch, parallel child and replayed packet completion;
- child completion by another identity or after replacement;
- tampered, expired, cross-scope or oversized work packet;
- altered result/failure envelope, nonce replay and cross-runtime completion;
- direct-skill selection without an active root, with a forged capability, by a
  leaf specialist or while a child is active;
- schema-v1 child packet used for a new skill selection;
- schema-v1 root or child packet used as the parent of a new delegation.
- high-leverage recommendation, consequential trade-off or external artifact
  returned without the resolved Walter handoff, or a low-leverage skip without
  its reason and evidence;
- Walter packet opened before the producer closes, bound to a forged source
  digest/scope, or completed through the generic return path;
- more than three objections, an approved verdict with objections, or a
  missing-the-mark verdict without a concrete fix and exit condition;
- review prose, audience or uncertainty leaked into the durable receipt.

## Explicitly unavailable

Neither `adapters/claude/` nor `adapters/codex/` yet wires the runtime-neutral
Pilot recovery root into native product lifecycle events. Their README files
describe the required mapping and evidence only. The canonical capability
manifest therefore keeps `agent_orchestration` unavailable for both runtimes;
no local fixture, CLI bridge or development hook changes that state.
