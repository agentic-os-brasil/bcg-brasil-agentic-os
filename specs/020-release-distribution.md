# Spec 020 - Release distribution

Status: accepted contract; packaging, production signing, provider
authentication and activation are separate implementation tracks.

## Objective

Make one Maestro release verifiable and portable across artifact providers
without coupling trust to a GitHub owner name. Pilot users consume releases;
they never clone or update the product through Git.

## Release authority

The release manifest is the portable authority for a release set. It names the
Maestro product, immutable release and component versions, compatibility,
artifact identities, detached signatures, migrations and release notes.

The detached manifest signature must be verified against an approved local
issuer/key registry before its fields are trusted. Parsing a manifest,
validating JSON or comparing a checksum does not establish signature trust.
The manifest issuer is a Maestro identity and key ID, not a repository URL.

`schemas/release-authority-registry.schema.json` defines that local trust root.
It contains public Ed25519 keys only, scoped to the Maestro product and an
issuer/key identity. Each key has a bounded UTC validity window and is either
active or explicitly revoked. Unknown fields, duplicate JSON keys, duplicate
issuer/key identities, non-canonical public keys and invalid status/time
combinations fail closed. A future or expired key remains in the inspectable
registry but cannot verify a release.

Private signing keys, certificate material and custody operations never enter
the registry or repository. The stable bootstrapper must receive the approved
registry through its independently signed seed channel; a managed bundle or
provider response cannot replace the trust root that authenticates it.

The release provider is an adapter. GitHub private Releases is the pilot
provider, but provider responses cannot add, replace or rename artifacts after
manifest verification.

## Historical v1 components

- The stable bootstrapper owns installation and activation. It is not
  self-updated by the first release contract.
- V1 CLI component metadata describes the historical `bcgos` executable and
  its compatible bundle range. The removed executable is not a modern product
  entrypoint; the modern profile below declares the actual MCP server.
- The base bundle is one platform-neutral, immutable archive containing only
  reviewed managed product content.
- V1 currently recognizes optional `runtime_pack` artifacts. Their authenticated
  adapter compatibility and placement remain separately specified; Spec061 and
  DIPK govern the supported Docling input closure. Presence alone does not make
  a runtime available, and the modern profile below uses typed adapter/target
  selection rather than the legacy single-runtime placement.

CLI and bundle versions are independent. A release is valid only when the
declared current versions accept each other. Update planning must also prove
that each intermediate activation state is compatible.

## Manifest v1

`schemas/release-manifest.schema.json` defines the structural JSON contract.
`internal/releasecontract` is the normative semantic validator for rules that
JSON Schema cannot express:

- exact canonical `MAJOR.MINOR.PATCH` versions;
- closed-open compatibility ranges using `>=A.B.C <X.Y.Z`;
- mutual compatibility of the current CLI and bundle;
- valid CLI component metadata and exactly one platform-neutral base bundle;
  the current v1 implementation permits no CLI artifact, which does not prove
  that such a release is installable;
- unique artifact names, signature references and kind/platform tuples;
- basename-only references with no traversal, mutable `latest` aliases or
  local/workspace paths;
- lowercase SHA-256 digests and detached `.sig` references;
- migration targets that match the declared component version and are outside
  the source range.
- the `practice-agent-to-pa-expert` migration, when present, pins its bounded
  source/expiry versions and the exact bundle, agent-catalog and agent-policy
  SHA-256 identities.

Strict parsing rejects unknown fields, duplicate object keys, oversized input
and trailing JSON values. CLI and bundle artifact names contain their component
version. V1 has no silent compatibility fallback.

The manifest wire files are exactly `release-manifest.json` and
`release-manifest.json.sig`. The signature is a raw 64-byte Ed25519 signature
over the exact manifest bytes as downloaded; there is no JSON reserialization
or whitespace normalization before verification. An implementation may parse
only `issuer.id` and `issuer.key_id` as untrusted routing hints, selects a key
only from its approved local Maestro release-key registry, verifies the
signature, and only then trusts or semantically validates the manifest fields.
Every artifact `.sig` likewise signs the exact artifact bytes.

A release version is globally immutable across channels. Promotion from
`canary` to `beta` or `stable` requires a new release version and a newly signed
manifest. Publication state must reject reusing a release version with another
channel or changing the manifest bytes behind an existing version.

## Distribution boundary

Allowed release content is selected through an explicit factory allowlist.
Repository copies, globbing the entire bundle tree and generated source
archives are not product bundles.

Releases may contain code, official skills, sanitized templates, schemas,
policies and runtime adapters. They may never contain:

- source development harnesses or contribution hooks;
- credentials, tokens, signing keys or provider responses;
- user preferences, memory, logs or local configuration;
- workspace metadata, case content, client data or personal data;
- caches, Git history or unreviewed generated files.

Factory validation must reject symlinks, traversal and an allowlisted file that
changes type.

## Release states

An unsigned deterministic output is a **release candidate**, not a pilot
release. A pilot release additionally requires:

1. manifest and artifact signatures from approved production identities;
2. native Windows and macOS signing/notarization evidence;
3. authenticated private-provider publication;
4. install, update and rollback acceptance on clean corporate Windows and
   macOS devices.

Missing authority is reported as `unavailable`; no production command accepts
an unsigned override. The existing unsigned trial acknowledgement remains
confined to `installers/trial`.

A controlled Canary may install real user-space product bytes before
organization-owned Apple or Microsoft signing custody exists only through a
packaging-time platform-local-beta profile defined by decisions `CARY`, `PONB`
and `DZIP`. This is not a production override and is not selectable by an end
user. The current cohort handoffs are `windows-portable-local-beta` for Windows
amd64 and `macos-portable-local-beta` for macOS arm64. Each factory requires an
Ed25519-authenticated `canary` manifest, exact active issuer/key registry,
exact registry and target bootstrapper SHA-256 pins and a version-matched
seeded bootstrapper. Windows additionally requires Authenticode status exactly
`NotSigned`. macOS requires the bootstrapper and selected CLI to carry an exact
ad-hoc code-signature container; a native macOS build must also observe
`codesign` status exactly `Signature=adhoc`. Developer ID, invalid, unsigned,
partial or structurally contradictory signatures are rejected from this local
beta profile. Any partial
profile, different channel or identity, digest drift, invalid native-signature
state or verification failure remains fail-closed.

