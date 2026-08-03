# Spec 019 - Non-blocking product hook execution

Status: policy implemented and validated; Claude mapping implemented with
native conformance evidence pending.

## Objective

Keep Maestro responsive even while memory, wiki, ingestion or refinement work is
pending. A lifecycle hook is a bounded interface to the running agent, never a
place to synchronize or perform durable background work.

## Execution contract

| Semantic event | Inline responsibility | May block? |
|---|---|---|
| `session_start` | Read the last committed bounded snapshot or report an omission. | No |
| `context_inject` | Read the last committed bounded snapshot, route at most two installed policy-allowed method pointers, or recognize one exact pending user confirmation. | No |
| `pre_action_guard` | Evaluate a local deterministic safety rule and atomically consume a matching external-action confirmation. | Only to deny an unsafe or unconfirmed action. |
| `post_action_observe` | Emit an idempotent signal for later processing. | No |
| `stop_finalize` | Emit an idempotent signal for later processing. | No |

No event may wait for a worker lock, call a model, use the network, perform a
rollup, compile the wiki, ingest a document, reconcile state or retry work. A
snapshot reader uses the last fully committed version. If that version is
unavailable or stale, it returns an explicit partial/omitted state rather than
waiting.

## Worker boundary

The future worker owns serialized, idempotent writes and any expensive work.
Signals may be duplicated or dropped without corrupting a committed state; the
worker recovers from durable due-state and source watermarks. Its lock belongs
to the worker alone. A hook must never contend for it.

`pre_action_guard` is the only synchronous exception. It is limited to an
in-process deterministic decision over local committed data; it never waits for
another process to resolve a race. If safety cannot be established immediately,
the guard denies the action with an actionable explanation.

External publication and mutation use a short-lived local challenge bound to
runtime, workspace, locally attested owner actor, native session, canonical
action, target, input digest and expiry. The actor is derived from the confirmed
owner personalization enrollment plus the authenticated local OS principal;
native payload fields cannot declare it. Durable bindings are HMAC-SHA256 values
under a private workspace-local key, not replayable unkeyed hashes.
`UserPromptSubmit` recognizes only `CONFIRM MAESTRO <challenge-id>` for the
same pending binding. `PreToolUse` consumes that confirmation atomically once;
missing identity, expiry, replay, mutation, lock contention or a command outside
the bounded grammar denies without changing external state. Ordinary local
actions do not enter this path, and protected-root destruction remains an
absolute denial.

Non-shell external tool protection uses an exact namespace/tool-ID allowlist
for the supported GitHub, Outlook email, Teams and Slack mutations. Internal,
collaboration and workspace-agent messaging with a similar method suffix is not
classified as external publication by substring.

## Executable policy

`bundles/base/runtime/hook-policy.json` is the versioned contract. The parser
requires exactly the canonical semantic events and rejects a policy that lets
any event wait for a worker, make a network request or call a model. The
development validation and unit tests load this policy directly.

The Claude adapter maps these events and configures `PostToolUse` and `Stop` as
asynchronous command hooks. The Codex adapter maps the same five events to its
native command-hook surface; Codex command hooks are synchronous because its
runtime does not support asynchronous handlers yet. Both runtimes stay
`unavailable` in the capability manifest until their qualifying native
conformance evidence exists.
