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

## WIZR - Give the signed installer one branded, user-space wizard

- Date: 2026-07-29
- Status: accepted
- Owner: Daniel Scardini
- Context: Pilot users can install software in their corporate user profile but may not have administrator permissions or technical context. The release trust and rollback contracts already exist, but the first visual interaction is still missing.
- Decision: The Maestro installer uses one dependency-free, cross-platform visual wizard with a midnight/teal/gold identity derived from the Maestro conductor mark. It explains four states (welcome, verification, installation and ready), states the user-space/no-admin boundary, and delegates every trust-bearing operation to the signed `bcgos-bootstrap` process. Static assets are deterministic SVG and the wizard never provides an unsigned fallback.
- Consequences: A future executable bridge can reuse the same screens and status vocabulary on Windows and macOS. The visual layer can be reviewed independently, while pilot readiness still requires signed artifacts, native signing/notarization and clean-device evidence. The global PATH, workspaces and owner data remain outside the installer transaction.
- Refs: installers/wizard; docs/installer-wizard.md; specs/020-release-distribution.md; specs/022-guided-pilot-release.md
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

## SKIX - Compile a managed skills index for bounded session navigation

- Date: 2026-07-23
- Status: accepted
- Owner: Daniel Scardini
- Context: A session needs to discover available operating procedures without loading every SKILL.md or relying on model recollection. Maintaining a separate hand-written summary would drift from the managed bundle.
- Decision: Compile a deterministic managed skills index from canonical product SKILL.md frontmatter and runtime metadata. Ship compact JSON and human-readable Markdown views as derived bundle artifacts. The index provides identity, trigger summary, default prompt and pointer only; it does not copy complete skill instructions, client data, profile state or execution history.
- Consequences: `bcgos skills index` may expose the same catalog for inspection. Development validation rejects stale generated artifacts, and a dedicated generator refreshes them when product skills change. Future Session Start consumes a bounded pointer to this catalog before reading any individual skill.
- Refs: specs/012-skills-index.md; bundles/base/skills/catalog.json; bundles/base/skills/INDEX.md; internal/skillsindex
- Supersedes: none

## OWNR - Keep owner context local, human-readable and pointer-based

- Date: 2026-07-24
- Status: accepted
- Owner: Daniel Scardini
- Context: Session Start needs a durable SELF and operating state, but treating either as memory would make explicit self-definition hard to inspect and correct.
- Decision: Store owner context in user-local Markdown surfaces with a small machine-readable registry of pointers. SELF and operating state are human-authored, never distributed, and runtime consumers receive pointers and availability diagnostics before reading content under an explicit budget.
- Consequences: `bcgos owner init|status` owns the minimal local surface. Owner context remains separate from workspace content, client data, profile preference, skills index and memory rollups. Tasks are a future governed pointer rather than a local task taxonomy invented by initialization.
- Refs: specs/013-owner-context.md; internal/ownerctx
- Supersedes: none

## SELF - Make the professional self facet-based, consent-aware and auditable

- Date: 2026-07-24
- Status: accepted
- Owner: Daniel Scardini
- Context: A monolithic SELF cannot distinguish collaboration preferences from external voice, durable work rules or sensitive psychological material. The Agentic OS needs a useful cold start and continuous refinement without opaque inference or broad injection of private data.
- Decision: Represent owner context as local professional facets with declared sensitivity, permitted readers and refinement policy. Cold start asks only non-sensitive facets and always shows answers before a write. Psychological-profile is optional, local and restricted to explicitly authorized professional purposes such as Walter calibration. Voice, communication style and preferences may later refine automatically only with evidence, a visible change record and reversal; decision rules require a proposal and sensitive facets require confirmation. Importing assessment reports remains a separate consented local-adapter capability.
- Consequences: Session composition can request narrowly scoped owner pointers rather than a full SELF. The current CLI can initialize, inspect and expose the interview contract without pretending to ingest reports or run automated refinement. Future adapters must honor declared reader and refinement policies, record provenance and fail closed when unavailable.
- Refs: specs/013-owner-context.md; internal/ownerctx; internal/cli/cli.go; docs/OPEN-QUESTIONS.md
- Supersedes: none

## REFI - Enforce self refinement through a local proposal and audit core

- Date: 2026-07-24
- Status: accepted
- Owner: Daniel Scardini
- Context: Declaring a self-refinement policy is insufficient if a future hook or model can write directly to owner files. The system needs automatic learning for eligible facets without opaque edits, and guarded facets must remain owner-controlled.
- Decision: Route every automated or adapter-produced refinement through a local proposal core containing facet, evidence summary and proposed body. The core applies only `automatic_with_audit` facets automatically, writes a protected before-version and audit receipt, and requires explicit confirmation for all other policies. Every applied change is explicitly reversible. Observation capture and model synthesis are separate unprivileged producers; they cannot edit self files directly.
- Consequences: Voice, communication style and preferences can learn automatically as soon as an approved producer exists. Decision rules, boundaries and psychological profile stay protected even after hooks arrive. CLI users and runtime adapters share one observable contract, while sensitive proposal bodies remain in local protected storage and receipts omit them.
- Refs: specs/013-owner-context.md; internal/ownerctx/refinement.go; internal/ownerctx/ownerctx_test.go; internal/cli/cli.go
- Supersedes: none

## AUTC - Require an authorized producer and conflict-safe reversal for automatic self changes

- Date: 2026-07-24
- Status: accepted
- Owner: Daniel Scardini
- Context: A policy label alone cannot prove that a proposal came from an approved runtime adapter, and a simple restoration can erase an intervening self update. Both failures would make automatic learning opaque despite an audit file.
- Decision: An automatic refinement requires an owner-authorized producer ID plus a local capability whose hash is stored in the owner registry. A proposal without that capability remains proposed until the owner confirms it. Before applying a change, persist a protected before-version and a prepared audit journal; only then write the facet. Reversal must compare the current facet hash to the original after-hash, journal a reversion event before writing, and fail on conflict rather than overwrite newer content.
- Consequences: Direct CLI submission is reviewable but cannot auto-apply merely by naming an automatic facet. Future adapters receive narrowly scoped capabilities through an approved private credential surface. Interrupted metadata writes remain diagnosable and every successful change has a prior audit journal. Reverting an older change requires an explicit new resolution if the facet has evolved.
- Refs: specs/013-owner-context.md; internal/ownerctx/refinement.go; internal/ownerctx/ownerctx_test.go
- Supersedes: REFI

## ATLS - Adopt a scoped human atlas alongside canonical memory

