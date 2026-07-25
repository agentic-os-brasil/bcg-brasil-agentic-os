# Clean-device operator evidence

These operator scripts produce three sanitized, non-interchangeable receipts
for one managed Windows or macOS device: `install`, `update` and `rollback`.
The resulting report is an unauthenticated operator attestation. It does not
publish a release, approve an installer, prove corporate acceptance or decide
pilot readiness. An approved external countersignature or evidence system is
still required before the report can be treated as corporate acceptance.

## Preconditions

- The approved OS installation channel has placed the native-signed stable
  bootstrapper at the protected `managed-root/` path and the exact public authority
  registry at `managed-root/trust/`.
- The signed release has been downloaded through the authenticated provider.
- The operator has the immutable provider release ID, release tag, manifest
  SHA-256, approved native signer identity and one-way device identifier.
- A sanitized fixture file exists outside the managed root. Record its SHA-256;
  each phase fails if the file changes. Pass its local workspace root
  separately; the install phase initializes that workspace and all phases
  require `status`, `doctor`, private auth and update capabilities to be ready.
- The user reviewed `bcgos update --check` and explicitly confirmed the exact
  plan ID before the operator runs the update phase.

The scripts require explicit absolute managed/data paths because approved
Windows and macOS application directories remain an external policy decision.
They never write usernames, hostnames, serial numbers, raw paths or logs into a
receipt. The Windows path is amd64-only, matching the v1 release target; macOS
supports Intel and Apple silicon.

## Sequence

Run the platform script three times with one shared run ID:

1. `install` release A on a device with no Maestro CLI or install state,
   passing release A's version and manifest digest;
2. `update` from A to B with the exact separately confirmed plan ID, passing
   release B's version and manifest digest;
3. `rollback` from B to A, passing release A's version and manifest digest.

The report validator requires the exact continuity `none -> A -> B -> A`, the
baseline manifest on install/rollback and the update manifest on update. Do not
rewrite a receipt to make versions or digests match.

Each platform script also requires the phase's provider release ID and tag,
the same device-identity hash and approved signer ID on every invocation. The
update phase reads the provider release ID from the pending plan. Preserve its
activation receipt and pass that file to the rollback phase; both receipts
must bind the same digest.

Validate each receipt:

```text
go run ./dev/pilot-acceptance validate-phase --receipt <receipt.json>
```

After all three receipts refer to one operator evidence identity, an approved
operator creates the report:

```text
go run ./dev/pilot-acceptance corporate \
  --install-receipt install.json \
  --update-receipt update.json \
  --rollback-receipt rollback.json \
  --operator <operator-id> \
  --device-id-hash <one-way-sha256> \
  --policy-id bcg-managed-standard-v1 \
  --channel canary \
  --support-owner <support-owner-id> \
  --output corporate-device-report.json
```

The CLI derives provider releases/tags, manifest digests, bootstrapper,
registry and native-signer bindings from the receipts instead of accepting
independent report flags. Windows and macOS operator attestations are both
required inputs to the external acceptance decision, followed by the two-user
canary and the human release decision.
