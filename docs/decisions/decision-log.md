# Project decision log

This is the canonical record of durable product, architecture, security, data and development decisions. It is not a changelog or task tracker.

Codes contain exactly four uppercase letters. They are globally unique, permanent and non-sequential. A mnemonic is convenient but carries no authoritative meaning. Entries are append-only; later decisions supersede earlier ones through a new code.

Never include secrets, credentials, personal data, client-identifying context or case content.

## Foundation snapshot - 2026-07-19

This is a frozen milestone for navigation, not a separate decision, live index or status report. The coded entries below remain authoritative; future decisions do not require this table to be rewritten.

| Theme | Current baseline | Canonical decisions |
|---|---|---|
| Scope and audience | Professional work only; serve classic consultants, BCG X, data scientists and engineers progressively from observed needs. | `WORK`, `USER` |
| User experience and distribution | Make `bcgos` CLI-first; distribute versioned private releases rather than Git clones; keep CLI and bundle updates separately validated and reversible. | `CLIF`, `RELS`, `UPDT` |
| Architecture and trust | Separate managed core from local/client data; prefer a thin Go CLI; validate Windows first while supporting macOS/Linux; verify artifacts and credentials safely. | `DATA`, `GOCL`, `WNDS`, `SECU` |
| Runtime portability | Keep canonical contracts runtime-neutral, with Claude primary and Codex semantically compatible through thin adapters. | `PORT` |
| Development model | Keep the development harness outside the product; use specs, four-letter decisions and contract-focused tests; guide novice contributors through safe branches, validation and human-reviewed PRs. | `HARN`, `NOVC` |

## WORK - Keep the OS professional-only

- Date: 2026-07-17
- Status: accepted
- Owner: Daniel Scardini
- Context: The BCG Brasil Agentic OS needs a clear domain boundary.
- Decision: The OS serves work at BCG Brasil; personal-life domains are out of scope.
- Consequences: Product defaults, examples and future agents remain professional-only.
- Refs: specs/000-foundation.md
- Supersedes: none

## USER - Serve classic and technical consultants progressively

- Date: 2026-07-17
- Status: accepted
- Owner: Daniel Scardini
- Context: The long-term audience spans classic consulting, BCG X, data science and engineering.
- Decision: Record all target personas now, but add workflows only from observed needs.
- Consequences: The foundation stays small while preserving a broad product direction.
- Refs: README.md; specs/003-pilot-success.md
- Supersedes: none

## CLIF - Make the product CLI-first

- Date: 2026-07-17
- Status: accepted
- Owner: Daniel Scardini
- Context: The pilot includes users who are not versed in agents or software development.
- Decision: Installation, initialization, diagnosis and update are product capabilities from the first pilot through the working CLI name `bcgos`.
- Consequences: Low-friction user experience is an architectural requirement, not later polish.
- Refs: specs/001-cli-distribution.md
- Supersedes: none

## RELS - Distribute releases rather than Git clones

- Date: 2026-07-17
- Status: accepted
- Owner: Daniel Scardini
- Context: Users should not need Git knowledge to install or update the OS.
- Decision: Contributors clone the private source repository; pilot users consume versioned private GitHub Release artifacts.
- Consequences: The repository is the factory and signed release artifacts are the product distribution unit.
- Refs: specs/001-cli-distribution.md
- Supersedes: none

## UPDT - Separate CLI and bundle update transactions

- Date: 2026-07-17
- Status: accepted
- Owner: Daniel Scardini
- Context: The user needs one simple update experience while CLI and OS bundle evolve independently.
- Decision: `bcgos update` is one user action backed by separate validated and reversible CLI and bundle transactions.
- Consequences: Version compatibility, staging and rollback must be explicit.
- Refs: specs/001-cli-distribution.md
- Supersedes: none

## DATA - Separate managed core from work data

