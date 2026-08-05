---
name: data-pipeline-quality
description: Design or assess an operable data pipeline through explicit data contracts, quality checks, lineage, idempotency and recovery. Use for data engineering work, analytical pipelines and technical consulting deliverables that process data.
---

# Data Pipeline Quality

Treat a pipeline as an operational product rather than a script that happened
to run once. This skill frames quality requirements; it does not authorize data
access, execute a pipeline or promote an output.

## Interaction profile

Resolve the canonical `interaction-profile` first. For standard users, explain
the quality risks and the recommended checklist in plain language. Advanced and
power users may receive schemas, thresholds and observability options. The
profile never changes data classification, retention, access approval or
production authority.

## Workflow

1. Define the business decision, consumers and acceptable freshness of the
   output.
2. Establish the input and output data contracts: grain, keys, types, required
   fields, allowed values and schema-evolution policy.
3. Specify quality checks for completeness, nulls, duplicates, referential
   integrity, ranges, drift and freshness; classify each check as block, warn
   or observe only with a stated rationale.
4. Record lineage from source through transformations to published artifact,
   including the intended retention and classification boundary.
5. Define idempotency, incremental/backfill behavior, late-arriving records,
   failure handling and a safe rollback path.
6. Produce a validation report that distinguishes observed results from
   untested assumptions and routes failed blocking checks to a human owner.

## Invariants

- No track, role or skill authorizes reading client data or running a job.
- Never copy source records, credentials, queries or error payloads into a
  managed skill, catalog or receipt.
- A quality check does not prove a pipeline is fit for every business use.
- A failed block condition must remain visible; do not silently downgrade it.
