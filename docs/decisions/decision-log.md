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

## FILO - Federate pilot improvement through typed automatic batches

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: Maestro needs to learn from pilot skills, operating friction and qualitative patterns without requiring users to operate Git or approve each report, while client/workspace material must remain local.
- Decision: Adopt the Maestro Federated Improvement Loop. Enrolled pilot installations automatically emit only typed, bounded batches through a GitHub App bridge; local Darwin federates workspace-scoped signals and structural skill candidates, while central Darwin curates cross-participant batches into advancement proposals. Workspace-private sources may never export prose, code or other content-bearing artifacts; complete skills are eligible only from an explicit born-portable root.
- Consequences: GitHub is the system of action rather than a raw telemetry store; the bridge, portable-skill collector and Darwin adapters are separately deliverable capabilities. Central curation cannot modify source or release without maintainer acceptance. The compiler and tests must fail closed on unknown fields and prove non-interference for workspace-private canaries.
- Refs: specs/002-data-boundaries.md; specs/004-runtime-portability.md; specs/016-workspace-agent-boundaries.md; specs/028-federated-improvement-loop.md
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

## WSAG - Make workspace agents enforce context boundaries

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: A single user can work across multiple clients and projects. Treating all local work as available context would create a confidentiality risk and an unusable information blob.
- Decision: Each registered workspace has an owning workspace agent that is the only default reader and writer of its raw context. The OS uses default-deny, runtime-enforced workspace scopes for files, memory, search, indexes, logs and intermediate outputs. Client/account context is curated through explicit promotion; cross-workspace work uses a minimal, expiring and audited delegation packet.
- Consequences: The product requires a workspace authorization contract and conformance tests, not only agent prompts. Client/account agents cannot browse project workspaces. Capability specialists receive bounded work packets. A user can have multiple independent workspaces for one client or across clients.
- Refs: specs/002-data-boundaries.md; specs/016-workspace-agent-boundaries.md; specs/004-runtime-portability.md
- Supersedes: none

## BRIF - Bootstrap workspace agents through approved research

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: A workspace agent needs useful initial context, but a large persistent prompt state, untraceable web research or automatic client disclosure would compromise quality and confidentiality.
- Decision: Initialize each workspace agent through a guided user interview, an explicitly approved and minimized external-research plan, a versioned workspace dossier and a compact operational state. Keep a separate public-only economic rollup; it may be versioned into a workspace but never reads from or writes back to workspace data.
- Consequences: The product needs approval, provenance, freshness and dossier contracts in addition to workspace authorization. States remain pointer-first; facts, research and bullish/bearish hypotheses carry sources, uncertainty and invalidation signals. Creation may pause before research rather than leaking information through automatic queries.
- Refs: specs/017-workspace-agent-initialization.md; specs/016-workspace-agent-boundaries.md; specs/002-data-boundaries.md
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

## DIST - Make the signed manifest the portable release authority

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: The pilot needs one verifiable release contract even if the source repository or artifact provider changes, while CLI, managed bundles and optional runtime packs evolve independently.
- Decision: Treat a signed, versioned release manifest as the portable authority for Maestro distribution. Bind trust to the Maestro product identity and an explicit signing-key identifier rather than to a GitHub owner name. Record CLI and bundle artifacts separately with immutable versions, compatibility ranges and allowlisted content; keep the release provider behind an adapter. Runtime packs remain a future, separately versioned schema extension rather than an underspecified v1 entry.
- Consequences: A GitHub transfer does not silently redefine the trust root. The bootstrapper and `bcgos` must reject unknown issuers, incompatible versions, unsigned manifests, duplicate JSON keys, unlisted artifacts and content from workspace or user-local data roots. Release versions cannot be relabeled across channels; promotion requires a new version and signed manifest. Production key custody, platform code-signing identities and provider registration remain explicit release-environment approvals.
- Refs: RELS; UPDT; SECU; DATA; specs/001-cli-distribution.md; specs/020-release-distribution.md; schemas/release-manifest.schema.json
- Supersedes: none

## AUTH - Use GitHub App device flow with native-store fail-closed authentication

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: Pilot users need browser-based access to a private release without cloning source or managing personal tokens, while provider credentials must not enter files, environment variables, logs or the managed bundle.
- Decision: Use a least-privilege GitHub App device flow for the pilot provider, with read-only Contents access to the selected release repository and short-lived user credentials stored only through an approved native Keychain or Windows Credential Manager adapter. Keep provider transport behind an adapter, strip authorization on cross-host asset redirects and authenticate the signed manifest before accepting its artifact list. If the native store or provider registration is absent, report `unavailable`; do not fall back to plaintext, Git helpers, environment variables or `gh`.
- Consequences: Authentication, refresh and verified download are testable independently of production configuration. The CLI exposes schema-versioned auth/update states and binds one confirmation to a deterministic update plan, but remains unavailable until native-store adapters, GitHub App installation and production key registry are approved.
- Refs: DIST; SECU; UPDT; specs/020-release-distribution.md; specs/021-private-release-provider.md; internal/releaseprovider; internal/updateplan
- Supersedes: none

