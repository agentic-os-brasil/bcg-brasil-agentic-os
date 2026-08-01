# Spec 036 - Base engineering quality methods

Status: implemented in the base bundle; specialized engineering capability
activation remains governed by Spec 035.

## Objective

Ship the smallest useful quality loop in every Maestro installation. A
consultant should be able to inspect coverage risk, run a bounded test wave,
capture a real regression, qualify a change, review a pull request and repeat
the loop without selecting an engineering persona or activating a separate
bundle.

## Included methods

The base bundle includes these six managed skills:

- `coverage-diagnose`
- `unit-test-wave`
- `xfail-bug-capture`
- `qa-gate`
- `pr-review`
- `pr-quality-loop`

They are procedural and evidence-oriented. They do not grant tools, write to a
repository by implication, approve a release, merge a pull request or persist
source, prompts, client data, credentials or full command output.

## Bundle boundary

These methods are part of `bundles/base/distribution.json`, are generated into
the active `bundles/base/skills/catalog.json` and `INDEX.md`, and are available
to the normal skills-index surface. The specialized `engineering-core` bundle
is optional and is activated only by confirmed interview selection; its skills
are not copied into the base bundle or activated by dependency resolution
alone.

Development lifecycle hooks are not product skills and remain outside the base
distribution. They may be supplied as a separate contributor/development pack
with the same metadata-only and fail-closed principles.

## Acceptance evidence

- Every included skill has canonical frontmatter, an OpenAI metadata projection
  and a reference to the shared `interaction-profile` contract.
- `go run ./dev/harness skills-index` produces a catalog and Markdown index that
  include exactly the six new skills in sorted order.
- The base distribution allowlist names both files for each included skill.
- `go run ./dev/harness validate --full` passes while specialized data bundle entries
  remain unavailable.
