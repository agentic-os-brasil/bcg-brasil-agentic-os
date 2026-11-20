# Spec 060 — Product owner, onboarding and memory

Status: accepted for construction under the complete-product instruction.
Scope: E3 of the product construction plan. E1/E2 source contracts remain in
effect; native qualification and distribution require their own evidence.

## Observable journeys

An admitted participant chooses a quick or complete interview, proposes answers,
reviews the exact current profile, confirms it and resumes after restart. A stale
review cannot replace newer work. Existing canonical owner state remains the
source of truth; the adapter never writes its own profile.

The participant also captures eligible workspace memory, discovers capture IDs
after restart, searches and reads bounded context, corrects or removes an identified
capture and applies retention. Removed content
does not reappear from a historical manifest, consolidation, context cache or
L3/lifetime carry-forward. Rebuilding with surviving sources remains possible.

## Authority and review

The three product tools retain closed operation schemas and local HOME/workspace
binding. No operation accepts a filesystem root, producer capability, key,
sanitizer assertion or model-supplied confirmation as authority.

Prepare owner metadata and fixed product capture authority only through explicit
local initialization. Ordinary context/query/startup remains read-only. Existing
migrated state is preserved; old relocated credentials remain inactive.

Explicit memory initialization also prepares the existing capture-v2
provenance key for body-free observations. This key attests only closed
metadata; it grants no tool, agent, client or native-execution authority.
Runtime producers never bootstrap it. Observations retain their own evidence
class and cannot borrow `owner_reviewed` provenance. Text capture/correction
continues to require the exact review below.

Server-originated MCP form elicitation may obtain explicit human confirmation.
Keep the reviewed envelope server-side, display the full bounded review to the
client, require action=accept and an explicit approved=true in returned content,
and revalidate the digest/base after the interaction. Accept with missing content,
decline, cancel, unsupported capability or transport failure grants no authority.
The model cannot supply the elicitation response through tool arguments.

Use the effective protocol and client capabilities on the current request;
session initialization is the legacy fallback. An empty elicitation capability
permits form review; URL-only support does not. Refusal diagnostics on stderr
contain only bounded protocol labels, capability-presence booleans and a closed
failure stage. Never log reviewed content, raw capabilities, identities, paths
or transport error text. A diagnostic or passing SDK test is not native host proof.

An explicit local review/confirm command is a supported fallback for clients
without elicitation. An operator may also explicitly select the foreground-only
local-browser channel described in `docs/product-local-owner-review.md`. Explicit
startup selection takes precedence over advertised native forms and rejects
native InputResponses; empty selection stays native-first. It
reuses this exact review and canonical apply contract without claiming native
form or lifecycle qualification. It uses the same canonical review and digest checks. Never
silently convert a missing host capability into approval or claim native support
from an SDK test.

Ordinary model queries return owner metadata and interview questions. After
current final onboarding confirmation, `owner.context` additionally delivers only
the selected track's canonical facets whose Readers explicitly authorize the
server-bound `session` reader. It uses bounded pages and a freshness generation;
unconfirmed, out-of-track and reader-restricted facets remain unavailable. The
local MCP session cannot attest an individual hosted subagent's identity. This
projection is personalization data and grants no tool or effect authority. See
[accepted owner context](../docs/product-owner-context.md).

Full profile/proposal review remains available only through trusted client review
or the explicit local operator. Review receipts contain bounded state/digests and
omit private paths, secrets and reviewed text. `maestro_context` remains metadata
only and advertises the separate explicit read operation.

## Canonical owner module

Use ownerctx initialization, track selection, refinement and onboarding rules.
Add the missing bounded proposal/onboarding review and reviewed-apply interface
there, preserving existing callers. Review binds proposal identity, exact body,
current base, policy and track where relevant. Applying a different or stale
review refuses before changing the canonical facet. Serialize transitions that
would otherwise race a read/validate/write sequence.

Track selection binds the current selection revision as well as its value.
Compare the review and commit the choice under the same canonical transition
lock; even a competing selection of the same value invalidates an earlier
approval, including during final profile confirmation. Final review and status
digests bind the same revision; revision-zero legacy confirmed profiles retain
their previous digest until a new explicit selection. A monotonic optional registry
counter supplies that revision. Older
strict readers may refuse the extended registry after a selection; preserve the
registry and upgrade the reader rather than removing authority metadata.

