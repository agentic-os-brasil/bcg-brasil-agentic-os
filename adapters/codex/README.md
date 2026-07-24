# Codex product adapter

This is the thin product adapter boundary for Codex. Policies, memory and
capability states remain canonical in `bundles/base/runtime/capabilities.json`.

Current state: `bcgos doctor` discovers a local Codex executable, while every
product lifecycle event remains explicitly unavailable. Codex must not inherit
Claude-specific development hooks as a product capability.

Future wiring may map Codex-native mechanisms to `session_start`,
`pre_action_guard`, `post_action_observe`, `stop_finalize` and
`context_inject`. It must add conformance fixtures before changing a capability
state. At Session Start it must also resolve the user-local interaction profile
and inject only its bounded ID and managed policy pointer; the profile must not
be derived from or persisted into memory.
