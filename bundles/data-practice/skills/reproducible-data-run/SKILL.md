---
name: reproducible-data-run
description: Make a data or analytical run reproducible through a bounded manifest of code, configuration, inputs, environment, seed, artifacts and rerun or rollback criteria. Use for analyses, experiments and data pipelines whose conclusions need review.
---

# Reproducible Data Run

Make the result inspectable and repeatable without turning an execution log
into a copy of sensitive data. This skill defines a reproducibility manifest;
it does not execute code, retain source data or authorize production changes.

## Interaction profile

Resolve the canonical `interaction-profile` first. Standard users receive a
plain-language explanation of what another reviewer needs to repeat the result.
Advanced and power users may receive version, environment and artifact details.
The profile never changes data retention, access controls or release approval.

## Workflow

1. Identify the question, expected output, accountable owner and authorized
   execution environment.
2. Record immutable or versioned pointers for code, configuration, input data
   contract/version, environment, dependencies and deterministic seed where
   applicable.
3. Define output artifacts, checksums or controlled references, acceptance
   checks and the boundary between retained metadata and prohibited payloads.
4. State the rerun procedure, expected variance and criteria for a valid
   comparison.
5. Define rollback or invalidation conditions when source, code, assumptions or
   environment change materially.
6. Produce a concise manifest and evidence summary that a reviewer can use to
   request a rerun without receiving raw data.

## Invariants

- Never log raw source data, credentials, prompts, environment secrets or full
  error bodies in a reproducibility manifest.
- Reproducibility does not imply authorization to reuse a dataset elsewhere.
- A rerun cannot overwrite a prior result without an explicit versioned
  replacement or invalidation record.
