# Spec 028 - Maestro Federated Improvement Loop

Status: accepted architecture; local contract/outbox/federator and central
HTTPS inbox/digest/curator implemented. The central GitHub App publisher takes
only a bridge-owned installation-token source. Runtime scheduling, pilot
provisioning and portable-skill transport remain pending.

## Objective

Let a Maestro pilot improve the managed product automatically from local use
without making pilot users operate Git, approve every upload or export
workspace-private content. The **Federated Improvement Loop** (FIL) turns
safe, bounded local signals into reviewed product advances.

## Product loop

```text
workspace-private use and skills
  -> local deterministic compiler
  -> local Darwin federation analysis
  -> typed automatic batch
  -> GitHub bridge and federation inbox
  -> central Darwin curation
  -> advancement proposal
  -> human acceptance, PR, test and release
```

GitHub is the system of action: it stores the pilot inbox, accepted advances,
implementation Issues, pull requests and release links. It is not a raw
telemetry lake. A narrow GitHub bridge owns GitHub App credentials and accepts
only a valid typed batch; it may not accept arbitrary logs, Markdown, prompts,
files or metadata passthrough.

## Automatic enrollment

Pilot enrollment is one explicit contract rather than a sequence of per-send
approvals. It discloses the exported artifact vocabulary, GitHub destination,
retention, visible cohort/maintainer access, the absence of performance
evaluation and how a participant leaves the pilot. Leaving the pilot stops
future export; it does not silently make a pilot installation compliant while
the required reporting contract is disabled.

The product must not require a pilot user to hold a GitHub token. The bridge
uses an organization-installed GitHub App with minimum repository and Issues
permissions. Contributors are a separate role: only they receive source
repository branch and pull-request access.

## Data classes and non-interference

There are three origin classes:

| Class | Local location | Automatic export |
| --- | --- | --- |
| `workspace_private` | Registered workspace, its memory, indexes and local skills | Typed signals and structural skill candidates only |
| `born_portable` | Managed user-local portable-skill root | Candidate metadata now; complete skill package after the dedicated collector contract exists |
| `managed` | Distributed Maestro bundle | Managed identifiers and version metadata only |

No artifact influenced by `workspace_private` may export prose, code,
instructions, examples, prompt/output content, paths, file names, workspace or
client identifiers, exception messages, names, emails, hostnames or arbitrary
metadata. Changing private text or workspace identity must not change an
exported payload except through approved enums, counters and buckets. The
compiler must prove this with non-interference fixtures using seeded canaries.

A complete skill can be exported automatically only when it comes from the
explicit `born_portable` root. A skill discovered in a workspace can produce a
structural candidate but never its body. This preserves the automatic loop
while keeping client-specific implementation local.

## Typed outbound artifacts

The version-one batch allows only:

- product and runtime version;
- opaque installation identifier;
- bounded time period;
- enumerated signal kind, capability, workflow stage, evidence bucket,
  confidence and outcome;
- structural skill candidate class, managed dependencies, safety flags and
  evidence bucket;
- deterministic batch and artifact fingerprints.

The wire format has no generic object, free-text summary, exception message,
workspace ID or private-content field. Unknown enum values, fields, oversized
collections or invalid fingerprints fail locally before transport.

## Darwin roles

### Local Darwin - federator

The local Darwin receives a deterministic, workspace-scoped health packet. It
does not read other workspaces, use network tools or send data itself. It
selects an approved typed signal or structural candidate and hands it to the
compiler. Qualitative perception is expressed by a controlled taxonomy and
template slots, not generated prose.

### Central Darwin - curator

The central Darwin receives only valid, compiled cross-participant batches. It
may cluster recurring signals and produce an `AdvancementProposal`: a candidate
shared skill, guidance/UX improvement, safety policy, test/evaluation or
documentation change. It cannot inspect source workspaces, change code,
publish a skill, merge a pull request or release a bundle. A maintainer must
accept the proposal before implementation begins.

## Delivery phases

1. **FILO contract** - decision, spec, schema/types and non-interference tests.
2. **Local batch exporter** - user-local queue, enrollment state, bounded
   retry and born-portable skill collector are implemented. A skill body is
   verified against a strict manifest and never passes through a typed batch.
3. **GitHub bridge** - central HTTPS inbox, aggregate digest, GitHub App Issue
   publisher seam and actionable incident routing are implemented. Deployment
   must provide the bridge-owned short-lived App installation-token source.
4. **Federator** - local Darwin packet adapter and structural skill/insight
   compilation are implemented. Its qualitative input is a closed perception
   taxonomy; it cannot emit local prose or send data itself.
5. **Curator** - central batch compiler and Central Darwin proposals are
   implemented. It creates draft GitHub proposal/incident artifacts only;
   human acceptance remains required before source work. Semantic Issue-to-PR
   promotion is a maintainer workflow, not autonomous behavior.
6. **Pilot hardening** - Windows/macOS conformance, enrollment/revocation,
   offline recovery, canary-leak tests and pilot runbook.

Each phase is a semantic pull request with its own observable contract. A
later phase must not claim an earlier unavailable capability is active.

## Initial executable contract

`schemas/federation-batch.schema.json` and `internal/federation` validate the
closed version-one batch vocabulary. Its contract tests prove that unknown
fields/values fail closed and that changing workspace-private identity or text
cannot affect a compiled structural export.
It does not read files, invoke a Darwin, enroll a pilot user or contact GitHub.

## Acceptance criteria

1. A payload cannot represent raw workspace-private text or identifiers.
2. Unknown data fails locally before any transport adapter is invoked.
3. A private-source canary changes neither emitted JSON nor its fingerprint.
4. Claude and Codex adapters can produce equivalent typed batches or report
   federation unavailable.
5. GitHub bridge credentials never enter a pilot device, workspace or bundle.
6. Central curation results in proposals, never autonomous source changes.
