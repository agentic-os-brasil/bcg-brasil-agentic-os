---
name: record-learning
description: Distill a user-supplied experience into a durable professional learning candidate without writing to private memory. Use for “what did I learn?”, “capture this lesson”, “save this insight” or “turn this into a learning”.
---

# Record Learning

Resolve the canonical `interaction-profile` before responding. It changes
presentation only; it never reads or writes owner-private development context.

## Contract

- Accept only a user-supplied experience and reflection.
- Return a learning candidate, evidence, boundary of applicability and a future
test that could refine or disprove it.
- Reject case-specific facts that should remain workspace context.
- Do not create a learning record, infer psychological traits or disclose the
content outside the current conversation.

## Completion

Return a candidate for the owner to review. Owner-private persistence remains
unavailable until that separate contract exists.