- Date: 2026-07-24
- Status: accepted
- Owner: Daniel Scardini
- Context: The Agentic OS needs professional knowledge that people can navigate and correct directly, while its canonical memory must remain compact, derived and safe for budgeted continuity. A contributor proposal supplied a useful entity-oriented taxonomy, but a single atlas or workspace would violate the existing boundaries among managed content, owner-private context and client/workspace data.
- Decision: Adopt a human-readable, Markdown-first atlas as the authoritative navigation and correction surface, physically separated into managed, owner-private and workspace-private roots. Canonical memory L1/L2/L3/lifetime remains derived from approved sanitized signals and points back to authoritative atlas sources; it never becomes the record of a client, project or person. Map owner profile material to `owner/self/`; keep tasks as governed pointers until a task-source contract exists; let daily human logs enter memory only through approved sanitization; and treat human folder indexes as orientation pages while the wiki/OKF layer remains the derived machine navigation authority.
- Consequences: The implementation can reuse entity-oriented segments such as clients, projects, concepts, learnings, daily, people and development without collapsing privacy scopes or duplicating sources of truth. The Owner Context, memory, wiki and future Session Packet have one compatible routing model. Taxonomy templates, lifecycle writers and the private-atlas bootstrap remain implementation follow-up work; no client, person or assessment data belongs in the managed bundle or repository.
- Refs: specs/006-memory-persistence.md; specs/007-content-navigation.md; specs/008-wiki-update-okf.md; specs/013-owner-context.md; ROADMAP.md
- Supersedes: none

## SIGN - Define the future composition of L1 from conversation and daily-log signals

- Date: 2026-07-24
- Status: accepted
- Owner: Daniel Scardini
- Context: A human daily log preserves deliberate operational context that a conversation transcript may miss, while Claude/Codex session logs capture decisions and collaboration signals that might never be written into a daily page. Treating either source alone as L1 would create a partial continuity layer; copying either raw source would violate the memory boundary.
- Decision: The future L1 model will combine two workspace-scoped inputs: selected sanitized signals from human daily logs and selected sanitized signals from Claude/Codex conversation adapters. Before a daily-log signal may enter the memory engine, the capture contract must be extended with source kind, provenance and verifiable sanitization attestation; a self-declared CLI `--sanitized` flag is insufficient. L1 remains bounded, append-only and derived; it is not a mirror of a daily page or a session transcript.
- Consequences: The human atlas and runtime adapters can contribute to continuity without becoming memory themselves only after that contract extension and its adapter tests exist. Until then, daily logs remain human-readable sources and cannot be ingested into L1. The existing core remains provider-neutral; capture-contract extension, adapter implementation and exact selection/retention policy remain follow-up work.
- Refs: specs/006-memory-persistence.md; specs/014-human-atlas-bootstrap.md; internal/memory; bundles/base/skills/dream-memory/SKILL.md
- Supersedes: none

## PACK - Establish a bounded Session Context Packet before runtime hooks

- Date: 2026-07-24
- Status: accepted
- Owner: Daniel Scardini
- Context: Claude and Codex need the same initial orientation, but direct Session Start injection would risk loading private SELF, client or memory content before native lifecycle adapters can resolve authorization, purpose and context budget consistently.
- Decision: Define a local, runtime-neutral Session Context Packet that exposes only bounded states and pointers for interaction profile, workspace, session-readable owner facets, operating state, atlas availability and the managed skills catalog. It must explicitly report unavailable memory injection and omitted sources, and may not include source bodies or Walter-only facets. Native adapters remain responsible for authorized reading and actual injection.
- Consequences: `bcgos session packet` gives future adapters one testable input without claiming a product hook exists. Claude and Codex can implement different mechanics over the same packet and failure states. Memory, tasks, private atlas content and sensitive owner facets stay unavailable or pointer-only until their separate contracts are implemented.
- Refs: specs/004-runtime-portability.md; specs/011-interaction-profile.md; specs/012-skills-index.md; specs/013-owner-context.md; specs/015-session-context-packet.md; internal/sessionctx
- Supersedes: none

## MAES - Name the professional product Maestro

- Date: 2026-07-24
- Status: accepted
- Owner: Daniel Scardini
- Context: The product needs a memorable human name for contributors and future pilot users, while the private repository, current CLI command and distribution mechanics are already referenced by existing development contracts.
- Decision: Name the BCG Brasil professional Second Brain product **Maestro**. Preserve `bcg-brasil-agentic-os` as the repository identifier and `bcgos` as the technical CLI command during the foundation and pilot-design phase. A future installer and user-facing command migration require a separate compatibility and distribution decision.
- Consequences: Product documentation presents Maestro as the user-facing name without breaking current clone, CI, release or CLI references. No command, GitHub organization, release artifact or local-data path changes in this decision.
- Refs: README.md; specs/001-cli-distribution.md; ROADMAP.md
- Supersedes: none

## EXEC - Materialize resumable execution without creating a task authority

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: Long-running agent work must survive session and agent changes without copying the full contract, transcript or evidence back into model context. The existing task pointer deliberately remains unavailable until an authoritative task-source contract exists.
- Decision: Add a workspace-scoped local execution ledger whose immutable execution items, attempts, bounded checkpoints and evidence-backed completion state are distinct from business tasks. V1 items use `local_execution` authority only. Runtime and Session Context consumers receive opaque pointers before any separately authorized read.
- Consequences: The ledger may track execution state but never owns business priority, due date, owner or external task status. Every mutation is revision-checked, takeover invalidates the prior attempt, completion requires core-witnessed evidence, and persisted history excludes prompts, responses, tool payloads, absolute paths and client bodies. Task-provider synchronization, generic tracing, unattended execution and evaluator plugins remain outside V1.
- Refs: specs/029-execution-ledger-v1.md; schemas/execution-state.schema.json; internal/execution
- Supersedes: none

## TCAL - Trace tool lifecycle without persisting tool payloads

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: Resumable execution needs enough breadcrumbs to show which agent invoked which tool and whether the call finished, while prompts, arguments, queries, output and errors may contain contracts, credentials or professional content that must not become trace data.
- Decision: Record only an immutable tool-call lifecycle linked to the current execution item and fenced attempt: a runtime identity and tool class from closed canonical registries, opaque call ID, timestamps and `started`, `succeeded`, `failed` or `unavailable` state. Every event advances the execution revision. Tool arguments, prompts, stdout, stderr, error bodies and payload digests remain forbidden.
- Consequences: Explicit export can reconstruct ordering and unresolved calls without replaying model context or tool content. Free-form IDs cannot become a content channel. Until native adapters emit the events, CLI receipts are declared breadcrumbs rather than authenticated runtime provenance. The ledger is not an observability backend, event bus, cost tracker or process sandbox.
- Refs: EXEC; specs/018-execution-ledger-v1.md; schemas/execution-state.schema.json; internal/execution/toolcall.go
- Supersedes: none

## CLVE - Wire the Claude lifecycle vertical behind neutral contracts

- Date: 2026-07-26
- Status: accepted
- Owner: Daniel Scardini
- Context: Maestro has a bounded Session Start command and portable lifecycle vocabulary, but a pilot cannot infer support from configuration alone or allow a safety guard to depend on workspace inspection.
- Decision: Wire Claude SessionStart, UserPromptSubmit, PreToolUse, PostToolUse and Stop to the canonical lifecycle. Parse pre-action input under a fixed bound before workspace access and return a native fail-closed denial for malformed, oversized or unevaluable input. Keep post-action and stop asynchronous and persist only validated metadata receipts. Leave every capability unavailable until qualifying native-session evidence exists.
- Consequences: Claude has a complete locally installable lifecycle vertical without turning hooks into workers or weakening its native permission flow. The protected-root rule recognizes only a bounded simple-command grammar, receipt path components are validated, Codex remains incomplete, and capability promotion still requires the pilot conformance protocol.
- Refs: specs/004-runtime-portability.md; specs/019-nonblocking-hook-execution.md; specs/021-pilot-hook-conformance.md; specs/025-native-session-start-hook.md; specs/026-workspace-local-adapter-installation.md; specs/030-claude-lifecycle-vertical.md; internal/claudeadapter; internal/lifecycle
- Supersedes: none

