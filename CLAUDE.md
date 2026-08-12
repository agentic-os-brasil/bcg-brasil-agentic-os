# Claude runtime orientation

Claude Code is the primary development runtime for this repository and the reference contributor experience. Read `AGENTS.md` and follow the same canonical repository contract. Claude-specific hook names, payloads and paths are adapter concerns rather than canonical architecture.

## Mandatory skill routing

Route contribution work through the native skills declared in `.claude/skill-routing.json`. When a route applies:

1. invoke the matching `$skill` exposed under `.claude/skills/`;
2. read that projection completely;
3. read and follow its canonical `dev/skills/<name>/SKILL.md` completely;
4. do not reconstruct or bypass the workflow manually while the skill is available.

Use `$start-contributing` for a first contribution, `$start-work` to start or resume work, `$develop-change` for general implementation, `$evolve-memory` for memory persistence and dreaming, `$maestro-local-beta-installer` to build a source-repository local-beta installer, `$release-export` to export and publish a release, `$generate-portable-zip` to orquestrar candidate + bootstrappers + release-export em um único fluxo local-beta, `$record-decision` for a durable choice, `$prepare-pr` for delivery and `$recover-work` whenever Git state or the safe next action is unclear.

If a required skill, projection or hook is missing or broken, stop the affected workflow, explain the gap and repair or escalate it. Do not silently continue without the harness.

For a contributor who is new to Git, read `.claude/README.md` and begin with `$start-contributing`. Guide one action at a time, explain terms in plain language and never discard work to repair a Git state. A human always decides merge.

Canonical development skills remain under `dev/skills/`. Claude projections must stay thin and development harness content must never enter the user bundle. Codex remains compatible through the shared contract in `AGENTS.md`, but Claude is the primary, first-tested development surface.

## Folder structure

The user-facing installation is organized around three top-level surfaces:

- `.claude/` — Claude Code harness: skills projections, hooks, settings.
- `bundles/` — canonical OKF-1 bundles that make up Maestro. Includes:
  - `base/` — user-facing skills, agents, contracts, memory and profile policies, distribution and manifest.
  - `tech-core/` — engineering skills bundle (tests, review, pipelines, spec-driven delivery). Loaded on demand.
  - `catalog/` — reserved extension slot for user-populated skills in future releases.
- `data/` — user workspace, created by the first-run scaffold and never overwritten by updates. Subtree:
  - `agents/` — per-agent state (working memory, decisions, context).
  - `cases/` — active client cases; each case has `brain/` (projects, decisions, tasks, deliverables, sources, canon). `cases/.active` points at the active case ID.
  - `memory/` — tiered memory used by dream-memory and session injection:
    - `lifetime/` — L3 long-term compact, high-signal facts.
    - `medium-term/` — durable summaries between L2 and L3.
    - `weekly/` — L2 weekly syntheses.
    - `recent/` — L1 daily consolidated logs.
    - `policies/` — memory rules read by dream-memory.
  - `owner/` — owner context tree:
    - `self/` — ten SELF facets (identity, preferences, quality bar, decision rules, etc.) filled by `/maestro-onboarding`.
    - `interview/` — interview drafts and confirmations.
    - `observations/` — append-only observations log.
    - `operating/` — work-state continuity between sessions.
  - `workspaces/` — active projects that are not case-scoped.
  - `profile/` — identity, preferences, style and onboarding markers.
- `VERSION` — bundle version marker (source of truth for `bundle_version` in every manifest).
- `README-INSTALL.md` — first-run and update ritual for the non-technical user.

## Runtime dependencies

The scaffold and session-start hooks depend on the `CLAUDE_PROJECT_DIR` environment variable, which Claude Code injects when a project is opened through the standard flow. The hooks include a hardened fallback that locates the project by looking for a `VERSION` file next to the hook, but in some non-standard setups the injection may still fail:

- Paths under `/tmp`, `/private/tmp` or other system-managed temp locations
- Paths with spaces or unicode characters
- External drives mounted after the CLI started

When `CLAUDE_PROJECT_DIR` is unset and no `VERSION` file is found nearby, the scaffold hook exits fail-open with a note on stderr instead of scaffolding into the wrong directory. If your `data/` workspace is missing after a first session, open the folder from a canonical path (e.g. `~/Documents/Maestro`) or invoke `maestro-doctor` to regenerate the workspace explicitly. The full caveat is tracked in `bundles/base/known-issues.md` under `claude-project-dir-nonstandard-path`.
