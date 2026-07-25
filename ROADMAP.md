# Roadmap

The roadmap separates things to build from things to decide. Detailed dates remain open until the corporate Windows and macOS environments are validated. Unresolved choices live in `docs/OPEN-QUESTIONS.md`; accepted choices move to the four-letter decision log.

## Current foundation

- [x] Define the professional-only scope and broad target audience.
- [x] Select CLI-first private-release distribution.
- [x] Separate source, managed core, workspace metadata and local data.
- [x] Define Claude-primary, Codex-compatible runtime portability.
- [x] Establish specs, four-letter decisions, unit tests and a cross-platform development harness.
- [x] Add novice-safe Git guards, recovery, CI and human-reviewed PR flow.
- [x] Require equal pilot acceptance on Windows and macOS.
- [x] Establish the L1/L2/L3, lifetime and dreaming memory contract.
- [x] Add a sanitized default memory policy, contract validator and development skill.
- [x] Adopt a Karpathy-inspired compiled wiki for governed content and memory navigation.
- [x] Define Docling as the local-first ingestion substrate and progressive user profiles.
- [x] Define and implement a canonical user-local interaction profile across product skills.
- [x] Define and compile a managed skills index for bounded session navigation.
- [x] Define and implement a local, professional owner-self registry with facet policies and a cold-start interview contract.
- [x] Decide that the future human atlas is scoped across managed, owner-private and workspace-private roots, alongside canonical derived memory.
- [x] Implement a non-overwriting owner/workspace human-atlas bootstrap with no task taxonomy or compiled navigation.
- [x] Define workspace agents as project specialists and context gatekeepers with compact state and versioned dossiers.
- [x] Implement a bounded runtime-neutral Session Context Packet with pointers and omission diagnostics only.
- [x] Define the execution-ledger authority boundary and implement the initial local contract, attempt, revision and inspection store.

## Track A - Contributor onboarding

### Build

- [x] Create a single Windows prompt covering prerequisites, browser authentication and clone.
- [x] Create the deterministic repo-local Windows bootstrap.
- [x] Add native Claude skill projections backed by canonical `dev/skills`.
- [ ] Add Marcelo as a collaborator after confirming his GitHub username.
- [ ] Run the complete onboarding on Marcelo's corporate Windows device.
- [ ] Complete one small real PR through `start-work` and `prepare-pr`.
- [ ] Capture time, friction and failure evidence without credentials or client content.
- [ ] Create and validate the equivalent macOS contributor bootstrap.

### Discuss before closing the track

- Approved Windows software-installation channel and corporate restrictions.
- Exact Claude distribution Marcelo will use.
- First contribution and who reviews it.
- Whether contributor bootstrap should later become a signed standalone tool.

## Track B - Product CLI skeleton

### Build

- [x] Implement `bcgos version`, `status` and product-facing `doctor`.
- [x] Implement versioned runtime-capability manifest, schema and `bcgos doctor` report.
- [ ] Implement canonical lock and runtime configuration schemas.
- [x] Implement user-space directories on Windows and macOS.
- [x] Add Claude and Codex adapter skeletons against the same canonical contract.
- [x] Add initial conformance fixtures for equivalent capability identity, semantic events and criticality.
- [x] Implement a runtime-neutral Session Start bridge that gives Claude and Codex the same bounded packet without claiming native injection.
- [ ] Add conformance fixtures for actual lifecycle injection and failure reporting once adapters exist.
- [ ] Wire the Session Context Packet into Claude and Codex lifecycle adapters with equivalent authorization and omission reporting.
- [x] Implement idempotent `bcgos init` with data-preservation tests.
- [x] Implement `bcgos profile show|set` with a user-local, runtime-neutral policy.
- [x] Implement `bcgos owner init|status|interview` with inspectable professional facets and no silent sensitive-data use.
- [x] Implement local refinement submission, policy enforcement, audit snapshots and explicit reversal for owner facets.
- [ ] Implement approved local assessment ingestion with explicit consent, provenance and confirmation before synthesis.
- [ ] Implement lifecycle observation capture and approved synthesis adapters that submit refinements to the local core.
- [x] Implement `bcgos work create|start|inspect|export|delete` over a workspace-scoped local execution ledger.
- [x] Add bounded checkpoint, pause, active-work projection and explicit resume with prior-attempt invalidation.
- [x] Add core-witnessed artifact snapshots, command checks and evidence-backed completion.
- [ ] Expose only the active execution pointer through the Session Context Packet.
- [ ] Prove handoff across two sessions or agents without transcript or contract reinjection.
- [x] Bootstrap one local agent per workspace with a reviewed interview and versioned briefing.
- [x] Implement explicit research-plan approval, hostname allowlists and sourced evidence persistence.
- [x] Implement attested public economic snapshots with per-claim provenance
  outside workspace roots and attach them by immutable ID.