The factory emits two deterministic target-specific ZIPs, never one universal
runtime seed. Each carries the same complete closed signed release, pinned
registry and its one native bootstrapper under the conventional managed-root
seed, one seeded `maestro-os/` workspace, bounded provenance and one internal
activation command. Other target artifacts may remain inert inside the closed
release, but no activator can select, install or execute them. The package
contains no wizard, installer bridge or user-facing activation command. The
workspace ships a first-use `CLAUDE.md`: the owner
opens that folder in Claude Code and sends a natural-language message. Claude
explains the bounded preparation, asks for the one setup confirmation and,
only after an affirmative answer, invokes the internal activation command.
The owner is never instructed to use a terminal or run a command. Native Claude
Code and operating-system permission prompts remain visible and cannot be
bypassed.

The internal activation command delegates first install to the stable
bootstrapper and then invokes the existing one-and-done Claude setup against
the fixed packaged workspace. Runtime projection preserves the seed
orientation and appends the managed Maestro block and skills, so the first-use
guide hands off to the ordinary onboarding without creating a second workspace
structure. The ZIP and its executables remain
unsigned controlled-beta candidates and require controlled delivery plus an
independently published ZIP SHA-256. This does not satisfy native signing,
clean-device, pilot or production gates. Moving the package after activation
is unsupported because the adapter contract binds the absolute installed CLI.

After adapter verification, platform-portable activation completes workspace
and onboarding activation without enrolling a native scheduler under the
technical setup confirmation. Scheduler enrollment is offered separately only
when the exact native adapter reports it available and relevant. An unavailable
adapter creates no owner-facing choice; an unavailable or declined scheduler
remains honestly unscheduled and must not roll back an otherwise valid
installation or be presented as active automation. Spec 051 defines target
closure, product parity, guided next actions and the complete portable
acceptance contract.

## Acceptance evidence

- Schema and Go semantic validation agree on every v1 example.
- Tampering with the manifest, artifact bytes or signature fails closed.
- CLI and bundle compatibility is observable independently.
- Deterministic packaging excludes every non-allowlisted path.
- Release notes disclose capability gaps and migration requirements.
- Validation distinguishes candidate build, signed publication, local
  installation, operator-attested device evidence, approved corporate-device
  acceptance and pilot readiness.

The `0.2.0` release opens the bounded role-migration boundary. An update from
`0.1.x` must carry that signed migration evidence; releases that still accept
such an installation carry it as well. After the boundary, legacy practice
roles and IDs fail closed. Install state carries the migration identity and
every preparation/activation path enforces the same source range. Rollback
refuses to reactivate a legacy authority without a canonical PA Expert binding,
including from later canonical states that legitimately omit the migration ID.

## Modern MCP local delivery — MPDI construction contract

This section is the accepted modern construction contract. It does not assert
implementation, production signing or participant-installation acceptance.
Preserve historical v1 parsing, signed bytes, state and migration meanings.
Legacy portable ZIP and last-ZIP migration retain their bounded contracts;
they do not establish this profile's executable or installation identity.

### Authenticated modern components

Manifest schema version 2 uses the closed profile `mcp-local-v1`. It retains
product/release/channel/issuer, signed artifacts, migrations and release notes,
and requires these discriminated components:

- `server`: version, compatible_bundle, protocol `maestro-mcp-identity-v1` and
  embedded_inputs_sha256. It declares the real `mcp_server`, with one artifact
  per supported concrete target, rather than a legacy CLI.
- `bundle`: version and compatible_server, checked mutually against the server.
- `delivery`: profile, independent installer_version, installer_protocol
  `maestro-install-seed-v1` and client_inputs_sha256.
- Exactly one any/any base bundle and optional runtime_pack artifacts with
  adapter `docling` and an exact separately qualified target. Adapter fields on
  other artifact kinds and mixed/duplicate runtime identities refuse.

Reject unknown fields, mixed v1/v2 component fields, partial profiles and
unsupported protocols. Artifact names bind actual component versions. Preserve
one canonical exact-byte verifier; dispatch strict wire versions only after
signature authentication. The installer trust root is independently seeded and
is not self-updated by this contract.

The real server has a bounded read-only identity command reporting product,
component, protocol, actual linked version/range, public registry pin and the
complete sorted public embedded-resource inventory/digest. Bound output to
1 MiB and execution to a finite deadline. Include all methods, fourteen agent
assets, policies, licenses and provenance; exclude the executable's own hash
from its embedded inventory. Verify the actual executable file separately
against its authenticated artifact. Identity needs no home/workspace admission.

Use one documented canonical UTF-8 JSON representation with lexicographic object
keys, no insignificant whitespace and explicit required fields for inventories
and digest vectors. Sort artifact arrays by kind/OS/architecture/name. Compute
client_inputs_sha256 over profile/protocols/installer version, server artifact
identities and embedded digests, base bundle identity and sorted optional runtime
identities, excluding that digest itself. Factory and consumer independently
recompute it; a later feed can bind it without a circular archive hash.

### One transaction and real first installation

Build actual maestro-mcp and maestro-install entrypoints from the existing MCP
module on the actual target runner. Compose the existing verifier, installer,
installtx, home/workspace admission and packet builders. No renamed legacy
binary, absent command, alternate transaction or injected healthy checker can
establish modern activation.

The stable installer reports strict seed-status v2 with product, installer
component/protocol, actual installer version and linked registry digest. Its
versioned artifact is maestro-install_VERSION_OS_ARCH; installed name is
maestro-install. Legacy seed-status and names remain confined to v1.

Modern activation plan/state use schema version 3; activation receipts use
schema version 2, carrying authenticated profile and typed server identity/compatibility.
Current and Previous snapshots retain their own profile. Prepare, prepared-plan
validation, activation, payload/receipt validation, reconciliation, backup and
rollback select the same canonical built-in protocol. Prove an unchanged old
strict reader/writer refuses modern state. Preserve role/catalog/policy migration
checks; never infer modern authority from a filename or callback.

Review is read-only and binds signed release/seed, exact target, managed root,
private installation-state root, proposed admitted home, workspace, packet
destinations and all effects. Explicit confirmation recomputes the same digest
before canonical staging/activation. A narrow canonical home-address preview
shares Resolve's selection/origin/locator logic without requiring a pre-existing
managed payload or creating an origin, key, locator, directory or authority.
Apply resolves the actual installed root again and refuses selection drift.

