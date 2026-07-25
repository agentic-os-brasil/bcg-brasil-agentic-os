# Spec 020 - Guided pilot release

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

## Evidence classes

`internal/dev/pilotacceptance` recognizes two non-interchangeable report modes:

- `isolated_ci` proves engineering behavior on disposable Windows/macOS
  runners and may claim only `engineering_evidence_only`;
- `corporate_device` requires install, update and rollback passes plus an
  operator, hashed device identifier, policy context, approved channel, signed
  manifest, native code signing and authenticated provider. Only it may claim
  `corporate_device_acceptance`.

A report is evidence, not automatic permission to publish or expand a pilot.

## Pilot gates

1. **Engineering gate** - full harness, release verification, transactional
   update/rollback and isolated trial installation pass on Windows and macOS.
2. **Authority gate** - production release key, Windows/macOS native signing,
   native credential stores, private provider, support owner and incident path
   are approved.
3. **Clean-device gate** - one managed Windows and one managed macOS device
   each pass first install, update and rollback with valid corporate reports.
4. **Two-user canary** - one Windows and one macOS user complete the guided
   flow without Git or developer dependencies. Observe for five business days.
5. **Ten-person pilot** - expand only after both canary users succeed, there is
   no severity-1/2 incident or data-boundary breach, rollback works, and the
   support owner accepts the observed demand.

The ten-person cohort balances Windows/macOS and classic/technical users.
Expansion, redesign or stop is a human release decision.
