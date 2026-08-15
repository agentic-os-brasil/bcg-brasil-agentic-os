# Spec 049 — Claude native-agent beta projection and enforcement

## Decision

The Claude beta projects five Maestro-owned project subagents under
`.claude/agents/`: Client Account Agent, Case Agent, Yoda, Darwin and PA
Expert. Maestro remains the main-session identity supplied by SessionStart and
is not projected as a child agent.

An exact adapter inspection reports this surface as `operational_beta` only
when every managed agent definition and all owned lifecycle bindings match the
installed contract. Native qualification remains independent telemetry and
does not disable a working beta path.

## Native route

Maestro chooses depth. The hook-enforced client paths are:

1. strategic: Client Account framing -> Case -> Client Account validation ->
   optional Yoda;
2. direct: Case -> optional Yoda.

Once Client Account framing starts, Claude `Stop` blocks until Case completes
and Client Account has validated the returned result. A re-entrant Stop event
with Claude's `stop_hook_active=true` is allowed to finish instead of issuing
the same block indefinitely; the incomplete route remains visible in bounded
telemetry. PA Expert is a
consultative, tool-free leaf. Darwin is a separate system-health route and may
not be mixed with client execution in the same turn. Only one managed
specialist may be active at a time. Hook retries are idempotent and flow state
contains identities and transitions only, never prompts, results or client
content.

## Authority and safety

- Specialists cannot delegate because the native Agent tool is absent.
- Client Account, Yoda, Darwin and PA Expert are tool-free in the beta.
- Case Agent receives only local file tools. Its CWD and every explicit path
  must remain inside the exact installed workspace; traversal and symlink
  escapes fail closed.
- The existing global guard continues to block protected-root deletion,
  external mutation without user-bound confirmation and cross-workspace work.
- User-owned agent files are never replaced or removed.
- Qualification receipts are telemetry. Missing optional evidence is not a
  feature gate; missing or contradictory installation state is.

## Installation

The Claude adapter installation owns the five agent projections and seven
native lifecycle bindings: SessionStart, UserPromptSubmit, PreToolUse,
PostToolUse, Stop, SubagentStart and SubagentStop. PostToolUse stays async;
Stop is synchronous so it can issue Claude's native completion block.

MarkItDown installation is deliberately outside this change. Claude may guide
that future optional setup, but this beta neither installs nor promotes it.
