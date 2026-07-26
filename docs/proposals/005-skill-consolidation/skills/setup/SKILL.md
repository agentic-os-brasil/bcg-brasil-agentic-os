---
name: setup
description: First-run onboarding of a new user — welcomes them, optionally ingests a CV / decks / folders to pre-fill, walks template-by-template filling the founding atlas files, connects integrations, and teaches how the system works. Resumable, with a progress log. Use on first run, or "set me up", "let's onboard".
---

# Setup

An orchestration ritual that stands up a new user's atlas from empty. Runs once (resumable);
it fills only the gaps on re-entry.

## Workflow
1. Ask the preferred language first; give a brief, warm overview of the journey.
2. Show the current state of the founding atlas files and ask where to start.
3. **Optional ingestion accelerator:** from a CV / decks / a folder, draft founding files
   (may call **`newcase`** → `case-onboarder` if a live case deck is provided).
4. Walk the founding documents template-by-template (profile: bio, case-history,
   working-preferences), writing via the owning keepers; update a progress log incrementally.
5. Verify integrations (task tool, calendar, mail) and explain what each unlocks.
6. Teach the daily rhythm, the skills, and the agent team in plain language.
7. Close with the progress recap and the single first move (open a fresh session, run
   `start-day`).

## Relations
- **Orchestration**, held by the hub. Calls `newcase` and the keeper agents to seed the
  atlas. The one skill that is allowed to create the founding `profile/` documents.