## PILT - Gate the ten-person pilot through two users and classified device evidence

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: Passing CI or an isolated installer smoke test cannot prove that Maestro installs and updates safely under corporate Windows/macOS policy, and sending a first release directly to ten users would combine distribution, usability and support risk.
- Decision: Keep natural-language `maestro-setup-update` guidance as the primary pilot experience, with deterministic CLI/bootstrapper enforcement and one confirmation bound to an exact update plan. Classify isolated Windows/macOS runs as engineering evidence only. Require separate corporate-device reports for install, update and rollback, then run a two-user canary with one Windows and one macOS user for five business days before considering the ten-person cohort.
- Consequences: The repository cannot promote itself to pilot-ready. Expansion requires production authorities, both clean-device reports, success by both canary users, working rollback, no severity-1/2 incident or data-boundary breach and support-owner acceptance. The ten-person cohort remains a human go/redesign/stop decision.
- Refs: AUTH; DIST; specs/022-guided-pilot-release.md; bundles/base/skills/maestro-setup-update/SKILL.md; docs/pilot-release-runbook.md
- Supersedes: none

## HUBS - Govern Maestro through a lean hub-and-spoke core

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: Maestro needs a canonical user-facing hub and bounded internal governance roles without copying Kowalski's broad tool access or delegation model.
- Decision: Define Maestro as the only user-facing hub. Maestro has no direct tool access and delegates sequentially with one active spoke and no nested delegation. Walter and Darwin are packet-only internal agents without tools, delegation or direct user speech.
- Consequences: Runtime adapters must enforce tool denial, one-active-spoke and no-nesting invariants before reporting orchestration as available.
- Refs: MAES; PORT; WSAG; specs/004-runtime-portability.md; specs/016-workspace-agent-boundaries.md; specs/018-maestro-core-agents.md; bundles/base/agents/catalog.json
- Supersedes: none

## BRCH - Allow multiple governed chains with role-gated depth

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: A single global depth-one rule would prevent legitimate specialist chains while preserving the one-active-branch invariant.
- Decision: Allow multiple governed chain types under Maestro while keeping one active branch by default. Delegation remains sequential, role-gated and maximum depth two; Walter, Darwin, errands and leaf specialists never delegate.
- Consequences: Cross-chain exchange uses a minimum sanitized packet mediated by Maestro; parallel branches and deeper delegation require a new decision and conformance evidence.
- Refs: HUBS; WSAG; specs/016-workspace-agent-boundaries.md; specs/018-maestro-core-agents.md; bundles/base/agents/catalog.json; internal/agentcatalog
- Supersedes: HUBS

## WKPK - Delegate through bounded signed work packets

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: Role-gated chains prevent topology drift, but delegation would still leak context or become replayable if Maestro or a workspace agent passed free-form conversation state, raw dossiers or reusable scope claims to the next agent.
- Decision: Every root and child delegation uses a signed, expiring and pointer-only work packet with strict objective, pointer and constraint budgets. A root packet ID is the unique branch instance and a child packet ID is the unique child dispatch instance. Children inherit the parent's scope root and kind. Packet verification, orchestration state and registered identity/capability checks all fail closed.
- Consequences: Maestro can coordinate sequential specialists without becoming a context blob. Old packets cannot close later work in the same workspace, practice packets cannot point to workspace resources and native runtimes must persist dispatcher/orchestration state before activation.
- Refs: BRCH; WSAG; specs/016-workspace-agent-boundaries.md; specs/018-maestro-core-agents.md; specs/023-sequential-agent-dispatch.md; internal/agentdispatch; internal/agentorchestration
- Supersedes: none

