---
name: recover-work
description: Diagnose confusing or blocked Git states without discarding work. Use when the tree is dirty unexpectedly, a pull or branch switch failed, a hook blocked commit or push, the contributor is on main, or a novice says they are lost. Provide one safe next action and require human confirmation before any mutation.
---

# Recover Work

Protect the contributor's files first. Diagnosis is read-only until the situation is understood.

## Workflow

1. Run `go run ./dev/harness recover`, `git status --short --branch` and `git diff --stat`.
2. State what exists in plain Portuguese: current branch, modified files, staged files, untracked files and whether commits are ahead or behind.
3. Say explicitly that nothing was deleted or overwritten during diagnosis.
4. Choose one non-destructive next action. Prefer finishing the current work, creating a branch that preserves it, or asking the contributor who owns unfamiliar files.
5. Explain the expected result, ask for confirmation, then run only that action.
6. Re-run `go run ./dev/harness doctor`.

Never use `reset --hard`, `clean -f`, branch deletion with `-D`, checkout/restore to discard files, force push or automatic stash. Never offer a menu of advanced Git repair commands to a novice. If safe ownership cannot be established, stop and ask Daniel or another maintainer.
