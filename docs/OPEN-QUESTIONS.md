# Open questions

This file contains decisions that still require discussion. It is not a decision log, implementation tracker or place for client content. When a question is decided, record the durable choice with a new four-letter entry and update the relevant spec.

## Before Marcelo starts

- **Q-001 - Repository access:** What is Marcelo's GitHub username, and has it been added to the private repository?
- **Q-002 - Windows installation channel:** Are Git, GitHub CLI, Go and Claude Code already available, or is `winget` approved on his corporate device?
- **Q-003 - Corporate restrictions:** Does the device require proxy, certificate, endpoint-security or Software Center steps that the bootstrap must diagnose?
- **Q-004 - Claude entry point:** Will Marcelo use Claude Code CLI, the desktop application with a local workspace, or another managed BCG distribution?
- **Q-005 - First contribution:** What is the smallest real documentation or test task that validates the complete branch-to-PR experience?

## Before implementing the product CLI

- **Q-006 - Product naming:** Is `bcgos` the final public/internal command name?
- **Q-007 - Application directories:** What are the approved managed-core and local-data paths on Windows and macOS?
- **Q-008 - Private release authentication:** GitHub CLI token reuse, browser device flow or a BCG-managed distribution identity?
- **Q-009 - Release trust:** Which signing, SmartScreen, Gatekeeper, checksum and provenance requirements apply?
- **Q-010 - Update policy:** Automatic checks, user-triggered updates, forced security updates and rollback retention.

## Before building the first OS bundle

- **Q-011 - Initial capability:** Which single work use case proves value for both classic and technical consultants?
- **Q-012 - Knowledge governance:** What can be shared organization-wide, what stays local, and who owns approval and retirement?
- **Q-013 - Workspace boundary:** What metadata may be placed inside a case/code workspace without creating client-data risk?
- **Q-014 - Runtime parity:** Which Claude/Codex capabilities are required, emulated, degraded or explicitly unavailable in v0?
- **Q-015 - Hooks policy:** Which events may block, warn or only observe in a corporate workspace?
- **Q-037 - Ingestion runtime pack:** What are the maximum install size and first-use time, approved model set, prefetch/offline policy, corporate-network behavior and update/rollback rules for the Docling runtime pack?

## Before the ten-person pilot

- **Q-016 - Pilot cohort:** Which personas and operating systems are represented, and who provides support?
- **Q-017 - Success metrics:** Time-to-first-success, weekly active use, update success, support demand and qualitative value.
- **Q-018 - Telemetry and privacy:** What may be measured without collecting prompts, client content or personal data?
- **Q-019 - Incident path:** Who owns broken installs, credential failures, unsafe outputs and rollback decisions?
- **Q-020 - Distribution after pilot:** Continue with private GitHub Releases or move to BCG-managed infrastructure?

## Before executing memory dreaming

- **Q-021 - L1 inputs:** Which session, filesystem, task and user-confirmed signals may be persisted, and which are prohibited before sanitization?
- **Q-022 - Synthesis provider:** Which model/provider may process professional memory, and must the pipeline support offline or local-only operation?
- **Q-023 - Retention and budgets:** What are the default retention windows, rollup windows and per-layer context budgets for the pilot?
- **Q-024 - Lifetime eligibility:** Which repeated evidence may the weekly deep dream promote automatically, and how can a user inspect, correct or remove a lifetime memory?
- **Q-025 - Scheduling policy:** Which local daily/weekly windows, timezone-change behavior, catch-up limits, retry/backoff and unattended-model permissions should the pilot use within the layered native-schedule plus presence-recovery architecture?
- **Q-026 - User rights:** Which inspect, explain, correct, export and delete guarantees must exist before memory persistence is enabled?
- **Q-027 - Interrupted runs:** When may `bcgos doctor` clear a leftover dreaming lock, and what evidence or confirmation is required before recovery?

## Before enabling wiki navigation

- **Q-028 - Managed allowlist:** Which product decisions, specs, skills, agents, playbooks and documentation enter the first managed atlas?
- **Q-029 - Page taxonomy:** Which initial topics, entities and relationships justify first-class wiki pages rather than index records?
- **Q-030 - Memory exposure:** Which memory layers may produce private summaries, and which are pointer-only for the pilot?
- **Q-031 - Private compilation:** Which approved provider may compile owner/workspace content, and must local-only or offline operation be supported?
- **Q-032 - Invalidation SLA:** How quickly must correction, deletion or source-access revocation disappear from pages, backlinks, indexes and caches?
- **Q-033 - Knowledge commands:** Should the user-facing surface be `bcgos wiki`, `bcgos knowledge` or an agent-only capability in v0?
- **Q-034 - Rollup facets:** Which temporal, topic, entity and active-thread facets may the private wiki derive from L2, L3 and lifetime for the pilot?
- **Q-035 - Update freshness:** What freshness target and retry/backoff policy applies to managed changes, memory commits and correction events?
- **Q-036 - Private erasure:** Which private versions, logs and receipts must be deleted or crypto-erased after user correction, deletion or access revocation?
