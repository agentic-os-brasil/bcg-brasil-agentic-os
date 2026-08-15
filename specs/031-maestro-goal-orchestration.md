# Spec 031 - Maestro goal orchestration

Status: accepted contract; authenticated review gate implemented in the local
execution core. Native Yoda/Claude/Codex signing adapters remain unavailable.

## Objective

Allow one Maestro goal to run across sessions and agents while keeping the
workspace-scoped execution ledger as the only durable completion authority:

```text
create -> start -> checkpoint/pause -> resume -> evidence
       -> authenticated Yoda review -> complete
```

## Authority

A Maestro goal is a governed view over one `local_execution` item. It does not
have a second state machine, lock, event head, evidence store or completion
method. Business priority, owner, due date and external task status remain
outside the execution contract.

The immutable contract may require Yoda review and bind one Ed25519 public
key. A contract without the review gate cannot carry that key. Changing the key
or gate after creation changes the contract digest and invalidates the item.

## Authenticated review

The external reviewer signs a closed envelope containing only:

- item, workspace and active attempt IDs;
- the exact pre-review state revision and immutable contract digest;
- `approved` or `rejected`;
- one opaque nonce and issuance timestamp.

The core verifies the signature against the public key frozen in the contract.
It then commits a metadata-only review receipt as the next immutable execution
revision. The receipt contains hashes of the signer and envelope, not the
signature, rationale, objective, checkpoint or professional content.

An approval is current only when its recorded revision equals the ledger
revision presented for completion. Any checkpoint, evidence, tool breadcrumb
or other mutation after review invalidates it and requires a new signed review.
A rejected review never satisfies completion. Evidence remains mandatory and
is re-witnessed immediately before completion.

The ledger's binary `approved`/`rejected` decision is not the conversational
Yoda verdict. Maestro may receive `refine-and-return` or
`missing-the-mark` first; those outcomes return the packet to the producing
owner and cannot be translated into completion. A qualified adapter may issue
the signed `approved` envelope only after Yoda independently reviewed the
sealed packet and the final evidence satisfies the contract.

The runtime-neutral pilot enforces the handoff before any material result is
presented as complete. Closed triggers are `material_recommendation`,
`consequential_tradeoff` and `external_artifact`; the trigger is signed into
the producer packet and a successful producer return becomes
`pending_review`, never `completed`. `RequireYodaReview` accepts only that
matching pending trigger, binds the source packet digest and scope, and opens
only the registered Yoda reviewer. `ReturnYodaReview` accepts only the
typed verdict envelope, caps objections at three, and promotes the producer to
`completed` only for `approved`; `refine-and-return`, `missing-the-mark` and
`unavailable` leave the producer pending and project compact review state.

## Durability and replay

Review commits use the existing revision-first publication and regenerable
state projection. A crash after immutable publication cannot erase approval.
Revision compare-and-swap prevents concurrent or replayed envelopes from
winning twice; the envelope digest is also rejected if already recorded.
Attempt fencing rejects reviews for superseded sessions or agents.

## Privacy and availability

Transition history remains metadata-only and does not identify the review
decision. Full review receipts are available only through explicit execution
export. No review rationale or arbitrary field enters the ledger.

The runtime-neutral core and signing payload are implemented. Native Yoda
key custody, signing and Claude/Codex orchestration adapters are unavailable,
so product status must not advertise autonomous long-running execution.

## Acceptance criteria

1. A forged, cross-item, cross-attempt, stale or future-dated envelope fails.
2. Rejection and missing review fail completion without changing state.
3. Any post-review mutation makes the prior approval stale.
4. Core-witnessed evidence remains required and is revalidated.
5. Projection crashes recover the immutable review revision.
6. Published JSON schemas reject arbitrary contract and receipt fields.
7. Native capability remains unavailable until key custody and adapter
   conformance are proven on Windows and macOS.
