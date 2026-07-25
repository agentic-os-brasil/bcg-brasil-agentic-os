# Spec 021 - Pilot hook conformance

Status: acceptance protocol implemented; native runtime receipts pending.

Maestro may install a bounded Session Start command in a workspace, but that
configuration alone is never evidence that Claude or Codex trusted, invoked or
rendered the hook. Before a runtime-specific lifecycle capability is promoted
from `unavailable`, the pilot must record one receipt for each supported
runtime and platform.

## Invariant

The installed command:

1. points to the released local `bcgos` executable, never a shell PATH lookup;
2. returns within two seconds from a last-committed, pointer-only snapshot;
3. does not read a pointed source, wait for a worker, call a model or use the
   network;
4. emits an explicit bounded omission rather than failing the agent session if
   the packet would exceed the output limit; and
5. remains removable without affecting unrelated runtime configuration.

If a target runtime configuration is already tracked by Git, Maestro refuses
the installation before changing it. A Git exclusion is preventative only; it
does not make a tracked machine-local path safe.

## Receipt required per runtime and platform

The pilot operator records, without client content or personal source bodies:

- Maestro release version, runtime version and operating-system version;
- workspace-local configuration path and the fact that it contains exactly one
  Maestro-owned Session Start command with a two-second timeout;
- successful direct invocation of that exact command;
- observation from the native runtime that a fresh session invoked the command
  and surfaced its bounded context (or the runtime's explicit failure); and
- adapter removal and confirmation that unrelated configuration remains.

The direct command proves Maestro's payload. The fresh native session proves
the runtime integration. Both are necessary. A failed or missing receipt keeps
the capability `unavailable`; it is not emulated by documentation or by a
development hook.

## Non-goals

This protocol does not enable memory ingestion, worker wake-up, post-action
observation, stop finalization or pre-action enforcement. Those hooks require
their own semantic contracts and conformance evidence.
