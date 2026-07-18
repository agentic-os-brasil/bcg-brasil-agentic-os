---
name: prepare-pr
description: Prepare completed source changes for human review in GitHub. Use when implementation is ready to validate, commit, push as a branch and turn into a pull request; derive the summary and test evidence from the actual diff. Never merge the pull request.
---

# Prepare PR

Turn a finished change into a reviewable handoff without hiding risk from a novice contributor.

## Workflow

1. Run `go run ./dev/harness doctor`; stop if the current branch is `main` or the state is unclear.
2. Review `git diff` and `git status --short`. Explain in plain language what changed and identify anything unrelated.
3. Run `go run ./dev/harness validate --full`.
4. Stage only explicit intended paths. Never use `git add .` when unrelated files exist.
5. Show the proposed commit message and staged file list. Ask for confirmation before creating the commit.
6. Commit. The local hook re-runs the full gate against the exact staged snapshot and blocks secrets, likely client files and main-branch commits.
7. Show the target remote and branch. Ask for confirmation before `git push -u origin HEAD`.
8. Draft the pull request from the real diff using `.github/pull_request_template.md`. Ask for confirmation before creating it.
9. Report the PR URL, validation evidence and any remaining review needs.

Never push directly to `main`, force push, merge a PR or claim success when validation failed, the tree is unexpectedly dirty or the push/PR did not complete. On a block, confirm no work was lost and provide exactly one next command.
