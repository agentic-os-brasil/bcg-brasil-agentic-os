# Spec 027 - Governed agent scaffolding

Status: managed Client Account, Case, PA Expert and specialist templates, atomic
local instance scaffolding and CLI implemented. Native Claude and Codex
registration, credentials and tool grants remain unavailable.

## Objective

Turn the governed role graph into concrete, inspectable agent stubs without
copying client context into managed prompts or claiming that a scaffolded
instance is already active.

## Managed templates and local instances

Managed templates live in:

```text
bundles/base/agents/templates/case_agent/AGENT.md
bundles/base/agents/templates/client_account_agent/AGENT.md
bundles/base/agents/templates/practice_agent/AGENT.md
bundles/base/agents/templates/capability_specialist/AGENT.md
bundles/base/agents/templates/subject_specialist/AGENT.md
```

They define role behavior only. They contain no workspace ID, account ID,
client fact, credentials, tool grant or runtime-specific identity.

`bcgos` materializes one private local instance below:

```text
<data-root>/agents/instances/<agent-id>/
  AGENT.md
  instance.json
  state.json
```

`instance.json` binds a path-safe agent ID to one role, immutable scope, parent,
input contract and the SHA-256 of its managed definition and compact state. The
manifest is authenticated with a private installation HMAC key stored outside
the instance tree. `state.json` remains small and reports both lifecycle and
runtime as unavailable until native registration succeeds.

The directory is assembled in a private staging path and atomically renamed
into place after file and directory synchronization. Repeated scaffolding is
idempotent only when every immutable field and the definition hash match.
Existing, partial, tampered or differently scoped registrations fail closed
and are never overwritten.

## Governed role matrix

| Scaffold role | Required parent | Scope | Delegation |
| --- | --- | --- | --- |
| `case_agent` | canonical `maestro` / `hub` | exact case/project workspace | one capability specialist |
| `client_account_agent` | canonical `maestro` / `hub` | exact client account | no direct child; Maestro mediates Case activation |
| `pa_expert` | canonical `maestro` / `hub` | exact PA Expert version and verified canon | none |
| `capability_specialist` | workspace or account agent | same workspace or account | none |
| `subject_specialist` | practice agent | same practice | none |

Case agents created by the existing workspace-first CLI use the compatibility
identity `workspace-agent-<workspace-id>` during migration; new explicit case
roots use `case-agent-<case-id>`. Both persist the canonical role
`case_agent`. Capability and subject specialists use the
`capability-` and `subject-` prefixes. The managed catalog's closed role
contracts and allowed edge at the correct depth are checked before any file is
created. A workspace stub additionally requires the concrete workspace-agent
registry produced by `bcgos init`. Account and practice roots require an
accountable owner and bounded mandate; practice roots also verify the bytes and
SHA-256 of a specific canon artifact through an OS-enforced practice root, so
symlinks cannot escape that scope. A
specialist's parent is resolved from a signed local instance, and its actual
role, scope kind and scope ID must match; caller-declared parent metadata is
never sufficient.

`bcgos init` automatically scaffolds the owning Case Agent after creating its
compact state and dossier. Client Account Agents, practice agents and leaf
specialists require an explicit command. A leaf always requires an already
registered matching parent:

```text
bcgos agent scaffold \
  --id capability-research \
  --role capability_specialist \
  --scope-kind workspace \
  --scope <workspace-id> \
  --parent workspace-agent-<workspace-id> \
  --parent-role workspace_agent

bcgos agent status --id capability-research
```

Practice roots additionally provide `--owner`, `--mandate`, `--canon` and
`--canon-sha256`; account roots provide `--owner` and `--mandate`. Their managed
prompts remain data-free: this bounded metadata stays only in the signed local
instance manifest.

The identity and ownership fields are part of the signed instance contract. A
manifest created before this contract was introduced is not silently upgraded:
its old HMAC and template digest require explicit re-scaffolding under the
canonical role. Compatibility aliases apply to new input requests only.

## Security and activation boundary

All paths are accessed through an OS-enforced local data root. Agent and scope
IDs are lowercase path-safe slugs. A scaffold carries no capability secret and
grants no tools. It is evidence that a governed instance definition exists,
not evidence that Claude or Codex can execute it.

The installation integrity key is created only when no scaffold instances
exist. If it is missing from a non-empty installation, creation and inspection
fail closed and require an explicit future recovery workflow; the system never
silently creates a second trust domain.

Native activation requires the runtime adapter to:

1. load the instance and verify its definition hash;
2. resolve the registered parent and exact immutable scope;
3. provision a private capability and exact tool-operation-resource grants;
4. register the resulting authorization with the shared enforcement controller;
5. persist dispatcher state and pass the cross-runtime conformance fixtures.

Until then, `runtime_state` is `unavailable` and dispatch must fail closed.

## Acceptance criteria

1. Every new workspace receives one idempotent concrete Case Agent stub.
2. Client Account and practice roots require a named owner and mandate; practice canon
   bytes must match the declared digest.
3. Capability and subject specialist stubs can be created only on catalogued
   role edges with a concrete registered parent in the exact same scope.
4. Managed templates contain no instance or client data.
5. Reusing an agent ID for another scope, parent or role is rejected.
6. Definition tampering is detected and never silently repaired.
7. Partial instances are not reported as initialized.
8. Scaffolding never grants tools, credentials or runtime availability.

## Identity, personalization and ownership

The first setup interview is exposed by `bcgos agent interview`. It explains
Maestro, Client Account, Case, Walter, Darwin and PA expert, offers name and
emoji-avatar suggestions, and asks for an explicit owner and ownership scope.
A confirmed profile is persisted with `bcgos agent personalize --stdin`.

Display names and emoji avatars are presentation data only. The signed agent
registration still owns the role, scope, parent relation, tool contract and
runtime state. `account_agent` and `workspace_agent` remain accepted only as
input aliases for `client_account_agent` and `case_agent`; they are not nodes
in the canonical graph. `practice_agent` is retained only as a time-bounded
migration input and cannot receive active authorization or delegate; re-register
it as `pa_expert` before expiry.
