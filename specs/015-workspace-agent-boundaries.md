# Spec 015 - Workspace agent boundaries

Status: accepted architecture; local agent registry, compact state/dossier
bootstrap and interview surface implemented. Runtime authorization enforcement
and adapter conformance remain pending.

## Objective

Make each workspace a hard boundary for client/project context. A workspace
agent is both the specialist for that work and the gatekeeper for its raw
context, preventing a user's unrelated client work from becoming one shared
context blob.

## Architecture

```text
OS global (operational metadata only)
  -> account agent (curated, promoted cross-project knowledge)
    -> workspace agent (raw context owner and gatekeeper)
      -> capability specialists (minimum work packet only)
```

- A client/account can contain multiple workspaces.
- A workspace belongs to one client/project interaction and has one owning
  workspace agent.
- A capability specialist may assist a workspace agent but does not receive
  general browsing, memory or persistent access to the workspace.
- The global OS layer may operate installation, version, health, scheduling and
  authorization metadata. It never has a fallback right to read workspace
  documents or memory.

## Enforced scope

Every workspace-bound operation carries a `workspace_id` and an authorization
scope. File access, search, ingestion, memory, indexes, logs and intermediate
outputs must reject resources outside that scope by default. Prompt guidance
alone is not sufficient enforcement.

The canonical workspace registration must include, at minimum:

- workspace ID and owner;
- client/account and project labels chosen by the user;
- owning workspace-agent identity;
- approved local roots;
- information classification and retention policy;
- lifecycle state: active, archived or revoked.

Archiving or revoking a workspace invalidates its authorization scope and
access to its indexes. It does not silently delete user files.

## Context promotion

The account agent does not browse project workspaces. It may hold only facts
explicitly promoted from a workspace, with source, author, classification and
review status. Promotion is a deliberate handoff, not automatic memory
aggregation.

## Cross-workspace delegation

Cross-workspace access is denied unless the user explicitly requests it. An
approved delegation records requester, source workspace, destination workspace,
purpose, allowed artifact classes, expiry and audit event.

The source workspace agent prepares the smallest useful, redacted work packet.
The receiving agent gets neither general search nor persistent access to the
source workspace. The grant expires or can be revoked independently.

## User experience

The active workspace and owning agent must be visible before substantive work.
Changing workspaces is an explicit user action and produces an auditable
context-switch event.

Workspace-agent setup, compact state, research provenance and public economic
rollups follow `specs/016-workspace-agent-initialization.md`.

## Non-goals

- This does not create a shared organization-wide knowledge lake.
- It does not require one workspace for every informal conversation; use the
  account layer only for durable, curated client context.
- It does not yet prescribe runtime-specific token, filesystem or index
  mechanisms. Those belong to the canonical authorization contract and thin
  Claude/Codex adapters.

## Acceptance criteria for implementation

1. Creating a workspace registers its agent, roots and lifecycle metadata.
2. An out-of-scope read, search, memory lookup, ingest or index lookup is
   denied deterministically.
3. Account context cannot enumerate or read a workspace without a promoted
   artifact.
4. A cross-workspace request creates a scoped, expiring audit record and gives
   the receiver only its work packet.
5. Archive/revoke makes existing workspace credentials and indexes unusable
   without deleting user work.
6. Claude and Codex adapters pass the same boundary conformance fixtures or
   report the workspace unsupported rather than silently degrading.
