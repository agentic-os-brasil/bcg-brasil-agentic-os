# Native Codex ZIP adapter

The v0.2.0 ZIP template provides AGENTS.md, .codex/config.toml, .codex/hooks.json,
six project agent TOMLs and native .agents/skills projections of canonical
bundles/base/skills. Project configuration and hooks require the owner's Codex project
trust. Installation never grants trust or overrides sandbox/approval policy.
The factory's shared Go libraries remain canonical; the ZIP helper is
maestro-runtime, not the retired bcgos command surface.

## Runtime behavior

Attended setup invokes the explicitly verified helper's bind-codex --root against
the owner-confirmed canonical installation. Before binding, the portable template
denies file-edit and agent tools while shell setup uses normal host approvals.
The binder verifies manifest/version/helper SHA256 and pins absolute shell-quoted
and PowerShell-encoded commands, preserving user hooks. Modified managed entries
and aliased roots/configs are refused. No ancestor search selects executable code.
The bound platform launcher resolves the installation from its own path, validates the
runtime manifest and helper SHA256, and invokes the packaged native executable.
Windows uses PowerShell 5.1/7; macOS uses Bash 3.2 and the shared native locator.
Neither Codex hook path requires Python or Git Bash. Manifest hashes detect
accidental corruption, not malicious replacement of the package; organization
signing/distribution remains a separate release gate.

Project trust is a separate owner action; reload the project after binding.
Relocation requires attended rebinding/reconciliation. A record bound elsewhere
is never silently retargeted. Local tests execute the real packaged macOS wrapper
from nested cwd containing a hostile nested .codex directory, plus PowerShell
quoted-path fixtures. These are local adapter tests, not native Codex qualification.

| Hook | ZIP implementation | Evidence limitation |
| --- | --- | --- |
| SessionStart | Transactional copy-only migration; stop on blocked/partial/rolled-back status; bounded operating pointers | Does not prove native hook invocation or synthesize personal context |
| UserPromptSubmit | Bounded canonical skill and scope pointers | No prompt capture or inferred consent |
| PreToolUse | 64 KiB input bound, protected-root removal guard, alias-aware case checks for Write/Edit/NotebookEdit/apply_patch including Move targets | Shell and specialized tool paths are not a complete enforcement boundary |
| PostToolUse | Shared metadata-only adapter_command receipt | Does not qualify native invocation |
| Stop | Shared metadata-only adapter_command receipt | Does not synthesize checkpoints, dream memory, or enforce the full account route |

File paths resolve existing ancestors, including symlink aliases. Malformed
write payloads fail closed using hookSpecificOutput.permissionDecision=deny.
The hook understands Agent and spawn_agent aliases but does not infer delegated
authority from their name. Native sandbox controls and bounded role instructions
still apply. PreToolUse does not use continue:false, which is unsupported there.

Codex project agent files name case-agent, client-account-agent, yoda, darwin,
gamma-guardian and pa-expert; each loads its canonical bundles/base/agents role. One concurrent specialist is
configured. The main session is Maestro. The existing internal/agentorchestration,
agentdispatch and Yoda custody contracts are libraries; these definitions do not
prove their full native mediation or tool-denial behavior. In particular,
specialist depth/no-child policy remains an instruction rather than an attested
native security boundary.

## caseOS

Use the caseos-connect skill. No endpoint path, token, private case or personal
allowlist is shipped. Setup needs an externally supplied official HTTPS endpoint
on the exact approved production host, valid TLS/MCP handshake, case-lead access,
ZPA, native Okta OAuth and explicit workspace/case consent. Unknown endpoint paths
and stale development URLs remain unavailable.

The helper caseos-check probes a supplied endpoint without credentials, rejects
redirects and persists nothing. RouteCaseOS is the reusable read-only connector
boundary: consent, workspace/case enrollment, live health/discovery, reviewed
read-only tool and live schema validation. The native host supplies the connector;

Enrollment binds each allowed tool to its exact case parameter in CaseParameters.
The request cannot select or redirect that parameter. Live discovery must still
declare that enrolled parameter as a required string property; missing or changed
bindings fail closed before the call. No production handshake is inferred.

the ZIP does not implement a second OAuth client or forcibly proxy all MCP calls.
The skill actively routes eligible work through discovered native tools. Export
requires separate approval and is not enabled by the read adapter. Offline work
continues locally with no automatic upload replay.

Codex must not collect SharePoint data or use browser/plugin/token fallbacks to
circumvent the existing corporate-policy boundary. It may query the verified
local metadata/pointer index produced by the approved Claude path.

## Qualification

Local fixture tests cover case isolation, patch targets, symlink aliases,
malformed/oversized payloads, migration, lifecycle metadata, missing consent,
wrong workspace/case, export denial, untrusted endpoints, redirects and schema
drift. Fake connector tests are local contract evidence. Installed configuration,
adapter observation and fresh attended native qualification are separate states.
Native Windows/macOS sessions, OAuth and production caseOS health must be tested
on the actual supported host. None is inferred from this documentation.

Official mechanics checked for this implementation:
- https://learn.chatgpt.com/docs/hooks (JSON commandWindows; project trust;
  apply_patch/Edit/Write matchers; Agent alias; specialized-path limitations)
- https://learn.chatgpt.com/docs/agent-configuration/subagents (standalone project
  TOML agents with name, description and developer_instructions)
- https://learn.chatgpt.com/docs/extend/mcp?surface=cli (native configuration/OAuth)
- https://code.claude.com/docs/en/mcp (native local/project MCP scopes)
