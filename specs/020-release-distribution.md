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
