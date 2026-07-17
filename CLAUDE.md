# Claude runtime orientation

Read `AGENTS.md` and follow the same canonical repository contract. Claude is the primary runtime and reference coverage target, but Claude-specific hook names, payloads and paths are adapter concerns rather than canonical architecture.

Use the canonical development skills under `dev/skills/`. Runtime projections must remain thin and must never move development harness content into the user bundle.
