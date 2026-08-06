# Spec 047 — Agent breadcrumbs and deterministic done contracts

Status: implemented in the runtime-neutral control plane; native Claude and
Codex event qualification remains a separate availability gate.

## Objective

Keep the agent loop resumable without asking a model to remember prior tool
calls in its context window, and prevent an agent from declaring completion
using an untyped prose assertion.

## Breadcrumb contract

Every authenticated orchestration event (`branch_start`, `child_start`,
`tool_request`, `child_finish` and `branch_finish`) produces a metadata-only breadcrumb after
the event decision. The durable orchestration snapshot keeps a hash-linked
tail of at most 64 breadcrumbs and a monotonic sequence. The tail contains
agent/branch/dispatch IDs, event and tool labels, decision code, timestamp and
one-way resource digest; it never contains prompts, arguments, outputs,
errors, client prose, absolute paths or credentials.

Breadcrumbs are evidence for recovery and diagnosis, not model context. The
Session Context Packet and `work next` projection remain bounded and do not
inject the tail automatically. Execution-specific tool history remains in the
workspace execution ledger and is exposed only through explicit export.

The digest chain and sequence are validated on every durable-state read. A
malformed, oversized, non-contiguous or tampered tail fails closed.

## Done contract

Every schema-v2 `WorkPacket` carries a signed `DoneContract`. The closed
policies are:

- `authenticated_return` for producing agents, with a bounded minimum and
  optional exact list of required evidence pointers;
- `typed_walter_verdict` for Walter, which can only close through the typed
  review envelope.

The target validates the contract before sealing a return. Maestro validates it
again before accepting the envelope, and the public receipt pins the contract
digest. Missing required evidence, an insufficient evidence count, a policy
mismatch or a generic Walter return cannot become `completed`.

The contract is structural and scope-bound. It does not store the result body;
the body remains ephemeral and its digest is the only durable result binding.
Failure and unavailable states remain explicit and do not satisfy the done
contract.

## Agent obligations

Maestro, Client Account Agent, Case Agent, PA Expert, Walter, Darwin and Gamma
Guardian all operate under the same rule: emit lifecycle/tool events through
the governed adapter, use only the bounded packet, and return only the typed
done contract plus metadata/evidence pointers. No agent may use transcript
memory as completion authority or bypass the control plane.

## Availability boundary

The contracts and local durable receipts are implemented and tested for both
runtime vocabularies. Native Claude/Codex adapters still require attended
runtime conformance evidence before the capability is called native-qualified.
