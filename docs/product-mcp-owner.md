# Product MCP: owner onboarding and methods

This engineering guide covers canonical owner onboarding in Code and Desktop.
Use a dedicated profile and synthetic content for qualification. Local source
tests do not establish native confirmation privacy or a signed team release.

## Prepare the binding

Build the server as described in [its module guide](../adapters/mcp/README.md).
Select the installed product, profile and workspace locally, then run explicit
`init`. Reuse the same three flags for `serve`, packets and operator commands.

```sh
maestro-mcp init --product-root /installed/product --user-home /owner --workspace /work/project
```

Initialization preserves admitted migrated state. Copied old registry,
interview, refinement and observation controls remain inactive history until
their owning workflows create fresh authority. Ordinary reads do not initialize
or enroll a producer. Keep owner content outside managed product directories.

## Admit an existing observation store after upgrade

Older Unix writers leave valid canonical observation logs without a persistent
selection lock after releasing their token lock. Observation status/intake and
capture review refuse this state until explicit local setup. Ordinary queries,
startup and hooks never repair it.

Close older Maestro writers, then repeat the same `maestro-mcp init` command
above with the already admitted owner/workspace binding. It works for an already
initialized owner: the canonical owner initializer validates the bounded private
log and admits its persistent native selection lock without rewriting canonical
observation bytes, recreating owner data or granting capture authority. Reconnect
and read observation status again; review intake, qualification and enrollment
separately as usual.

An active native lock, nonempty legacy token, unsafe alias or malformed/oversized
store refuses intact. Preserve those files and resolve the active writer or
underlying unsafe state explicitly. Do not delete a lock, reset owner data or
use a query as an installation workaround. Fresh owners without an observation
store retain that absence until an explicit observation mutation needs it.

## Complete the interview

Read `owner.status` first and follow the canonical `maestro-onboarding` method
available through the Code plugin and the Desktop MCP prompts/resources. Offer
`quick` or `complete` only when `track=selection_required`. Apply `owner.track`
for the explicit choice, then query `owner.interview` with the selected track
to obtain its canonical steps. Decline or missing form support leaves the choice
unchanged. Selecting any track again invalidates the previous final confirmation.

For `required` or `in_progress` with a selected track, resume `next_question`
and `remaining` instead of selecting the track again. `review_required` goes
directly to final review; `complete` continues ordinary work without restarting
the interview. Re-read status after each successful answer. Deferring onboarding
or declining a review preserves progress and does not block unrelated work.

For each answer, apply `owner.propose` with `facet`, an `evidence` summary and
`proposed_body`. This creates a proposal without automatic producer authority.
Query `owner.review` with `proposal_id` for metadata, then apply `owner.apply`
with that ID for trusted full review. The form shows all current/proposed text,
evidence and access policy. Approve only the exact intended content; the tool
returns a metadata receipt.

After the required answers, query `owner.onboarding_review` and apply
`owner.confirm_onboarding` to review and confirm the full profile. Query
`owner.status` for body-free progress. Restart with the same binding and query
status again to verify continuity. Operations are values of `operation` in
`maestro_query` or `maestro_apply`; their schemas reject additional fields.

The host must support form elicitation. Modern MCP uses official SDK multi-round
input requests; older supported sessions use elicitation. Missing approval,
cancellation, an invalid response or transport failure grants no authority.
A changed profile, proposal or track requires fresh review. Concurrent track
selections, including a repeated current value, invalidate an earlier approval.

## Explicit local fallback

For clients without forms, a local operator can perform the same review.
Append the three binding flags to every command below. Full review JSON is
private; keep it local and out of ordinary chat/tool output.

```sh
maestro-mcp owner-track --track quick --confirm
maestro-mcp owner-review --proposal-id PROPOSAL_ID
maestro-mcp owner-apply --proposal-id PROPOSAL_ID --review-digest REVIEW_SHA256 --confirm
maestro-mcp owner-onboarding-review
maestro-mcp owner-confirm-onboarding --review-digest REVIEW_SHA256 --confirm
```