## PROM - Promote workspace facts without account browsing

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: An account agent needs durable client-level context, but letting it browse project workspaces would collapse confidentiality boundaries and recreate the context blob the workspace architecture is designed to prevent.
- Decision: Promote one curated, approved and expiring statement at a time through capability-bound workspace and account grants. Verify the declared hash against source bytes, keep the raw artifact pointer in a workspace-owned signed receipt and expose only the statement, source hash and opaque receipt ID to the account. Use prepared/final signed receipts plus a trusted monotonic anchor; transition that anchor before writing non-destructive revocation evidence.
- Consequences: Account context becomes a deliberate rollup rather than automatic memory aggregation. Partial writes, coordinated tampering and marker-deletion rollback fail closed; revocation linearizes reads while preserving evidence. Native Claude/Codex adapters still must provision private capabilities and integrity keys, provide a durable atomic anchor store and enforce filesystem scopes before activation.
- Refs: WSAG; BRIF; WKPK; specs/016-workspace-agent-boundaries.md; specs/024-account-context-promotion.md; internal/contextpromotion
- Supersedes: none

## ADPT - Install product adapters locally per workspace

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: A user-wide hook configuration would affect unrelated work and make safe removal difficult, while the product must preserve existing runtime configuration in a consultant's workspace.
- Decision: Install Maestro runtime adapters as one owned entry in workspace-local runtime configuration. The installer is idempotent, preserves unrelated entries, uses an explicit short timeout and removes only its exact owned entry. Runtime trust remains a runtime concern; installation does not bypass it.
- Consequences: Claude and Codex receive separately managed local configuration and can be removed without deleting user settings. A workspace adapter may be absent in another workspace by design. Installation state and actual runtime execution remain distinct diagnostics.
- Refs: specs/026-workspace-local-adapter-installation.md; internal/adaptercfg; internal/cli/cli.go
- Supersedes: none

## RSLV - Resolve session pointers explicitly and under budget

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: A pointer-only Session Packet protects private sources at startup, but the runtime needs a governed way to obtain a narrowly authorized source later without turning the hook into a broad reader.
- Decision: Pointer resolution is an explicit post-start operation with declared `session` purpose and a bounded byte budget. It may read only owner pointers already exposed by the current packet, rejects traversal and returns an omission/budget state rather than broadening access. Hooks never call it.
- Consequences: The SELF can inform work progressively without injecting all facets. Atlas, skills and memory pointers require separate authorization/resolution contracts; sensitive and unreviewed owner facets remain unreachable.
- Refs: specs/015-session-context-packet.md; specs/019-authorized-session-pointer-resolution.md; internal/sessionresolve
- Supersedes: none

## PILH - Require native receipts before pilot hook promotion

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: A workspace-local hook configuration and a unit-tested command payload do not prove that a real Claude or Codex session trusted and invoked the hook. At the same time, pilot users cannot depend on PATH setup or tolerate a slow startup hook.
- Decision: Install one Maestro-owned Session Start command using the released local executable, with a two-second timeout, an 8 KiB output ceiling and an explicit source marker. Exclude the generated machine-local configuration from the workspace Git index when possible, but refuse installation before any write if that target configuration is already tracked. Keep lifecycle capability `unavailable` until an operator records a direct-command result and a fresh native-session result for each supported runtime/platform.
- Consequences: Reinstalling after an update changes only Maestro's own command and does not depend on a user shell. A missing native receipt fails closed in product reporting, while conformance remains practical to run in an empty non-client workspace. No memory, worker, network or model operation is added to Session Start, and no absolute machine path can be silently added to an already tracked runtime configuration.
- Refs: specs/017-native-session-start-hook.md; specs/018-workspace-local-adapter-installation.md; specs/021-pilot-hook-conformance.md; docs/onboarding/pilot-hook-conformance.md; internal/adaptercfg; internal/sessionhook
- Supersedes: none

## STUB - Scaffold governed instances without activating them

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: The role catalog, enforcement controller and dispatcher could validate specialist identities, but only Maestro, Walter and Darwin had concrete definitions. Workspace initialization created state and a dossier without a runtime-neutral agent stub, while capability and subject specialists existed only in tests.
- Decision: Maintain data-free managed templates for workspace, account, practice, capability-specialist and subject-specialist roles. Materialize private local instances durably and atomically with signed immutable role, parent and scope bindings, definition/state hashes and an explicit unavailable runtime state. Resolve the concrete workspace and registered same-scope parent before creation; require accountable owner/mandate for account and practice roots and verify practice canon bytes. Make workspace scaffolding automatic in `bcgos init`; require explicit CLI creation for other roles.
- Consequences: Every initialized workspace has an inspectable agent definition and teams can prepare bounded specialists without embedding client data or prematurely granting tools. Orphan parents, cross-scope binding, key split-brain, rebinding and tampering fail closed. Native Claude/Codex adapters still must verify the stub, provision private capabilities and pass conformance before activation.
- Refs: WKPK; WSAG; specs/018-maestro-core-agents.md; specs/023-sequential-agent-dispatch.md; specs/025-agent-scaffolding.md; internal/agentscaffold
- Supersedes: none
