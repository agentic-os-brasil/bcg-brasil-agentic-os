# Spec 047 — Agent breadcrumbs and deterministic done contracts

Status: implemented in the runtime-neutral control plane, including bounded
durable Pilot recovery; native Claude and Codex event qualification remains a
separate availability gate.

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

## Durable Pilot recovery

When Maestro constructs `NewDurablePilot` with an explicit owner-local recovery root, the
runtime persists the bounded packet body, receipt, errand state and consumed
nonce set in owner-only files using an atomic temporary-write, fsync and rename
sequence. These files are a private recovery boundary: they are not public
receipts, prompt context, execution-ledger records or model memory, and they
never contain capability credentials. On restart, every record is parsed with
strict schema validation, packet signature/digest checks and nonce uniqueness;
any malformed or tampered record prevents recovery rather than being ignored.

The process-local constructor intentionally remains `unavailable`. A durable
Pilot reports `available` only after this authenticated load, and active returns
still pass the live expiry, scope, target and done-contract checks. Terminal
records may be inspected after their original TTL without extending the packet's
authority.

## Done contract

Every schema-v2 `WorkPacket` carries a signed `DoneContract`. The closed
policies are:

- `authenticated_return` for producing agents, with a bounded minimum and
  optional exact list of required evidence pointers;
- `typed_yoda_verdict` for Yoda, which can only close through the typed
  review envelope.

The target validates the contract before sealing a return. Maestro validates it
again before accepting the envelope, and the public receipt pins the contract
digest. Missing required evidence, an insufficient evidence count, a policy
mismatch or a generic Yoda return cannot become `completed`.

The contract is structural and scope-bound. It does not store the result body;
the body remains ephemeral and its digest is the only durable result binding.
Failure and unavailable states remain explicit and do not satisfy the done
contract.

## Agent obligations

Maestro, Client Account Agent, Case Agent, PA Expert, Yoda, Darwin and Gamma
Guardian all operate under the same rule: emit lifecycle/tool events through
the governed adapter, use only the bounded packet, and return only the typed
done contract plus metadata/evidence pointers. No agent may use transcript
memory as completion authority or bypass the control plane.

## Availability boundary

The contracts and local durable receipts are implemented and tested for both
runtime vocabularies. Native Claude/Codex adapters still require attended
runtime conformance evidence before the capability is called native-qualified.
