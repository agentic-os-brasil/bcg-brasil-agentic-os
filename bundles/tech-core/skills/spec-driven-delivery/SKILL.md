---
name: spec-driven-delivery
description: Turn a bounded engineering need into an inspectable delivery contract with assumptions, acceptance criteria, risks and evidence. Use for software, data or technical consulting work before implementation starts.
---

# Spec-Driven Delivery

Turn an ambiguous technical request into a small, reviewable work packet. This
skill supports engineering work; it does not grant access to a client system,
approve a deployment or create a production change.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the plan.
For a standard user, use plain language and one recommended next step. For
advanced and power users, make assumptions, interfaces and alternatives
explicit. The profile changes communication only: it never changes permissions,
data scope or approval requirements.

## Workflow

1. State the decision or outcome that the work must support, not only the
   requested implementation.
2. Separate known facts, assumptions, out-of-scope items and open questions.
3. Define observable acceptance criteria, including one negative or failure
   condition when the work has material risk.
4. Name the smallest safe artifact: a specification, prototype, pull request,
   analysis or validation report.
5. Identify the evidence that would justify review, release or a later
   decision; do not treat implementation completion as evidence by itself.
6. Record durable technical or data decisions through the project’s governed
   decision-log route when one exists.
7. Return a compact work packet with objective, constraints, pointers, owner
   confirmations and the next reversible action.

## Invariants

- Do not infer client access, production authority, data classification or
  approval from a role or selected capability track.
- Do not place source code, client data, credentials or full requirements into
  a managed skill catalog or session packet.
- A capability bundle can suggest this workflow; it cannot execute it or
  authorize its outputs.