### Discuss before closing the track

- Final product/command name.
- Approved Windows and macOS application directories.
- Minimum Claude/Codex capability set for v0.
- What `doctor` may inspect and report on corporate devices.
- Retention and deletion policy for execution history after the V1 local pilot.

## Track C - Private distribution and updates

### Build

- [x] Implement and run an isolated local binary-installation trial with checksum verification on Windows, macOS and Linux. This is explicitly not a signed release or pilot distribution channel.
- [ ] Produce versioned Windows and macOS artifacts in CI.
- [ ] Define signed release manifest and compatibility rules.
- [ ] Implement browser-based private-release authentication.
- [ ] Implement verified download, staging, activation and rollback.
- [ ] Ensure updates never overwrite local configuration or work data.
- [ ] Run install/update/rollback tests on clean corporate devices for both platforms.

### Discuss before closing the track

- GitHub Releases versus BCG-managed distribution.
- Code-signing, SmartScreen, Gatekeeper and provenance requirements.
- Update cadence, forced security updates and support ownership.

## Track D - Memory persistence and dreaming

### Build

- [x] Define L1 daily memory, L2 weekly rollups, L3 rolling thematic memory and curated lifetime memory.
- [x] Define deterministic-first dreaming with source fingerprints, staging, validation, atomic activation and last-known-good preservation.
- [x] Ship a sanitized runtime-neutral policy and validate it in the development harness.
- [x] Add `evolve-memory` as the canonical development workflow for memory changes.
- [x] Separate light daily dreaming from weekly deep dreaming and assign lifetime consolidation to the weekly cycle.
- [x] Define the versioned local memory layout and provenance envelope.
- [x] Define and validate the atomic memory commit-manifest schema.
- [x] Define the durable run-state schema and runtime-neutral core for scheduling, bounded catch-up and diagnostics.
- [x] Implement append-only L1 capture and bounded daily digests in the runtime-neutral engine.
- [x] Implement idempotent L2 and L3 rollups with immutable versions and crash-interruption tests.
- [x] Implement governed weekly lifetime eligibility and version history in the core engine.
- [x] Implement bounded context assembly with drill-down pointers and diagnostics.
- [x] Add the runtime-neutral `dream-memory` product skill.
- [x] Connect sanitized capture, status and bounded context assembly to `bcgos memory`, with dreaming explicitly unavailable without an adapter.
- [ ] Implement lifetime correction and deletion flows.
- [ ] Implement stale-lock diagnosis and human-confirmed recovery in `bcgos doctor`.
- [ ] Implement synthesis and eligibility adapters without embedding provider policy in the core.
- [ ] Add executable dreaming plus `explain`, `export` and `delete` contracts before enabling persistence for pilot users.
- [ ] Add equivalent Windows and macOS scheduling or presence-based catch-up adapters.
- [ ] Add Claude and Codex conformance fixtures for injection and failure reporting.

### Discuss before executing rollups

- Which signals may enter L1 and what sanitization happens before persistence.
- Synthesis provider, model policy, offline behavior and corporate data constraints.
- Retention windows, context budgets and lifetime promotion eligibility.
- User controls for inspect, correct, export and delete.
- Recovery policy for locks left by interrupted or crashed dreaming runs.
- Daily/weekly windows, unattended-model permission, catch-up limits and retry/backoff policy.
- Whether shared organizational knowledge is a separate governed store rather than a memory layer.

