---
name: newcase
description: Onboards a new case from a proposal deck — extracts client, project, team and commercial information, drafts the atlas files, and walks the user through only the gaps the deck doesn't answer. Use when a new case starts or a proposal is shared.
---

# New Case

An orchestration ritual: a specialist agent does the heavy extraction, the hub handles the
gap conversation the agent can't have with the user.

## Workflow
1. Confirm the proposal file path (ask if missing).
2. Call **`case-onboarder`** to read the deck, extract everything, draft the client +
   project atlas files, and return a 5-line case summary + a precise gap list (≤6 questions
   the deck genuinely doesn't answer).
3. Put the gap questions to the user (batched), then finalize the drafted files with the
   answers and add the case-history entry.
4. Seed the backlog with the obvious next steps.
5. Confirm what was created/updated and flag any remaining open item.

## Relations
- **Orchestration**, held by the hub. Calls `case-onboarder`; the finalized client/project
  files are thereafter maintained by `client-keeper` / `work-logger`.
