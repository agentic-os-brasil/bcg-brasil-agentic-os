# Spec 022 - Guided pilot release

Status: accepted pilot gate; production authorities and device evidence remain
pending until recorded.

## Objective

Make natural-language guidance the primary setup and update experience for a
non-technical pilot user, while deterministic CLI and bootstrapper contracts
retain control of authentication, verification, activation and rollback.

## Guided experience

The `maestro-setup-update` skill resolves the interaction profile, diagnoses
the current state, runs authentication and update commands for the user,
explains impact, asks one short confirmation bound to the exact update plan,
and verifies the resulting state.

The skill cannot:

- invent or store a credential;
- approve its own update;
- install an unsigned candidate;
- substitute a source clone or developer tool;
- bypass unavailable native storage, provider or signing authorities;
- describe isolated CI as corporate-device acceptance or pilot readiness.

Decision `SHHK` permits a separate factory-built script-only profile under
Spec 053. It requires no developer tool on the endpoint, but it is intentionally
not a substitution for native CLI authority or native feature parity. The
guided skill must read its capability matrix, may rely on the explicitly
projected text-hook subset and must never claim unavailable native controls are
active.

Decisions `CARY` and `DZIP` define one narrower pre-pilot exception outside the
signed guided-pilot path: factory-built target-specific Windows amd64 and
macOS arm64 local-beta ZIPs may perform a real user-space installation for a
bounded Canary cohort while organization-owned native signing is unavailable.
Each remains bound to the exact test-only authority, registry and native
bootstrapper hashes and an authenticated `canary` manifest. The profile is
never enabled by a runtime flag, never promoted by the setup/update skill and
never counts as an authority, clean-device, two-user or pilot gate. Windows
accepts exactly `NotSigned`; macOS accepts exactly no code-signature load
command plus an agreeing native unsigned classification when built on macOS.
Ad-hoc, invalid or partially signed artifacts do not enter the unsigned
profile.

## Evidence classes

`internal/dev/pilotacceptance` recognizes two non-interchangeable report modes:

- `isolated_ci` proves engineering behavior on disposable Windows/macOS
  runners and may claim only `engineering_evidence_only`;
- `corporate_device` requires install, update and rollback passes plus an
  operator, hashed device identifier, strict policy identifier, approved
  channel and release bindings. It may claim only
  `corporate_device_operator_attestation`; corporate acceptance requires an
  approved external countersignature.

Corporate evidence uses schema-v2 reports assembled from three strict,
sanitized phase receipts. The receipts must prove the ordered transition
`none -> baseline -> update -> baseline`, bind the exact baseline and update
manifest SHA-256 values and preserve each receipt's own SHA-256 in the final
report. The report also binds both immutable provider release IDs/tags, the
native bootstrapper digest, authority-registry digest, approved native signer,
update activation receipt and accepted support owner. Unknown fields,
duplicate JSON keys, non-exact phase checks or mixed run/device/platform
identities fail closed.

A report is evidence, not automatic permission to publish or expand a pilot.

## Pilot gates

1. **Engineering gate** - full harness, release verification, transactional
   update/rollback and isolated trial installation pass on Windows and macOS.
2. **Authority gate** - production release key, Windows/macOS native signing,
   native credential stores, private provider, support owner and incident path
   are approved.
3. **Clean-device gate** - one managed Windows and one managed macOS device
   each pass first install, update and rollback with valid operator
   attestations that the approved external evidence owner countersigns.
4. **Two-user canary** - one Windows and one macOS user complete the guided
   flow without Git or developer dependencies. Observe for five business days.
5. **Ten-person pilot** - expand only after both canary users succeed, there is
   no severity-1/2 incident or data-boundary breach, rollback works, and the
   support owner accepts the observed demand.

The ten-person cohort balances Windows/macOS and classic/technical users.
Expansion, redesign or stop is a human release decision.