An existing complete plain-MCP packet may be preserved only after the original
closed packet validator proves its exact server, registry, version, target and
canonical contents. Directory and MCPB inputs share that validator; ZIP review
does not extract or initialize. Bind the preservation disposition and exact
inventory digest into review. Changed, linked or unknown destinations refuse
without replacement. Repeated explicit setup preserves admitted owner, memory,
authored workspace data and client registrations.

First-install directory custody is an exclusive claim with an immutable private
PID/plan marker, retained even after success. Incomplete roots refuse a live
claimant and can reconcile only the same exact plan after that process exits.
A healthy committed installation ignores the historical marker only after
exact server, seed and registry validation. Failed staging reports pending
installation and preserves staged files, later edits and successor markers;
there is no path-based destructive failure cleanup. Existing empty destinations
and ambiguous state are preserved. The marker is historical custody evidence,
never activation authority or permission to delete another object.

Modern installtx DataRoot is the reviewed private UserHome/.maestro-install,
outside both ManagedRoot and admitted Home. It contains only installation
state/updates/receipts and grants no home authority. Preserve it through rollback.
After activation, perform genuine canonical home/workspace/owner/memory admission,
then materialize both participant-local packets from the verified installed
server. Reuse already admitted state; ambiguous existing homes refuse. On later
admission/packet failure preserve coherent activated payload and existing owner
data, report setup pending and permit honest recovery. No ready acknowledgement
substitutes for completed admission/readback.

Installed bin/maestro-mcp loads ManagedRoot/trust/release-authority-registry.json.
Code scripts and MCPB server layouts load packetRoot/managed/trust from their
recognized fixed layouts. Every public registry must match the compiled pin;
do not scan arbitrary ancestors or accept model-supplied seed paths. SDK identity
and converter client compatibility use the actual linked server version, with
no independent 0.1.0 default. Unseeded development builds invent no release trust.

### Complete public closure and acceptance

Keep an exact factory allowlist with every required canonical agent/method,
policy, template, notice and provenance input. Any root LICENSE.md source
exception is exact and nonexecutable. Release kits contain immutable public
inputs and an independently admitted installer/registry; bound packets and
private owner/workspace data are materialized locally. Optional authenticated
Docling inputs reach their owning admitted-home location with signed metadata,
archive, actual client compatibility and exact target intact. No pack means
explicit unavailable content support, without ambient installation/acquisition.

Optional runtime placement retains failed staging for explicit inspection.
Successful publication does not authorize cleanup or permission changes at the
former staging pathname. Intentional pre-publication mode changes use the held
staging root; failure handling neither walks nor recursively deletes a path that
could now name another writer's tree.

The allowlist has an optional closed `role` of `data` or `executable`; omission
retains historical source-mode normalization. Modern factory inputs all declare
a role. Declared roles map only to archive 0644/0755 and must agree with admitted
POSIX source roles when those bits exist. Windows-native factories consume the
declared roles directly. The root license can only be data. The same authenticated
extractor enforces installed roles; no second archive inventory is introduced.

The focused first-install proof uses actual built binaries and engineering
signatures: factory, independent seed, exact review/confirmation, canonical
activation, genuine admission, complete Code/MCPB materialization, installed and
carrier identity/registry checks, then real MCP stdio continuation across restart.
Production native trust has no engineering bypass flag. Signing/device/client
registration/pilot remain separate evidence. Modern standard update stays
unavailable until its owning signed-source/pending/recompute path is implemented.
The cohesive delivery also requires signed synced generations/update/rollback/
recall and registration/recovery tasks; a schema-only PR does not complete it.

## Signed synchronized generations and qualified update (MPDU)

Accepted engineering construction contract. It extends MPDI through the
existing owners; it does not assert production policy or runtime qualification.

### 1. One authority and one signed generation representation

Add the closed `maestro-synced-generation-v1` record to releasecontract and its
exact-byte verifier to releaseverify, sharing the admitted registry's current
issuer/key/algorithm/validity rules. Authenticate before semantic parsing. The
record contains product/protocol, issuer/key, source ID/channel/exact target,
positive monotonic sequence, canonical published/expiry times, release version,
exact manifest/client-input digests, finite release-specific recalls and minimum
source-aware reader contract. No participant path or replacement registry belongs
in signed publisher bytes.

Publish immutable generation/release metadata and signatures first. The final
current pointer is the same signed generation bytes, promoted last. Require the
immutable generation record to match that pointer. Identical retry may return
unchanged; sequence equivocation and overwriting an immutable release refuse.
A later generation may reference the same unchanged release digest to refresh
expiry or add recalls. It cannot redefine that version's artifact identity.

### 2. Explicit local source enrollment and private snapshots

Trusted operator review selects one source ID/channel/target and an exact local
distribution-folder locator for the real installation. The path supplies transport,
not trust. Under SFRD, Windows distribution input may traverse held ordinary objects
or the finite documented Cloud Files tags. Resolve only the authenticated finite
file set; bound each leaf before requesting hydration of its measured range through
the operating system. Cloud directories may remain partially populated, while every
required file must be fully ready before and after reading. Junctions, symlinks,
unknown reparse types, aliases and partial/offline/error files remain unavailable.
Copy accepted bytes into disjoint ordinary private staging and retain the existing
complete signature verification. This source-input extension does not relax any
private owner/workspace/installation reader or authorize execution from the share.
Hydration bounds bytes, not provider/kernel wall time. Actual native Windows/OneDrive
qualification remains separate; the earlier experimental pilot artifacts are unchanged.
Later metadata cannot silently rebind that tuple or its independently pinned
registry. No SharePoint destination, endpoint or account is guessed.

Read bounded no-follow/unaliasable source files under the platform rule above and copy the exact
manifest-authorized release set into private install-state staging. Verify the
complete snapshot through the canonical directory verifier; execute no synced
file. Review reads metadata/manifests within finite in-memory bounds and grants
no admission or custody write. Confirmation precedes persistent source effects.
Candidate copying and its private bounded destinations must be declared effects.

Before metadata qualifies for source review/custody, enforce an exact raw-body
limit of 64 KiB for this generation protocol and for manifests declaring the new
source-aware execution contract. Signed whitespace counts toward that bound;
never normalize or reserialize signed inputs to fit. Historical manifest variants
retain their existing limits. Prove that the largest supported current/Previous
manifest bindings, exact source body and finite recall custody serialize with
headroom under the existing 1 MiB install-state limit. An oversized authentic
envelope is unsupported input before acceptance, not accepted denial silently
discarded by a later state-write failure.

