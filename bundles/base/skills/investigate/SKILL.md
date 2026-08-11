---
name: investigate
description: Find the root cause of a broken or surprising output — a number that will not tie, a run that failed, a result that contradicts itself — and return the cause with the evidence that establishes it. Use for "investigate", "why is this wrong", "this doesn't add up", "what happened here". No remediation is offered before the cause is established.
---

# Investigate

Something produced an output that is wrong, or right in a way nobody expected.
Find out why, and prove it.

Advisory and read-only. It diagnoses and never repairs: editing, re-running or
patching anything is the person's act, taken after they know what they are
fixing.

## Not the same as wayfinder

The two look similar and are opposites in practice.

| | `wayfinder` | `investigate` |
| --- | --- | --- |
| Input | an open question with no structure | a symptom: something already went wrong |
| Output | a tree of branches to explore | one diagnosed cause, with evidence |
| When | before analysis | after a failure |

The test: if nothing has happened yet, it is `wayfinder`. If something happened
and it was wrong, it is this.

## Interaction profile

Resolve `interaction-profile` before presenting the diagnosis. The method, the
stop rule and the refusals never vary by profile.

- `standard`: the cause, the evidence for it, the blast radius, the remediation.
- `advanced`: add the candidates ruled out and what ruled them out.
- `power`: add the boundary as it was fixed, each test and its result, and the
  point at which the framing was reconsidered.

## Workflow

1. State the symptom precisely, in terms someone else could reproduce. "It's
   broken" is not a symptom; "the total is 4% below the sum of its parts" is.
2. Fix the boundary. Name what lies inside the failing path and what lies
   outside it, so a cause cannot be attributed to something never involved.
3. Establish what changed. Most surprises have a change behind them; find it
   before theorizing.
4. Form candidate causes, ordered by what the evidence already favours.
5. For each candidate, name the test that would rule it out, and run or request
   that test. A candidate with no disproving test is a belief, not a hypothesis.
6. Apply the stop rule: after three candidates fail, the framing is suspect.
   Restate the symptom and re-fix the boundary rather than extending the list.
7. Return the diagnosis: the cause, the evidence establishing it, the blast
   radius — what else the same cause touches — and only then the remediation.
8. If the evidence is insufficient, say the symptom is undiagnosed and name what
   would settle it. An undiagnosed symptom reported as undiagnosed is a result.

## Invariants

- No remediation before a cause is established and evidenced. A suspected cause
  yields the next test, not a fix.
- No cause is asserted beyond what the evidence carries. Plausibility does not
  close a run.
- The blast radius is part of the diagnosis. A cause that explains one symptom
  usually explains others, and finding only the reported one is half the work.
- The skill diagnoses and never applies. Operations it does not hold are
  reported as unsupported, never simulated.
