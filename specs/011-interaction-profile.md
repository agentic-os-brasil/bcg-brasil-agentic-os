# Spec 011 - Interaction profile

Status: decision accepted; local configuration, CLI controls and canonical product policy implemented. Runtime injection remains pending its adapters.

## Objective

Make the Agentic OS feel appropriately approachable for a classic consultant
without making a technical user repeatedly ask for details. The same user must
receive a consistent approach across skills, memory navigation and future
agents, rather than a separate persona interpretation in each bundle.

## Canonical parameter

BCGOS owns one self-declared `interaction_profile` per local user:

| Profile | Default communication | Suggestions exposed by default |
|---|---|---|
| `standard` | Plain language, one recommended safe route and concise next action. | No implementation or provider alternatives unless they are necessary. |
| `advanced` | Concise rationale and options, with terms explained on first use. | Approved diagnostics, templates, batches, intermediate artifacts and configuration choices. |
| `power` | Direct technical detail, assumptions and observable trade-offs. | Explicit local-model, provider and custom-pipeline alternatives after the same policy and credential checks. |

The profile is configuration, not identity. It contains no name, role, client,
project or behavioral history. It is stored under user-local BCGOS application
data and is never copied to a workspace, memory layer, managed bundle, Git or
shared atlas.

`standard` is the default. A user may change it at any time with `bcgos profile
set <standard|advanced|power>` and inspect the effective value with `bcgos
profile show`.

## Non-negotiable invariants

- The profile changes progressive disclosure, language and optional
  suggestions; it does not grant authority.
- It never enables a remote provider, bypasses policy, relaxes release
  verification, changes retention, or weakens client-data boundaries.
- Product skills resolve the canonical policy and current local setting; they
  must not define another persona taxonomy.
- The development harness rejects a product skill that does not explicitly
  reference the canonical `interaction-profile` skill, except that skill
  itself.
- A runtime adapter may inject only the bounded profile ID and a managed policy
  pointer. It must not inject an entire brain or derive a profile from private
  memory.
- If configuration is missing, unreadable or unknown, agents fail safely to
  the managed `standard` default and diagnostics expose the condition.

## Runtime and brain behavior

The profile is an input to every product interaction. At session start, once
lifecycle adapters exist, a runtime resolves it alongside the effective policy
and injects a compact pointer such as `interaction_profile=advanced`. Skills
then use the managed behavior matrix to determine response style and whether
to proactively offer technical options.

It is intentionally not a memory layer. Memory persists work continuity and
governed observations; the interaction profile is an explicit, immediately
correctable preference. Human-readable brain pages can describe the work
without carrying this setting, while the CLI remains its source of truth.

## Delivery boundary

The CLI persists and reports the configuration now. Claude/Codex lifecycle
adapters, automatic Session Start injection and profile-aware execution of
future skills remain unavailable until their thin adapters and conformance
fixtures exist.