## LIFE - Separate adapter receipts from native lifecycle evidence

- Date: 2026-07-27
- Status: accepted
- Owner: Daniel Scardini
- Context: A metadata-only receipt can prove that the Maestro adapter command executed, but the same command can be invoked directly outside a native Claude or Codex session. Calling that receipt native would collapse local configuration, harness execution and runtime observation into one untrustworthy state.
- Decision: Persist lifecycle receipts with the explicit provenance `adapter_command`, meaning a bounded Maestro adapter command produced the receipt but native runtime invocation remains unverified. `bcgos doctor` must diagnose adapter-command receipts separately from native conformance. Native capability promotion requires a runtime/platform conformance record under the pilot protocol, never a local receipt, configuration entry or direct command. Environment probes may report blockers but cannot alter the capability manifest.
- Consequences: Post-action and Stop receipts remain useful for bounded diagnostics without overstating runtime evidence. Claude and Codex continue `unavailable` until their own fresh-session proof exists. Future trusted native attestation requires a separate producer and contract rather than adding an unverified flag to the hook command.
- Refs: CLVE; TCAL; specs/004-runtime-portability.md; specs/020-adapter-diagnostics.md; specs/021-pilot-hook-conformance.md; specs/030-claude-lifecycle-vertical.md; internal/lifecycle
- Supersedes: none

## AUTO - Prebuild a cross-platform maintenance plane in the base bundle

- Date: 2026-07-28
- Status: accepted
- Owner: Daniel Scardini
- Context: Daniel explicitly expanded the initial three-job scheduler discussion into a request for the maximum safe set of daily improvements: memory sync and rollups, wiki maintenance, runtime health, self observation and update diagnosis. Maestro must offer that sense of continuous improvement without copying personal, client-specific or macOS-only automation into a platform-neutral product bundle.
- Decision: The base bundle carries a declarative catalog of 14 universal maintenance contracts: sanitized L1 capture, daily and weekly memory cycles, retention checks, incremental wiki sync, wiki reconciliation and integrity checks, skills-index refresh, runtime health and drift checks, capability rechecks, self-observation proposals and update checks. This is an explicit catalog expansion, not activation: every initial job remains unavailable until its owning subsystem and executor are qualified. The catalog contains no operating-system schedule, local path, credential, provider command or activation grant. A presence/catch-up wake mechanism is a runtime adapter, not a catalog job. macOS LaunchAgents and Windows Task Scheduler definitions are disabled reference templates; a future installer may render and enable them only after qualification. Client integrations, Kowalski/Darwin governance, auto-commit, telemetry aggregation, briefs and external ingest providers remain optional packs or outside Maestro.
- Consequences: Deterministic read-only checks and approved local synchronization can run automatically when their adapters are available. Model-backed dreaming, self refinement and external ingestion remain prebuilt but fail closed until the user/workspace policy and runtime adapter explicitly permit unattended execution. Disabled templates cannot create recurring failure noise, and scheduler receipts never prove memory or wiki publication; each owning subsystem retains its durable success boundary.
- Refs: specs/006-memory-persistence.md; specs/007-content-navigation.md; specs/008-wiki-update-okf.md; specs/009-scheduler-catch-up.md; specs/020-release-distribution.md; bundles/base/runtime/maintenance.json; schemas/maintenance-jobs.schema.json
- Supersedes: none

## CAPS - Separate professional capability bundles from interaction profile

- Date: 2026-07-26
- Status: accepted
- Owner: Daniel Scardini
- Context: Maestro serves classic consultants, technical explorers, software engineers, data scientists and data engineers. A single base skills catalog would either overload nontechnical users or make technical workflows invisible, while reusing the interaction profile for role selection would turn a communication preference into identity and authority.
- Decision: Maintain an explicit source catalog of professional capability bundles, independent from the canonical interaction profile. `base` serves consulting; `engineering-core` serves technical explorers and software engineering; `data-practice` serves data science and data engineering and depends on engineering core. Optional bundles remain explicitly unavailable until a separately versioned release, compatibility and local activation contract exists. Track planning may inspect dependencies but may not persist selection, install a bundle or change workspace state.
- Consequences: Professional skills can be authored, cataloged and harness-validated now without shipping them in the base distribution or misrepresenting them as active. Capability tracks never grant permissions, data scope, provider access or approval. Future onboarding must obtain explicit confirmation and use a verified reversible activation transaction.
- Refs: USER; PROF; SKIX; RELS; specs/012-skills-index.md; specs/020-release-distribution.md; specs/035-professional-capability-bundles.md; bundles/catalog/catalog.json; internal/capabilitybundle
- Supersedes: none

## ORCH - Keep Maestro completion in the execution ledger

- Date: 2026-07-26
- Status: accepted
- Owner: Daniel Scardini
- Context: Maestro needs long-running goals, Walter review and recoverable completion, while the workspace-scoped execution ledger already owns immutable revisions, fenced attempts, evidence and crash recovery. A second goal store would create competing completion authorities and divergent recovery rules.
- Decision: Model a Maestro goal as a governed view over one canonical `local_execution` item. The execution ledger remains the only durable completion authority. Maestro may add typed review receipts and orchestration projections to that ledger, but may not create an independent goal state machine, lock, event head or completion store.
- Consequences: Existing execution revisions, attempts, checkpoints and core-witnessed evidence are reused. A current authenticated Walter approval may become an additional completion condition, but cannot replace evidence or grant external authority. Runtime adapters expose the capability only after authenticated review conformance exists.
- Refs: EXEC; TCAL; specs/029-execution-ledger-v1.md; specs/031-maestro-goal-orchestration.md
- Supersedes: none

## CANY - Keep pilot observability local and closed

- Date: 2026-07-26
- Status: accepted
- Owner: Daniel Scardini
- Context: The two-user canary needs time-to-value, resumption, lifecycle, intervention and capability-failure signals without turning professional work into telemetry or expanding federation before the first vertical is proven.
- Decision: Store canary receipts only in the user's local application state using a closed, versioned metadata schema. Receipts contain typed buckets and outcomes only; they contain no user, workspace, client, prompt, path, arbitrary attribute or error text. Aggregation is local and no network or federation adapter consumes the receipts.
- Consequences: Pilot operators can inspect a bounded aggregate report without exporting workspace content. Cross-device or organizational learning remains unavailable and requires a separate approved privacy, consent and transport decision.
- Refs: specs/003-pilot-success.md; specs/032-canary-observability.md; schemas/canary-receipt.schema.json; schemas/canary-report.schema.json; internal/canary
- Supersedes: none

## PAEX - Route accountable agents and PA experts through deterministic policy

