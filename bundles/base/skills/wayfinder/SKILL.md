---
name: wayfinder
description: Decompose a fuzzy, unstructured problem into a non-overlapping issue tree and name the first branch worth investigating. Use when the question is still open — "I don't know where to start", "help me structure this", "wayfinder", "how should I think about this" — and before any analysis has been scoped.
---

# Wayfinder

Turn a question with no shape into a structure someone can work through. This
is pre-analysis: it produces a map and a first move, not an answer.

Advisory and read-only. It reaches no file, no connector and no other role, and
it writes nothing.

## Interaction profile

Resolve `interaction-profile` before presenting the tree. The method, the
bounds and the refusals never vary by profile; only the explanation does.

- `standard`: the framed question, the branches, the first one to open.
- `advanced`: add why the tree splits where it does, and which branches were
  considered and folded in.
- `power`: add the correlation identifier, the constraints as they were
  applied, and the assumptions the framing rests on.

## Inputs

Whatever the person supplies: the question, and any constraints they state.
Constraints are load-bearing — a tree that ignores them is a different problem's
tree. Repeat them back before decomposing.

If the question is too vague to frame, say what is missing and ask. A structure
built on a guess is worse than no structure, because it looks finished.

## Workflow

1. Restate the question in one sentence the person would recognize as theirs.
   If they would not, the framing is wrong; ask again.
2. Name the constraints explicitly, including the ones that rule branches out.
3. Decompose into **three to six branches** that do not overlap and that
   together cover the question. Fewer than three usually means the question was
   already narrow; more than six usually means two levels were flattened into one.
4. Under each branch, state what would have to be true for it to matter, and
   what evidence would settle it.
5. Rank the branches by what would change the answer most, not by what is
   easiest to look at.
6. Name **one** branch to open first, and say why that one.
7. Issue a correlation identifier so a later run over the same question can be
   recognized as the same work rather than a second opinion.

## Invariants

- The method structures; it does not resolve. Branches are hypotheses, and
  presenting one as settled is the failure mode this skill exists to avoid.
- Nothing is invented about the subject matter. Where a branch depends on a
  fact not supplied, name the fact as needed evidence rather than assuming it.
- Constraints are never quietly dropped to make a cleaner tree.
- The person chooses. The skill does not decide for them, and does not create a
  task, a file or a calendar entry to make the choice feel made.
- Operations this skill does not hold — reading a file, checking a calendar,
  recording the tree anywhere — are reported as unsupported, never simulated.