- Date: 2026-07-17
- Status: accepted
- Owner: Daniel Scardini
- Context: Client work and local state cannot compete with product updates or enter distributed artifacts.
- Decision: Install the managed core in user-level application storage; place only minimal regenerable metadata and adapters in workspaces.
- Consequences: Memory, credentials, logs and client content remain outside bundles and are never overwritten by updates.
- Refs: specs/002-data-boundaries.md
- Supersedes: none

## GOCL - Prefer Go for the user CLI

- Date: 2026-07-17
- Status: accepted
- Owner: Daniel Scardini
- Context: Pilot users should not install language runtimes or developer dependencies.
- Decision: Implement a thin Go CLI, subject to validation on corporate devices.
- Consequences: Cross-platform static artifacts and user-space installation become primary engineering constraints.
- Refs: specs/001-cli-distribution.md; specs/003-pilot-success.md
- Supersedes: none

## WNDS - Treat Windows as the primary pilot platform

- Date: 2026-07-17
- Status: accepted
- Owner: Daniel Scardini
- Context: Classic consultants are expected to use Windows while technical users may also use macOS or Linux.
- Decision: Validate Windows first while keeping macOS and Linux in the supported build matrix.
- Consequences: Normal installation must not require administrator privileges.
- Refs: specs/003-pilot-success.md
- Supersedes: none

## SECU - Make release trust part of the MVP

- Date: 2026-07-17
- Status: accepted
- Owner: Daniel Scardini
- Context: The CLI will download and activate code and hooks in BCG work environments.
- Decision: Verify release manifests and artifacts before execution and store credentials in operating-system credential stores.
- Consequences: Checksums alone are not the final trust model; signing and corporate device constraints require validation.
- Refs: specs/001-cli-distribution.md; specs/002-data-boundaries.md
- Supersedes: none

## PORT - Keep Claude primary and Codex semantically compatible

- Date: 2026-07-17
- Status: accepted
- Owner: Daniel Scardini
- Context: The OS will primarily run on Claude but must operate on Codex with maximum practical compatibility.
- Decision: Keep canonical contracts runtime-independent and use thin adapters to preserve observable invariants across Claude and Codex.
- Consequences: Capabilities are classified as native, emulated, degraded or unavailable; critical enforcement never degrades silently.
- Refs: specs/004-runtime-portability.md
- Supersedes: none

## HARN - Establish a development-only harness

- Date: 2026-07-17
- Status: accepted
- Owner: Daniel Scardini
- Context: Multiple contributors and agents need recoverable reasoning and regression protection while developing the solution.
- Decision: Use this project decision log and contract-focused unit tests as the development backbone. Behavioral work follows decision or spec, failing test, minimal implementation and deterministic validation.
- Consequences: A cross-platform Go harness and runtime-neutral development skills live only under development paths and never enter the CLI, OS bundle or pilot installation.
- Refs: specs/005-development-harness.md; dev/skills/develop-change/SKILL.md; dev/skills/record-decision/SKILL.md
- Supersedes: none

## NOVC - Make source contribution novice-safe

- Date: 2026-07-18
- Status: accepted
- Owner: Daniel Scardini
- Context: Pilot contributors may have little or no Git and software-development experience, but must still contribute without risking local work, secrets, client content or the main branch.
- Decision: Provide progressive development skills, plain-language Claude guidance, deterministic doctor and recovery commands, repository-owned Git hooks, CI validation and human-reviewed pull requests as one layered contribution path.
- Consequences: Contributors receive one safe next action at a time; destructive Git, direct main changes and autonomous merge are blocked; local enforcement remains backed by remote review and CI.
- Refs: specs/005-development-harness.md; .claude/README.md; dev/skills/start-contributing/SKILL.md
- Supersedes: none

## DUAL - Support Windows and macOS equally