### 3. Monotonic custody belongs to installtx, outside Previous

Extend the existing installation-state owner with source-aware schema4. Its
current source control retains the enrolled tuple, highest accepted sequence and
body digest, signed metadata/expiry, cumulative known recalls, minimum floor and
last trusted observation time. Current/Previous release snapshots retain their
own signed manifest/client-input/server/bundle bindings. Source history is never
inside a restorable Previous snapshot. Keep one transaction/serialization owner;
no independent ledger service, queue or trust key is introduced.

Once the exact authenticated metadata observation is confirmed, its high-water
and recall effects persist even if the candidate payload is incomplete. Missing
payload returns unavailable and cannot activate; it cannot conceal an accepted
recall. Same-sequence/different-body and lower-sequence replay refuse. Bounds
overflow refuses rather than dropping recalls or history. A no-change observation
is distinguishable from a fresh cryptographic check/report.

If confirmed authentic higher metadata would overflow the finite cumulative
recall list, preserve the bounded existing list and persist the higher signed
body/sequence together with sticky `recall_capacity_exhausted` in this same
SourceControl owner. Refuse launch, candidate use and rollback with that stronger
denial; do not leave the older fresh generation launchable. All supported state
writers, compensation and recovery preserve the marker. A later fresh/higher
generation, restart or older replay cannot clear it. Independent admitted
installer metadata diagnosis remains available; resuming payload use would
require a separate explicit supported repair/migration contract. There is no
implicit reset, second ledger or unbounded recall retention.

Authentication, tuple/shape validation and confirmed observation precede freshness
admission. A newer authentically signed generation whose expiry is already past
still advances the accepted high-water and cumulative recalls after that
confirmation. It supplies no launch freshness or activation authority. Persist the
observation before rejecting temporal readiness; replay of an older still-
unexpired generation cannot undo it. Include an expired higher-sequence record
recalling the current release in the existing expiry/replay proof.

Every new supported state writer, compensation, reconcile and rollback preserves
or monotonically advances current source custody under existing serialization.
Strict old public installation/activation/reconcile/rollback paths must refuse
protected source-aware state. Prove those actual paths. The old exported blind
WriteState helper and arbitrary filesystem writes are not retroactively fenced;
do not claim they are. New qualified launchers require the source-aware state and
floor and refuse missing/downgraded/inconsistent authority instead of reinitializing.

### 4. Existing check, stage, confirmation and activation chain

Use typed modern update-plan schema3, pending schema2 and source-aware transaction/
receipt variants, preserving historical variants. The plan binds exact current
installation, source tuple/sequence/body, target, manifest/client inputs, compatible
installer, actual server/bundle and prepared bytes. Stage/confirm/prelaunch/apply
and crash reconciliation reload signed source truth and current private custody;
they never trust an approval boolean. Changed source, expiry, recall, state or
prepared bytes requires fresh review or explicit recovery. Repeat critical checks
under the existing activation serialization boundary.

The independently admitted installer gains the real update/reconcile/rollback
entrypoints. No successful process exit or cached receipt substitutes for actual
installed server/bundle readback. Report already-applied-but-blocked outcomes
honestly if source recall changes after payload commit. Owner/workspace bodies
remain outside managed activation/rollback effects.

### 5. Source-aware rollback and existing-state migration

Rollback is a separately reviewed operation that restores only an authenticated,
nonrecalled previous release supporting the current source contract/floor. Verify
its actual executable and bundle, preserving current high-water, recalls and
enrollment. Do not restore source history or permit a standard downgrade to become
an implicit floor reset.

The full tracer starts with newly built source-aware N and N+1. New qualified
first installation records the original signed manifest/client-input identity.
Schema3 has no such complete identity: migration/enrollment must authenticate the
actual current signed release and exact installed binding, and require compatible
source-aware execution. If that proof is unavailable, return an explicit
upgrade/migration-unavailable state without writes. Never infer it from version
or one executable hash, or bypass the first-install profile mismatch fence.

### 6. Qualified launcher, carriers and running new servers

The actual read-only ServerIdentity protocol must expose the explicit maestro-
synced-source-v1 execution contract in a strict new identity variant. The
authenticated server component binds the same contract, and candidate admission,
activation, qualified launch and rollback compare it with the real verified
executable. Version, schema or a manifest declaration alone cannot supply that
capability. Preserve historical identity variants explicitly. The negative uses
the real Task1 server with valid signature and compatible version but no execution
contract; it must refuse without inventing a healthy capability callback. Positive
proof exercises actual custody/launch/request guards in the new real server.

New participant carriers use the independently admitted launch owner. It reads
private source/current installation, enforces the accepted source freshness,
known recall and minimum floor, authenticates the exact executable before running
and starts only the admitted installed server. No model-selected binary, registry
or installation root is accepted. Cached N packets must refuse after installation
or authenticated client-input identity changes until canonical regeneration.
Task2 owns these necessary packet launch bindings; Task3 owns real host
registration. Carrier currency binds the active installed release and immutable
client inputs; a metadata-only observation updates source control without
automatically changing that release identity.

Enrollment follows the canonical installation state, rather than the presence of
an optional launch flag or the capability advertised by a binary. A new healthy
schema4 first installation with no enrolled Source keeps its prior plain-MCP
behavior, while qualified source launch remains unavailable. Ordinary unbound
portable packets retain that distinct profile and provide no installed-source
eligibility. Source-bound complete carriers and stable client registrations are
a separate reviewed client unit.

A recognized installed bin process pins its actual managed root through the
process-start executable identity and refuses a different root selection. Once
that installation is enrolled, all affected callbacks and existing publication
guards enforce its current source custody even with optional carrier pins omitted.
Missing/downgraded installed state cannot become an unenrolled first installation
through a launch flag. Explicit immutable carrier pins always require qualified
source admission. These checks protect the admitted installed execution path;
ordinary unbound copies are not silently relabelled as qualified source clients.

New running servers revalidate their actual installation/source binding at request
admission and existing long-running publication guards. A failure reports restart/
refresh/recovery required; it does not relabel copied packet bytes as current.
Existing startup executable pinning remains in force. This protects the qualified
new execution path, not arbitrary user-controlled Task1 copies or already-running
old binaries. No remote kill, DRM or kernel attestation is claimed.

