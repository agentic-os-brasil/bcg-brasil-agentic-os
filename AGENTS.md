# Agent orientation

This repository is the early foundation of the BCG Brasil Agentic OS.

Before changing it:

1. Read `README.md`.
2. Read `docs/FOUNDING-DECISIONS.md`.
3. Read the relevant file under `specs/`.
4. Preserve the boundary between managed core and local/client data.
5. Do not claim planned CLI behavior is implemented until tests prove it.
6. For source implementation, fixes or refactors, follow `dev/skills/develop-change/SKILL.md`.
7. For durable project decisions, follow `dev/skills/record-decision/SKILL.md`.
8. For first-time contributor setup, follow `dev/skills/start-contributing/SKILL.md`.
9. Start daily work with `dev/skills/start-work/SKILL.md`, prepare review with `dev/skills/prepare-pr/SKILL.md`, and diagnose Git problems with `dev/skills/recover-work/SKILL.md`.

The architecture is intentionally incomplete. Prefer small, reversible changes and record structural decisions before implementation.

Claude is the primary runtime, but not the architecture source of truth. Codex must consume the same canonical contracts, state schemas and policies through a thin adapter. Product runtime mechanics belong in `adapters/`; development-tool projections may live in `.claude/` while canonical policy remains under `dev/` and `internal/dev/`.

The development-only gate is `go run ./dev/harness validate --full`. Nothing under `dev/` or `internal/dev/` belongs in the distributed CLI or OS bundle.

Never push directly to `main`, force push, merge a PR autonomously or discard local work. Run `go run ./dev/harness doctor` when contribution state is unclear. Every safety block must explain the reason, confirm that nothing was lost and provide one safe next command.
