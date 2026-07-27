# Spec 035 - Professional capability bundles

Status: source topology, skill catalogs and inspection-only planning
implemented. Optional release identity, compatibility and local activation
remain unavailable.

## Objective

Offer professional workflows progressively without forcing every Maestro user
to receive technical procedures that are irrelevant to their work. A person
may describe the capability tracks they want to explore; the product resolves
the required bundles transparently and never infers data, system or deployment
authority from that choice.

## Terms

- **Interaction profile:** `standard`, `advanced` or `power`; controls only
  language and progressive disclosure.
- **Capability track:** a self-declared interest or professional practice such
  as `technical-explorer`, `software-engineering`, `data-science` or
  `data-engineering`.
- **Bundle:** a managed set of product skills with a stable identity,
  dependency list and generated compact skills catalog. Release versioning is
  part of the future activation contract.
- **Activation:** the separately governed transaction that makes a released
  optional bundle available in one local installation.

Interaction profile and capability track are independent. A classic consultant
can select `technical-explorer`; an experienced engineer can retain the
`standard` communication profile. Neither selection grants a tool, provider,
filesystem scope, client-data access or approval.

## Source topology

The source inventory is `bundles/catalog/catalog.json`:

| Bundle | Included now | Tracks | Depends on |
| --- | --- | --- | --- |
| `base` | Yes | `consulting` | none |
| `engineering-core` | No — explicitly unavailable | `technical-explorer`, `software-engineering` | `base` |
| `data-practice` | No — explicitly unavailable | `data-science`, `data-engineering` | `engineering-core` |

`engineering-core` contains reusable delivery practices: specification-first
delivery, proportionate tests/evidence and human review explanation.
`data-practice` adds data-pipeline quality, data-science evaluation and
reproducible data runs. These extract reusable principles from Kowalski's
engineering practice; they do not copy HDI project procedures, paths, owners,
data or automation assumptions.

Every bundle owns canonical `SKILL.md` files plus generated `catalog.json` and
`INDEX.md` pointers. The development harness validates every declared bundle,
its skill metadata, its interaction-profile reference and generated catalog.

## Current product surface

`bcgos bundles index` exposes the compact source inventory. `bcgos bundles
plan --track <track[,track...]>` resolves the selected bundles and dependencies
without changing local state.

For `consulting`, the result is `base_only`. Plans that require an optional
bundle return `unavailable` with the reason that release identity,
compatibility and local activation do not yet exist. The command must never
write a selection, install a package, modify a workspace, contact a provider or
present optional bundles as installed.

The existing `bcgos skills index` remains the index of the active base bundle.
It must not list source-only optional skills as active capabilities.

## Future activation contract

Before onboarding may persist a capability-track choice or activation may take
place, a later release contract must define all of the following:

1. separately versioned optional-bundle artifacts, identity and signatures;
2. CLI/base/optional-bundle compatibility and migrations;
3. verified download, staging, atomic activation, rollback and removal;
4. local selected-bundle state, explicit confirmation and recovery behavior;
5. Session Start catalog composition, authorization and bounded injection;
6. Windows and macOS clean-device acceptance evidence.

The release manifest v1 intentionally excludes optional packs. Until this
contract is implemented and tested, a conversational skill can explain the
plan but cannot claim that a track has been installed or cause any activation.

## Safety invariants

- Capability selection is not a job-title assertion, authorization or identity
  model.
- Bundles contain no workspace content, client data, personal context,
  credentials, execution history or development harness content.
- Skills create declarative work packets and evidence requests; they do not
  assume Write/Edit tools, unrestricted execution or autonomous promotion of
  data, models, code or releases.
- Every product skill resolves the canonical `interaction-profile`; it may not
  define a second novice/expert taxonomy.
- A runtime reports an optional bundle as unavailable rather than emulating an
  install from source files or a Git clone.

## Acceptance evidence

- The catalog rejects unknown dependencies, duplicate identities, duplicate
  tracks across the entire catalog and dependency cycles, and an
  optional bundle that claims to be included.
- Track planning resolves `data-science` through `base`, `engineering-core`
  and `data-practice`, but reports it as unavailable.
- Track planning resolves `consulting` to the base bundle only.
- The full harness validates all declared bundle skill directories and generated
  indexes without allowing optional content into `bundles/base/distribution.json`.
