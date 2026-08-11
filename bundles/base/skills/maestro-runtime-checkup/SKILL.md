---
name: maestro-runtime-checkup
description: Check and quietly repair a Maestro workspace's local runtime wiring. Use when a session does not feel connected, hooks or onboarding appear incomplete, Darwin maintenance needs attention, or an owner asks whether Maestro is ready to work.
---

# Maestro Runtime Checkup

Run a quiet, bounded health pass. This skill repairs normal local wiring; it does not turn temporary diagnostics into a reason to stop the owner from working.

Resolve `interaction-profile` before explaining the result. It controls only
the detail and pacing of the conversation, never what Maestro inspects or
repairs.

## Check and reconcile

1. Resolve the active workspace and exact installed Maestro CLI path from SessionStart. Use `$maestro-environment-setup` if the workspace has never been prepared.
2. Inspect workspace and runtime status. If the projection or hook wiring is missing, run the same idempotent `setup apply` consolidation that first-run uses. Do not ask the owner to copy commands or identify hook files.
3. Verify the result through the owning status surfaces. Re-run once only when the local cause changed. Preserve the workspace, local context and prior receipts; never replace a managed root just to make a check appear green.
4. On macOS, inspect Darwin's user-level LaunchAgent binding. The visual installer owns enrollment and repair of that schedule; if it needs attention, hand off to `$maestro-setup-update` rather than rewriting it from a runtime conversation. If a different managed schedule is already bound, explain that Maestro needs a choice rather than overwriting it. On other systems, report only the useful maintenance state and leave ordinary work available.
5. Check for a verified MarkItDown runtime pack. Until a versioned managed pack ships, describe advanced document reading as an optional future addition. Never install it from `pip`, a package manager or a copied command. Its absence affects document conversion only, never the rest of the workspace.

## Explain the result

Use a short result with four human categories:

- **Pronto para trabalhar** — the workspace and normal runtime route are set.
- **Ajustado** — Maestro repaired a local integration and preserved the work.
- **Um detalhe para concluir depois** — an optional component or background schedule is not ready; explain its owner-visible impact only.
- **Preciso de uma escolha sua** — the action would change another workspace, replace a bound scheduler, use credentials, publish externally or remove data.

Never present raw receipts, command output, absolute paths, internal state labels or diagnostics unless the owner explicitly asks for a technical view.
