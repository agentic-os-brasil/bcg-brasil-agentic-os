# Claude product adapter

This is the thin product adapter boundary for Claude. Policies, memory and
capability states remain canonical in `bundles/base/runtime/capabilities.json`.

Current implementation: a workspace-local adapter configures Claude
`SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse` and `Stop`
against the same canonical lifecycle vocabulary. Session and context are
bounded pointer-only packets; the guard only denies an unambiguous local root
deletion; post/stop emit asynchronous metadata-safe receipt signals. `bcgos
doctor` diagnoses those receipts separately from capability state.

Every product lifecycle event remains explicitly `unavailable` in the manifest
until a qualifying real Claude session is recorded through the pilot
conformance protocol. Local configuration, direct-command tests and development
hooks are not that evidence.

Codex remains fixture-parity only for this vertical. At Session Start Maestro
resolves the user-local interaction profile and injects only its bounded ID and
managed policy pointer; the profile must not be derived from or persisted into
memory.
