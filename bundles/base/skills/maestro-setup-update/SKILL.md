---
name: maestro-setup-update
description: Guide a non-technical Maestro user through setup, authentication, update, verification or recovery with one plain-language confirmation and fail-closed release boundaries. Use whenever the user asks to install, configure, update, repair or roll back Maestro.
---

# Maestro Setup and Update

Help the user reach a safe outcome through natural conversation. Do not turn
the interaction into a terminal tutorial.

## Interaction profile

Resolve the canonical `interaction-profile` before starting. Use it to control
technical detail and pacing, never to weaken authentication, confirmation,
signature, data-separation or acceptance requirements.

## Workflow

1. Ask only which outcome the user wants: first setup, update, repair or
   rollback. Infer the operating system from the runtime when possible.
2. Run `bcgos doctor` and `bcgos status` on the user's behalf. Summarize the
   result as ready, action needed or unavailable. Do not dump raw diagnostics
   unless the active profile asks for them.
3. For first setup or update, inspect `bcgos auth status`.
   - If authentication is unavailable, explain that Maestro is waiting for the
     approved company credential channel. Stop safely; never suggest a token,
     environment variable, credential file, `gh`, source clone or unsigned
     package.
   - If login is required, run `bcgos auth login`, show the approved browser
     address and short user code, and wait for completion.
4. On first setup, run `bcgos agent interview`. Explain each principal agent,
   show the suggested names and emoji-avatars, and ask the owner to choose or
   customize them. Explain that ownership and personalization are separate
   from authority. Persist only after explicit confirmation with
   `bcgos agent personalize --stdin`; malformed, unconfirmed or cross-scope
   profiles fail closed.
5. Run `bcgos update --check`. Explain the installed and proposed versions,
   whether CLI and bundle both change, and whether a migration is required.
6. If an update is available, ask one short confirmation naming the exact
   target version and impact. Do not confirm on the user's behalf and do not
   reuse confirmation for a different plan ID.
7. After confirmation, run `bcgos update --confirm <plan-id>`. Let the stable
   bootstrapper wait for the CLI to exit, activate and self-check. Do not try to
   replace the running executable directly.
8. Run `bcgos status` and `bcgos doctor` again. Report the active versions and
   whether rollback remains available.
9. If activation fails, explain that the last-known-good version was restored.
   Offer explicit rollback only when a valid previous state exists.

## Communication contract

- Lead with what the user can safely do now.
- Translate `unavailable` into the missing company approval or capability;
  never present it as the user's fault.
- Use one confirmation immediately before an update or rollback.
- Never call an unsigned candidate a release or an isolated CI run a corporate
  device acceptance.
- Never expose credential, device or release-signing material.

## Completion

Return the requested outcome, active CLI and bundle versions, authentication
state, rollback availability and any release-environment approvals still
missing. Keep engineering evidence, corporate-device acceptance and pilot
readiness visibly distinct.