## Track E - Content navigation and knowledge wiki

### Build

- [x] Define the managed and private atlas boundaries and the role of wiki versus canonical sources.
- [x] Define how the private atlas navigates L1/L2/L3/lifetime without becoming another memory store.
- [x] Define the wiki as downstream navigation over rollups produced by dreaming.
- [x] Adopt OKF v0.1 plus a governed BCGOS Atlas Profile for wiki bundles.
- [x] Define event-driven incremental updates, weekly reconciliation and synchronous revocation barriers.
- [ ] Define page and machine-readable index schemas.
- [ ] Implement the BCGOS Atlas Profile v1 schema and OKF validator.
- [ ] Implement the transactional outbox, source watermarks and atomic atlas manifest.
- [ ] Implement the deterministic managed product-atlas generator.
- [ ] Generate index, backlinks, orphan/broken-link diagnostics and a generation log.
- [ ] Add allowlist, provenance, determinism and boundary tests to CI.
- [ ] Expose bounded product-content pointers for Claude and Codex adapters.
- [ ] Implement owner/workspace private atlas only after enrollment, privacy and deletion contracts are ready.
- [ ] Extend the initial private-atlas taxonomy only through scoped templates, ownership rules and privacy tests.

### Discuss before enabling private navigation

- Exact managed content allowlist and initial page taxonomy.
- Owner/workspace enrollment and access-policy contract.
- Which memory layers may produce summaries versus pointers only.
- Which temporal, topic and entity facets may be derived from each rollup layer.
- Private compilation provider, offline behavior and corporate data constraints.
- Organizational knowledge approval, synchronization and retirement.
- User command vocabulary: `bcgos wiki`, `bcgos knowledge` or another surface.
- Freshness targets, retry/backoff limits and private crypto-erasure policy.

## Track F - First Agent OS bundle

### Build

- [ ] Select one high-value work use case from observed consultant needs.
- [x] Define the local-first ingestion contract, managed runtime-pack boundary and standard/advanced/power progressive-disclosure model.
- [ ] Run a Windows/macOS Docling runtime-pack distribution spike with sanitized fixtures, measuring size, first-use time, offline behavior and extraction quality.
- [ ] Implement verified installation, capability detection and removal of the ingestion runtime pack.
- [ ] Implement `bcgos ingest` and the Docling-first, deterministic-fallback execution contract.
- [ ] Define the minimal bundle manifest, version and compatibility range.
- [ ] Implement runtime-neutral contracts and thin Claude/Codex adapters.
- [x] Implement a deterministic managed skills index and `bcgos skills index` inspection surface.
- [ ] Add safe context injection, workspace boundaries and capability detection.
- [x] Add the managed `workspace-agent-setup` skill with fail-closed research approval and provenance workflow.
- [ ] Package only allowlisted product content; exclude all development harness paths.
- [ ] Validate install, init, update and rollback without client content.

### Discuss before closing the track

- First use case and target persona.
- Shared versus local knowledge governance.
- Which hooks block, warn or observe.
- Ownership and retirement model for agents and skills.
- Runtime-pack size, model-prefetch, corporate proxy and offline acceptance thresholds.

## Track G - Ten-person pilot

### Build

- [ ] Select a balanced Windows/macOS and classic/technical cohort.
- [ ] Onboard users without requiring Git, Go, Python, Node or Docker.
- [ ] Measure time-to-first-success, update reliability and support demand.
- [ ] Capture sanitized failure modes and prioritize the next shared capabilities.
- [ ] Run a pilot retrospective and decide the next distribution model.

### Discuss before launch

- Pilot cohort, support channel and incident owner.
- Privacy-safe telemetry and evidence of value.
- Exit criteria for expansion, redesign or stop.

## Later, only with evidence

- Additional agents and skills.
- Organization-level shared knowledge.
- Extension SDK or marketplace.
- Optional graphical interface.
- Additional runtime adapters and Linux pilot support.
