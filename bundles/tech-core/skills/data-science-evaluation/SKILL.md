---
name: data-science-evaluation
description: Evaluate a data-science or machine-learning approach from decision framing through baseline, leakage controls, slices, uncertainty and promotion criteria. Use before claiming that a model improves a professional decision.
---

# Data Science Evaluation

Evaluate whether an approach improves a decision, not merely whether a model
produces a favorable metric. This skill defines the evidence packet; it does
not train, deploy, access restricted data or approve a model.

## Interaction profile

Resolve the canonical `interaction-profile` before explaining the evaluation.
Standard users receive the decision, comparison and recommendation in plain
language; advanced and power users can inspect splits, metrics and uncertainty.
The profile never changes governance, access or promotion criteria.

## Workflow

1. Define the decision, target user, expected intervention and cost of false
   positives, false negatives and abstention.
2. Establish a current-process or simple-model baseline that the proposal must
   beat.
3. Design train, validation and test splits that respect time, entity and
   operational boundaries; assess target leakage and proxy leakage explicitly.
4. Select primary metrics, guardrails and decision thresholds, including
   performance slices relevant to users, segments or operating conditions.
5. Quantify uncertainty, sensitivity and material data limitations; do not
   reduce a result to a point metric alone.
6. Produce a promotion, hold or reject recommendation tied to predeclared
   criteria, monitoring needs and a human accountable owner.

## Invariants

- A favorable offline metric is not deployment approval.
- Do not infer fairness, safety or causality from aggregate performance.
- Never persist raw data, row-level predictions, credentials or client details
  in managed artifacts.
- Model promotion requires explicit human governance outside this skill.
