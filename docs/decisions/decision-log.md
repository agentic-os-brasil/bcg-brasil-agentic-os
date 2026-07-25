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

## WSAG - Make workspace agents enforce context boundaries

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: A single user can work across multiple clients and projects. Treating all local work as available context would create a confidentiality risk and an unusable information blob.
- Decision: Each registered workspace has an owning workspace agent that is the only default reader and writer of its raw context. The OS uses default-deny, runtime-enforced workspace scopes for files, memory, search, indexes, logs and intermediate outputs. Client/account context is curated through explicit promotion; cross-workspace work uses a minimal, expiring and audited delegation packet.
- Consequences: The product requires a workspace authorization contract and conformance tests, not only agent prompts. Client/account agents cannot browse project workspaces. Capability specialists receive bounded work packets. A user can have multiple independent workspaces for one client or across clients.
- Refs: specs/002-data-boundaries.md; specs/010-workspace-agent-boundaries.md; specs/004-runtime-portability.md
- Supersedes: none

## BRIF - Bootstrap workspace agents through approved research

- Date: 2026-07-25
- Status: accepted
- Owner: Daniel Scardini
- Context: A workspace agent needs useful initial context, but a large persistent prompt state, untraceable web research or automatic client disclosure would compromise quality and confidentiality.
- Decision: Initialize each workspace agent through a guided user interview, an explicitly approved and minimized external-research plan, a versioned workspace dossier and a compact operational state. Keep a separate public-only economic rollup; it may be versioned into a workspace but never reads from or writes back to workspace data.
- Consequences: The product needs approval, provenance, freshness and dossier contracts in addition to workspace authorization. States remain pointer-first; facts, research and bullish/bearish hypotheses carry sources, uncertainty and invalidation signals. Creation may pause before research rather than leaking information through automatic queries.
- Refs: specs/011-workspace-agent-initialization.md; specs/010-workspace-agent-boundaries.md; specs/002-data-boundaries.md
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
