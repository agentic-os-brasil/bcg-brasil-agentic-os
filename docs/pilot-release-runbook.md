# Maestro pilot release runbook

## Current status

The repository can build deterministic unsigned candidates, verify a signed
release set, stage and activate an update, roll back, model private-provider
authentication and generate explicitly isolated Windows/macOS evidence. A
macOS Keychain backend exists in source with conformance coverage, but current
candidates and CLI wiring still report native storage as unavailable.

That is engineering readiness, not a distributable pilot release. Production
signing, native Windows/macOS candidate wiring, provider registration,
production Keychain/Credential Manager approval and managed-device evidence
remain gates outside the repository.

## Gate checklist

### 1. Engineering

- Full development harness passes on Windows and macOS.
- Candidate bytes reproduce from the same source snapshot.
- Bundle contains only `bundles/base/distribution.json` entries.
- Manifest/artifact tampering, extra files and unsafe archives fail closed.
- Install, update, automatic restoration and explicit rollback tests pass.
- Isolated reports say `engineering_evidence_only`.

### 2. Release authorities

- Maestro Ed25519 release key has an approved custodian and registry entry.
- Windows Authenticode and macOS Developer ID/notarization identities are
  approved and applied before artifact hashing/signing.
- GitHub App has browser device flow enabled, read-only Contents access and is
  installed only on the private release repository.
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

The report uses a one-way device identifier hash and an operator ID. It does
not enter the product bundle.

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
