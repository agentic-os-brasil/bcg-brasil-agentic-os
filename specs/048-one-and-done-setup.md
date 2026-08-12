# Spec 048 - One-and-done Maestro setup

Status: accepted for implementation.

## Objective

Make Maestro behave as a Chief of Staff for owners who should not need to
understand commands, diagnostics or dependency topology. One explicit setup
authorization in the installer or first runtime session prepares the local Maestro surface,
persists the bounded authority and lets later sessions diagnose, retry, repair
and resume without asking the owner to approve every implementation step.

The experience is not invisible. Maestro shows concise progress and one final
summary, but command names, intermediate validation gates and reversible repair
decisions remain implementation details.

## Durable setup authorization

The first setup authorization is a strict local receipt bound to:

- the canonical initialized workspace ID and path;
- an opaque digest of the current OS principal and device;
- the setup policy version and fixed action-class allowlist;
- the exact selected-source fingerprint, or `none` when no source is selected;
- issuance and expiry timestamps.

The allowlist covers workspace initialization, signed managed-component
activation, runtime adapter installation, runtime projection repair,
read-only diagnostics, local reversible repair, retry, rollback/recovery and a
bounded derived projection from the exact selected-source fingerprint. The
receipt contains no source URLs, client content, credentials or raw principal
name. It is stored below the protected owner-local data root and is safe to
inspect from Session Start as a bounded status projection.

A valid receipt authorizes idempotent continuation. Internal CLI operations may
still use machine-facing `--confirm` switches to prove that the orchestrator
crossed the grant boundary; the runtime must not turn each switch into another
owner-facing question.

## Boundaries that require a new decision

The setup authorization does not cover:

- a different workspace, OS principal or device;
- a changed SharePoint folder fingerprint or a new client/tenant;
- expired authorization or a changed setup policy;
- account or tenant selection, MFA/SSO and administrator approval;
- privilege elevation, credential entry, signing-key use or secret custody;
- publication, external mutation, destructive work or work without bounded
  rollback;
- components outside the signed managed allowlist.

Those cases are consolidated into one clear owner or administrator action. An
optional external capability never degrades unrelated local setup.

The binding controls reuse of setup automation; it is not a license to stop
ordinary work. If the receipt is missing, stale or bound to another local
identity, Maestro continues safe local work through the host runtime's normal
permissions, records the mismatch and offers to tighten the receipt later. The
product prefers observable progress plus recovery over preventive ceremony.
Only external mutation, tenant/client change, privilege, secrets, destruction
or an operation without bounded recovery is a hard stop.

On Windows, every mutating entry point performs a non-elevated-token preflight
before the first write. A state file or private data root created under
`BUILTIN\\Administrators` cannot later satisfy the current-user ownership
contract, so the product refuses `Run as administrator` instead of attempting
an implicit ownership transfer. Existing mismatched state remains untouched and
is reported as a bounded repair condition. Native PowerShell/cmd or the visual
installer are the supported invocation paths; MSYS/Git Bash path translation is
not used as installation evidence.

## Setup result

One setup run returns one of three states:

- `complete`: all applicable local stages are ready;
- `complete_with_external_actions_pending`: local setup is ready and one or
  more optional corporate/runtime capabilities remain unavailable;
- `blocked`: a required local invariant failed, rollback failed or the requested
  action is outside the authorization.

Every stage is idempotent and reports `completed`, `repaired`, `already_ready`,
`pending_external` or `blocked`. Release/update authorization is reported only
under the optional release-update stage. It must never be represented as the
trust anchor for SharePoint enrollment or collection.

## Installer and runtime behavior

The installer owns the first local setup transaction and persists its receipt
before handing off to Claude or Codex. A runtime entering an initialized
workspace reads the bounded status and:

1. continues or repairs allowlisted local setup without asking permission;
2. does not rerun user-visible `status`, `doctor`, `init` or adapter approval
   sequences;
3. does not ask again to read an unchanged selected SharePoint scope covered by
   the authorization;
4. reports unavailable corporate capabilities once as external actions while
   remaining useful without them; and
5. never infers that private release authorization unlocks SharePoint.

The owner profile review remains explicit because it approves identity and
working-style content. It is independent from technical setup consent: the
installer may capture setup authorization first when its confirmation screen
plainly describes the bounded local preparation. Neither confirmation implies
the other, and technical setup still has only one owner-facing authorization.

## Acceptance evidence

Contract tests must prove:

1. one confirmation creates a grant and the same local actor can resume without
   another confirmation;
2. another workspace, principal, device, source fingerprint, expired receipt or
   policy version cannot reuse the grant, but the mismatch does not block
   unrelated ordinary local work;
3. optional unavailable capabilities remain confirmable and finish as pending
   external actions;
4. required local blockers still prevent mutation;
5. Session Start contains no instruction to ask for command execution,
   diagnostics or a second unchanged SharePoint read authorization;
6. setup guidance never links `private_release_auth` to prior-work enrollment;
7. receipts and projections contain no source URL, client body, credential or
   raw principal value; and
8. interrupted local work is retried or rolled back before a terminal state is
   reported.
