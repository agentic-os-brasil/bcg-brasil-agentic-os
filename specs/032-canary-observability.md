# Spec 032 - Local canary observability

Status: accepted contract; local receipt store and aggregate report implemented.
Native lifecycle emission remains unavailable.

## Objective

Measure whether the first pilot vertical creates value and remains operable
without exporting professional content or introducing a federation dependency.

## Receipt contract

Receipts are local application state and use one closed event-specific tuple:

- first value records one duration bucket and successful outcome;
- goal resume records only succeeded, failed or blocked;
- installation, update and rollback record only their typed outcome;
- manual intervention records one count bucket; and
- capability failure records one closed capability and failed or blocked.

No receipt contains a person, installation, workspace, client, objective,
prompt, transcript, path, pointer, arbitrary attribute or error string.
Unknown JSON fields and event-incompatible fields fail closed.

## Storage and reporting

Each receipt is an immutable, user-private JSON file published with
create-if-absent semantics. A report reads every receipt strictly and produces
only deterministic counts over the closed buckets. An invalid or unexpected
entry fails the complete report rather than being ignored.

The package has no network or federation dependency. Native Claude/Codex,
installer and updater adapters remain responsible for emitting receipts only
after their own action succeeds or reaches a typed terminal failure.

## Acceptance criteria

1. Unknown fields, arbitrary strings and invalid event combinations fail.
2. Receipt publication never overwrites an existing receipt.
3. Aggregation is deterministic for the same receipt set.
4. No package dependency or command exports the local report.
5. Capability status remains unavailable until a native producer is validated.