- Date: 2026-07-26
- Status: accepted
- Owner: Daniel Scardini
- Context: Client Account Agents, Case Agents and centrally maintained PA experts need repeatable activation and declassification boundaries. Prompt-only routing cannot guarantee when an expert participates, prevent client context from crossing into the PA Expert registry scope or give Darwin stable evidence for calibration.
- Decision: Introduce a closed, versioned activation policy with `direct`, `balanced` and `deliberative` postures, starting at `balanced`. In the first shadow slice, the policy deterministically evaluates D0 direct, D1 targeted, D2 governed or blocked routes; semantic planning may propose but never reduce a hard floor. PA expert selection requires an exact signed local scaffold plus a version published by the central PA Expert registry. Initial route proportions are shadow hypotheses, never quotas.
- Consequences: The executable slice can test intended composition, fail closed on missing expertise and produce caller-asserted shadow breadcrumbs, but it cannot authorize dispatch, export an advisory packet or mark execution complete. Case and Client Account Agents remain separate Maestro roots joined by a signed relation, avoiding cross-scope child inheritance. Darwin observes deduplicated window metadata and proposes later policy versions but cannot tune a live episode. Native authority requires authenticated envelope provenance, Execution Ledger budgets/receipts and a qualified privacy adapter. The PA Expert registry uses an explicit schema-version transition; legacy signed authorities and canon namespaces fail closed and require re-registration without changing canon bytes or digests.
- Refs: specs/033-deterministic-agent-activation.md; internal/activationpolicy; internal/agentscaffold
- Supersedes: none

## BQAC - Include neutral engineering quality methods in base

- Date: 2026-07-27
- Status: accepted
- Owner: Daniel Scardini
- Context: The first quality-loop skills were authored as source-only engineering capability content, but QA, test-wave and pull-request hygiene are transversal safeguards needed by every professional workflow. Keeping them unavailable would make the active product surface omit the minimum evidence loop while specialized engineering activation is still pending.
- Decision: Include `coverage-diagnose`, `unit-test-wave`, `xfail-bug-capture`, `qa-gate`, `pr-review` and `pr-quality-loop` in the active base bundle and its signed distribution allowlist. Keep specification-first delivery, human review explanation and test/evidence methods in the unavailable `engineering-core` source bundle. Keep development hooks outside product distribution.
- Consequences: `bcgos skills index` exposes the six quality methods without selecting an engineering persona or granting tools. The base bundle grows only by an explicit allowlist, while specialized engineering/data skills remain fail-closed and require the future activation contract. Development hooks can be delivered separately to contributors without entering release artifacts.
- Refs: specs/035-professional-capability-bundles.md; specs/036-base-engineering-quality.md; bundles/base/distribution.json; bundles/base/skills/catalog.json; bundles/base/skills/INDEX.md
- Supersedes: CAPS
## MIDO - Add MarkItDown as a bounded local ingestion fallback

- Date: 2026-07-28
- Status: accepted
- Owner: Daniel Scardini
- Context: The Maestro ingestion contract needs a local, deterministic path for document formats that are not covered well by the primary Docling route. Microsoft MarkItDown converts several document and text formats to Markdown, but it has broad I/O behavior, optional network/cloud integrations and does not own Maestro's policy, provenance or workspace boundaries.
- Decision: Integrate the built-in offline MarkItDown converter only as an optional, managed-runtime adapter behind the Docling-first route. The adapter accepts approved local files or streams, uses an explicit format allowlist, disables plugins and network/cloud routes, enforces workspace and size/time boundaries, emits provenance and quality classification, and reports `unavailable` or `degraded` rather than silently falling back. MarkItDown does not replace Docling and is not added to the Kowalski canonical pipeline.
- Consequences: The Maestro runtime pack may pin MarkItDown and only the required format extras with hashes and platform validation. Initial implementation focuses on a provider-neutral ingestion contract, a fail-closed adapter runner and sanitized fixtures; ZIP, URL, YouTube, plugin, Azure and other remote routes remain out of scope. The Kowalski pipeline may use MarkItDown only as a comparative benchmark until separate evidence justifies a change.
- Refs: specs/010-local-ingestion-runtime.md; specs/031-markitdown-ingestion-adapter.md; internal/ingest; adapters/ingest/markitdown; https://github.com/microsoft/markitdown
- Supersedes: none

## PROJ - Install the human runtime projection and real base skills

- Date: 2026-07-29
- Status: accepted
- Owner: Daniel Scardini
- Context: A pilot user should be able to clone or receive the CLI and immediately understand the Agentic OS without learning Git or development. The prior adapter installer configured hooks but left the runtime without its orientation or product skills.
- Decision: `bcgos adapter install` materializes a concise, rich `CLAUDE.md` or `AGENTS.md` from the managed orientation template and installs every active base-bundle `SKILL.md` under the runtime's local skills directory. A workspace manifest and explicit markers define ownership; user-authored orientation content is preserved, and modified or symlinked managed files fail closed.
- Consequences: The installed runtime is navigable by a human and can invoke the actual product skills without copying the whole repository. Updates are idempotent and hash-aware. This projection is not native runtime evidence and never promotes a capability in the manifest.
- Refs: specs/026-workspace-local-adapter-installation.md; bundles/base/runtime/orientation.md.tmpl; internal/runtimeprojection
- Supersedes: none

## DARN - Make Darwin the scoped operational surgeon

- Date: 2026-07-28
- Status: accepted
- Owner: Daniel Scardini
- Context: The current Darwin definition is packet-only and recommendation-only, while the intended Agentic OS role is the operational counterpart of Kowalski's Darwin: it diagnoses and repairs Maestro system drift through a headless housekeeping mode. A no-tools Darwin cannot close the health loop or recover bounded product failures.
- Decision: Darwin remains a non-user-facing governance leaf, but receives a signed `scoped_system_maintenance` authority with explicit read, probe, write/edit and validation grants over managed Maestro state only. Interactive and headless housekeeping invocations use the same Darwin identity, tool contract, bounded health packet, remediation plan, fail-closed executor and metadata-only repair receipt. Darwin may repair reversible system issues, but cannot access client/workspace content, credentials, broad network, release or merge authority. Material policy or source changes return through Maestro and Walter.
- Consequences: The catalog, role contracts, specs and adapters must replace Darwin's `tool_access: none`/recommend-only posture with scoped maintenance authority. Deterministic surfaces still prepare packets and enforce grants; Darwin's headless execution owns diagnosis and remediation. Native runtime qualification remains required before claiming active runtime capability. Housekeeping failures remain recoverable through receipts and scheduler state.
- Refs: specs/018-maestro-core-agents.md; specs/028-federated-improvement-loop.md; specs/032-canary-observability.md; bundles/base/agents/darwin/AGENT.md; internal/agentcatalog; internal/scheduler; internal/federation; internal/lifecycle
- Supersedes: none

## OKFW - Use Karpathy-style maintenance with Google OKF bundles

