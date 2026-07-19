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

## Before the ten-person pilot

- **Q-016 - Pilot cohort:** Which personas and operating systems are represented, and who provides support?
- **Q-017 - Success metrics:** Time-to-first-success, weekly active use, update success, support demand and qualitative value.
- **Q-018 - Telemetry and privacy:** What may be measured without collecting prompts, client content or personal data?
- **Q-019 - Incident path:** Who owns broken installs, credential failures, unsafe outputs and rollback decisions?
- **Q-020 - Distribution after pilot:** Continue with private GitHub Releases or move to BCG-managed infrastructure?
