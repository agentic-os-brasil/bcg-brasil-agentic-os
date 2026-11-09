# Spec 065 — Methods through the three-tool MCP interface

Status: accepted construction contract. Part of #460; functional slice #463.

## Outcome

A Code or Desktop client that exposes ordinary Maestro tools can discover all 92
canonical methods, read their complete governed resources and inspect the exact
requirements of a manually selected method. Reuse the immutable package exposed
by MCP resources/prompts and Code projection. The two Code entry skills remain
entry points, not extra canonical methods.

## Closed read-only interface

`maestro_query` adds three variants:

- `methods.list`: sorted canonical metadata, offset and limit 1–32, default 16,
  explicit next offset and completion. Catalog reads do not initialize state.
- `methods.read`: exact `resource_uri` from the immutable methods namespace,
  byte offset and max_bytes 4–4096, default 4096. Return canonical full-resource
  SHA256/size plus valid UTF-8 text and next offset. Shared policy, provenance,
  catalog/license and every nested method resource use the same whitelist.
- `methods.plan`: exact canonical `method_id`, with its invocation restriction,
  required implemented operations, declared effects/output kinds and named
  domain/host/connector/provider dependencies. Conditional follow-on effects name their condition and optional capability separately from unconditional effects. No lexical selection or execution.

Unknown IDs, namespace aliases, URI traversal/encoding, private references and
invalid pages refuse. Resource reads never resolve a URI into a filesystem path.

## Requirements and authority

One embedded declaration covers every canonical ID exactly once. Required MCP
operation names are validated against the actual registered query/apply schemas.
Missing domain routes may declare a finite `fulfillment_operations` list instead;
the plan checks registration and reports missing names without executing them.
This declaration cannot fulfill a host, connector, provider or runtime dependency.
The declaration names requirements; it cannot grant tools, confirmation,
filesystem access, external effects, model use or a qualified native profile.

Keep content availability separate from execution requirements. Dependencies
without an admitted implementation or observed host adapter remain explicitly
unavailable with a next action. Pure conversational methods remain manual
instructions. Implemented operation names do not prove their inputs, human review,
native-client behavior or external dependencies. Explicit-user-only methods stay
manual and cannot be auto-selected by an intent matcher or caller approval flag.

## Acceptance

Use ClientSession.CallTool over the actual SDK and restarted compiled packet
server: list exactly 92 IDs, reconstruct canonical resources through bounded
pages, read shared policy/provenance, reject unknown/private/traversal/namespace
references, and preserve the complete state snapshot during read-only queries.
Validate malformed declarations and nonexistent operation names at the owning
catalog interface. Resolve the continuation method's real operation requirements,
create/start/checkpoint/pause one synthetic work item, restart the packet server
and resume the same item with the actual revision. Keep model execution, client
UI discovery and all-method functional parity separate from these proofs.