- Date: 2026-07-28
- Status: accepted
- Owner: Daniel Scardini
- Context: Maestro needs a durable, human-readable and agent-readable knowledge surface. Karpathy's LLM-Wiki pattern provides the maintenance model, while Google's Open Knowledge Format provides an open interchange boundary. Neither format nor pattern supplies Maestro's authority, privacy, revocation or publication guarantees.
- Decision: Build the managed Maestro atlas as a deterministic, versioned Google OKF v0.2 bundle, using the Karpathy pattern for incremental source integration and cross-link maintenance. Preserve a namespaced BCGOS profile for scope, provenance, lifecycle and policy; managed content is allowlist-first and private/owner/workspace content remains physically separate and unavailable to this compiler. Darwin may detect drift and invoke a bounded managed reconciliation, but it may not synthesize, publish private content or bypass review/ release gates.
- Consequences: The first implementation can be consumed by any OKF-aware tool without a Google runtime dependency. Generated concepts, indexes, backlinks and logs remain reviewable in Git and reproducible from pinned sources. LLM-generated enrichment, private atlas compilation, provider selection and autonomous publication remain separate follow-up contracts.
- Refs: specs/007-content-navigation.md; specs/008-wiki-update-okf.md; specs/014-human-atlas-bootstrap.md; internal/atlas; https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f; https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md
- Supersedes: none

## SPWK - Keep SharePoint prior-work retrieval separate and Claude-collected

- Date: 2026-07-29
- Status: accepted
- Owner: Daniel Scardini
- Context: A senior office stakeholder needs to recover prior work with natural-language requests spanning themes, clients, projects, years and audiences. The corporate environment permits SharePoint access in Claude but forbids the equivalent connection in Codex. Adding this corpus to the managed wiki, owner memory or every workspace would leak scope and make ordinary sessions traverse cross-client metadata.
- Decision: Create a physically separate organizational `sharepoint-work` OKF bundle containing bounded metadata, facets and source pointers from explicitly enrolled SharePoint roots. Claude is the only V1 collection adapter; Codex collection is `unavailable/corporate_policy` and cannot use browser, token, Graph or copied-link fallbacks. The normalized snapshot, compiler and query engine remain runtime-neutral. The bundle is selected only for explicit prior-work retrieval and is never injected at Session Start or searched by general wiki routing.
- Consequences: Periodic refresh uses full or provider-delta snapshots, watermarks, synchronous deletion/access-revocation barriers, immutable versions and an atomic active manifest. The catalog stores no raw deck body, prompt, transcript or credential. Query returns ranked source pointers and rechecks SharePoint authorization when opened. Native readiness still requires an approved Claude MCP trial over a sanitized SharePoint scope; local fixtures cannot promote the capability.
- Refs: specs/007-content-navigation.md; specs/008-wiki-update-okf.md; specs/009-scheduler-catch-up.md; specs/037-sharepoint-work-retrieval-wiki.md; schemas/sharepoint-work-catalog.schema.json
- Supersedes: none

## WIRE - Make Maestro's Walter gate executable and context-lean

- Date: 2026-07-29
- Status: accepted
- Owner: Daniel Scardini
- Context: Maestro already defines a tool-free hub, a sealed Walter reviewer and fail-closed delegation, but the pilot dispatch path can finish a material branch without an explicit Maestro-to-Walter handoff. Kowalski's useful enforcement pattern is a deterministic trigger, an authenticated owner relationship and a bounded escalation state; copying its broad prompt/history surfaces would increase token cost and blur the professional boundary.
- Decision: Add a runtime-neutral Walter wire to the Maestro dispatch path. Material recommendations, consequential trade-offs and external-facing artifacts must produce a sealed review packet from Maestro to the registered Walter leaf after the producing branch closes. The packet carries only bounded review fields and scoped artifact/evidence pointers; public state carries only IDs, digests, trigger, verdict state and objection count. Walter returns one of `approved`, `refine-and-return` or `missing-the-mark`, with no more than three objections and a concrete fix plus exit condition for each. Ordinary factual or mechanical work does not enter the gate, and the execution ledger's binary authenticated approval remains a separate completion contract.
- Consequences: The relationship is enforced by code and conformance tests rather than description alone, while no prompt, rationale, client body or response text enters durable state or receipts. Native Claude/Codex activation and signed runtime qualification remain unavailable until their adapters prove this wire in fresh sessions. No new agent or domain is introduced; the existing Maestro and Walter roles remain the only hub/reviewer pair.
- Refs: specs/018-maestro-core-agents.md; specs/031-maestro-goal-orchestration.md; internal/agentdispatch; internal/agentorchestration; docs/agent-orchestration-assurance.md
- Supersedes: none

## BETA - Keep technical beta unsigned until corporate release authority exists

- Date: 2026-07-29
- Status: accepted
- Owner: Daniel Scardini
- Context: A useful technical beta can proceed before the organizational signing accounts, certificates and custody controls are funded and provisioned. Using a personal platform identity as a bridge would create the wrong ownership boundary and could make an engineering rehearsal look like an approved corporate release.
- Decision: Continue the beta with local or controlled `unsigned-candidate` and technical-rehearsal artifacts, explicitly labeled as engineering evidence only. Do not purchase or use a personal Apple Developer membership or personal Windows signing identity, including for beta; technical beta remains unsigned. Production distribution requires organization-owned Apple Developer ID and notarization, Windows Authenticode, and a new organization-controlled Ed25519 production key/custody process. A beta Ed25519 key, if needed for isolated testing, lives in a separate test registry and is never promoted; its public key may be retained only for read-only historical verification of beta artifacts.
- Consequences: The repository may demonstrate deterministic packaging, installation simulation and local closure without claiming authenticity, publication or pilot readiness. Release operators must provision corporate authorities before signing or publishing. The production authority registry and workflow must exclude the beta issuer/key ID (or mark it revoked) and reject it for installer/update trust and all new production artifacts. Historical verification must remain an archival operation outside production trust. This keeps cost, ownership, revocation and audit boundaries explicit.
- Refs: docs/releasing.md; docs/release-gates-checklist.md; docs/installer-package.md; specs/020-release-distribution.md; specs/022-guided-pilot-release.md
- Supersedes: none

## PABN - Make the PA Expert boundary a declassified, receipt-boundary

- Date: 2026-07-30
- Status: accepted
- Owner: Daniel Scardini
- Context: Case and Client Account Agents need functional or industry advice without exporting client, stakeholder or workspace context into the centrally maintained PA Expert.
- Decision: PA Expert is the sole canonical FPA/IPA advisory role. `internal/activationpolicy/advisory.go` is the single declassification and receipt contract: it binds the exact expert version and canon digest, accepts only closed public/internal codes and produces a bounded non-export-authorizing shadow receipt. `practice_agent` and `subject_specialist` remain rejected legacy identities; they are never scaffolded, authorized or delegated and require explicit PA Expert re-registration.
- Consequences: Account stakeholder intelligence, case-local raw context and governed case-to-account promotion remain separate. Raw pointers, scoped identifiers, excerpts, duplicate fact codes, empty registry state and forged canon bindings fail closed. Native runtime activation remains unavailable until a qualified adapter proves the same contract.
- Refs: specs/033-deterministic-agent-activation.md; specs/039-pa-expert-advisory-boundary.md; internal/activationpolicy/advisory.go
- Supersedes: none

## CADN - Keep Darwin cadence bounded, proposal-only and non-blocking