Use the digest returned by the immediately preceding full review. On refusal,
inspect the binding and current review; do not remove authority files or weaken
permissions. An older strict reader may refuse the optional track revision
field; upgrade it while preserving the registry. Embedded NUL in malformed
legacy profile text requires explicit cleanup before review. Valid-text digests
remain compatible.

## Discover methods

Code packets include 92 canonical methods and two operator/resume aliases.
Desktop discovers those methods through MCP prompts and immutable
method/policy/provenance/license resources. The catalog records canonical source
hashes; Code's separate `methods-projection.json` records projection hashes.
Method resources cannot fetch owner-local files. Check
available operations before following instructions: packaging a method does
not establish that every native hook, agent or provider is available.

Generate a new plugin directory or `.mcpb` for each version; both reuse external
owner state. MCPB installs a server and manifest, not Code Skills. Native loading,
human-form privacy, corporate signing, team publication and pilot acceptance
remain separate qualification steps.

## Personalize agent presentation

Identity personalization is a separate optional phase after core owner
onboarding. Query `agent.identity.interview` for the canonical next question,
profile revision and any pending draft. The three main-agent questions resume
one at a time. Missing preferences keep installed defaults active; invalid
existing state blocks instead of becoming a new interview.

Apply `agent.identity.answer` for one exact name/emoji and the returned
question/base. One full native review authorizes canonical draft creation and
confirmation. `agent.identity.propose` is the explicit draft-only variant;
`agent.identity.review` returns its metadata and `agent.identity.apply`
reviews/resumes that same draft. Model arguments cannot supply consent,
approval, scope or review digests. Query again after a successful write and
a new process to observe the actual persisted profile.

Human interaction holds no canonical/control lock. Publication revalidates the
complete base/question/proposal after acquiring the transition lock. Decline
before ordinary draft creation leaves no identity proposal/profile effect;
a declined explicit draft remains pending. A later I/O error can leave durable
recovery state, which must be reported rather than described as no filesystem
change. Exact applied-draft receipt replay never overwrites a newer profile.

The adapter uses the existing private `state/agents/personalization.json` and
canonical draft lifecycle. Reads are bounded and noncreating; storage aliases,
corruption, stale reviews and unregistered/foreign instances refuse. Signed
instance definitions remain unchanged. Names and emoji grant no tools, runtime,
provider or delegation authority. Native client acceptance remains a separate
qualification gate from these source contracts.

Ordinary `maestro_context` also consumes the confirmed main-agent names/emoji
through the canonical managed roster after restart. IDs and roles remain fixed.
An absent optional profile uses defaults; invalid preferences are explicitly
`profile_invalid` with deterministic default presentation and preserved data.
That condition blocks identity onboarding, while unrelated workspace context
remains available. Immutable compiled agent resources are unchanged.

## Record attended professional work

The owner atlas is the canonical professional Markdown corpus under the
admitted data home's `atlas/owner`. It is distinct from Owner Context facets,
workspace pages and generated memory. The `start-day`, `eod`, craft, feedback,
retrospective and learning methods use this corpus through the shared
`start-day/resources/owner-atlas-mcp.md` method resource.

Query `owner.atlas.status` without creating directories or repairing pages.
If its fixed scaffold is incomplete, explicitly apply `owner.atlas.prepare`.
The host reviews the exact canonical pages and any existing content before
the non-overwriting preparation. A partial failure is not a completed setup;
inspect status and explicitly retry. Existing daily and hand-authored pages
remain unchanged.

Query `owner.atlas.collect` with a concrete `purpose` and one to 24 named
relative `pages`. It returns bounded content, page revisions and explicit
omissions; it cannot enumerate or disclose the whole corpus. A missing atlas
is a refusal, while a missing named page in an existing atlas is an omission.
The server selects the canonical root and fixes the reader to the attended
owner session. Callers cannot supply a filesystem root or impersonate a
delegate.