Trusted installer diagnosis/source-refresh/upgrade commands remain available when
the product server is expired, recalled or unavailable. Startup/status stays
read-only; it never automatically refreshes custody or activates an update. A
recovery instruction must be usable without launching the blocked server.

For source diagnosis/refresh, use the independently admitted installer's exact
pinned public authority when the active MCP cannot supply its identity, including
missing or corrupt active bytes. Reauthenticate the signed stored/current source
and its installation bindings; never infer a healthy MCP or accept a caller's
replacement registry. A source-blocked authentic payload must still support the
guarded upgrade path. Replacing a missing/corrupt active payload is a distinct
repair variant: Task2 does not bypass the canonical verified-backup requirement
or claim rollback to absent/untrusted prior bytes. Until an explicit reviewed
repair variant exists, return precise payload-repair-unavailable diagnosis and
preserve payload/owner state rather than quarantine or reinstall automatically.

### 7. Finite expiry, offline and clock policy

Publisher expiration is required explicitly: positive TTL, at most 30 days for this
supported engineering contract, never beyond an admitted key's validity. Synthetic
pilot examples use 7 days; neither is asserted as an organization-approved SLA.
Production authority must deliberately issue its chosen supported expiry.

An admitted cached generation permits qualified offline launch only until its
signed expiry and while not known recalled/incompatible. An expired record cannot
grant freshness. Explicit trusted source checking may recover freshness through
new authenticated metadata; local presence alone cannot. Recall becomes effective
through accepted authenticated observation, with the declared offline-expiry
boundary; there is no globally instantaneous SharePoint freshness claim.

Use the trusted process wall clock, with no model/CLI/environment clock override.
Reject backwards movement below the last persisted accepted observation and
implausibly future publication (bounded five-minute skew). Do not claim protection
against owner-controlled clock tampering beyond that contract. Test-clock seams
remain trusted in-process composition only.

### Routine availability and deliberate update (SFRD)

Offer `update-availability` as an opt-in owner/managed-installation job on the
existing maintenance worker. After genuine qualification and reviewed enrollment,
observe the enrolled source once per local calendar day on first active use;
coalesce missed days and concurrent/reconnected workspaces. This metadata-only
observation alone may bypass idle. Reuse bounded authenticated `ReviewSynced`
reads, with no source discovery/enrollment, custody advancement, payload staging,
activation or model call. Preserve the configured private brain location.

Keep bounded local scheduler/diagnostic records distinct from source-content
writes and installation custody. Availability reports retain observation time,
leave payload `not_checked` and never describe offline/partial/expired/recalled
input as current. The conversational update entry uses the existing exact reviewed
installer and a deliberate supported reconnect boundary. An old running session,
copied instruction library or retained qualification is not evidence that the new
complete product is loaded and functional. Preserve stale-proof refusal and
report the actual revalidation/client phases. Spec062 owns this job lifecycle;
the separate attended `update-check` below retains all its existing effects.

### 8. Real attended update-check and focused complete proof

The existing `update-check` job remains unattended-never and cannot enroll as
recurring work. Declare its real source-custody and bounded local-diagnosis/plan
writes explicitly in the catalog and trusted review. Its exact captured signed
source input and private current state must be revalidated before effects. A
decline or stale review grants no new authority. It does not activate an update.
CLI/host forms use the existing exact review/confirmation conventions.

Unchanged source/candidate replay cannot mint qualification through observation
time or a timestamp-derived pending-plan ID. Keep exact plan/state binding for
activation, while diagnostic semantic identity excludes that derived PlanID.
Reviewed-no-change clears the job's own one-shot under live authority and
preserves its existing genuine qualification/receipt, or remains unqualified if
none exists. Preserve the stored plan's actual provenance; separately verify a
bounded actionable current-pending projection through the canonical read-only
confirmation path. A stale stored plan is historical/unavailable, not relabelled
as the current plan or newly healthy execution.

Implement one cohesive real-binary N→N+1 check/stage/confirm/activate/restart and
compatible rollback tracer using synthetic signed local generations and existing
canonical owners. Prove actual executable/resource/bundle and unchanged owner
data. Include partial sync and pointer pairs, aliases, wrong tuple/target,
equivocation/replay, expiry/recalls, pending drift, crash reconciliation, retained
source history/floor, genuine old public workflow refusal, stale carriers and
running new-server refusal. Keep native-positive fixtures labelled engineering;
actual client/device/signing/pilot remain external gates. No full unrelated suite.

Root owns canonical docs/decision/schema/CI and Git. Source production starts only
after this contract is accepted and the combined modern/Wiki baseline is saved.

## Stable current client materialization (SCAR)

Task2 is independently approved at tree b6f19c34fdb64e17445b58d206630937ca9f8ec1
and integrated at f398d32478c19259932c8ecdaebdc4bcf1380cb9. Task3 may implement
the following explicit profile; native compatibility remains separately evidenced.

### Profile and finite owner

Keep release-bound packet schema1 and its exact pins unchanged. Stable packets
use separately recognized inventory schema2, profile stable-local-v1, protocol
maestro-stable-carrier-v1, fixed random 32-lowercase-hex carrier ID and carrier
version 1.0.0 independent of server release. Stable behavior never follows from
omitted release pins. Preserve strict seed-status v2; discover the supported
profile/protocol/version through a separate bounded client-capabilities command
on the independently admitted installer and actual current server.

The existing installer owns one strict private current record, at most 64 KiB,
at UserHome/.maestro-install/clients/<id>.json. Derive this path from the fixed
binding only. Bind profile/protocol/ID, admitted roots/workspace/registry,
current release/manifest/client-input/server/embedded-input/bundle identities,
exact Code tree and Desktop package/native-contract identities, and bounded
separately classified host observations. Keep historical inert Desktop bootstrap
provenance explicit. This record stays outside SourceControl and Previous; it
cannot grant freshness, select another executable/key/root, reset source history
or manufacture NativeHost qualification.

### Checked construction and launch

Under existing installation serialization, authenticate Current and source,
invoke only the authenticated installed current server's plugin/mcpb generators,
independently inspect complete packet inventories/resources and recheck binding
before atomic record publication. The stable installer must not generate a new
release using its own older embedded methods. Trusted in-process materializer
composition grants no caller/model path or trust authority.

