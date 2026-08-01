# Maestro agent-orchestration assurance

This matrix separates enforced runtime-neutral contracts from evidence that a
native Claude or Codex integration has actually invoked them. A local Go test
does not promote a product capability from `unavailable`.

| Implemented contract | Local evidence | Native evidence required | Current state |
| --- | --- | --- | --- |
| Agent identity is path-safe, unique and capability-bound | `agentcatalog`, `agentorchestration` and scaffold tests reject malformed, duplicate and forged identities | Installed adapter resolves private capabilities and sends authenticated native events | unavailable |
| Only registered targets and role edges may start work | Catalog validation and controller conformance reject unknown targets and disallowed edges | Native dispatch never bypasses the controller | unavailable |
| Scope and tool/resource grants are exact and default-deny | Controller tests reject cross-scope pointers, forged scope, unknown tool operations and unbound prefixes | Native tool events carry the same canonical resource and scope values | unavailable |
| One branch and one child are active at a time | Shared state and controller tests reject parallel branches and children | Adapter uses one durable shared state store across restarts/processes | unavailable |
| Delegation state can recover safely | Snapshot restore validates policy, root, child and recovery capability; stale recovery is capability-gated | Durable atomic persistence plus restart/partition conformance | unavailable |

| Packets and completion authority are bounded | Dispatcher and pilot tests verify signed packets, scope inheritance, target-authenticated execution envelopes, nonce replay rejection and finish authority | Adapter delivers only authenticated packets/envelopes without exposing capabilities | unavailable |
| High-leverage output cannot bypass Walter, while low-leverage work may skip it explicitly | Maestro independently resolves `account_consultation_required` (client/stakeholder strategic lens) and `walter_required` (high leverage). Account-assisted chains prove Account framing → Case → Account validation; direct Case chains prove an execution-only/no-client-lens reason. Required Walter packets are source/digest-bound; only a calm typed `approved` verdict with authenticated installation-scoped custody can satisfy the execution ledger. `refine-and-return`/`hold` never approve, and low-leverage skips carry a Maestro reason/evidence receipt | Native adapter emits the sealed packet, invokes the same Claude/Codex core and observes the typed verdict in the same governed session; qualification must cover custody and stale/replay rejection | unavailable |
| Direct skill selection stays with the active owner | Dispatcher tests require a signed active root packet, matching agent capability and no active child | Native agent execution proves the same root/capability binding | unavailable |
| Claude/Codex semantic parity | Shared controller fixtures execute both event vocabularies and denial cases | Native session conformance from installed Claude and Codex adapters | unavailable |

The pilot exposes the restart boundary programmatically as `Pilot.Recovery()`
with state `unavailable`; a `delegated` receipt is never a claim that its packet
can be completed after process restart.

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
- material recommendation, consequential trade-off or external artifact returned
  as completed without a signed trigger, pending-review state and Walter handoff;
- Account consultation omitted when client strategic-lens signals require it,
  or post-Case validation omitted after Account framing;
- direct Case routed through a fake Account edge, nested agent call or
  Capability Specialist alias;
- Walter packet opened before the producer closes, bound to a forged source
  digest/scope, or completed through the generic return path;
- more than three objections, a blocking cosmetic/nitpick objection, an empty
  refine/hold verdict, or a blocking verdict without a concrete fix and exit
  condition;
- forged, stale, replayed, cross-item, cross-attempt or unavailable Walter
  custody/receipt;
- review prose, audience or uncertainty leaked into the durable receipt.

## Explicitly unavailable

Neither `adapters/claude/` nor `adapters/codex/` installs native product event
wiring for agent orchestration or durable dispatch-state persistence. Their
README files describe the required mapping and evidence only. The canonical
capability manifest therefore keeps `agent_orchestration` unavailable for both
runtimes; no local fixture, CLI bridge or development hook changes that state.