- Date: 2026-07-19
- Status: accepted
- Owner: Daniel Scardini
- Context: The pilot must support both classic and technical BCG users, whose corporate devices include Windows and macOS.
- Decision: Treat Windows and macOS as equal first-class pilot platforms with equivalent observable contracts for installation, initialization, diagnosis, update, rollback and data preservation.
- Consequences: Release artifacts and acceptance tests must pass on both platforms before a capability is pilot-ready. Platform-specific mechanics may differ. Linux remains a supported build and development target but is not an initial pilot parity requirement.
- Refs: README.md; ROADMAP.md; specs/003-pilot-success.md
- Supersedes: WNDS

## BOOT - Start contribution through one guided bootstrap

- Date: 2026-07-19
- Status: accepted
- Owner: Daniel Scardini
- Context: A contributor with little Git or development experience needs to obtain access, clone the private repository and reach a safe first session without translating an engineering checklist alone.
- Decision: Begin contributor onboarding with one shareable agent prompt for prerequisite, authentication and clone guidance, then transfer control to deterministic repo-owned platform bootstrap and canonical development skills.
- Consequences: Credentials never enter chat or repository files; software installation and authentication require human confirmation; bootstrap stops before feature work. Windows is implemented first for Marcelo, with a macOS counterpart tracked separately.
- Refs: docs/onboarding/windows-contributor-prompt.md; dev/bootstrap/windows.ps1; specs/005-development-harness.md
- Supersedes: none

## MEMO - Persist memory through governed layers

- Date: 2026-07-19
- Status: accepted
- Owner: Daniel Scardini
- Context: A professional second brain needs continuity across sessions without injecting raw history into every prompt or allowing updates to rewrite user and client work.
- Decision: Adopt a runtime-neutral memory pyramid with L1 daily memory, L2 weekly rollups, L3 rolling thematic memory and a separately curated lifetime index. A dreaming pipeline promotes information upward through staged, validated and idempotent transformations while preserving source layers and the last known-good outputs.
- Consequences: Memory remains user-local and workspace-isolated; managed releases ship schemas and policy only. Context injection follows explicit budgets and drill-down pointers. Claude and Codex adapters consume the same policy, and automation, retention, provider choice and user-facing commands remain separately testable decisions.
- Refs: specs/006-memory-persistence.md; bundles/base/memory/policy.json; internal/memory/policy_test.go
- Supersedes: none

## DREM - Consolidate lifetime memory weekly

- Date: 2026-07-19
- Status: accepted
- Owner: Daniel Scardini
- Context: Daily continuity and durable learning have different cost, depth and stability requirements. Promoting lifetime memory on every session or day would create churn, while waiting for manual consolidation would weaken persistence for non-technical users.
- Decision: Run a light daily dreaming cycle that maintains L1 only, and a deep weekly dreaming cycle that consolidates the week's evidence through L2 and L3 into governed lifetime updates. The weekly cycle may activate eligible lifetime updates automatically, but every update requires provenance, version history and validation and may not overwrite lifetime state in place.
- Consequences: Weekly deep dreaming becomes the owner of lifetime consolidation. Hooks, schedulers and manual commands remain interchangeable triggers for the same idempotent operation; missed weekly runs require presence-based catch-up. Eligibility, retention and exact scheduling remain policy decisions before execution is enabled.
- Refs: MEMO; specs/006-memory-persistence.md; bundles/base/memory/policy.json; bundles/base/skills/dream-memory/SKILL.md; internal/memory/engine.go; internal/memory/engine_test.go
- Supersedes: none

## ATOM - Commit generated memory as one atomic view

- Date: 2026-07-20
- Status: accepted
- Owner: Daniel Scardini
- Context: A weekly deep dream generates L2, L3 and sometimes lifetime together. Replacing layer files sequentially can expose a mixed state after a process or machine interruption, while per-period locks allow different weeks to race over shared L3 and lifetime state.
- Decision: Publish generated artifacts as immutable transaction versions and make them visible only through one validated, durable commit manifest. Serialize all dreaming activation for a workspace with one fail-closed lock. Idempotency and context readers recognize only artifacts reachable from the newest fully valid commit.
- Consequences: Readers see the old complete view or the new complete view, never a partial combination. Interrupted pre-commit versions remain invisible and diagnosable; prior manifests preserve history. Recovery and orphan cleanup belong to a future explicit doctor flow rather than automatic deletion.
- Refs: MEMO; DREM; specs/006-memory-persistence.md; schemas/memory-commit.schema.json; internal/memory/store.go; internal/memory/engine_test.go
- Supersedes: none