Stable native argv fixes the installed maestro-install executable, stable-launch,
product root, user home, workspace and carrier ID. Resolve only that finite record.
Native-admit Self and revalidate current source expiry, known recall, writer
floor, actual server, client inputs and exact packets before returning the same
exact pinned server launch as release-bound mode. Existing startup/request/
publication process-identity checks remain effective. Status/startup inspection
is nonmigrating and never refreshes source or invokes a client. Only normal
stable startup may recover an exited activation owner through the scoped DSRC
boundary below; read-only status and lifecycle hooks do not create that guard.

Materialization currency binds immutable active inputs, not source-generation
timestamps. Metadata-only refresh or repeated identical release needs no
regeneration. Activation and compatible rollback invalidate mismatched records
until checked regeneration, preserving advanced source custody. Failure reports
activation-applied/client-materialization-pending rather than reversing source.
Stage and inspect complete packets before publication; publish the current record
last. First publication is exclusive and unchanged reconciliation performs no
replacement. A differing existing Code tree or Desktop package must preserve
both current and candidate bytes and return an explicit manual client step
unless the actual deployment profile proves strong physical custody against an
uncooperative concurrent writer. Hash-then-rename/unlink is insufficient. This
construction does not claim that custody on ordinary shared client profiles.

An operator-retained prior packet may be recorded only at the immutable same-
parent path derived from its complete prior byte/mode digest. Retain the bounded
complete Code/Desktop backup inventory, validate every recorded entry and never
overwrite unknown bytes or garbage-collect earlier backups. Identical known
backups are reused only after exact binding/full-byte checks. Reconciliation
reauthenticates Current and actual package state and may finish an already
operator-applied current publication without repeating replacement. Unknown,
changed or aliased packets/backups remain preserved and refused. An exit status
or saved approval cannot advance binding.
An existing staged candidate is not binding authority merely because inspection
succeeds. Verify its exact admitted native launch binding before using its
contract as the reference; both adoption branches compare the destination's
actual contract with that reference before swaps or record publication.
Explicit digest-bound client reconciliation and normal stable-client startup
may use the canonical dead-owner reconciliation lock after a real process
interruption (DSRC). Startup first authenticates exact installed admission,
current signed source, carrier, owner/workspace and current materialized
selection; it repeats the original guarded validation after acquiring the
activation lock. All canonical reclaimers share the existing OS-released
private lock on one retained empty private per-root recovery guard, held across
fresh stale-owner/identity/body verification, removal and exclusive reacquisition.
The retained guard is never unlinked and has an exact regular empty private-file
inventory contract. An ordinary writer winning exclusive reacquisition is
preserved and causes refusal. Live/reused/unknown owners and malformed,
aliased or changed locks refuse. Status, lifecycle hooks and ordinary
materialization never reclaim locks or reset custody; recovery changes no
source, client packet, profile or native qualification.

### Supported client effects and evidence

Code uses selected absolute executable/hash, bounded version/help admission and
supported JSON semantics, with at least 2.1.289 in-place readback; the separate
native planner minimum remains 2.1.295. Use fixed marketplace maestro-local-<id>,
qualified maestro-product identity, relative local plugin source and explicit
user/local/project scope. Read marketplace/plugin JSON before and after supported
add/install/update operations. Refuse foreign same-name sources, ambiguous scope
and duplicates. Preserve disabled state, auth and normal permissions; never
update by uninstall/reinstall or raw profile editing. Registration readback is
distinct from a loaded session and functional execution.
Unrelated cached plugin rows need only their common inventory fields; strict
local-in-place observations apply to the selected Maestro row. A participant
who initially defers Code can select the first executable later through an
exact reviewed same-carrier change. Fixed roots/scope remain bound, and an
already selected executable is not silently replaced.

Desktop state separates bundle_ready, open_requested, awaiting_user_install,
user_confirmed_install, client_connection_observed and functional_check_passed,
with operator-attested versus observed provenance. No undocumented headless
private-install/inventory API is assumed. Reuse an old exact MCPB only when its
complete native-facing launch/config/permission/compatibility contract and
carrier version are unchanged, after independently verifying new current inputs.
Only deliberately inert packaged-server/inventory provenance can be excluded
from that native-contract comparison. Live resources come from the external
current server; old bundled provenance stays historical. Any native-contract
change requires manual installation. External absolute launch/reuse is a
candidate requiring actual Desktop qualification. Manual updated-MCPB
installation retains this same carrier/absolute-launch contract; it cannot
bypass a rejecting client policy. In that policy the stable source-managed
carrier is unavailable. An ordinary unmanaged MCPB is a separate profile,
not a source-managed fallback. Never claim MCPB automatic updating.

### Complete versioned clients from one synced source (SFRD)

New reviewed selections may use private versioned client slots. Carrier, signed
manifest and a bounded reviewed preparation-generation ID determine finite sibling
names; the private current record selects the exact verified generation. Publish
that record last. Preserve previous, failed, occupied and foreign candidates;
never replace an unknown shared tree to make an update succeed. Legacy records
without this explicit selection retain their original serialization and behavior.
The admitted workspace and physical Developer brain binding survive every phase.

For this profile, Code uses its supported command plugin source in COPY mode.
A fixed participant-local provider invokes only the independently admitted
installed client's current-plugin-path reader. Before and after readback validate
Current, source custody and exact private-record identity; this reader cannot
repair, stage, approve or activate anything. It does not reacquire the native
registration owner's lock. Supported scoped install/update and canonical copied
payload readback preserve disabled state and refuse foreign entries. Host policy
may reject command sources; no alternate updater or profile edit bypasses it.
The existing relative local source remains the legacy selected profile.

The original read-only registration review binds the exact generated provider
catalog, launcher script, current packet and narrow supported host command digest.
Operator confirmation may supply only `--accept-command <sha256>` for that exact
command, catalog, COPY mode and plugin identity. Missing host support remains
pending; no dry-run install, broad approval or Claude-session guard bypass is
allowed. Current native COPY observations that omit folder fields require exact
selected-profile cache custody, actual manifest identity, independently derived
host content-version hash and canonical payload/topology equality, with only the
bounded typed root host lease role admitted. Preserve the raw host version and
separate manifest/content evidence. Neither consent nor registration proves
session loading, native enforcement or functional acceptance.

