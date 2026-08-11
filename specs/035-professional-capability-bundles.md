# Spec 035 - Professional capability bundles

Status: source topology and skill catalogs implemented. Professional methods
remain in the base bundle; the included `tech-core` bundle contains engineering,
data, AI and transversal quality skills and is projected from the first local
installation.

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
  as `technical-explorer`, `software-engineering`, `data-science`,
  `data-engineering` or `ai-engineering`.
- **Bundle:** a managed set of product skills with a stable identity,
  dependency list and generated compact skills catalog. Release versioning is
  part of the future activation contract.
- **Activation:** the separately governed transaction that makes a released
  optional bundle available in one local installation. Included bundles do not
  require activation or interview confirmation.

Interaction profile and capability track are independent. A classic consultant
can select `technical-explorer`; an experienced engineer can retain the
`standard` communication profile. Neither selection grants a tool, provider,
filesystem scope, client-data access or approval.

## Source topology

The source inventory is `bundles/catalog/catalog.json`:

| Bundle | Included now | Tracks | Depends on |
| --- | --- | --- | --- |
| `base` | Yes | `consulting` | none |
| `tech-core` | Yes — projected with the default installation | `technical-explorer`, `software-engineering`, `data-science`, `data-engineering`, `ai-engineering` | `base` |

`tech-core` combines six transversal quality methods (`coverage-diagnose`,
`decision-log-entry`, `pr-quality-loop`, `pr-review`, `unit-test-wave` and
`xfail-bug-capture`) with specification-first delivery, proportionate
tests/evidence, human review explanation, data-pipeline quality, data-science
evaluation and reproducible data runs. These methods do not grant tools,
filesystem scope or client-data access.

Every bundle owns canonical `SKILL.md` files plus generated `catalog.json` and
`INDEX.md` pointers. The development harness validates every declared bundle,
its skill metadata, its interaction-profile reference and generated catalog.

## Current product surface

`bcgos bundles index` exposes the compact source inventory. `bcgos bundles
plan --track <track[,track...]>` resolves the selected bundles and dependencies
without changing local state. `bcgos agent interview` exposes the same tracks;
`bcgos agent personalize draft --stdin --consent --no-client-data`, followed by
review and digest-bound confirm, persists a confirmed selection and a managed,
selection-scoped policy at `.bcgos/agent-skill-policy.json`. The default policy
admits the included Tech Core methods immediately; declared tracks can further
personalize routing. Future optional bundles remain denied until their
selection is confirmed, even if their source is embedded in the verified local
distribution.

For `consulting`, the result includes both `base` and `tech-core`; the active
skills index is complete from the first projection. Declared technical tracks
continue to tailor routing and progressive disclosure, but no track selection
is required to make the bundled methods available. The plan command must never
write a selection, install a package, modify a workspace, contact a provider or
grant authority.

The existing `bcgos skills index` remains the index of the active base bundle.
The adapter projection adds every `tech-core` method by default without
changing the base catalog or granting tools.

## Future activation contract

For future optional bundles shipped in the signed distribution, onboarding may
use the declared professional function to make a bounded recommendation and
persist a capability-track choice; the local adapter may then project the bundle
only after explicit confirmation. A later release contract must still define
all of the following before remote or separately downloaded packs are allowed:

1. separately versioned optional-bundle artifacts, identity and signatures;
2. CLI/base/optional-bundle compatibility and migrations;
3. verified download, staging, atomic activation, rollback and removal;
4. local selected-bundle state, explicit confirmation and recovery behavior;
5. Session Start catalog composition, authorization and bounded injection;
6. Windows and macOS clean-device acceptance evidence.

The release manifest v1 intentionally excludes remote optional packs. The
included `tech-core` bundle is embedded in the verified local distribution; a
conversational skill cannot emulate activation from source files or a Git clone.

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
- A runtime activates included bundles and only explicitly selected optional
  bundles embedded in the verified local distribution; it does not emulate
  remote installation from source files or a Git clone.
- Projection, inspection and removal hash the selection-scoped policy in the
  runtime manifest. A missing, modified, symlinked or unmanaged policy path
  fails closed and preserves the existing file.

## Acceptance evidence

- The catalog rejects unknown dependencies, duplicate identities, duplicate
  tracks across the entire catalog and dependency cycles, and an
  optional bundle with an invalid availability state.
- Track planning resolves `software-engineering` through `base` and
  `tech-core`, and projection activates the full technical catalog.
- Track planning resolves `data-science` through `base` and `tech-core`, and
  projection activates the same governed technical catalog.
- Default and `consulting` planning resolve both `base` and `tech-core`.
- The dispatcher's direct-skill gate admits selected and dependency methods for
  the active Case Agent and rejects an embedded method from an unselected
  bundle.
- The full harness validates all declared bundle skill directories and generated
  indexes; the distribution allowlist contains the signed Canary `tech-core`
  content.
