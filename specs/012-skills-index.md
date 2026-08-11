# Spec 012 - Managed skills index

Status: compiler, generated catalog, inspection command and bounded
UserPromptSubmit routing implemented; native qualification remains pending.

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

Session context receives a bounded pointer to the catalog, not the full catalog
by default. Every SessionStart first selects the integrity-checked
`bcgos-operator` pointer with reason `deterministic_operational_method`.
`UserPromptSubmit` may additionally select at most two integrity-checked,
installed and policy-allowed skill entries from an explicit `$skill-id`
reference or deterministic lexical intent. It injects IDs, reasons and runtime
pointers only; prompt text and skill bodies are neither returned nor persisted.
Unknown or ambiguous intent selects nothing. The active
interaction profile changes how that capability is presented; it does not
filter the catalog or authorize execution.

Pending owner onboarding adds the exact integrity-checked
`maestro-onboarding` guide with reason `deterministic_onboarding_state` after
the operating method. It does not suppress the owner's requested Case method.
The packet carries at most three pointers total, preserving operating,
onboarding and task order. These are method pointers, not direct Maestro tools
or delegation grants.

After onboarding, the Case policy includes `ingest-content` and
`find-prior-work`. Both retain their own explicit source, consent and runtime
boundaries: selecting the method does not read a source, authorize SharePoint
collection or promote a runtime capability.

## Lifecycle and validation

The compiler is deterministic for the same ordered managed skill set. The
development harness verifies that both generated artifacts exactly match the
canonical sources. A product-skill change that leaves the index stale fails
validation. The generator is a development convenience only; the generated
artifacts, not development paths, ship inside the managed bundle.

## Delivery boundary

`bcgos skills index` exposes the managed catalog. Contextual routing is a
method-selection hint, never tool, data, delegation or publication authority.
Private skills and organization-specific bundles remain unavailable until
their separate ownership and authorization contracts exist. Adapter output
does not promote native capability without qualifying runtime evidence.