Human forms show all domain content with clear Portuguese labels, including
current/proposed values, evidence, sensitivity and readers. Opaque digests and
internal counters stay out of the form message. The full canonical envelope,
including these proofs and admitted scope, still binds the MRTR response and
the canonical compare-and-apply. Local operator review may show the full JSON.

Profile proposal/facet text rejects embedded NUL and malformed UTF-8 before
persistence or review. Private JSON is validated before decoding can replace
malformed bytes.
This preserves existing valid-text digest framing while refusing ambiguous
legacy bodies; malformed existing files remain intact for explicit operator
cleanup and cannot acquire or retain confirmed onboarding status.

Reads and publication in the exposed path must be bounded, regular-file,
single-link and descriptor-anchored/no-follow on admitted platforms. Registry
paths cannot redirect a product review or write outside the canonical owner
scope. Preserve explicit source facts and existing policy; no inferred identity
or automatically authorized producer is introduced by a model tool.

## Canonical memory module

Extend internal/memory and its existing manifest/activation pipeline rather than
creating an adapter-specific memory database. Keep the existing attested
skill-route capture contract unchanged. Human-reviewed owner text has a distinct
explicit kind and a fixed trusted producer/sanitizer; the seal binds the exact
reviewed payload and is produced only after trusted review.

Expose stable capture identity, bounded paged discovery/search and revision-aware
lifecycle results. Search must observe the same visibility as context and readback;
it cannot recover removed text from an independent stale index. Correction,
deletion and retention validate the selected workspace, current revision and
reviewed operation before effects. New-capture approval binds the reviewed
visibility in the opaque input seal; compare it under the canonical activation
lock before publication so concurrent reuse cannot create two records. No model
argument supplies that fence. A correction retains truthful provenance;
deletion and retention revoke visibility before physical cleanup. Return bounded
metadata, never keys or whole capture files.

For bulk retention, the complete reviewed action is the explicit cutoff against
the current visibility snapshot. Show cutoff, workspace, affected count and date
range; bind a deterministic fingerprint of sorted affected IDs, revisions and
source digests with the exact visibility revision. Recompute before applying
the canonical visibility CAS. Individual bodies remain separately inspectable;
do not copy all bodies into a giant form or truncate the approved action. Single
correction/deletion retains the exact private per-record review.

Revocation must cover source selection, historical latest-manifest fallback,
artifact reads, context assembly and derived self-carry. A barrier is not complete
if a previous valid commit can restore the removed content. Empty visible state
is intentional after deleting the final source. Fresh consolidation may publish
only descendants of surviving sources under the current visibility revision.
Interrupted changes are recoverable or safely unavailable without exposing the
previous forbidden content. Concurrent stale writers cannot erase a later change.

Lifetime eligibility counts actual canonical L3 transitions from verified
committed ancestry in the current visibility generation. A captured literal,
quoted heading or generated text marker cannot supply generation evidence.
Missing or unverifiable ancestry never grants promotion. Keep that in-process
evidence separate from user text and preserve valid persisted artifact bytes
and readback; no second adapter-specific eligibility gate is permitted.

The deterministic weekly candidate is a bounded selection of complete units,
not recursively embedded prior summaries. Prioritize current evidence, collapse
exact normalized repeats, preserve authored multiline units, and apply the
existing context byte ceiling as well as artifact rune validation. Failure to
fit a complete current unit refuses publication; omission of older whole units
does not authorize fabricated synthesis. Keep the exact selected candidate in
the existing trusted review, with original source/ancestry and eligibility
rules unchanged. Unchanged inputs remain skipped; this formatting-quality fix
does not itself create operational history or lifetime eligibility.

Bound visibility metadata with an atomic checkpoint rather than an unbounded
transition history. A required-checkpoint sentinel prevents fallback after lost
or corrupted authority. The current capacity is 4096 lifetime capture identities,
including permanent denial tombstones. Reserve removal capacity: new captures
may return typed nonretryable capacity exhaustion, while existing captures stay
readable, correctable and removable through deletion/retention. Cleanup never
reclaims denial identities silently, raises limits or restores revoked content.
Preflight serialized record/journal limits before publishing a replacement.

