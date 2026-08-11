# Spec 036 - Tech Core engineering quality methods

Status: implemented in the included `tech-core` bundle; projection happens by
default and remains governed by Spec 035.

## Objective

Ship the quality loop with the technical capability bundle. Professional users
keep the base consulting surface, while every installation receives coverage,
test, bug-capture and PR methods together with engineering and data practices.

## Included methods

The `tech-core` bundle includes these six managed skills:

- `coverage-diagnose`
- `decision-log-entry`
- `pr-quality-loop`
- `pr-review`
- `unit-test-wave`
- `xfail-bug-capture`

They are procedural and evidence-oriented. Case Agent can use the broader
method set for delivery work; Gamma Guardian receives only the bounded quality
subset (`coverage-diagnose`, `unit-test-wave`, `pr-review` and
`pr-quality-loop`) when Tech Core is activated as a longitudinal evaluation
method. Neither role receives
tools or write authority from a skill, approves a release, merges a pull
request or persists source, prompts, client data, credentials or full command
output.

## Bundle boundary

These methods are part of `bundles/base/distribution.json` as signed `tech-core`
content, are generated into `bundles/tech-core/skills/catalog.json` and
`INDEX.md`, and are available from the first local projection.

Development lifecycle hooks are not product skills and remain outside the base
distribution. They may be supplied as a separate contributor/development pack
with the same metadata-only and fail-closed principles.

## Acceptance evidence

- Every included skill has canonical frontmatter, an OpenAI metadata projection
  and a reference to the shared `interaction-profile` contract.
- `go run ./dev/harness skills-index` produces a catalog and Markdown index that
  include exactly the six new skills in sorted order.
- The base distribution allowlist names both files for each included skill.
- `go run ./dev/harness validate --full` passes with `tech-core` embedded as
  included: a default projection exposes its full technical catalog without
  granting tools or authority.
