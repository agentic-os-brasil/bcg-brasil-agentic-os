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

## Track D - First Agent OS bundle

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

## Track E - Ten-person pilot

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
