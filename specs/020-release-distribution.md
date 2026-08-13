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

## Components

- The stable bootstrapper owns installation and activation. It is not
  self-updated by the first release contract.
- The `bcgos` CLI is a platform-specific executable with its own version and
  compatible bundle range.
- The base bundle is one platform-neutral, immutable archive containing only
  reviewed managed product content.
- Optional runtime packs remain a separately versioned future extension.
  Manifest v1 intentionally excludes them until their identity, compatibility
  and migration model is specified. Docling belongs in that future runtime
  pack rather than the thin CLI or base bundle.

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
- at least one CLI artifact and exactly one platform-neutral base bundle;
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
version. V1 has no silent compatibility fallback and no runtime-pack entries.

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

Decision `SHLL` and Spec 053 define a separate non-native exception for endpoints
without a local compiler. It projects only allowlisted managed content and text
scripts, carries an explicit reduced-capability matrix and contains no Maestro
native executable or compiler payload. It is not the release-v1 CLI and cannot
claim native feature parity or publisher authentication.

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
