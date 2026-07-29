# Spec 026 - Workspace-local adapter installation

Status: bounded Session Start implemented for Claude and Codex; complete Claude
lifecycle configuration implemented.

`bcgos adapter install --runtime claude|codex [workspace]` adds only
Maestro-owned commands to the runtime's workspace-local configuration. Codex
receives one bounded Session Start entry. Claude receives `SessionStart`,
`UserPromptSubmit`, `PreToolUse`, `PostToolUse` and `Stop` entries mapped to the
canonical lifecycle. Claude uses `.claude/settings.local.json`; Codex uses
`.codex/hooks.json`. This avoids mutating a user-wide configuration and keeps
the adapter scoped to a professional workspace.

The same command also installs the user-facing runtime projection from the
active base bundle. Claude receives a managed `CLAUDE.md` and the complete
base-skill bodies under `.claude/skills/<skill-id>/SKILL.md`; Codex receives the
equivalent `AGENTS.md` and `.codex/skills/<skill-id>/SKILL.md`. The orientation
explains the Agentic OS blocks (session/hooks, owner SELF, memory, brain/wiki
navigation and agents) while remaining pointer-oriented. The projection writes
`.bcgos/runtime-projection.json` with hashes and uses explicit Maestro markers.
Reinstallation replaces only the managed block and unchanged managed skill
files. User-authored orientation text is preserved; modified or symlinked
managed files fail closed and are reported as conflicts.

Installation preserves unrelated configuration entries and is idempotent.
The commands point to the local released executable, rather than relying on a
consultant's PATH; reinstalling after an update replaces only Maestro's owned
entries. Claude `status` is installed only when every lifecycle binding has its
expected timeout and async mode; `uninstall` removes only those owned entries.
The installer also records the generated local configuration path in the
workspace Git exclusion file when one exists, so an absolute machine-specific
executable path is not accidentally committed.
If that configuration is already tracked by Git, installation fails before any
write; an ignore rule cannot protect a file already in the index.
Every installed command has a two-second timeout. Claude `PostToolUse` and
`Stop` are explicitly asynchronous; the other bindings perform only their
bounded inline responsibility. No binding starts a worker or makes a
network/model request.

The projection is local workspace materialization, not a capability claim:
native runtime capabilities remain `unavailable` until the conformance protocol
produces qualifying evidence.

The runtime still requires its ordinary local trust/review behavior. An
installed configuration is not proof that a runtime executed the hook; later
doctor and conformance work will report that distinction. See Spec 021 for the
runtime receipt required before capability promotion.