- Date: 2026-07-30
- Status: accepted
- Owner: Daniel Scardini
- Context: Darwin must improve Maestro continuously across event, daily, weekly and monthly windows without turning lifecycle hooks into workers or allowing unattended structural changes. The existing scheduler plans occurrences and the Darwin contract can execute scoped remediation, but they do not yet define a non-blocking event gate, reentrancy boundary or explicit timeout contract.
- Decision: Add a runtime-neutral cadence gate and worker-owned lease for Darwin maintenance. Lifecycle hooks may emit bounded typed wake signals only; they never wait for a worker, acquire a worker lock, call a model or apply maintenance inline. Every worker command carries an explicit deadline, is authorized by a concrete runtime-qualified catalog/attendance policy, is bound to an exact scheduler occurrence and Darwin-owned job/trigger matrix, and produces a metadata-only attempt receipt. Event, daily and weekly work remains recoverable through enrollment and bounded catch-up. Monthly structural evolution emits reviewable proposals only after explicit activation and attended authority; approval and application are separate transactions. D0/D1/D2 remain experimental and no cadence or executor receives a silent default. Capabilities and native scheduler templates remain unavailable until qualifying runtime evidence exists.
- Consequences: Claude, Codex, macOS and Windows share one cadence and receipt contract while retaining thin adapters. Reentrancy returns a bounded ephemeral busy/unavailable result rather than blocking; occurrence-keyed leases use unique fencing tokens and an OS guard spanning side effects plus terminal publication, so stale workers cannot overlap, release or overwrite successors. Terminal receipts carry an opaque occurrence digest and suppress retries across command IDs; failed attempts remain due. Darwin cannot mutate code, policy or release state from a scheduled run. The implementation adds typed command/receipt/lease schemas, authority, concurrency, path and timeout tests, lifecycle conformance fixtures and unavailable Darwin catalog entries without promoting any runtime capability.
- Refs: DARN; SCHD; AUTO; LIFE; specs/009-scheduler-catch-up.md; specs/019-nonblocking-hook-execution.md; specs/030-claude-lifecycle-vertical.md; specs/036-maintenance-plane.md; internal/darwin; internal/scheduler

- Supersedes: none

## DEVO - Persist Darwin evolution as an append-only, policy-pinned evidence plane

- Date: 2026-07-30
- Status: accepted
- Owner: Daniel Scardini
- Context: Darwin's Wave 1 contract can emit proposal-only structural evolution and separate health receipts, but it does not yet survive restart or prove which policy and approved PA Expert portfolio were in force for each episode.
- Decision: Add a local, versioned Darwin evolution store with immutable evidence windows, episode bindings, proposal artifacts and caller-asserted acceptance/rejection claims. Every episode pins an opaque policy ID and version plus an approved PA Expert portfolio snapshot and digest. Replay is idempotent for the same digest and rejects conflicting duplicates; recovery ignores incomplete projections and reports native persistence as unavailable until qualified. Decision claims are atomically fenced by proposal ID, remain `caller_asserted_shadow`, and cannot authorize or apply change. Health/housekeeping receipts remain in their existing store, and no evolution path may mutate live routing, the registry, canon or policy; only the existing signed reversible-repair scope may execute repairs.
- Consequences: Darwin gains durable auditability without a second execution ledger or a context-bearing contract store. Local files remain metadata-only, native persistence and native runtime provenance remain unavailable, and a decision claim never promotes a proposal automatically or proves Walter identity. Future authoritative approval requires a separately qualified signed envelope and consumer contract; future policy changes require a new pinned episode and explicit human authority.
- Refs: specs/038-darwin-durable-evolution.md; internal/darwin/evolution.go; internal/darwin/evolution_store.go; schemas/darwin-evolution-*.schema.json
- Supersedes: none

## MAST - Make Maestro native delegation and high-leverage review explicit

- Date: 2026-07-31
- Status: accepted
- Owner: Daniel Scardini
- Context: Maestro needs a deterministic topology that keeps Client Account consultation focused on client strategic lens and stakeholder pressure-testing, while Walter remains a calm Senior Advisor & Refiner for genuinely high-leverage output. Configuration and caller role strings must not silently create authority or inflate review loops.
- Decision: Use a typed Maestro planner with independent `account_consultation_required` and `walter_required` decisions. Account consultation is selected by closed client-strategy, stakeholder, relationship, narrative, cross-case and promotion signals; explicit execution-only work may use direct Case, while insufficient routing evidence fails safe to Client Account. Walter is selected by consequence, leverage, external/reputational exposure, hard-to-reverse decisions, materiality or a closed review trigger. Ordinary low-leverage work may carry an auditable Walter skip. The runtime keeps one active spoke, depth one, zero children, durable Claude/Codex state fencing, and constructive Walter verdicts with actionable refinements and exceptional holds.
- Consequences: Account framing and return validation are paired; direct Case never invokes Client Account. Walter approval is not a routine veto: cosmetic observations remain non-blocking, load-bearing gaps require proposed refinements and acceptance criteria, and hold is exceptional. Content or risk mutation invalidates stale decisions and re-enters the bounded loop. Native qualification remains unavailable until fresh runtime evidence exists.
- Refs: specs/016-workspace-agent-boundaries.md; specs/018-maestro-core-agents.md; specs/023-sequential-agent-dispatch.md; specs/034-vertical-agents-and-transversal-skills.md; specs/040-maestro-native-delegation.md; schemas/maestro-plan.schema.json; internal/maestro; internal/agentdispatch/review.go; internal/agentorchestration
- Supersedes: none

## INTN - Make Walter an intent proxy and keep self learning evidence-bound

- Date: 2026-08-01
- Status: accepted
- Owner: Daniel Scardini
- Context: Walter's high-leverage review needs to approximate the user's calm, context-rich self-review without impersonating the user or turning a selective reviewer into the only learning path. The product also needs longitudinal learning when Walter is skipped, while keeping private self material local and auditable.
- Decision: Maestro captures a typed, metadata-only `InteractionObservation` after every interaction and stores it in an append-only provisional log with source event/hash, timestamp, scope, confidence, sensitivity and expiry/recheck. The canonical Owner Context facets remain the sole authority; a versioned `UserSelfSnapshot` is only a stale-checked projection of those confirmed professional preferences, principles, decision rules, communication style, motivations and boundaries. Walter receives a versioned, digest-bound `IntentReviewPacket` containing the literal request, chosen plan, draft/output, minimum relevant context, self snapshot version/digest, applicable provisional observations and audience/consequence/reversibility. Walter returns a typed intent hypothesis with evidence references, confidence, purpose satisfaction, constructive refinement, unresolved uncertainty and `approve`, `refine`, `clarify` or exceptional `hold`. Explicit instruction/correction can deterministically update canon; endorsement reinforces an existing rule; isolated inference remains provisional. Repeated independent evidence or explicit confirmation is required for promotion. Contradictions create a versioned superseding record, and the user can inspect, edit, reject, reset, export or delete self data.
- Consequences: Walter is a proxy of likely user review, never an impersonation or mind-reading claim. Low-confidence/high-consequence intent returns to Maestro for a user clarification. Walter receipts pin self version plus prompt/output digests without storing raw content. Darwin may deduplicate, detect drift, measure utility and propose promotion/decay, but cannot silently mutate canonical self. Self changes never rewrite historical receipts; raw client content and personal data remain outside the public repository.
- Refs: specs/006-memory-persistence.md; specs/002-data-boundaries.md; specs/004-runtime-portability.md; specs/013-owner-context.md; specs/040-maestro-native-delegation.md; schemas/intent-review-packet.schema.json; schemas/user-self-snapshot.schema.json; internal/maestro; internal/ownerctx
- Supersedes: SELF