## MCLI - Connect safe memory operations before synthesis

- Date: 2026-07-20
- Status: accepted
- Owner: Daniel Scardini
- Context: The memory engine needs a real CLI connection, but synthesis provider, lifetime eligibility, approved application directories and default context budgets are still open decisions. Inventing temporary defaults would silently turn unresolved governance into product behavior.
- Decision: Connect sanitized capture, status and bounded context assembly to `bcgos memory` now. Expose daily and weekly dream commands as a machine-readable unavailable capability until approved synthesis and eligibility adapters exist. Require the data directory and per-layer budgets explicitly until `bcgos init` owns versioned configuration.
- Consequences: The CLI exercises real persisted state without embedding a provider or unsafe fallback. Pilot UX still requires init/configuration and adapters before dreaming is usable. JSON output provides a stable seam for Claude and Codex adapters.
- Refs: specs/001-cli-distribution.md; specs/006-memory-persistence.md; cmd/bcgos/main.go; internal/cli/cli.go; internal/cli/cli_test.go
- Supersedes: none

## WIKI - Navigate content through a compiled LLM wiki

- Date: 2026-07-20
- Status: accepted
- Owner: Daniel Scardini
- Context: The Agentic OS needs a persistent and progressively improving way for users and agents to navigate professional content, including governed memory, without re-deriving knowledge from raw sources on every query or injecting an entire corpus into each session.
- Decision: Adopt a Karpathy-inspired compiled LLM wiki as the primary content-navigation model. Original sources and governed canonical artifacts remain authoritative; the wiki is a derived, interconnected and regenerable knowledge layer built from explicit allowlists. It may navigate owner and workspace memory through scoped pointers to valid L1, L2, L3 and lifetime artifacts, but it may not turn private memory into shared or distributed content.
- Consequences: Managed product content and private owner/workspace content use physically separate atlas roots, pipelines and access policies. Dreaming remains the only producer of memory rollups; the private wiki compiles topic, entity and time navigation over valid rollups and preserves drill-down pointers to them. Generated indexes, backlinks, provenance, freshness, invalidation, orphan detection and lint support navigation. Session context receives intent-routed wiki pointers rather than the complete corpus. Source correction or deletion must invalidate affected derived entries. The first implementation is limited to the managed product atlas; private memory and rollup navigation waits for approved owner-context, storage, privacy and deletion contracts.
- Refs: specs/006-memory-persistence.md; specs/007-content-navigation.md; ROADMAP.md
- Supersedes: none

## OKFP - Publish wiki views as governed OKF bundles

- Date: 2026-07-20
- Status: accepted
- Owner: Daniel Scardini
- Context: The compiled wiki needs a human-readable, agent-readable and portable representation, while the Agentic OS still requires stronger authority, privacy, freshness, invalidation and atomic-update guarantees than a minimal exchange format provides.
- Decision: Represent each managed, owner-private and workspace-private atlas as a separate Open Knowledge Format v0.1 bundle, extended by a versioned BCGOS Atlas Profile. OKF owns the portable Markdown, YAML frontmatter, concept-path identity, standard links, `index.md` and `log.md` conventions. The BCGOS profile owns scoped metadata, policy enforcement, transactional update events, revocation barriers, validation and atomic publication.
- Consequences: Three atlas roots remain physically separate and V1 permits no cross-bundle links. BCGOS extension keys use the `x-bcgos-` namespace and consumers preserve unknown OKF fields. Managed bundles use Git and review; private bundles use local versioned storage and metadata-safe logs rather than Git by default. Session start only reads a valid authorized view and never compiles it. Deletion or access revocation takes effect synchronously through a denial barrier and always overrides last-known-good preservation.
- Refs: WIKI; specs/007-content-navigation.md; specs/008-wiki-update-okf.md; specs/006-memory-persistence.md
- Supersedes: none

