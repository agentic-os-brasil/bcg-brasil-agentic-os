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

Trusted operator review selects one source ID/channel/target and local hydrated
folder locator for the real installation. The path supplies transport, not trust.
Later metadata cannot silently rebind that tuple or its independently pinned
registry. No SharePoint destination, endpoint or account is guessed.

Read bounded ordinary no-follow/unaliasable source files and copy the exact
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