## PHST - Keep bounded owner prompt history separate from self learning

- Date: 2026-08-01
- Status: accepted
- Owner: Daniel Scardini
- Context: Walter needs relevant dense context from prior user prompts without turning the runtime into an unbounded transcript or mixing prompt retention with self promotion.
- Decision: Add a private PromptHistoryStore containing only explicitly retained user prompts, each bound to owner, timestamp, language, source/session, scope and SHA-256. Enforce bounded count, bytes, age and scope selection with secure local paths plus inspect, export, delete and reset controls. Maestro preserves the current prompt as highest precedence, selects bounded history, then normalizes/translates selected entries into the configured working language before deriving the typed IntentHypothesis. Historical bodies exist only in the ephemeral sealed Walter packet; receipts and ledgers keep digests only. Prompt retention remains independent from material authenticated self observation and canonical self promotion.
- Consequences: Owner prompts may remain locally raw by explicit policy, but never enter managed bundles, telemetry, receipts, federation or release artifacts. Historical prompts are quoted data and not executable instructions. Missing translation evidence fails closed for the pre-review normalization stage; low-confidence high-consequence intent returns to Maestro for clarification.
- Refs: specs/006-memory-persistence.md; specs/013-owner-context.md; specs/040-maestro-native-delegation.md; schemas/prompt-history.schema.json; schemas/intent-review-packet.schema.json; internal/ownerctx/prompt_history.go; internal/maestro/intent.go
- Supersedes: none

## QLHD - Harden prompt context and durable orchestration boundaries

- Date: 2026-08-01
- Status: accepted
- Owner: Daniel Scardini
- Context: The first PromptHistoryStore slice normalized only prior prompts, selected history by recency and scope, and serialized mutations only within one process. Independent review also found unbound Case-to-Account routing, tamperable snapshot projections, incomplete intent evidence validation, and stale durable adapter instances.
- Decision: Normalize the current prompt first into a digest-bound working representation while retaining the original as authority. Select history with deterministic lexical relevance plus explicit keys under hard scope/age/count/bytes and eight-prompt/32 KiB packet ceilings, exposing score/reason metadata only in the ephemeral packet. Bind each history root to one owner and serialize mutating operations with a symlink-safe cross-process lock. Require explicit Case-to-Account parent binding, validate snapshot facet content/readers/policy/path digests, require current-prompt evidence in both intent hypothesis and Walter result, and make the Maestro CLI dispatch boundary record a fresh owner attestation under the OS-user-local data-root boundary, persist chain metadata and stop at a metadata-only model-unavailable boundary. The attestation is not cryptographic principal authentication. Durable Claude/Codex state refreshes under a cross-process lock before CAS/fenced mutations.
- Consequences: Historical prompt bodies remain local and never enter receipts or errors. Stale or ambiguous routing, translation, ownership, evidence, locking or tampering fails closed. Native model execution remains unavailable until qualifying evidence exists.
- Refs: specs/013-owner-context.md; specs/040-maestro-native-delegation.md; schemas/intent-review-packet.schema.json; schemas/maestro-input.schema.json; internal/ownerctx; internal/maestro; internal/agentorchestration
- Supersedes: none

## QLDR - Bind intent, fencing and dispatch commit recovery

- Date: 2026-08-01
- Status: accepted
- Owner: Daniel Scardini
- Context: Follow-up pressure testing found that bounded context must cover translated representations as well as originals, stale durable state must not authorize a branch, and dispatch persistence cannot rely on an unverified best-effort rollback.
- Decision: Bind Walter's intrinsic-intent string exactly to the evidence-bound hypothesis, require independent per-representation and combined packet ceilings, reject oversized owner facets instead of truncating authority, and seal the packet before recording the current prompt. The Maestro dispatch boundary uses an occurrence-bound metadata-only CAS/receipt store with append-only epochs and an atomic current pointer; it does not authenticate a native adapter or fabricate credentials. Chain persistence uses the repository's cross-platform advisory lock; prompt/chain commit failures compensate both sides and write a metadata-only recovery marker if compensation itself fails. Recovery markers are serialized, idempotent for the same incident and reject silent replacement of an unresolved incident.
- Consequences: Earlier same-session or repeated prompts remain eligible history while the current occurrence cannot self-duplicate. Durable fence epochs are real transition evidence rather than an opened-store snapshot. Raw prompt, client or generated content never enters recovery markers, receipts or errors. Low-confidence intrinsic-intent hypotheses remain task-local and can clarify through Maestro; they never update canonical self state.
- Refs: specs/013-owner-context.md; specs/040-maestro-native-delegation.md; schemas/intent-review-packet.schema.json; internal/maestro; internal/ownerctx; internal/agentorchestration; internal/cli
- Supersedes: none

## SILE - Keep weekly self learning silent and bounded

- Date: 2026-08-02
- Status: accepted
- Owner: Daniel Scardini
- Context: Walter's recurring self review must help the professional self learn from interactions without becoming a second user-facing conversation, an ever-growing transcript, or an uncontrolled canonical-self writer. Darwin's weekly deep maintenance must also prevent installed agent state summaries from accumulating stale detail.
- Decision: Treat Walter's weekly cycle as a silent, owner-local ingestion and refinement pass. It selects a bounded, expiry-aware weekly interaction window, produces at most the policy-permitted self refinement through the existing Owner Context boundary, and emits only metadata-safe lifecycle evidence; it has no user channel, notification, browsing, delegation or canonical bypass. Boundaries, retention and no-change semantics are explicit so the cycle cannot grow input, output or durable self material indefinitely. Extend Darwin deep weekly with a deterministic, managed-only review of registered `states.md` summaries that enforces per-file bounds and emits only a concise review proposal after validation; it may not read client/workspace state, rewrite policy/canon, or change release/code state.
- Consequences: Walter remains invisible unless the normal Owner Context policy requires owner confirmation; its silent execution does not imply broad permission or a visible proposal queue. Darwin state hygiene is a separate bounded operational concern, not a new memory system or a source of self evidence. Both tasks remain unavailable until their individual runtime activation and qualification requirements are met.
- Refs: INTN; PHST; CADN; DARN; specs/013-owner-context.md; specs/036-maintenance-plane.md; specs/037-darwin-lifecycle-cadence.md; specs/041-model-backed-maintenance-activation.md; internal/walterselfreview; internal/darwin
- Supersedes: none

## BSEL - Activate optional engineering skills from confirmed interview selection

