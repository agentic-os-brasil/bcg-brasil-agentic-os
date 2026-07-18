---
name: start-contributing
description: Guide a first-time or non-technical contributor through setting up this source repository safely. Use when someone has just cloned the repository, does not know Git, needs Git identity or local hooks configured, or asks how to make a first contribution. This is contributor onboarding, not pilot-user installation through bcgos.
---

# Start Contributing

Make the first contribution feel like a guided conversation. Do not assume knowledge of Git vocabulary.

## Workflow

1. Explain the four concepts in one sentence each: a branch isolates work, a commit saves a named checkpoint, a push sends that branch to GitHub, and a pull request asks a human to review it.
2. Run `go run ./dev/harness doctor`. Translate every warning into plain Portuguese.
3. If Git name or email is missing, ask the contributor for the exact values before configuring them locally. Never invent an identity.
4. Run `go run ./dev/harness setup` to install this clone's repository-owned hooks.
5. Run `go run ./dev/harness validate` and resolve only concrete failures.
6. Show the contributor the current state and give exactly one next action: use `$start-work` with a short description of the intended change.

Do not teach rebase, force push, detached HEAD or recovery internals in the golden path. Do not modify files, branch or remote history merely to demonstrate Git. Never request, display or store credentials in repository files.

If any command fails, say what happened, confirm that no work was discarded, and provide one safe recovery action. Use `$recover-work` when the state is unclear.
