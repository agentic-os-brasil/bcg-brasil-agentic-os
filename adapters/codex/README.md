# Codex product adapter

This is the thin product adapter boundary for Codex. Policies, memory and
capability states remain canonical in `bundles/base/runtime/capabilities.json`.

Current state: `bcgos doctor` discovers a local Codex executable, while every
product lifecycle event remains explicitly unavailable. `bcgos session bridge
--runtime codex [workspace-path]` supplies the same bounded Session Start
envelope for a future adapter to consume; it does not install a hook or inject
content. Codex must not inherit Claude-specific development hooks as a product
capability.

Future wiring may map Codex-native mechanisms to `session_start`,
`pre_action_guard`, `post_action_observe`, `stop_finalize` and
`context_inject`. It must add conformance fixtures before changing a capability
state. At Session Start it must also resolve the user-local interaction profile
and inject only its bounded ID and managed policy pointer; the profile must not
be derived from or persisted into memory.

## Maestro long-running boundary

The shared `internal/longrun` core and `bcgos goal` lifecycle are implemented.
A future Codex adapter must act as a workspace-loop adapter: it may resolve
workspace context under existing authorization, but must return only a typed
`WorkspaceCheckpoint` or `WorkspaceResult`. Specialist output must return
through that workspace adapter; it may not be written into a Maestro goal
directly. Walter receives only `WalterRecord` and returns a revision-matched
`WalterReview` to Maestro. Native scheduling or hooks remain unavailable until
this mapping has equivalent Claude/Codex conformance fixtures. The adapter
must also provide a secure monotonic anchor outside the user-local state root;
macOS uses the shared Keychain implementation and Windows uses Credential
Manager. Without it, the core fails closed rather than allowing recovery.