A complete native selection uses the existing genuine review/qualify producer,
with stable carrier and destination generation bound before the probe. Run the
qualifier outside installer publication serialization, then publish only its
same-call native_observed result and unchanged canonically inspected candidate
under the existing guards. Imported proof, engineering witnesses or missing
native capabilities cannot qualify it. Do not silently downgrade a complete
selection to resources-only. A failed attempt remains preserved; an explicit
new review selects a fresh bounded generation. Existing D3 custody stays pending
where its independent requirements are unmet.

The actual current server also generates and verifies the complete Desktop live
method bootstrap described in Spec063. Compatible external-launch MCPB bytes may
remain exact while current backend resources change. Distinguish connector
installation, bootstrap installation, current backend, current live methods,
loaded session and functional acceptance. Native contract or bootstrap protocol
changes still require the host's normal review; external launch needs actual
Desktop qualification and cannot be inferred from packaging or an opener.

The fixed manage/update intent composes existing source, native, packet,
registration and per-job review owners. Whole-executable and CLI-version changes
make maintenance proof stale. Offer only previously qualified stale jobs to their
original real qualifiers, preserving opt-ins, enrollment/timezone and paused or
revoked state; absent real inputs remain pending. No old proof is relabeled.
Reload/reconnect at a deliberate safe point; do not terminate an active client or
move the brain to create an activation claim. The closed MCP handoff in Spec057
opens this manager without granting any of those subsequent approvals.

Construction tests do not admit an experimental installation. The existing
runtime/converter ZIP is not an installed-seed/source-update authority. A bounded
experimental N-to-N+1 rehearsal needs its exact separate installer-owned admission;
organization production signing and production verification remain separate.

### Closed experimental installed-seed rehearsal (EISA)

A separate compile-time experimental installer entry may consume only a closed
verified installed-seed admission. Its paired shared-MCP build uses the same
closed engineering class and fixed installed role; the default production MCP
entry accepts no engineering input or override. Bind each exact child artifact
and refuse mixed production/experimental children. Keep the production command,
strict native verifier and default wire bytes unchanged; it must refuse the same unsigned
installer and must not acquire an experimental trust override. The existing
runtime/converter pilot remains a distinct profile and keeps its enrolled-source
refusals. Signature validity in a general registry does not confer this new role.

The experimental admission has a literal purpose/protocol, a pinned public
engineering registry, exact installer digest/version/target, exactly bounded
release N/N+1 manifest/client/server/bundle/runtime identities, fixed source tuple
and finite expiry no later than the existing accepted authority window. No
arbitrary commands, alternate executable or source selection, key renewal or
new issuer is admitted. The record and exact local review are independently
verified; generic publisher records contain no participant-private path. Preserve
the existing physical Developer brain/workspace binding and all user data.

Use an opaque verified type at the owning boundary and retain the explicit
engineering evidence class through plan, installed custody/status, child process
composition, first activation, source enrollment/check/stage/apply/recovery/
rollback, stable launch, current plugin provider and update-manager handoff.
Do not inject an engineering callback into a path that then reports native-strict.
Production readers refuse the distinct engineering status/profile. Existing
production installations cannot be adopted or relabeled. Reuse the original
transactions, source custody, reviews, client slots and independently verified
current records; this profile introduces no replacement updater or service.
The actual native client qualifier, its exact host evidence and D3 custody remain
unchanged; a role record cannot qualify a missing host or native Agent execution.

In this experimental build, final full-native hooks use the existing installed
stable binding and a closed native-hook route through the independently admitted
installer. One canonical recipe owns generation, fingerprinting and inspection
of all seven events; ordinary builds retain their exact recipe and command
surface. Private qualification probes still execute the actual MCP directly,
with the verified experimental arguments, before any native client record is
published. Subsequent final hooks require the current native materialization,
its exact files/carrier/workspace and the existing qualified launch. The schema1
native bit alone is no proof: only the existing genuine qualifier, exact candidate
inspection and guarded publication can establish that materialization. Keep the
three-event passive owner separate; introduce no packet schema, proof importer
or D3 bridge.

The participant entry reuses the existing prepare/test/report flow and manager.
Prefill authenticated source metadata and admitted local selections; retain
necessary human choices and exact reviews. Distribution is read from the selected
synced source using the existing admitted ingress, while private state remains in
Developer. Do not reuse the runtime-only KIT-ID/raw-init path for an existing
brain or weaken that old profile's cloud-input refusal. Reuse the selected
absolute Code executable after the once-only choice.

The installed-profile checker derives current paths, binding and packet/native
contract currency from the verified current client record. A compatible reused
MCPB may contain historical inert server provenance; it must not fail merely
because that bundled server hash differs from the current backend. Preserve the
original profile's exact archive checks and all installed contract checks. Reuse
the existing test menu and automatic prompts, collect its existing host-observation
answers into the report and label them participant attestations. Distinguish
preparation, registration, actual loading, native qualification and functional
results; automated file/SDK checks cannot stand in for host observations.
Desktop instructions identify the exact admitted stable connector, including a
compatible retained MCPB; a generic or legacy product name is insufficient.
The complete test action prepares the existing public synthetic DOCX in the
selected workspace without replacing any existing file and includes conversion
and paginated artifact readback against the independently verified Current
manifest and pack. An unavailable runtime remains explicitly pending. The
read-only checker does not prepare workspace files. Shared conversion-test
expectations are diagnostics, never admission to the separate old runtime-pilot
profile or evidence that a real conversion has succeeded.

Source acceptance uses ephemeral test keys and private synthetic fixtures, covers
closed-parser/purpose/target/hash/source/expiry/revocation negatives, production
unsigned refusal, actual child role propagation and bounded N-to-N+1/rollback
readback through the existing owners. These tests issue no real role. Actual
signed-purpose issuance/use, exact reviewed artifact placement, native clients
and activation remain separate gates. Build artifacts and present exact hashes
and effects before requesting any outstanding real action; never infer that
source approval authorizes signing or placement.

### Current authenticated converter updates (PCRU)

Complete source-managed updates include a required converter pack when the
signed current release declares one. Reuse docling.PlaceVerified, complete pack
verification and the existing exact local review. Extracted payloads already
have SHA-addressed slots; add only the versioned authenticated metadata/current
selection needed to preserve differing manifests and prior runtime generations.
Never overwrite unknown or prior bytes, or delete old generations to make an
update pass. A repeated identical placement remains idempotent. A malformed,
unsafe or unknown selector refuses without fallback to an obsolete legacy pack.
Legacy calls without an authenticated Current requirement retain their original
verifier. An initial schema4 consumer already has an authenticated Current
binding before source enrollment: enforce its exact required manifest and pack
in availability, reuse decisions, placement and use. An older compatible legacy
pack cannot satisfy a different initial requirement or cause a successful no-op;
preserve its existing bytes while preparing the actual current generation.
Before enrollment, reuse the installation lock and authenticated activated-current
checks for this schema4 binding; this is neither native qualification nor source
enrollment or lock recovery. Enrolled sources retain the original qualified
current guard. Missing or malformed current evidence never selects a legacy pack.

