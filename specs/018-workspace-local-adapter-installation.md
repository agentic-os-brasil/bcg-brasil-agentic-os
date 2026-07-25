# Spec 018 - Workspace-local adapter installation

Status: implemented for bounded Session Start only.

`bcgos adapter install --runtime claude|codex [workspace]` adds only one
Maestro-owned Session Start command entry to the runtime's workspace-local
configuration. Claude uses `.claude/settings.local.json`; Codex uses
`.codex/hooks.json`. This avoids mutating a user-wide configuration and keeps
the adapter scoped to a professional workspace.

Installation preserves unrelated configuration entries and is idempotent.
The command points to the local released executable, rather than relying on a
consultant's PATH; reinstalling after an update replaces only Maestro's owned
entry. `status` identifies whether the owned entry is present; `uninstall`
removes only that entry. The installer also records the generated local
configuration path in the workspace Git exclusion file when one exists, so an
absolute machine-specific executable path is not accidentally committed.
If that configuration is already tracked by Git, installation fails before any
write; an ignore rule cannot protect a file already in the index.
The installed command has a two-second timeout and invokes the bounded,
read-only Session Start entrypoint. It does not start a worker or make a
network/model request.

The runtime still requires its ordinary local trust/review behavior. An
installed configuration is not proof that a runtime executed the hook; later
doctor and conformance work will report that distinction. See Spec 021 for the
runtime receipt required before capability promotion.
