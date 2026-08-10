# Spec 025 - Native Session Start hook payload

Status: native command payload and local configuration installation implemented;
runtime conformance receipts pending.

`bcgos hook session-start --runtime claude|codex [workspace-path]` is the thin
direct-conformance entrypoint. Installed Claude configuration invokes the
equivalent runtime-specific `bcgos hook claude session-start` entrypoint, while
Codex invokes the shared command with `--runtime codex`. Each emits one bounded
Session Context Packet as additional context. Claude and Codex use separate
adapter serializers; the shared envelope is not evidence that their native
hook-output protocols are interchangeable.

The command is deliberately read-only: it builds the existing pointer-only
packet, resolves only the newest fully valid generated memory commit for the
exact workspace, does not wait for a worker and does not make a network or
model request. It never reads raw captures or history as a fallback. If memory
is missing it reports an active empty state; invalid state is unavailable. The
native configuration installer is a separate concern, and even an installed
command leaves native lifecycle capabilities `unavailable` until qualifying
native-session evidence exists.

Claude and Codex receive the same packet body, differing only in the explicit
runtime field. Each native adapter must prove its own output shape and use a
short explicit timeout. Its output is capped at 8 KiB; an oversized future
packet becomes an explicit omission rather than a failing or slow Session
Start.

The Codex serializer follows the documented `SessionStart` command-hook output
shape, including `hookSpecificOutput.hookEventName` and `additionalContext`.
That evidence does not make it a shared Claude/Codex protocol: each serializer
remains versioned and tested independently.

The same pointer-only envelope serializer serves each runtime's
`UserPromptSubmit` binding. That binding may add at most two governed installed
skill pointers and reasons, or a metadata-only `confirmed` state after an exact
pending external-action challenge response. It never injects a skill body,
stores the submitted prompt, or treats emitted payload as native conformance
evidence.

Only `SessionStart` carries the full Maestro startup protocol: workspace
identity, one deterministic onboarding action, and the bounded local open-task
state. It also renders the Spec 044 continuous-use state, including only an
opaque active-work pointer, checkpoint presence, per-runtime evidence classes
and the first safe next action. It does not resolve or inject execution
content. When a valid local memory commit exists, Session Start also carries its
generated layers in canonical broad-to-recent order within managed per-layer
budgets and the existing 8 KiB total ceiling. The serialized packet retains
only `bcgos://memory/<layer>` pointers and truncation state. `UserPromptSubmit`
carries a short identity-continuity reminder plus the same pointer-only packet;
it must not repeat the greeting, memory bodies or interview on every prompt.
Both payloads expose
`adapter_delivery_state=adapter_payload_emitted` while retaining
`injection_state=unavailable`, so successful serialization is visible without
being mislabeled as native qualification. The field reports native evidence,
not product availability; configured context injection remains enabled.

While onboarding is `required`, `in_progress` or `review_required`, the
lifecycle selects the installed, integrity-checked `maestro-onboarding` pointer
with a closed reason code and suppresses unrelated contextual Case methods.
After onboarding becomes `complete`, ordinary explicit/lexical method routing
resumes. The selected pointer must remain under the active runtime's managed
`.claude/skills` or `.codex/skills` projection and must match its skill ID;
absolute, mismatched or caller-invented pointers fail validation.

At the onboarding review boundary, Session Start presents the bounded
`review_digest` and the exact confirmation command. The command must echo that
digest; confirmation without it, or after any facet changes, fails closed and
leaves onboarding in `review_required`.
