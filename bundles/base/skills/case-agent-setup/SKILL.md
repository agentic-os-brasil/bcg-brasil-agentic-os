---
name: case-agent-setup
description: Create or resume the first governed case in the admitted workspace, turn a minimal interview into a complete reviewed brief, save it explicitly through MCP and optionally finish with a requested execution checkpoint.
---

# Case Agent Setup

Turn the owner's answers into a useful first brief without making the owner run
commands, choose internal IDs, copy paths or construct JSON. Keep the current
workspace as the confidentiality boundary. Resolve the packaged
[interaction profile](../interaction-profile/SKILL.md) for presentation only;
it never grants data, tool or effect authority.

## Select the admitted route

Start with the intended Maestro connector and verify its workspace. Inspect the
current closed tool schemas before effects. This MCP workflow requires
`case.setup.status`, `case.setup.prepare`, `case.setup.finish` and the existing
authored-brain operations. If a required operation is absent, preserve the
conversational draft and report that capability unavailable. Do not substitute
shell, a compatibility CLI, direct files or hand-written markers.

Read `case.setup.status` first. It is noncreating. Use its current binding,
setup identity, revisions and destination; never reconstruct an existing case
from a project title or a remembered folder. On restart, inspect and continue
that same setup rather than replaying the interview or creating another case.
If setup is already finished, read its current brief and return the existing
handoff; do not repeat prepare, Save or finish merely because the session is new.
Continue an optional existing work item only when requested, through the
[execution-continuity](../execution-continuity/SKILL.md) method at its current
revision/state. Already running work is reported, not resumed again. Pending
selection cleanup follows the separate exact-finish rule below.

- **Client engagement:** prepare the exact locally registered Client Account
  Agent and the separate Maestro-owned Case Agent with their immutable account
  relation. New cases default to the current opaque workspace ID. The human
  project label belongs in the brief; it is not an invitation to rename IDs.
- **Direct internal project:** use the workspace-scoped Case Agent with no
  account relation. Do not invent an internal account, account markers or an
  account brief merely to make the workflow fit a hierarchy.
- **Existing binding:** its exact account/case IDs and brief destination win,
  including valid legacy IDs. A different second case in this already-bound
  workspace is a conflict for this first-case workflow, not an automatic switch.
  Preserve it and ask for the intended admitted workspace.

The status response exposes `state`, `workspace_id`, `setup_revision` and,
when present, `setup` with its server-derived `setup_id`, `brief_page`, `case_id`,
`case_agent_id`, optional `account_agent_id`, saved `brief_revision`, `item_id`
and accepted `work`. `selection.active`/`selection.pending` describe client
selection; do not turn them into file-write instructions. Use these returned
values internally without asking the person to copy them.

The local registration is bounded instance integrity, authenticated with the
existing owner-local HMAC. It is not release/platform/publisher signing. It does
not launch an agent, hire a PA Expert, activate a runtime, delegate, grant tools
or authorize another workspace. Never request signable bytes, roles, parent
paths, keys, key replacement or a generic scaffold action from the model/user.

## Ask only the missing six questions

Use normal language, one question at a time, reusing answers already supplied
for this case. Clarify client versus internal work and its label within the
conversation; do not turn the interview into a technical form.

1. What decision or delivery should this case support, and by when?
2. Who will use the result, and what classification, constraints or dependencies matter?
3. What concrete result would make this first step useful?
4. Which materials are authorized, and what must stay outside scope?
5. What are the main upside/downside hypotheses, evidence, assumptions,
   counter-evidence and signals that would change each view?
6. What remains open, and what is the next practical step and accountable owner?
   Clarify whether to register a resumable work item only if that was not
   already requested; do not infer task acceptance from a request for a draft.

Keep answers in the conversational draft until the owner requests persistence.
Show the complete normalized brief and a plan of one to three actions. Each
action names an outcome, accountable owner, next step and observable completion
criterion. Preserve unknowns as open questions; do not invent client facts.
Keep the substantive brief, evidence pointers and reasoning in the reviewed
brain page, not in gatekeeper metadata, identity files or a transcript store.
Respect current input/review bounds: agree a smaller complete brief if needed;
never truncate a displayed review and treat its unseen tail as approved.

## Prepare, save, then finish

1. **Prepare the exact binding.** Use `case.setup.prepare` with the current
   status and the owner's client/internal choice. Pass accepted `mode`
   (`client` or `internal`) and `project_name`; copy `expected_setup_revision`
   from current status. On resume reuse the exact existing `setup.input`.
   Only a fresh status supplies the literal `absent`. Client mode also needs
   `account_id`, `account_owner`, `account_mandate`; omit all account fields for
   internal mode. These are agent-internal tool inputs, not a questionnaire or
   JSON for the person. Let the server derive the root, workspace scope, Case
   role, parent and setup identity. The native review must show the resolved account/case registration, current revisions,
   destinations and complete bounded owner/mandate metadata. If any reviewed
   field changes, get a fresh review. A conversational yes is not a substitute
   for the client's native review mechanism. If prepare is incomplete
   (`preparing`), inspect current status and the remaining effects, then review
   that same prepare; do not save/finish against an incomplete registration.
