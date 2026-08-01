# Spec 035 - Professional capability bundles

Status: source topology and skill catalogs implemented. The neutral engineering
quality methods are included in the base bundle; the first specialized
engineering and data bundles are optional and every skill in a selected bundle
and its dependencies is activated through confirmed interview selection.

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
| `base` | Yes | `consulting` (plus transversal quality methods) | none |
| `engineering-core` | Optional — activated by confirmed interview selection | `technical-explorer`, `software-engineering` | `base` |
| `data-practice` | Optional — activated by confirmed interview selection | `data-science`, `data-engineering` | `engineering-core` |

The base bundle contains six neutral engineering quality methods: coverage
diagnosis, focused unit-test waves, strict expected-failure capture, QA gates,
pull-request review and the pull-request quality loop. They are transversal
quality controls, not a software-engineering identity or tool grant.

`engineering-core` retains specialized delivery practices: specification-first
delivery, proportionate tests/evidence and human review explanation.
`data-practice` adds data-pipeline quality, data-science evaluation and
reproducible data runs. These extract reusable principles from a mature,
runtime-neutral engineering practice; they do not copy client procedures,
paths, owners, data or automation assumptions.

Every bundle owns canonical `SKILL.md` files plus generated `catalog.json` and
`INDEX.md` pointers. The development harness validates every declared bundle,
its skill metadata, its interaction-profile reference and generated catalog.

## Current product surface

`bcgos bundles index` exposes the compact source inventory. `bcgos bundles
plan --track <track[,track...]>` resolves the selected bundles and dependencies
without changing local state. `bcgos agent interview` exposes the same tracks;
`bcgos agent personalize --stdin` persists a confirmed selection, and the next
adapter installation projects the selected optional skills.

For `consulting`, the result is `base_only` and its active skills index includes
the neutral quality methods. Plans that require either optional bundle return
`optional` and explain that the selection must be confirmed in the interview.
The plan command must never write a selection, install a package, modify a
workspace, contact a provider or grant authority.

The existing `bcgos skills index` remains the index of the active base bundle.
It lists the six explicitly included quality methods by default. After a
confirmed selection, the adapter projection adds every selected
engineering-core and/or data-practice method, including dependencies, without
changing the base catalog or granting tools.

## Future activation contract

For optional bundles shipped in the signed Canary distribution, onboarding may
persist a capability-track choice and the local adapter may project the bundle
only after explicit confirmation. A later release contract must still define
all of the following before remote or separately downloaded packs are allowed:

1. separately versioned optional-bundle artifacts, identity and signatures;
2. CLI/base/optional-bundle compatibility and migrations;
3. verified download, staging, atomic activation, rollback and removal;
4. local selected-bundle state, explicit confirmation and recovery behavior;
5. Session Start catalog composition, authorization and bounded injection;
6. Windows and macOS clean-device acceptance evidence.

The release manifest v1 intentionally excludes remote optional packs. The
Canary's two optional bundles are embedded in the verified local distribution;
a conversational skill cannot emulate activation from source files or a Git
clone.

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
- A runtime activates only selected bundles embedded in the verified local
  distribution; it does not emulate remote installation from source files or a
  Git clone.

## Acceptance evidence

- The catalog rejects unknown dependencies, duplicate identities, duplicate
  tracks across the entire catalog and dependency cycles, and an
  optional bundle with an invalid availability state.
- Track planning resolves `software-engineering` through `base` and
  `engineering-core`, and projection activates all three engineering-core
  skills.
- Track planning resolves `data-science` through `base`, `engineering-core`
  and `data-practice`, and projection activates all six selected/dependency
  skills.
- Track planning resolves `consulting` to the base bundle only.
- The full harness validates all declared bundle skill directories and generated
  indexes; the distribution allowlist contains the six explicit quality methods
  and the signed Canary engineering-core and data-practice content.