Apply `owner.atlas.create` with `page` and complete `text` for a missing page.
Creation preserves an occupied page. Apply `owner.atlas.append` with `page`,
the actual Markdown `section`, entry `text` and the last observed
`expected_page_revision`. A reviewed correction uses `owner.atlas.update`
with `page`, full replacement `text` and the expected revision. All mutations
require trusted full-content review. A changed page or review is refused;
Decline, unavailable confirmation and transport errors grant no write.
An identical retry retains its canonical operation receipt and cannot append
the same entry twice. Read the named page again after a successful mutation,
including after a new MCP process starts.

These operations create real owner pages through the existing atlas journal,
publication and revision rules. They grant no Owner Context promotion, memory
capture, background producer, provider access or workspace-to-owner transfer.
Only sanitized professional reflection and pointers belong here. A scheduled
marker cannot replace an attended review; without its separately admitted
adapter, a scheduled result remains an unrecorded proposal. No command or raw
filesystem fallback is available for this MCP family.

## Review a professional self declaration

The optional `interaction-profile` self-observation recipe uses the same admitted
owner through Code and Desktop. Read `owner.observation.status` first. Its bounded
pages contain intake IDs, revisions, states and capture metadata, without claims.
Status, startup and hooks never create intake, capture evidence or authority.

When the owner explicitly supplies an instruction, correction or endorsement,
`owner.observation.intake` accepts only `signal`, `facet`, a normalized bounded
`claim` code, `confidence` and `expires_at`. A fresh future expiry must be within
90 days. Raw prose, prompt/document content, inferred patterns, hypotheses,
source hashes, episode IDs, scope, approval and authority flags are refused.
Do not invent a preference or translate a generic acknowledgement into one.
The exact supported codes and public call shape are in the method recipe.

Full trusted review shows the normalized declaration, its owner-global
professional proposal-input scope and the unchanged active SELF. The owner must
attest that this is their relevant/useful declaration without client, document
or conversation material and explicitly authorize that scope. This is owner
attestation, not an automatic sanitizer or native-history claim. Decline,
cancellation, unsupported review or changed source leaves no approved intake.
A missing confirmation channel requires returning to an available trusted review
channel; no model-provided boolean replaces it.

Approval stores a bounded eligible pending envelope only. The server derives its
stable identity from canonical owner binding and the exact closed declaration;
reconnecting or repeating review cannot create independent interaction episodes.
All such declarations share one conservative cohort and cannot corroborate each
other or enter weekly Yoda selection as independent observations. Actual capture
requires the separately reviewed, qualified and enrolled
[`self-observation-capture` maintenance job](product-mcp-maintenance.md#capture-an-explicitly-reviewed-owner-declaration).
Its result is a new canonical observation version as proposal input, never an
active facet or native delegation permission.

Use `owner.observation.reject` with the intake ID and `next: rejected` or
`next: redacted` for exact trusted control review. Current intake and canonical
states are displayed separately. Terminal replay cannot revive a declaration.
Redaction retains the existing append-only denial/tombstone semantics; it is not
a promise of physical historical-body erasure. Derived-self reset also denies
pending intake and preserves the existing refusal for promoted canonical facets.
An explicit new declaration does not erase or contradict an older one by itself;
review the old record's control when replacing it. Ordinary query and report
output remain body-free. Local source/SDK tests do not qualify native client
privacy or Windows operation.

Cancellation remains effective through binding validation and observation-lock
acquisition: the owning mutation checks it again immediately before its first
persisted declaration/control change. After a canonical control append commits,
its coupled denial metadata is completed; a later I/O failure retains real
partial evidence rather than a rollback claim. Legacy digest-case normalization
also preserves the old owner's canonical IDs and read/CAS revisions without
rewriting the source log.
