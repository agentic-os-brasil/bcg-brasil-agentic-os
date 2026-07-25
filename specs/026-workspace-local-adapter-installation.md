# Spec 026 - Workspace-local adapter installation

Status: implemented for bounded Session Start only.

`bcgos adapter install --runtime claude|codex [workspace]` adds only one
Maestro-owned Session Start command entry to the runtime's workspace-local
configuration. Claude uses `.claude/settings.local.json`; Codex uses
`.codex/hooks.json`. This avoids mutating a user-wide configuration and keeps
the adapter scoped to a professional workspace.

Installation preserves unrelated configuration entries and is idempotent.
`status` identifies whether the exact Maestro entry is present; `uninstall`
removes only that exact entry. The installed command has a two-second timeout
and invokes the bounded, read-only Session Start entrypoint. It does not start
a worker or make a network/model request.

The runtime still requires its ordinary local trust/review behavior. An
installed configuration is not proof that a runtime executed the hook; later
doctor and conformance work will report that distinction.
