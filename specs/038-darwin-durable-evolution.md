# Spec 038 — Durable Darwin evolution and recovery

## Status

Accepted for a local metadata-only slice. Native persistence and native
runtime provenance remain unavailable.

## Objective

Persist Darwin's structural improvement loop across process and session
boundaries without creating a second execution ledger or placing contracts in
model context:

```text
open episode -> append evidence window -> write proposal -> interrupt
            -> restart -> recover -> replay safely -> accept/reject externally
```

## Authority and boundaries

- Every episode, evidence window and proposal pins an opaque `policy_id`,
  `policy_version`, route-plan digest and an approved PA Expert portfolio
  snapshot with exact expert/version/canon digests.
- The portfolio snapshot is an immutable copy of the approved registry view;
  later registry changes do not rewrite historical episodes.
- Darwin can create proposal artifacts and observe metadata-only evidence. It
  cannot self-approve, self-evaluate, mutate live routing, change the agent or
  PA Expert registry, edit canon or change policy.
- Acceptance and rejection are append-only receipts from an independent
  authority. They do not apply a proposal.
- Health and headless-housekeeping receipts remain owned by the existing Darwin
  health store. Evolution files use a separate namespace and schema.
- Repairs continue through the signed `maestro-system` scope from the Darwin
  foundation and are reversible only. Evolution persistence has no repair or
  tool invocation path.

## Durable layout and replay

The local store uses private, append-only files below an `evolution/` directory:

```text
<root>/evolution/
  episodes/<episode-id>/episode.json
  episodes/<episode-id>/events/<revision>.json
  windows/<window-id>/v<version>.json
  proposals/<proposal-id>.json
  decisions/<receipt-id>.json
```

Writes are no-clobber. Replaying the same ID and digest is idempotent; a
different payload for an existing identity fails closed. Recovery reads only
complete validated JSON, ignores temporary projections, requires contiguous
episode revisions and never treats a regenerable projection as authority.

The local capability is explicit. A native persistence adapter returns
`unavailable/native_persistence_not_qualified` until a separately qualified
runtime proves it. No network, federation or release surface is introduced.

## Evidence contract

Evidence windows contain only closed route, outcome, duration and budget/receipt
flags. They contain no objective, prompt, response, file path, client name,
stakeholder, error text or raw tool payload. Depth thresholds remain
experimental metadata outside this contract; V1 chooses no final defaults.

## Acceptance criteria

1. A two-session test recovers an interrupted episode and resumes it without
   duplicating a window, proposal or receipt.
2. Mixed policy versions, portfolio digests or window versions fail closed.
3. Darwin and unknown actors cannot create an acceptance receipt.
4. Health receipts and evolution records cannot be read through each other's
   store paths.
5. Incomplete files and conflicting replays do not advance recovered state.
6. JSON Schema rejects unknown fields and content-bearing fields.
7. Native persistence remains explicitly unavailable.
