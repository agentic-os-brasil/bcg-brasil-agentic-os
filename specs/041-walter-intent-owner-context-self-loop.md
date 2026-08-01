# Spec 041 — Walter intent proxy and owner-context self loop

Status: accepted contract; private owner-context storage and native lifecycle
qualification remain local-only capabilities.

## Authority

`UserSelfSnapshot` is a versioned projection of the single owner-context
authority, not a second database. Precedence is:

```text
explicit current instruction > explicit correction > canonical snapshot
  > recent observation > Walter hypothesis
```

Corrections supersede prior claims and invalidate dependent snapshot/proposal
digests. Historical receipts remain immutable. Promotion is facet-specific:
explicitly confirmed communication style/preferences may be eligible for an
auditable owner-controlled update; professional role and decision rules remain
proposals; boundaries, psychological profile and intrinsic-motive claims always
require explicit owner confirmation. `ok`, silence or artifact acceptance is
not endorsement. Corroboration requires independent episodes.

The public repository contains only schemas, contracts and synthetic fixtures.
Owner content is local-only and must remain purpose-ACL protected with inspect,
export, revert and delete/crypto-erasure controls in a qualified owner store.

## Walter contract

Maestro builds a deterministic, digest-bound `SelfReviewPacket` containing the
literal prompt, selected route, draft/output digests, minimal context refs,
the owner-context snapshot version/digest, relevant observation metadata,
audience, consequence and reversibility. An optional Client Account receipt is
metadata-only. Account validates client/stakeholder framing; it does not
evaluate the owner's self or intent.

Walter separately evaluates the literal request and a typed
`intrinsic_intent_hypothesis`. The hypothesis includes evidence refs,
confidence, alternatives, materiality and a disconfirmation condition. It is
never a fact or a canonical self update. Low-confidence consequential
hypotheses may return `clarify` to Maestro; they must not invent intent,
redirect the task or become blockers by themselves.

Intent review receipts pin packet, prompt, draft and owner-context digests plus
the cycle. Walter is read-only, tool-less and does not write the self store.
If a Walter refinement changes stakeholder-relevant content, the applicable
Client Account validation is stale and the existing Maestro-mediated flow must
re-enter it.

## Observation and Darwin boundary

The self-learning evaluator runs after every interaction, but the provisional
append-only log persists only material, authenticated owner speech/action. It
stores minimized claim/provenance hashes, evidence type, scope, confidence,
sensitivity, expiry/recheck and lifecycle; never transcript, prompt, client,
document or generated-agent content. Global, workspace, account and case
scopes remain distinct; nothing rises to global scope without declassification
and owner action.

Darwin may deduplicate hashes, detect contradictions, independent-episode
corroboration gaps, confidence decay, expiry/recheck and snapshot/proposal
drift. Its receipts are metadata-only and its CAS-bound proposals can only say
`reevaluate_facet`. Darwin's canonical mutation count is contractually zero:
promotion, rejection, redaction, deletion and canonical self edits belong to
the owner-context policy/owner, never Darwin.
