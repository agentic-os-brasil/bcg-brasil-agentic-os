# First-case setup through Maestro MCP

The first-case workflow turns a short conversation into one reviewed, saved
brief in the current admitted workspace. A resumable work item/checkpoint is
optional and created only when requested. It does not deliver the entire
account relationship/narrative domain, run a specialist or grant tools.

Use the packaged `case-agent-setup` method. `workspace-agent-setup` is a
compatibility alias of that same workflow, not another setup engine. The
current closed MCP schemas are the authority for supported inputs and bounds;
if the setup operations or native review are absent, keep the draft/effect
pending instead of using shell, compatibility commands or direct state files.

## What the person does

1. Start in the intended Maestro workspace. Describe the client engagement or
   direct internal project in ordinary language; no folder plan or agent ID is
   needed.
2. Answer only the missing questions about decision/horizon, audience and
   constraints, useful result, authorized material, balanced hypotheses and
   next owner/step. Ask for a tracked checkpoint if one is useful.
3. Review the full normalized brief and one-to-three-action plan. Registration
   and Save use their normal exact native review. A draft request alone does
   not authorize Save; an already explicit Save request needs no redundant
   chat confirmation.
4. Maestro reads the saved page back, finishes the current setup and reports
   the actual saved revision and optional real checkpoint. A later client or
   session inspects that same setup and source instead of creating another one.

An installed method or registered definition does not prove that a particular
Code/Desktop host supplies its required review forms. If a write reports
`confirmation_unavailable`, preserve the draft/pending effect and use an admitted
client with normal native review. Do not retry by changing permissions, sending
a chat yes, inventing a token or writing files directly. Read permission does
not create content-review capability.

## Two valid scope choices

A client project has a canonical Client Account Agent relation and a separate
Case Agent. New cases default to the current opaque workspace ID, with the
brief at `accounts/<account-id>/cases/<workspace-id>/<workspace-id>.md` relative
to that workspace's brain. Account narrative, stakeholder history and cross-case
promotion are separate work, not side effects of local registration.

A direct internal project uses the workspace-scoped Case Agent with no account
relation or account pair markers, and `projects/<workspace-id>.md`. No fictional
internal account is required. Existing exact registrations, legacy case IDs and
returned destinations are preserved; a different second case cannot silently
rebind the same workspace through this first-case unit.

Registration uses the existing owner-local HMAC for instance integrity. This is
not platform/release/publisher signing, a request for the person's key, arbitrary
agent hiring, background enrollment, runtime activation or dispatch authority.

## Owning operation sequence

| Surface | Effect boundary |
| --- | --- |
| `case.setup.status` via `maestro_query` | Noncreating inspection of current setup, exact binding, revisions and destination. |
| `case.setup.prepare` via `maestro_apply` | Native-reviewed current registration/selection; inputs cannot select arbitrary roots, roles or parents. |
| Existing `brain.create` / `brain.read` | Explicit full brief Save and bounded read-back. Creation preserves an existing page; a separately requested correction uses `brain.update` at the actual revision. |
| `case.setup.finish` via `maestro_apply` | Review current setup/page revisions, reconcile only requested work through the existing ledger and finish selection/progress. |

Prepare's agent-internal inputs are accepted `mode` (`client`/`internal`) and
`project_name`, plus `expected_setup_revision` copied from current status.
Resume reuses the exact existing `setup.input`. Client mode adds
`account_id`, `account_owner` and `account_mandate`; internal mode omits them.
Fresh `setup_revision` is `absent`; a refusal or legacy marker is not permission
to invent that value. The response's `setup.brief_page` and existing IDs win
over a reconstructed path or human project title.

Finish takes `expected_setup_revision` from current status and
`expected_page_revision` from the complete consistent-revision brain read.
`objective`, `next_step` and `summary` are an optional all-three set for requested
work; omitting the set creates none. Once `setup.work` exists, retries keep its
exact three values, even for pending cleanup. The person supplies ordinary
answers and reviews, never these JSON/control details.

Status is `absent`, `preparing`, `pending`, `active`, `active_cleanup_pending` or
`reconciliation_required` according to the owning read. It also returns
`workspace_id`, `setup_revision`, optional `setup` (identity, input, destination,
registration references, saved revision/work when present), and
`selection.active`/`selection.pending`. `preparing` must be reconciled by the
same reviewed prepare before Save/finish. `reconciliation_required` preserves
legacy selection and may reuse only the exact verified registration; a bare
marker without proof is not new state to adopt or repair.

The server derives one stable setup identity. Requested work is atomically
created or recovered by that identity, not by title search or a new standalone
execution create after every retry. Finish reconciles the normal
start/checkpoint/pause states with current revisions/attempts. With no requested
work, a real saved brief can finish without inventing a ledger item.

## Interruptions are visible

Status does not create a registry, integrity key, marker or page. Native review
happens outside locks; the owning apply revalidates the complete resolved tuple
under custody afterward. Changed template/personalization, scope metadata,
registration relation or revisions require another exact review.

A finished setup is inspected, not prepared/saved/finished again on every new
session. Read its existing brief and handoff. Continue a returned optional work
item only on request through the existing execution-continuity workflow using
current revision/attempt state; already running work is not resumed again.

Registration, page Save, optional ledger work and marker selection are owned
boundaries, not one fictional transaction. Read back current state and retain
committed identities/content after interruption. A tampered or missing key with
existing instances, marker alias/corruption, foreign scope or binding conflict
is refused and preserved, never treated as an empty workspace to repair.

For client setup, a valid first pending pair is authorable even without a prior
active selection. Finish commits active selection durably, then removes only
that exact reviewed pending snapshot. `active_cleanup_pending` reports an active
selection whose cleanup remains. Retry only the exact owning finish while the
committed selection/setup/pending snapshots still match; do not restore old
active state, delete a newer pending marker or repeat the saved brief/checkpoint.
Internal setup tracks progress in the existing revisioned gatekeeper instead.

## A bounded exercise

Use synthetic information in a fresh admitted workspace. Ask Maestro to set up
one internal project, save the complete reviewed brief and, if desired, register
the accepted next step as paused work. Restart the MCP/client, ask it to inspect
the case and read that brief, then continue the same work at the current
revision. Repeat with a client fixture to exercise the exact account/case
relation, without reading another case or populating account narrative.

Record saved page/revision, setup state, optional returned work identity and
actual pending effects. A local test, simulated form, installed plugin or
registered operation is separate from hosted OS CI and actual native-client
qualification. Do not report broader domain delivery or successful cross-client
resume without observing it.

Source contracts: [Spec 017](../specs/017-workspace-agent-initialization.md),
[Spec 033](../specs/033-workspace-first-value.md),
[authored brain](product-authored-brain.md), and
[case-agent-setup](../bundles/base/skills/case-agent-setup/SKILL.md).
