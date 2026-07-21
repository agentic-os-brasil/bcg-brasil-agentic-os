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

- [ ] Implement `bcgos version`, `status` and product-facing `doctor`.
- [ ] Implement canonical manifest, lock and runtime-capability schemas.
- [ ] Implement user-space directories on Windows and macOS.
- [ ] Add Claude and Codex adapter skeletons against the same contracts.
- [ ] Add conformance fixtures for platform and runtime parity.
- [ ] Implement idempotent `bcgos init` with data-preservation tests.

### Discuss before closing the track

- Final product/command name.
- Approved Windows and macOS application directories.
- Minimum Claude/Codex capability set for v0.
- What `doctor` may inspect and report on corporate devices.

## Track C - Private distribution and updates

### Build

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
- [ ] Define the durable run-state schema for scheduling, catch-up and diagnostics.
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
- Whether shared organizational knowledge is a separate governed store rather than a memory layer.

## Track E - Content navigation and knowledge wiki

### Build

- [x] Define the managed and private atlas boundaries and the role of wiki versus canonical sources.
- [x] Define how the private atlas navigates L1/L2/L3/lifetime without becoming another memory store.
- [x] Define the wiki as downstream navigation over rollups produced by dreaming.
- [ ] Define page and machine-readable index schemas.
- [ ] Implement the deterministic managed product-atlas generator.
- [ ] Generate index, backlinks, orphan/broken-link diagnostics and a generation log.
- [ ] Add allowlist, provenance, determinism and boundary tests to CI.
- [ ] Expose bounded product-content pointers for Claude and Codex adapters.
- [ ] Implement owner/workspace private atlas only after enrollment, privacy and deletion contracts are ready.

### Discuss before enabling private navigation

- Exact managed content allowlist and initial page taxonomy.
- Owner/workspace enrollment and access-policy contract.
- Which memory layers may produce summaries versus pointers only.
- Which temporal, topic and entity facets may be derived from each rollup layer.
- Private compilation provider, offline behavior and corporate data constraints.
- Organizational knowledge approval, synchronization and retirement.
- User command vocabulary: `bcgos wiki`, `bcgos knowledge` or another surface.

## Track F - First Agent OS bundle

### Build

- [ ] Select one high-value work use case from observed consultant needs.
- [ ] Define the minimal bundle manifest, version and compatibility range.
- [ ] Implement runtime-neutral contracts and thin Claude/Codex adapters.
- [ ] Add safe context injection, workspace boundaries and capability detection.
- [ ] Package only allowlisted product content; exclude all development harness paths.
- [ ] Validate install, init, update and rollback without client content.

### Discuss before closing the track

- First use case and target persona.
- Shared versus local knowledge governance.
- Which hooks block, warn or observe.
- Ownership and retirement model for agents and skills.

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
