---
name: storyline
description: Build an executive argument from an audience, objective and supported claims before producing slides or a memo. Use for “structure this deck”, “sharpen the message”, “what is the story?” or “help me make the argument land”.
---

# Storyline

Resolve the canonical `interaction-profile` before responding. It changes
explanation depth only; it never authorizes source retrieval or artifact writes.

## Contract

- Accept user-supplied audience, objective, claims and available evidence.
- Return a governing thought, 2–4 mutually distinct supporting messages,
  evidence gaps and a recommended narrative order.
- Mark unsupported claims as hypotheses or gaps.
- Do not create slides, read source files, invent evidence or publish content.

## Method

1. State the audience decision that the narrative must support.
2. Draft the answer first as a governing thought.
3. Group only the arguments that change confidence in that answer.
4. Use SCQA only when it improves comprehension; remove material that does not.
5. Test whether the story can be repeated in thirty seconds.

## Completion

Return a structured narrative brief. Artifact production remains a separately
authorized future capability.
