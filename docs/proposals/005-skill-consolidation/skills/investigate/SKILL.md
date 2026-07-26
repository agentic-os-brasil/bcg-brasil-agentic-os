---
name: investigate
description: Systematic root-cause investigation of something broken or surprising — a number that won't tie, a model throwing off results, data that contradicts itself. Use for "investigate", "why is this wrong", "this doesn't add up", "debug this". No fix without an investigation first.
---

# Investigate

Find the cause before touching the fix. Reason backward from the symptom.

## Method
1. State the symptom precisely: expected vs. actual, with the concrete numbers/behavior.
2. Trace backward through the data/logic — read the actual formulas, cells, inputs; don't
   guess.
3. List ranked hypotheses for the cause.
4. Test ONE hypothesis at a time. Stop-rule: after ~3 failed fixes, reframe the problem
   rather than trying a fourth patch.
5. Fix, then verify the symptom is gone AND nothing downstream broke.
6. Close with: root cause · the fix · blast radius · the gotcha (a candidate learning).

## Relations
- **Used by the hub** for light traces, and by **`quant-analyst`** when the investigation
  needs heavy data work (reading a model, re-running a pipeline). The hub delegates the
  heavy-data leg to `quant-analyst`; the reasoning loop is the same either way.
