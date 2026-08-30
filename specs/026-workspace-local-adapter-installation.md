# Spec 026 - Workspace-local adapter installation

Status: seven-event Claude and five-event Codex projection cores implemented;
the direct Git Repo/Worktree lifecycle is exposed through `bcgos workspace`.
The older initialized-workspace `bcgos adapter` public route described below
is not exposed by the current narrow CLI. Native runtime qualification remains
separate.

The initialized-workspace adapter contract historically grouped installation
under `bcgos adapter install --runtime claude|codex [workspace]`. Its core first ensures the
workspace-local installation dependencies (`workspace.json`, durable
orchestration state, owner registry, Case Agent dossier and signed agent
scaffold) exist idempotently, then adds only Maestro-owned commands to the
runtime's workspace-local configuration. Both
Claude receives `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`,
`Stop`, `SubagentStart` and `SubagentStop`; Codex receives the original five
events. Claude uses
`.claude/settings.local.json`; Codex uses `.codex/hooks.json`. This avoids
mutating a user-wide configuration and keeps the adapter scoped to a
professional workspace.

For an ordinary Git repository or linked worktree, Spec 055 provides the
implemented public route:

```text
bcgos workspace enroll|status|repair|remove --runtime claude|codex <path>
```

That route does not create the older managed-workspace authorities. It resolves
Git repository/worktree identity, stores the private binding under `data_root`
and invokes the projection/adapter cores transactionally with explicit
`managed_root`, `data_root` and `workspace_root`. The distinction avoids
claiming that the historical `bcgos adapter` command exists when only its
underlying projection contract remains implemented.

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

Spec 055 adds one direct-Git exception: when the orientation file is already
tracked, direct enrollment leaves it byte-for-byte user-owned and records a
`preserved_tracked` projection mode. The bounded Session Start hook supplies
the Maestro orientation in that checkout. This prevents a local installation
from dirtying a tracked team instruction file or altering the Git index.

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
Every installed command has a five-second timeout. Claude `PostToolUse` is
explicitly asynchronous. Claude `Stop` is synchronous because
it owns the native-agent completion gate; the other bindings perform only their
bounded inline responsibility. No binding starts a worker or makes a
network/model request.

Both adapters preserve the bounded native session, prompt and complete
tool-input JSON in memory long enough to route method pointers or digest an
external mutation. A caller-supplied `actor_id` is ignored; the gate resolves
the actor from the confirmed owner enrollment and authenticated local OS
principal. Raw prompt/tool input is not written to challenge state. External
mutation remains denied until the same runtime, workspace, locally resolved
actor and session submit the exact short-lived challenge phrase and
`PreToolUse` atomically consumes it. Binding values at rest use a generated,
private workspace-local HMAC key.
The adapter never converts an environment variable or installation flag into
user approval.

The projection is local workspace materialization. Exact Claude adapter and
managed-agent inspection enables `operational_beta`; native qualification
remains independent telemetry until the conformance protocol produces fresh
evidence.

The dependency bootstrap is data-free and does not select an onboarding track,
write owner answers, ingest a memory source or install an external runtime.
Malformed or symlinked state fails before adapter files are written; a valid
existing state is preserved.

The installed projection is also the source of the first-use guide. Until the
reviewed owner onboarding is complete, lifecycle hooks select only the exact,
hash-checked `maestro-onboarding` pointer and suppress unrelated Case methods.
This startup precedence does not grant tools, data access, agent authority or
native-runtime evidence. Once onboarding is confirmed, ordinary governed
method selection resumes from the same verified projection.

A target-specific portable Windows or macOS package may seed user-authored
first-use guidance before adapter installation. The guidance may ask for one
conversational setup confirmation and direct Claude Code to invoke the matching
package-internal deterministic activator, but it must never ask the owner to
type or run a command. The normal projection preserves that seed text, appends
only the marked managed block and then becomes the authority for subsequent
onboarding. The seed cannot grant tools, suppress native host permissions,
invoke another platform's payload or substitute for readiness checks.

The runtime still requires its ordinary local trust/review behavior. An
installed configuration is not proof that a runtime executed the hook; later
doctor and conformance work will report that distinction. See Spec 021 for the
runtime receipt required before capability promotion.
