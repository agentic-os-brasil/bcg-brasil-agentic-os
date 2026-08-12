# Archived release workflows

Preserved as historical artifacts on 2026-08-11 during the Wave 2 ZIP-pivot
(surgical Go delete). These workflows described the abandoned
signed-release/bootstrapper distribution path. Kept here so future reviewers
can trace why the ZIP-by-email route was chosen without needing
`git log --diff-filter=D`.

- `pilot-evidence.yml.disabled` — pilot evidence capture step of signed pipeline.
- `release-candidate.yml.disabled` — candidate build pipeline.
- `signed-prerelease.yml.disabled` — signed prerelease publish.

Do not re-enable. See the Wave 2 ZIP-pivot PR for the current distribution
contract (`installers/zip/`).