Resolve current required pack, target, signed metadata and actual server version
from the independently authenticated current installation. Tool/model arguments
and the older installer version do not choose that requirement. Runtime status
and actual conversion enforce it as well as original signature/expiry, complete
input/license inventory, compatible client range, confinement and platform
qualification. An older merely compatible pack cannot be used as current when
another pack is required. An unchanged pack remains reusable only if all current
checks, including current-build Windows qualification where needed, succeed.

The manager update/reconcile chain composes the existing placement owner after
its exact reviewed effects and current-state revalidation. Keep source custody,
reader/writer floors, original private brain, opt-ins, auth and retained client
state. Typed experimental context from the distinct installed-seed profile must
survive branch composition; this is no new converter purpose or authority. A
failure or decline after backend activation reports source-updated/runtime-pending
and a fixed recovery action. No whole-update success or rollback is fabricated.
Rollback may select retained authenticated runtime metadata when the accepted
current source requires it, with the same current checks and requalification.
If initial placement failed before a private release snapshot existed, manager
recovery can materialize only the independently authenticated exact Current
release through the existing source snapshot owner, after foreground review of
those effects and final Current revalidation. Runtime review itself stays
read-only; recovery must not require a participant to discover archive paths.

The local runtime-review/runtime-apply consumer route requires schema4 with its
retained signed manifest/client-input binding and the actual installed executable.
A historical schema3 modern consumer lacks that binding and command contract;
converter setup reports explicitly pending before invoking a child. It neither
claims qualification built by the older installer belongs to that consumer nor
silently falls back. Use a compatible source-aware release. Raw, uninstalled and
EPRT PlaceVerified calls without an authenticated Current requirement retain
their original fixed metadata layout and authority/profile exclusions.

Source acceptance proves changed-pack N-to-N+1 placement, current identity/use,
prior-byte preservation, rollback/readback, same-pack idempotence, declined or
stale review, changed current source, unsafe/foreign/incompatible/expired inputs
and failed qualification with truthful partial outcomes. Use existing public
APIs and private synthetic signed fixtures; no extra test engine or relaxed
native evidence. Source tests, Windows compilation, actual kernel qualification,
conversion, signing and placement remain separately reported. Complete source
and manager wiring precede the cohesive reviewed front; same-pack-only tests do
not close changed-runtime support.

### Construction acceptance

Provide exact reviewed clients-review/apply/status/reconcile, explicit
desktop-confirm and stable-launch operations plus a small participant-local
manage launcher composing existing source/update/rollback reviews. No persisted
wizard engine or recurring activation authority is introduced.
The first admitted participant can reach checked client materialization through
that small console using explicit choices and a locally generated carrier ID;
hand-written JSON or manual ID generation is not the required user journey.

Prove first materialization/registration, current-server resources, disabled
preservation, foreign-source refusal, N-to-N+1 refusal before regeneration and
correct restarted SDK/stdio continuation afterward, exact unchanged compatible
MCPB bytes, metadata-only/repeated-release currency, rollback regeneration,
workspace isolation and interrupted swaps/registration/record publication.
Preserve recall/expiry/floor, stale-process/release-bound and old capability
refusals. Native-positive process composition is explicitly engineering-only;
shipping unsigned native refusal remains real. Four final independent front
lenses precede the cohesive PR. Actual clients, signing, hosted CI, SharePoint
publication and participant acceptance remain separate gates.


### Explicit experimental installed-carrier owner review (ICRV)

A fresh experimentally admitted versioned client carrier may deliberately select
owner_review_channel local-browser in the original first-client manager, before
the original complete client review and confirmation. Empty/omitted selection
retains native-first behavior and byte-compatible legacy JSON. Unknown channels,
non-experimental or non-versioned nonempty profiles, and mode changes on an
existing carrier refuse. The request and materialization bind this optional
selection into their existing review/current-record identity. The full review
discloses the owner, brain, workspace, carrier and common effect on both Code
and Desktop; selecting the channel approves no individual operation.

The original current materializer must return the reviewed mode unchanged.
Cold request reconstruction and supported source updates retain it. Ordinary
stable-launch derives the existing serve --owner-review-channel option only
from the authenticated current record; it does not pass the option to native
or passive hooks, qualification arguments or generic packet generators.

The installed receiver independently reads and verifies that current carrier,
compares the effective startup mode even when empty, and pins the whole semantic
ClientRecord through the existing ClientRecordDigest. Before and after trusted
review and before protected publication, the same original nonlocking readers
recheck current source, role, owner, admitted brain/workspace, materialized files
and record identity. Drift requires a new current connection; no installation
lock spans browser review or domain handlers. An installed browser flag without
an authenticated carrier refuses. Legacy native-first installed diagnostics
without management carrier metadata remain supported, while incomplete declared
carrier metadata refuses. The read-only installed checker derives its mode from
the same already-authenticated record and never opens an approval screen.


### Explicit installed Desktop target (DTON)

The current client request may explicitly select `target: "desktop"`. An omitted
target retains legacy both-client JSON and meaning; missing Code inputs never
infer a target. Unknown targets and Code-specific fields on Desktop requests
refuse. The exact target participates in review, generation, current-record,
readback and update validation. Existing carrier targets cannot change implicitly.

Desktop-only publication uses the original authenticated stable launcher, current
source/manifest/server, owner/workspace and Desktop native-contract custody, and
a complete live method frontend in Desktop-owned finite slots. It requires a
target-aware actual installed reader and emits a distinct record schema that old
readers refuse. Code fields/profiles/provider/native proof remain absent; Code
selection retains every genuine qualifier/no-downgrade requirement.

Participant selection precedes Code discovery/login. Desktop-only never requests
Code OAuth; independent foreground agents retain their separately reviewed
profile/runtime/login. Existing complete both-client records and JSON remain
unchanged. A new compatible kit and actual host acceptance are required; initial
portable packet import, source construction or copied files do not prove loading.