2. **Offer the full brain save explicitly.** Show the complete Markdown,
   including frontmatter. Confirm that Save is part of the owner's request;
   if already requested, do not add another verbal or command-level approval.
   A request for a draft alone is not Save. Use the returned brief destination
   with the current `brain.create` contract and its required native review.
   The eight fields are `id`, `title`, `summary`, `type`, `scope`, `status`,
   `sensitivity`, `updated`; `id` is the returned brain-relative path without
   `.md`, the title matches H1 and `type` is `project-brief`. New client briefs
   have account/case scope and `client-confidential` sensitivity; direct internal
   briefs use `workspace/<workspace-id>` and the admitted classification.
3. **Read back the source.** Use `brain.read` for a declared purpose in UTF-8
   pages of at most 4096 bytes, following returned `next_offset` until `-1`.
   Every page must report the same revision; stop and inspect afresh if it changes.
   Compare the complete saved brief and actual revision with the
   reviewed content, even when creation reports an existing page. Never overwrite
   a pre-existing or concurrently created page to force setup completion. A
   requested correction uses `brain.update` with the actual current page
   revision and another complete native review.
4. **Finish at current revisions.** Use `case.setup.finish` only with the current
   setup state: `expected_setup_revision` from status and
   `expected_page_revision` from complete `brain.read`. If the owner requested
   work, include nonempty `objective`, `next_step` and `summary` together in
   this reviewed finish; omit all three when no work was requested. On retry,
   reuse the exact three accepted values from existing `setup.work` if present,
   including when only selection cleanup remains. Do not replace them with a
   new progress summary or omit a previously accepted work request. The setup
   owner creates or recovers one ledger item by its stable server-derived setup identity and reconciles the existing execution
   start/checkpoint/pause flow. Do not call `execution.create` independently or
   title-search for a substitute item after interruption. If no work was
   requested, finish without creating or requiring a checkpoint.
5. **Return the actual handoff.** Report the registered scope, saved page and
   revision, completed/pending selection effects, and the real work/checkpoint
   only when returned. Lead with the saved brief and next owner/step in the
   person's preferred level of detail; internal IDs/revisions are not something
   the person must copy or supply. A registered definition remains
   scaffolded with runtime activation unavailable; do not describe it as a
   running specialist or claim a second client has resumed it before observation.

For a new client case, the usual brain-relative brief is
`accounts/<account-id>/cases/<workspace-id>/<workspace-id>.md`. A direct internal
brief uses `projects/<workspace-id>.md`. These describe the default layout, not
a path argument or permission to override the destination returned for an
existing exact binding. Keep account narrative and private owner memory outside
this first-case brief. Registration does not populate relationship history or
automatically append a case to an account narrative page.

## Resume and partial effects

Re-read status and saved source before a retry. Reuse exact returned setup/work
identities and current revisions; never ask the owner to transport a run ID,
revision or command envelope. A stale review, scope conflict, missing/tampered
registration, unavailable integrity key or marker alias/corruption is a refusal,
not fresh state or permission to reset files.

`reconciliation_required` is not fresh state: existing selection needs a
verifiable canonical binding. An owning prepare may reuse only that exact
reviewed registration/relation and legacy destination. A bare marker without
that proof remains refused. Do not clear markers, invent another account/mode,
re-sign an instance or send `absent` to force a fresh start.

The shared owner selects a valid first client pending pair even without a prior
active case. It publishes active selection only through reviewed finish. Never
write, overwrite or remove `.pending`/`.active` directly; a prior pending marker
is not disposable. Internal mode uses the existing revisioned gatekeeper and no
account pair markers.

`active_cleanup_pending` means active selection committed but conditional pending
cleanup remains. Inspect the current state and retry only the owning reviewed
finish for that exact setup/selection. Do not roll back active selection, remove
a newer pending marker, repeat the brain save or duplicate a checkpoint. Other
interrupted stages must also report completed and pending effects honestly;
there is no cross-owner all-or-nothing transaction to claim.

If native review is unavailable, leave the effect pending and continue only the
draft/read-only work the owner requested. Use a client with the normal required
review capability for persistence; never substitute tool permission, chat
approval, a JSON consent field or a raw filesystem write.

## Separate optional work

The first-case bridge does not browse, ingest documents, compile navigation,
dream memory, refresh economics, schedule work or contact anyone. A source or
SharePoint URL in a brief is a pointer, not a connection or collection grant.
The legacy index compiler (`brain-index.py`) is not invoked by this workflow.
External research needs its own explicit minimized plan, approved source scope
and an available enforceable capability; otherwise leave it pending. Never send
confidential names or client-derived facts in public searches by inference.

An economic snapshot requires its separate independent-public-source owner and
attestation; the case may reference an admitted immutable snapshot, not copy
workspace bodies into a shared rollup. Cross-case reading and promotion to a
Client Account narrative remain separate explicit work. This first-case unit
does not declare that broader account or every method is implemented.
