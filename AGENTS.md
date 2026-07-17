# Agent orientation

This repository is the early foundation of the BCG Brasil Agentic OS.

Before changing it:

1. Read `README.md`.
2. Read `docs/FOUNDING-DECISIONS.md`.
3. Read the relevant file under `specs/`.
4. Preserve the boundary between managed core and local/client data.
5. Do not claim planned CLI behavior is implemented until tests prove it.

The architecture is intentionally incomplete. Prefer small, reversible changes and record structural decisions before implementation.

Claude is the primary runtime, but not the architecture source of truth. Codex must consume the same canonical contracts, state schemas and policies through a thin adapter. Runtime-specific mechanics belong only in `adapters/`.