- Date: 2026-07-31
- Status: accepted
- Owner: Daniel Scardini
- Context: The Canary already ships professional engineering skills as source content, but the bundle catalog marked them unavailable and the initial interview had no capability-track selection. That made the useful code methods invisible at runtime and conflated missing data/runtime packs with an optional local capability.
- Decision: Mark `engineering-core` as `optional`, expose `technical-explorer` and `software-engineering` in the canonical agent interview, persist confirmed `capability_tracks` with the local personalization profile, and have the adapter project the embedded engineering skills only for a valid selected plan. Keep `data-practice` unavailable until its runtime and release contract are qualified. Capability selection remains independent from interaction profile, tools, data scope, provider access and authority.
- Consequences: A user can opt into code-oriented methods during onboarding and receive them in Claude/Codex projection without a new agent or implicit grant. Selection is validated against the catalog and fails closed before persistence or workspace writes when a required bundle is unavailable. Remote/downloaded optional packs, migrations, signatures and separate runtime authorities remain future work.
- Refs: USER; specs/035-professional-capability-bundles.md; bundles/catalog/catalog.json; internal/agentidentity; internal/capabilitybundle; internal/runtimeprojection
- Supersedes: CAPS

## OPTS - Make all Canary professional bundles interview-selectable

- Date: 2026-07-31
- Status: accepted
- Owner: Daniel Scardini
- Context: The Canary's engineering and data practice content is already embedded, sanitized and runtime-neutral. Leaving `data-practice` as `unavailable` made the interview offer a choice that could not be fulfilled even though its three methodological skills require no external provider or file-generation capability.
- Decision: Remove `unavailable` from the professional capability-bundle surface. `engineering-core` and `data-practice` are both `optional`, appear as selectable tracks in the canonical interview, and every skill in the selected bundle is projected together with its dependencies after confirmed local selection. Compose and project a manifest-owned, selection-scoped agent-skill policy so only the confirmed methods and dependencies become selectable by the Case Agent. Keep `unavailable` for unrelated native/runtime capabilities whose evidence or authority is not present.
- Consequences: A user can select software, data or both practices during onboarding; the active projection and dispatcher policy remain deterministic, hash-bound and tool-neutral. Embedded methods from unselected bundles remain denied, and a modified or unmanaged policy path fails closed. The base catalog remains unchanged until selection, and no track grants tools, data scope, provider access, publication or agent authority.
- Refs: USER; specs/035-professional-capability-bundles.md; bundles/catalog/catalog.json; internal/agentidentity; internal/runtimeprojection
- Supersedes: BSEL

## IVRY - Bind post-install readiness to canonical local identities

- Date: 2026-08-02
- Status: accepted
- Owner: Daniel Scardini
- Context: A successful `bcgos init` and workspace-local adapter install can still leave a missing, redirected or tampered workspace projection, hook configuration or executable binding. Configuration presence alone is too weak for an installer handoff, while directly invoking hooks would still not prove native runtime execution.
- Decision: Add one deterministic, read-only Codex post-install verifier. It anchors the running CLI to the canonical managed root recorded in owner-local install state, requires a canonical non-symlink workspace and exact initialized workspace identity, verifies the managed Codex projection against the active embedded bundle and selected tracks, and requires exactly one Maestro-owned binding for every canonical lifecycle event pointing to that installed CLI and workspace-local orchestration state. The report remains configuration-only: lifecycle capabilities must stay fail-closed and native observation must remain `not_observed`.
- Consequences: Installers can stop on missing, mismatched, tampered, duplicate or symlinked local surfaces without mutating global Codex settings or starting a model session. The check does not qualify native hooks, release signing, clean-device acceptance or pilot readiness; those retain their separate evidence gates.
- Refs: specs/001-cli-distribution.md; specs/020-adapter-diagnostics.md; specs/026-workspace-local-adapter-installation.md; specs/035-lifecycle-evidence-matrix.md; specs/042-post-install-readiness.md; internal/installreadiness
- Supersedes: none

## IDLE - Make idle continuity bounded and capability-honest

- Date: 2026-08-02
- Status: accepted
- Owner: Daniel Scardini
- Context: A fifteen-minute native wake can keep Maestro continuity responsive, but treating every pulse as permission to run would create loops, model cost and false success. Memory also needs a useful deterministic boundary that does not pretend model synthesis exists.
- Decision: Keep the native pulse as a wake only. The runtime-neutral scheduler derives due work and a depth-one worker checks activation, qualification, explicit idle evidence and cooldown before dispatch. Unknown activity state is not idle. `memory-checkpoint` is a workspace-scoped, deterministic three-hour interval job anchored first to enrollment and then to its last successful attempt, with `MaxCatchUp=1`; success requires a versioned, fsynced and atomically activated watermark over allowlisted durable scheduler metadata plus its own terminal receipt, with no body-bearing source and last-known-good preservation. `memory-light-dream` has the same due contract but remains unavailable without a qualified synthesis adapter; `memory-deep-dream` remains weekly and model-backed. Suppression is a distinct metadata-safe terminal attempt state that never advances scheduler success or the due anchor. Idle suppression is pulse-cooldown-bounded, while recent failed or unavailable attempts use a per-job/occurrence circuit breaker and become retryable after expiry.
- Consequences: A fifteen-minute macOS LaunchAgent improves discovery latency without defining cadence, running inline synthesis or creating catch-up storms. Only explicit idle evidence can cross the worker gate; unsupported idle observation fails closed and remains visible as suppression. Checkpoint success proves only a metadata checkpoint, while light or deep dream success requires its owning memory commit and runtime qualification. Catalog configuration, installed native adapters, local activation and successful execution remain separate capability claims.
- Refs: MEMO; SCHD; CADN; specs/006-memory-persistence.md; specs/009-scheduler-catch-up.md; specs/036-maintenance-plane.md; internal/scheduler; internal/maintenance; adapters/macos
- Supersedes: none

## DLIT - Activate deterministic light dreaming only at L1

- Date: 2026-08-03
- Status: accepted
- Owner: Daniel Scardini
- Context: The enrolled three-hour light-dream occurrence had no executable owner, and the macOS pulse supplied `unknown` idle state, so the safe worker could never cross either boundary. Relying on a headless model would also make continuity depend on provider authentication and create cost or retry pressure during unattended idle time.
- Decision: Qualify one local deterministic L1 synthesizer over already-durable captures that carry the existing sanitized workspace attestation. It uses the managed runtime configuration of 12,000 runes and 64 complete entries, stable ordering, exact duplicate collapse, immutable provenance and the existing staged atomic memory commit. It may write only L1. A source-free occurrence closes as `reviewed_no_change`; invalid, unsanitized, cross-workspace or over-budget input fails without changing active memory. Enroll this light job with the metadata checkpoint, and let the macOS adapter resolve `--idle-state auto` from the bounded `IOHIDSystem` HID idle counter with a five-minute eligibility threshold. Missing, ambiguous or unsupported observation remains `unknown` and fails closed.
- Consequences: The 15-minute LaunchAgent pulse is still only a discovery mechanism; the scheduler remains anchored to elapsed three-hour success with `MaxCatchUp=1`. Daily/manual light dreaming no longer needs a model, while weekly deep dreaming, L2/L3/lifetime promotion and raw prompt or daily-log ingestion remain unavailable without their separate adapters and eligibility evidence. A terminal scheduler receipt still cannot substitute for the active memory commit.
- Refs: IDLE; MEMO; SCHD; specs/001-cli-distribution.md; specs/006-memory-persistence.md; specs/009-scheduler-catch-up.md; specs/036-maintenance-plane.md; internal/memory; internal/maintenance; internal/macosadapter
- Supersedes: none
