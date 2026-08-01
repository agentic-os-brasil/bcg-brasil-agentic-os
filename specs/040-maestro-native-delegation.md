# Spec 040 — native Maestro delegation and topology correction

Maestro is the only user-facing hub and the only authority that selects a
spoke. The runtime has one active spoke, depth one and zero agent children.
Every transition is mediated by Maestro; an agent-to-agent call is denied.

The closed planner input contains intent class, active scope kind/ID,
sensitivity, materiality, review trigger, health/governance intent, requested
capability, client/stakeholder/strategy/promotion implications, reversibility
and the available registered agents. Caller role strings are ignored for
authority.

The planner resolves two independent booleans:

- `pre_account_used` chooses account-assisted framing or direct Case.
- `walter_required` resolves materiality and review risk; uncertainty selects
  Walter. Low-materiality skips carry a Maestro reason code and evidence.

The invariants are `post_account_validation_required == pre_account_used` and
`walter_invocation == resolved_walter_required`.

| Path | Sequence |
| --- | --- |
| A | Maestro → Client Account framing → Maestro → Case → Maestro → Client Account validation → Maestro → Walter → Maestro → User |
| B | Maestro → Client Account framing → Maestro → Case → Maestro → Client Account validation → Maestro → User |
| C | Maestro → Case → Maestro → Walter → Maestro → User |
| D | Maestro → Case → Maestro → User |

Case output is always returned to Maestro. In paths A/B, Client Account
validation is required because framing occurred; in paths C/D, Client Account
does not participate. Walter is the internal Senior Advisor & Refiner: a calm,
constructive high-leverage advisory step, tool-free and never user-facing. An
`approved` verdict may include optional non-blocking polish. A
`refine-and-return` verdict requires a load-bearing issue, a proposed concrete
refinement and an acceptance condition; `hold` is exceptional and reserved for
material safety, governance or evidence blockers. Walter preserves a
defensible user thesis and does not manufacture objections or block cosmetics.

Any content or risk mutation clears approvals and re-plans materiality before
the next mediated Case attempt. Account cycles, Walter cycles and Case
attempts have deterministic budgets and append receipts; exhaustion fails
closed. Bindings include exact agent ID, role contract, scope, authorization
digest, capability digest, plan digest and state snapshot digest.

Claude and Codex adapter command paths point to the same durable installation
state store. Restart, replacement, replay and parallel branch tests prove
fencing and recovery. Adapter-observed receipts are not native evidence;
native-qualified status requires fresh runtime evidence.