## SCHD - Recover scheduled work on presence

- Date: 2026-07-21
- Status: accepted
- Owner: Daniel Scardini
- Context: Corporate Windows and macOS laptops may be asleep, powered off, offline or unauthenticated when a recurring memory or wiki job is scheduled. Treating an exact OS wake-up as the consistency boundary would make maintenance fragile and runtime-dependent.
- Decision: The native scheduler accelerates execution but does not own consistency. The Agentic OS derives missed work from durable local state and recovers it on the next authorized presence trigger. Native OS schedulers, Claude/Codex lifecycle adapters and manual commands invoke the same idempotent core; Session Start remains read-only and never runs model work synchronously.
- Consequences: Windows and macOS may use different per-user wake-up mechanics while preserving one observable contract. Enrollment prevents historical backfill, catch-up is bounded, and only a successful owning-subsystem commit satisfies an occurrence. Failed or unavailable work remains recoverable. Exact windows, unattended model permission, retry/backoff, budgets and native adapter installation remain configurable follow-up decisions.
- Refs: DREM; PORT; specs/006-memory-persistence.md; specs/008-wiki-update-okf.md; specs/009-scheduler-catch-up.md; schemas/scheduler-state.schema.json; internal/scheduler/scheduler.go
- Supersedes: none

## DOCL - Make Docling the local ingestion substrate

- Date: 2026-07-22
- Status: accepted
- Owner: Daniel Scardini
- Context: The pilot serves users who should not install Python, manage models or provide an API key merely to ingest professional material. At the same time, document extraction needs a structured, multimodal, privacy-preserving default that can support later memory and wiki workflows.
- Decision: Use Docling as the default local extraction substrate for supported ingestion intents. Distribute it as a separately versioned, managed per-platform ingestion runtime pack invoked by the thin `bcgos` CLI, not as a prerequisite exposed to users or an implicit remote service. Treat `standard`, `advanced` and `power` as progressive-disclosure preferences; no profile grants automatic access to a provider or bypasses policy.
- Consequences: The standard route is local and keyless. Docling runs before approved deterministic fallbacks, and remote models/providers require explicit selection, policy approval and OS-managed credentials. The pack must be verified, preflighted and tested on Windows and macOS for size, first-use downloads, offline behavior and corporate-network compatibility before implementation is claimed.
- Refs: CLIF; DATA; PORT; specs/010-local-ingestion-runtime.md; bundles/base/skills/ingest-content/SKILL.md
- Supersedes: none

## PROF - Keep one user interaction profile across the Agentic OS

- Date: 2026-07-23
- Status: accepted
- Owner: Daniel Scardini
- Context: The pilot spans people with very different technical fluency. A skill-by-skill profile would create inconsistent language and suggestions, while embedding the preference in memory, projects or client content would make it stale, duplicated and difficult to correct.
- Decision: Store one self-declared, user-local interaction profile (`standard`, `advanced` or `power`) as a canonical configuration parameter. Every product skill and runtime adapter resolves that same parameter before choosing language, explanation depth and optional technical suggestions. The profile controls progressive disclosure and communication only; it never grants permissions, changes data boundaries, enables providers or becomes memory.
- Consequences: `bcgos init` creates the default profile and `bcgos profile show|set` lets the user inspect or change it. Bundles consume the managed profile policy; owner/workspace brains and memory receive only a bounded profile pointer when an adapter is available. Future skills must use the canonical profile rather than redefine persona tiers.
- Refs: specs/011-interaction-profile.md; bundles/base/profile/policy.json; internal/profile; bundles/base/skills/interaction-profile/SKILL.md
- Supersedes: none
