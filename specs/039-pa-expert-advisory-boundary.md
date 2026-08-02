# Spec 039 — PA Expert advisory boundary

## Status

Accepted for the runtime-neutral shadow contract. Native PA Expert activation
remains unavailable until a qualified Claude or Codex adapter proves the same
boundary.

## Ownership model

```text
Client Account Agent  -> stakeholder and relationship intelligence
Case Agent            -> case-local context and deliverables
Maestro               -> deterministic declassification boundary
PA Expert (FPA/IPA)   -> centrally maintained, scope-free advice
```

A PA Expert is not a client or case workspace, does not own execution, cannot
delegate and never receives inherited client context. `practice_agent` and
`subject_specialist` are rejected legacy identities, not executable migration
roles. Existing artifacts require explicit re-registration as a versioned
`pa_expert`.

## Canonical contract

`internal/activationpolicy/advisory.go` is the single advisory contract. There
is no parallel PA Expert boundary package.

The request contains only:

- an opaque request ID plus episode and route digests;
- exact published PA Expert identity, kind, version and canon digest;
- one allowlisted question code;
- closed public or internal fact codes;
- allowlisted output sections; and
- a complete declassification attestation.

Client, account, case, workspace, stakeholder and person identifiers are
forbidden. Raw excerpts, prompts, attachments, scoped pointers, paths and
confidential or restricted claims fail closed.

Equivalent fact and output-section order is canonicalized before the request
digest is computed. Duplicate fact codes fail closed. The receipt binds the
request digest, policy version, exporter and exact expert/version/canon tuple.

## Advisory response

The response is bounded to findings, assumptions, challenges and application
cautions. It must repeat the request digest and exact expert/version/canon
binding. Scoped pointers and empty or oversized values fail closed.

The local receipt remains
`shadow_assessed_not_export_authorized` with `may_export: false`. It may prove
that the closed shadow contract was evaluated, but cannot authorize native
dispatch, export content, change a route, promote client context or mutate
policy.

## Runtime boundary

The managed PA Expert registry may remain empty. Scaffolding creates only a
draft, unavailable identity and does not imply published knowledge. Native
activation requires separately qualified provenance, runtime parity and
explicit acceptance under a future contract version.
