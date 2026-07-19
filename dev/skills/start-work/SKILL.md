---
name: start-work
description: Start a safe daily contribution in the BCG Brasil Agentic OS source repository. Use when a contributor wants to begin or resume work, needs a feature branch, or is unsure whether local changes make it safe to update. Especially suitable for contributors with little Git experience.
---

# Start Work

Begin from evidence and leave existing work untouched.

## Workflow

1. Run `go run ./dev/harness doctor` and `git status --short --branch`.
2. If there are local changes, do not pull, switch branches, stash or discard anything automatically. Explain what is present and use `$recover-work` if ownership is unclear.
3. If the tree is clean and the contributor is on `main`, run `git pull --ff-only origin main`.
4. Ask for a short work description, derive a lowercase hyphenated branch name, show it, then create it with `git switch -c <branch>`.
5. Restate the task and route implementation through `$develop-change`.

Never work directly on `main`. Never use rebase or force push in the default path. A failed fast-forward pull is a stop condition: nothing was lost, and the next action is `$recover-work`.
