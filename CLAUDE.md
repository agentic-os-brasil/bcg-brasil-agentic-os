# Claude runtime orientation

Claude Code is the primary development runtime for this repository and the reference contributor experience. Read `AGENTS.md` and follow the same canonical repository contract. Claude-specific hook names, payloads and paths are adapter concerns rather than canonical architecture.

## Mandatory skill routing

Route contribution work through the native skills declared in `.claude/skill-routing.json`. When a route applies:

1. invoke the matching `$skill` exposed under `.claude/skills/`;
2. read that projection completely;
3. read and follow its canonical `dev/skills/<name>/SKILL.md` completely;
4. do not reconstruct or bypass the workflow manually while the skill is available.

Use `$start-contributing` for a first contribution, `$start-work` to start or resume work, `$develop-change` for general implementation, `$evolve-memory` for memory persistence and dreaming, `$record-decision` for a durable choice, `$prepare-pr` for delivery and `$recover-work` whenever Git state or the safe next action is unclear.

If a required skill, projection or hook is missing or broken, stop the affected workflow, explain the gap and repair or escalate it. Do not silently continue without the harness.

For a contributor who is new to Git, read `.claude/README.md` and begin with `$start-contributing`. Guide one action at a time, explain terms in plain language and never discard work to repair a Git state. A human always decides merge.

Canonical development skills remain under `dev/skills/`. Claude projections must stay thin and development harness content must never enter the user bundle. Codex remains compatible through the shared contract in `AGENTS.md`, but Claude is the primary, first-tested development surface.
