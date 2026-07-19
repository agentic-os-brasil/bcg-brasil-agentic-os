# Claude runtime orientation

Read `AGENTS.md` and follow the same canonical repository contract. Claude is the primary runtime and reference coverage target, but Claude-specific hook names, payloads and paths are adapter concerns rather than canonical architecture.

Use the canonical development skills under `dev/skills/`. Runtime projections must remain thin and must never move development harness content into the user bundle.

For a contributor who is new to Git, read `.claude/README.md` and use `$start-contributing`. Guide one action at a time, explain terms in plain language and never discard work to repair a Git state.

Daily path: `$start-work` -> `$develop-change` -> `$prepare-pr`. If anything is unclear or blocked, use `$recover-work`. A human always decides merge.
