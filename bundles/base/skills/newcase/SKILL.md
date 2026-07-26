---
name: newcase
description: Prepare a new-workspace kickoff packet from user-supplied proposal details and identify missing information without creating a workspace. Use for “start a new case”, “onboard this project” or “prepare a case kickoff”.
---

# New Case

Resolve the canonical `interaction-profile` before responding. It changes
presentation only; it never reads a deck, creates a workspace or promotes context.

## Orchestration contract

- Accept only proposal details, references and constraints supplied now.
- Return a kickoff packet: mandate, stakeholders, scope, risks, open questions
  and a pointer to the separately invoked `workspace-agent-setup` route when
  the user chooses to initialize a workspace.
- Do not invoke `workspace-agent-setup`, `bcgos` or any command; do not ingest
  a file, create atlas records, seed tasks, inspect a workspace or infer client
  facts.

## Completion

Return a reviewed kickoff proposal. Workspace initialization and ingestion are
separate authorized capabilities that must be invoked in their own interaction.
