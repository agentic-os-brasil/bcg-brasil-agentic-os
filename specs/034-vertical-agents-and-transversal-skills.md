# Spec 034 - Vertical agents and transversal skills

Status: accepted architecture; direct-skill selection and a bounded delegated
skill wire are implemented in the runtime-neutral core. Native Claude and
Codex activation remains unavailable until their adapters deliver the same
verified contract.

## Objective

Preserve the context and accountability of a professional case while making
methods such as deck construction, qualitative analysis, quantitative analysis,
onboarding and logging reusable across cases and client accounts.

## Core distinction

An **agent** owns a bounded context, authorization scope, lifecycle, task and
Definition of Done. A **skill** is a managed, transversal and atomic method.
It supplies procedure, not authority. A skill never grants a tool, expands a
resource scope, creates persistent memory, addresses the user or delegates.

The current role graph maps this distinction as follows:

```mermaid
flowchart TD
    M["Maestro\ncontrol plane; no tools"] --> A["Client Account Agent\nreviewed longitudinal account context"]
    M --> C["Case Agent\nauthoritative lifecycle and workspace context"]
    C -. "selects directly" .-> S["Managed transversal skills\ndeck · quali · quant"]
    C --> P["Reviewed promotion only"]
    P --> A
```

Client Account Agent, Case Agent and PA expert are the authoritative lifecycle
roles. The former `workspace_agent` and `account_agent` names are input-only
compatibility aliases; new registrations and the delegation graph use only the
canonical roles. Existing technical workspace IDs remain recognizable during
migration, but manifests created before the identity/signature schema must be
explicitly re-scaffolded; legacy role names are never emitted as new role
nodes.

## Selection and delegation

The Case Agent receives the intent, authorized pointers, constraints
and Definition of Done. It may select one or more compatible skills directly
without opening a child delegation. Direct selection is local to a current
signed root packet, authenticated by the root agent capability, and is denied
while a child is active. It is not a transfer of ownership, a tool grant or a
lifecycle event.

Every skill selection remains local to the active Case root. No child
WorkPacket is created: if another spoke is needed, Case returns to Maestro and
Maestro starts the next direct spoke sequentially. The legacy
`practice_agent` → `subject_specialist` graph is retired: FPA and IPA advice
has one canonical `pa_expert` role, selected from the versioned PA Expert
registry.

The current dispatcher remains intentionally sequential: one active branch,
with no nesting or direct agent-to-agent delegation. Parallel fan-out requires
a future contract and is not enabled by raising a catalog limit.

## Boundaries

- Maestro routes and synthesizes; it does not choose or execute case methods
  through tools.
- Client/Case lifecycle roles receive only their approved context; the Case
  Agent does not broaden its existing workspace boundary.
- A skill is not a tool grant. Tool-operation-resource authorization remains
  governed by Spec 018 and the shared enforcement controller.
- Walter reviews a sealed packet independently; Challenger and final-review
  are review modes, not separate agents.
- Darwin observes operating-model health and drift and may perform only
  reversible `health/maestro-system` maintenance; it never becomes a case
  execution worker.

## Managed skill policy

The product bundle contains the canonical skills catalog and a runtime-neutral
agent-skill policy. The policy lists only stable managed skill IDs and the role
contexts in which they may be selected. It has no user, client, case,
tool, credential or runtime-specific data.

For delegated capability execution, the dispatcher verifies that the parent
role may assign the selected skill and the child role may execute it. It signs
that selection as part of the WorkPacket. Unknown, duplicate, missing or
tampered skill IDs fail closed for capability specialists. New packets use
schema version 2. A schema-version-1 child with no skill selection may only be
verified and finished as an in-flight completion during rollout; it cannot
express a new method choice or open a new delegation. PA Expert advisory
packets are separate registry-bound contracts and reject an unexpected skill
ID. Root packets issued by Maestro do not select a case skill: the receiving
vertical agent remains responsible for method choice.

## Acceptance criteria

1. The managed bundle describes the skills without changing the authoritative
   Client/Case/PA expert lifecycle or removing staged runtime compatibility.
2. A direct skill selection is allowed only for a role explicitly listed in the
   managed policy and never changes its tool or resource grants.
3. A delegated capability WorkPacket names exactly one catalogued skill and
   rejects unknown, duplicate, missing or tampered values.
4. The Case Agent parent remains accountable for the child result and no child
   receives general browsing, persistent memory or child-delegation rights.
5. Claude and Codex shared-core conformance exercise identical allow and
   denial cases; this is not native adapter evidence, so activation remains
   unavailable until installed adapter delivery is proven.
6. The current sequential limit remains enforced until a later parallel join
   contract is accepted and tested.
