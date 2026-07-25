# Spec 017 - Native Session Start hook payload

Status: native command payload and local configuration installation implemented;
runtime conformance receipts pending.

`bcgos hook session-start --runtime claude|codex [workspace-path]` is the thin
command entrypoint that a future native runtime configuration invokes. It emits
one bounded Session Context Packet as additional context. Claude and Codex use
separate adapter serializers; the shared envelope is not evidence that their
native hook-output protocols are interchangeable.

The command is deliberately read-only: it builds the existing pointer-only
packet, never reads a pointed owner, atlas or memory source, does not wait for
a worker and does not make a network or model request. If a source is not ready
the packet reports an omission. The native configuration installer is a
separate concern; until it is installed, product capabilities remain
`unavailable`.

Claude and Codex receive the same packet body, differing only in the explicit
runtime field. Each native adapter must prove its own output shape and use a
short explicit timeout. Its output is capped at 8 KiB; an oversized future
packet becomes an explicit omission rather than a failing or slow Session
Start.

The Codex serializer follows the documented `SessionStart` command-hook output
shape, including `hookSpecificOutput.hookEventName` and `additionalContext`.
That evidence does not make it a shared Claude/Codex protocol: each serializer
remains versioned and tested independently.
