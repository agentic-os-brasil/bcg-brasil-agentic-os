# Maestro pilot release runbook

## Current status

The repository can build deterministic unsigned candidates, verify a signed
release set, stage and activate an update, roll back, model private-provider
authentication and generate explicitly isolated Windows/macOS evidence. A
macOS Keychain backend and a Windows Credential Manager backend exist in
source with conformance coverage. The candidate workflow builds each CLI on a
matching native runner and assembles the release set from those exact binaries,
and CLI auth wiring constructs native storage only behind a complete approved
managed provider registration. The checked-in registration is intentionally
`unavailable`, so no real login is exposed yet. When all three provider,
authority-seed and native-store gates are approved, `bcgos update --check`
persists one exact signed plan and `--confirm` starts the stable bootstrapper
only for that plan; `status` and `doctor` report the same availability
boundary. The stable bootstrapper now prepares its own first-install activation
plan after verifying the pinned registry and complete signed release; it does
not accept a caller-authored plan or managed root. Clean-device operator scripts
for Windows and macOS verify native signatures, registry-seed identity, exact
manifest/plan bindings and a sanitized owner-data sentinel before writing phase
receipts.

That is engineering readiness, not a distributable pilot release. Production
signing, provider registration, production Keychain/Credential Manager
approval and managed-device evidence remain gates outside the repository.

## Gate checklist

### 1. Engineering

- Full development harness passes on Windows and macOS.
- Each candidate CLI is built on its matching native runner before assembly.
- Each native binary records source commit, workflow run, runner image,
  Go/compiler identity, CGO mode, size and SHA-256 provenance.
- Byte-identical native rebuilds across runner-image or toolchain updates are
  not claimed; reproducibility requires a separately pinned toolchain and
  two-run equality evidence.
- Bundle contains only `bundles/base/distribution.json` entries.
- Manifest/artifact tampering, extra files and unsafe archives fail closed.
- Update plans bind the immutable provider release ID and authenticated
  manifest digest before asking for confirmation.
- Pending confirmation and the bootstrapper both revalidate the signed
  release, complete activation semantics and exact staged artifact bytes.
- The CLI emits one JSON document for check, unavailable, authentication,
  current, error and activation-started states; bootstrapper output is isolated
  in owner-data logs.
- Confirmation launches only the fixed regular bootstrapper under the managed
  root and passes the current CLI PID, exact plan ID and owner-data root.
- The bootstrapper resolves trust from its protected managed root, consumes
  only the exact durable plan ID and rejects post-confirmation state changes.
- The registry must match the digest embedded by the approved bootstrapper
  seed; the development build remains unavailable without that digest.
- An activation intent makes pre-State and post-State crash retries idempotent
  only after dead-lock, backup, CLI, full bundle-tree and repeated CLI
  self-check reconciliation.
- Existing schema-v1 install state migrates only with an authoritative managed
  root, preserving update and rollback continuity.
- Install, update, automatic restoration and explicit rollback tests pass.
- Isolated reports say `engineering_evidence_only`.

### 2. Release authorities

- Maestro Ed25519 release key has an approved custodian and registry entry.
- Windows Authenticode and macOS Developer ID/notarization identities are
  approved and applied before artifact hashing/signing.
- GitHub App has browser device flow enabled, read-only Contents access and is
  installed only on the private release repository.
- Managed provider configuration contains the approved public client ID and
  exact selected repository, with no secret or partial-registration fallback.
- Keychain and Windows Credential Manager adapters pass conformance tests.
- Bootstrapper seed has an approved signed installation channel.
- Support owner, incident path and rollback retention are named.

### 3. Corporate clean devices

Use one managed Windows and one managed macOS device with no prior Maestro
state. For each platform:

1. install without Git, Go, Python, Node or Docker;
2. authenticate through the company-approved browser flow;
3. verify first `doctor` and `status`;
4. update to a separately signed version after one confirmation;
5. verify local configuration and a sanitized fixture workspace are unchanged;
6. perform rollback and verify last-known-good state;
7. validate a `corporate_device` report without storing hostnames, usernames,
   serial numbers or other raw device identifiers.

Use `acceptance/clean-device/` to record the ordered transition
`none -> baseline -> update -> baseline`. The final schema-v2 report consumes
the three immutable receipt files, binds both provider release identities and
manifest digests, one native signer, one bootstrapper/registry seed, the update
activation receipt, and the named operator/support owner. It uses a one-way
device identifier hash and never records raw paths, hostnames, usernames,
serial numbers or logs. This is an operator attestation, not authenticated
corporate acceptance: the approved external evidence owner must countersign it
before the clean-device gate can close. Reports and operator scripts do not
enter the product bundle.

## Cohort progression

Start with exactly two users: one Windows, one macOS. Observe for five business
days. Expand to ten only if both complete setup and update, rollback is proven,
there is no severity-1/2 incident or boundary breach, and the support owner
accepts the workload.

For the ten-person cohort, balance operating system and classic/technical
profiles. Track time-to-first-success, update success, rollback use, support
contacts and sanitized failure categories. Do not add telemetry that captures
client, conversation, memory or workspace content.

## Stop conditions

Stop distribution immediately for signature/key mismatch, native-signing
warning, credential persistence outside the native store, unexpected bundle
content, workspace/config mutation, failed restoration or a severity-1/2
incident. Preserve evidence and last-known-good artifacts; do not force-update
through a broken trust path.
