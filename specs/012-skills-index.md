# Spec 012 - Managed skills index

Status: decision accepted; compiler, generated catalog and inspection command implemented. Session Start consumption remains pending.

## Objective

Allow people and runtimes to discover the managed capabilities of the Agentic
OS without injecting every operating procedure into a session or maintaining a
second hand-written skills summary.

## Contract

The canonical source remains each product `SKILL.md` and its adjacent runtime
metadata. The compiler produces two deterministic views:

- `catalog.json`: compact machine-readable routing data;
- `INDEX.md`: human-readable navigation page.

Each entry contains only a stable skill ID, display name, trigger summary,
default prompt and relative pointer to the canonical skill. It never embeds
the complete skill instructions, a user profile, memory, workspace data,
client material, logs or execution state.

## Context behavior

Future Session Start receives a bounded pointer to the catalog, not the full
catalog by default. It may select one or a small number of skill entries based
on explicit intent, then read the canonical SKILL.md on demand. The active
interaction profile changes how that capability is presented; it does not
filter the catalog or authorize execution.

## Lifecycle and validation

The compiler is deterministic for the same ordered managed skill set. The
development harness verifies that both generated artifacts exactly match the
canonical sources. A product-skill change that leaves the index stale fails
validation. The generator is a development convenience only; the generated
artifacts, not development paths, ship inside the managed bundle.

## Delivery boundary

`bcgos skills index` exposes the managed catalog now. Session Start injection,
intent routing, private skills and organization-specific bundles remain
unavailable until their separate ownership, authorization and adapter
contracts exist.
