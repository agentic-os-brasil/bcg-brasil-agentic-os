# Roadmap

The roadmap is progressive. Dates and detailed feature scope remain open until the pilot environment is validated.

## Phase 0 - Foundation

- [x] Define professional-only scope and target audiences.
- [x] Select CLI-first distribution.
- [x] Select private GitHub Releases as the pilot source.
- [x] Separate source repository, managed bundle, workspace metadata and local data.
- [x] Define Claude as primary runtime with semantic portability to Codex.
- [x] Create initial collaboration and architecture specs.
- [ ] Add Marcelo as collaborator.
- [ ] Confirm pilot device and corporate security constraints.

## Phase 1 - CLI skeleton

- [ ] Implement a small Go CLI.
- [ ] Commands: `version`, `status`, `doctor`, `init`, `update`.
- [ ] Define project manifest and lock schemas.
- [ ] Define the canonical runtime capability manifest and parity states.
- [ ] Implement Claude and Codex adapter skeletons against the same contracts.
- [ ] Implement user-space directories for Windows, macOS and Linux.
- [ ] Add deterministic tests for init idempotency and data preservation.
- [ ] Add adapter conformance fixtures and capability detection to `doctor`.

## Phase 2 - Private distribution

- [ ] Build cross-platform artifacts in CI.
- [ ] Publish a private GitHub Release manifest and bundle.
- [ ] Implement browser-based authentication for pilot users.
- [ ] Verify signatures before installation.
- [ ] Implement staged update and rollback.
- [ ] Validate on at least one BCG Windows device and one BCG X device.

## Phase 3 - Pilot

- [ ] Onboard approximately ten users.
- [ ] Measure time-to-first-success and support demand.
- [ ] Capture failure modes without collecting client content.
- [ ] Prioritize the first shared skills from observed work.
- [ ] Decide whether the next distribution channel remains GitHub or moves to BCG infrastructure.

## Later, only with evidence

- Additional agents and skills.
- More runtime adapters.
- Corporate distribution channel.
- Extension SDK or marketplace.
- Optional UI.
- Organization-level shared knowledge layer.
