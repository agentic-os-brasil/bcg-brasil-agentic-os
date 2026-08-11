---
name: bcgos-operator
description: Operate the installed Maestro/BCGOS control plane for setup, health, version, update and exceptional recovery. Use when Maestro must inspect or repair its own runtime. Do not use the CLI as the normal workflow for professional work, onboarding, agents, memory, ingestion or continuity; route those intents to the runtime agent, governed skills and canonical workspace artifacts.
---

# BCGOS Operator

Use this method as Maestro's installed control-plane manual. The runtime agent
owns normal work; `bcgos` supports setup, health, version, update and bounded
recovery. Keep mechanics silent unless the owner requests a technical
explanation or must make a consequential choice.

Resolve the canonical `interaction-profile` before presenting a user-facing
result. It changes explanation depth, never commands, access or authority.

## Resolve the exact installed CLI

Use the absolute Maestro CLI path and active workspace root injected by
SessionStart. Never assume `bcgos` is on `PATH`, execute a placeholder
literally, substitute another checkout's binary or cross the active workspace.
Replace `<maestro-cli>` and `<workspace>` below with those exact values.

## Runtime owns normal work

Do not use BCGOS commands as the first route for ordinary professional work.
Use the runtime agent and the installed domain skill:

| Owner intent | Normal route | Canonical working surface |
|---|---|---|
| Start or refine onboarding | `/maestro-onboarding` | reviewed local owner context |
| Configure agent presentation | `/agent-identity-setup` | managed identity profile |
| Create, pause or resume work | `/execution-continuity` | `brain/tasks/` linked to project and deliverable artifacts |
| Use memory | `/dream-memory` | governed generated memory layers |
| Find prior work or SharePoint context | `/find-prior-work` | authorized source pointers |
| Ingest approved local material | `/ingest-content` | verified bounded content pack |
| Execute case work | the relevant Case skill | `brain/projects/`, `brain/decisions/`, `brain/deliverables/` and `brain/sources/` |

Do not ask the owner for JSON envelopes, run IDs, revisions or command
sequences. Compatibility commands may remain callable for an older workspace
or typed recovery, but they are not the product workflow.

## Inspect before acting

Start with the narrowest public control-plane surface that answers the
question:

```text
<maestro-cli> status <workspace>
<maestro-cli> doctor <workspace>
<maestro-cli> setup status --workspace <workspace>
<maestro-cli> version
<maestro-cli> update --check
```

Do not repair, reinstall or rewrite configuration merely because evidence
telemetry is pending. A working beta capability remains usable while
qualification is tracked separately.

## Control-plane intent map

| Owner intent | First route | Follow-up |
|---|---|---|
| Check workspace or runtime health | `status <workspace>` then `doctor <workspace>` | report the typed state and smallest next action |
| Inspect or repair setup | `setup status --workspace <workspace>` | `/maestro-setup-update`; preserve managed roots and workspace data |
| Confirm installed version | `version` | compare only with a verified release source |
| Check or apply an update | `update --check` | `/maestro-setup-update`; preserve confirmation, last-known-good and rollback |
| Recover an exceptional failure | inspect the typed failing state first | use only the recovery action named by the installed method or error |

Do not expose hidden compatibility commands through help exploration or trial
and error. A domain skill may use an internal command silently when its current
contract requires it; that does not make the command the owner-facing flow.

## Interpret state without disabling working features

- `ready`, `installed` or `operational_beta`: continue with the requested work.
- `action_required` or `partial`: use the reason and bounded next action; do not
  suppress unrelated safe work.
- `unavailable`: treat as a real absence for that capability. Optional absence
  does not disable the rest of Maestro.
- `conflict` or `error`: diagnose the named boundary and recover deliberately.
- `denied`: never bypass the guard. Ask only for the missing consequential
  confirmation or authority.
- `native_qualified`: evidence, not a prerequisite for an already operational
  controlled-beta capability.

## Verify the outcome

After setup, repair or update, re-run the narrow status surface that owns the
changed state. For a runtime-wide change, finish with:

```text
<maestro-cli> status <workspace>
<maestro-cli> doctor <workspace>
<maestro-cli> version
```

Report the outcome, useful next step and supporting evidence. Never claim
installed, active, updated or recovered from a command exit alone when a state
inspection exists.

## Recover without guessing

1. Read the typed error and its stated next safe action.
2. Re-inspect the specific control-plane state.
3. Preserve the workspace, managed roots, receipts and last-known-good release.
4. Retry once only after the cause changed.
5. If the boundary remains, explain the concrete blocker and minimum owner
   choice needed. Do not loop, broaden access or replace managed files.

Never expose credentials, raw client content, receipts or trust material. Do
not install global Python packages or bypass runtime-native permissions.