Reuse existing private filesystem and activation/locking primitives. Do not add
a generic transaction framework or a second ledger. Unsupported ownership or
confinement remains a typed refusal; never weaken Windows admission for tests.

## Product integration and methods

Register closed owner and memory operations at the existing product seam, with
typed errors, bounded inputs, pagination where needed and accurate annotations.
Every operation rechecks the admitted binding. Use canonical modules for all
domain effects; product helpers provide host interaction and argument mapping.

The Code plugin includes the methods/resources needed for both complete journeys.
Desktop discovers equivalent instructions through supported MCP prompts/resources
and executes domain operations without assuming Code hooks or local shell access.
Resources are bounded, integrity-backed and cannot expose owner-local files.
MCPB still installs a server/manifest, not the Code plugin's Skills.

## Public acceptance

1. Explicit init, quick/complete interview, proposal, trusted review, confirmation
   and restarted readback work with the managed product read-only.
2. Unknown fields, roots, credentials and confirmation flags in model arguments
   refuse. Missing/declined/cancelled/nil elicitation grants no mutation authority.
3. Changed proposal/base/track after review refuses; concurrent transitions do
   not overwrite one another. Existing source owner workflows remain compatible.
4. Oversize, duplicate/trailing metadata, aliases, hardlinks, special files and
   unsafe ancestors refuse before exposing content or writing outside the scope.
5. Reviewed capture and bounded context/readback work after restart. Cross-scope
   selection and invalid/unreviewed attestation refuse without mutation.
6. Correction, deletion and retention propagate across all readers/derivatives.
   Force historical fallback and run consolidation again; a removed unique marker
   must never return. Deleting the final capture produces a valid empty view.
7. Interruption and stale revision tests preserve later work and revoked
   visibility. Existing attested skill-route and manifest integrity tests stay
   green at directly touched contracts.
8. Official SDK and compiled stdio tests exercise the complete product journeys,
   including method discovery and prompt/resource access from shipped packets.

Use synthetic data and focused local tests. Four independent source review lenses
and hosted CI precede eligible integration. Real clients/models, corporate signing,
publication and pilot acceptance are distinct and remain explicit.

## Reviewed self-declaration product route (OSIN)

Expose `owner.observation.status` as bounded body-free read-only metadata and
`owner.observation.intake` / `owner.observation.reject` as closed reviewed writes.
Intake accepts exactly the five-field normalized declaration described by
Spec013; the server derives stable owner-bound payload identity and a single
non-independent declaration cohort. Never accept model trust, source, episode or
scope assertions. Full review distinguishes declared preference from native
observed evidence and explicitly obtains material relevance, no-client-material
and owner-global professional proposal-input attestation.

Hold no owner lock while awaiting the person. Revalidate canonical review/source
and admitted binding after confirmation, and execute owning mutations inside the
existing installed-source publication fence. Cancellation, recall, changed source
or stale review cannot publish approval/control. A successful intake queues only;
maintenance qualification and enrollment are independent exact reviews. Controls
preserve canonical terminal denial and unchanged active SELF. The canonical
`interaction-profile` conditional recipe must discover the available route;
method packaging alone grants no consent, scheduled work or native proof.

For already initialized owners with released legacy observation logs, the existing
explicit local `init` route must reach canonical owner initialization and admit
its persistent selection lock without recreating data. Observation queries and
review refuse missing required locks without repair. Expose the explicit setup
prerequisite in the operator guide and conditional method; it is not model
permission to delete locks or reset the owner.

Propagate the actual request context into both owning declaration mutations.
After acquiring the observation lock and revalidating current input/review, check
cancellation immediately before the first authoritative write. Installation
custody validation alone does not supply this check, and a pre-closure check does
not cover cancellation during binding validation or owner-lock wait. Keep the
installed-source fence outside the owner lock; never reacquire it inside. The
first canonical control append is the commit point: finish its coupled pending
denial metadata, and retain/report real partial state if a later I/O error occurs.
Cancellation before that point cannot approve, reject or redact an observation.
