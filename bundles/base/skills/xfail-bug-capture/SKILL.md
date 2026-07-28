---
name: xfail-bug-capture
description: Turn a confirmed production bug exposed by a test into a strict expected-failure record and an actionable backlog item.
---

# Expected-Failure Bug Capture

Use only when a test exercises a real production path and the failure is in
production code, not in the test, fixture or contract. The skill creates a
visible, bounded debt record; it does not silently make a failing suite green.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the capture
packet. The profile changes language and detail only; it never changes the
required evidence or authority boundary.

## Workflow

1. Reproduce the failure and compare the test, fixture, contract and source.
   If the failure is test drift, fix the test instead. If the path is dead or
   the fix is trivial and authorized, fix it directly instead of recording an
   expected failure.
2. Confirm the repository's issue or decision-record format. Never invent an
   identifier or claim ownership that the repository does not establish.
3. Add the framework's strict expected-failure marker only when supported
   (for example, `xfail(strict=True)` in pytest), with a short reason and
   source/test pointers.
4. Create one backlog/decision record for one bug, including the predicted
   signal after the fix (strict expected-failure becomes an unexpected pass).
5. Search for the same production pattern nearby and record separate follow-up
   items when needed.
6. Report the pre-fix result, the expected post-fix result and the next owner
   confirmation required.

## Invariants

- Strict mode is mandatory; a future fix must produce a visible signal.
- One bug maps to one record. Do not bundle unrelated failures.
- Do not add an expected failure for another owner's code without an explicit
  handoff path.
- The record contains metadata and pointers, never raw customer data or full
  error payloads.
