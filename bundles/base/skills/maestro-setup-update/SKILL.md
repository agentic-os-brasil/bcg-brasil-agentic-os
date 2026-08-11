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

## Product posture

Bias toward completing the owner's work and tightening the system along the
way. Missing optional evidence, stale local receipts or incomplete diagnostics
trigger silent inspection, repair and a concise warning; they do not become a
global blocker. If a setup grant cannot be reused, continue ordinary local work
through the host runtime's normal permissions and renew the grant when useful.
Hard stops are reserved for external mutation, tenant/client change, privilege,
secrets, destruction or actions without bounded recovery.

## Windows installer boundary

For a first installation on Windows, the recommended user-facing entrypoint
is the single `Maestro-Installer-<version>-windows-amd64.exe` produced by the
self-contained factory. It carries the complete validated package internally,
extracts it into a temporary user directory and opens the same visual bridge.
The factory/debug form remains a transportable folder or archive and must
carry these sibling inputs together:

- `wizard/`;
- `release/` with the exact signed release set;
- `authority-registry.json`; and
- exactly one versioned `bcgos-bootstrap_<version>_windows_amd64.exe`.

The file named `bcgos_<version>_windows_amd64.exe` is the installed runtime
CLI, not an installer. Never offer it as a first-install download, never tell
the user to double-click it, and never fall back to it when the visual
installer package is incomplete. If `maestro-installer.exe` is missing any
required sibling input, report `installer_package_incomplete` and stop before
asking for credentials, changing a managed root or suggesting a raw CLI
command.

The `maestro-setup-update` skill is part of the signed base bundle and is
installed by the visual installer; it is not distributed as a separate skill
file. A successful single-file handoff therefore means “double-click the
Maestro Installer `.exe`”; for the debug package, open
`maestro-installer.exe`. In both cases, the CLI becomes available only after
the installer has completed and its final self-check has passed.

## Workflow

1. Ask only which outcome the user wants: first setup, update, repair or
   rollback. Infer the operating system from the runtime when possible.
2. For first setup, use the installer receipt and `bcgos setup status
   --workspace <workspace>`. If one-and-done authorization is not active, ask
   one plain-language question covering all local, allowlisted, idempotent and
   reversible preparation, diagnostics, repair, retry and recovery for this
   workspace. State in the same question that external, privileged,
   destructive, secret-bearing and cross-tenant actions remain outside it.
3. After that single agreement, run `bcgos setup authorize --workspace
   <workspace> --confirm` and complete local setup silently. Run any required
   `status`, `doctor`, adapter, verification, maintenance, retry or reversible
   repair commands on the user's behalf without asking again. Show concise
   progress and one outcome summary; do not expose the command sequence.
   Once the installer and its local preparation complete, hand ordinary
   workspace readiness and later quiet repairs to `$maestro-environment-setup`
   and `$maestro-runtime-checkup`; they do not replace this installer/update
   flow.
4. Treat optional unavailable capabilities as
   `complete_with_external_actions_pending`. Keep all unrelated Maestro
   capabilities useful and consolidate true administrator actions into one
   notice. `private_release_auth` governs private distribution and updates; it
   is never a SharePoint enrollment or collection trust anchor.
5. On first setup, run `bcgos agent interview`. Explain each principal agent,
   show the suggested names and emoji-avatars, and ask the owner to choose or
   customize them. Explain that ownership and personalization are separate
   from authority. Persist only after explicit confirmation with
   `bcgos agent personalize draft --stdin --consent --no-client-data`, followed
   by review and digest-bound confirm; malformed, unconfirmed or cross-scope
   profiles fail closed. Ask only the returned `next_question`; stage the
   strict profile with `bcgos agent personalize draft --stdin --consent
   --no-client-data`, show `review --id <id>`, and apply only `confirm --id
   <id> --digest <sha256> --confirm` after the owner reviews it. The input
   must use the canonical `selections[]` envelope returned by
   `bcgos agent interview`; do not send interview labels such as `agent_names`,
   `agent_emojis`, `scope` or a top-level `ownership_scope`.
6. For an update only, inspect `bcgos auth status`. If authentication is
   unavailable, report the approved company release channel as an external
   action pending; do not degrade local setup, onboarding or SharePoint source
   selection. Never suggest a token, environment variable, credential file,
   `gh`, source clone or unsigned package. If login is required, run `bcgos
   auth login`, show the approved browser address and short user code, and wait.
7. Run `bcgos update --check`. Explain the installed and proposed versions,
   whether CLI and bundle both change, and whether a migration is required.
8. If an update is available, ask one short confirmation naming the exact
   target version and impact. Do not confirm on the user's behalf and do not
   reuse confirmation for a different plan ID.
9. After confirmation, run `bcgos update --confirm <plan-id>`. Let the stable
   bootstrapper wait for the CLI to exit, activate and self-check. Do not try to
   replace the running executable directly.
10. Run final verification silently. Report the active versions and
   whether rollback remains available.
11. If activation fails, explain that the last-known-good version was restored.
   Offer explicit rollback only when a valid previous state exists.

## Communication contract

- Lead with what the user can safely do now.
- Translate `unavailable` into the missing company approval or capability;
  never present it as the user's fault.
- Use one setup authorization, then no repeated confirmation for local
  diagnostics or reversible repair. Updates and rollbacks retain their exact
  plan-bound confirmation because they change installed release state.
- Never call an unsigned candidate a release or an isolated CI run a corporate
  device acceptance.
- Never expose credential, device or release-signing material.

### Standard-user language

For the `standard` profile, make the experience welcoming to a non-technical
adult who may be installing software during a busy workday. Use Brazilian
Portuguese unless the user clearly chooses another language. Explain one safe
next step at a time and keep engineering detail behind the scenes.

- Start with a one-line outcome: `✅ Vou preparar sua instalação.` or
  `🔄 Vou atualizar o Maestro e conferir se tudo ficou bem.`
- Use emojis as wayfinding, not decoration: normally one emoji per step or
  warning, never an emoji wall.
- Explain technical terms with adult analogies on first use:
  - release/signature: `📦 uma caixa lacrada; a assinatura é o selo que prova
    que ninguém trocou o conteúdo`;
  - provider: `🏦 o cofre corporativo de onde a versão aprovada é retirada`;
  - update plan: `🧾 uma ordem de serviço com a versão exata e o que vai mudar`;
  - rollback: `↩️ o botão de desfazer que volta à última versão boa`;
  - workspace: `🗂️ sua pasta de trabalho, que não deve ser mexida pela troca
    do programa`.
- Prefer adult, respectful analogies such as a bank transfer, a sealed
  package, a backup copy or an approved building pass. Do not use baby talk,
  childish diminutives, jokes about the user's technical ability or language
  that implies the person is at fault.
- Use this compact shape when useful:
  1. `✅ O que vou fazer` — one sentence;
  2. `🔎 O que estou conferindo` — signature, version and compatibility in
     plain language;
  3. `👍 O que preciso de você` — one short confirmation only when required;
  4. `🛟 Se a ativação falhar` — explain that, when a last-known-good version
     exists, the system attempts to restore it, then confirms the result and
     says when rollback is unavailable.
- Before confirmation, say the impact in human terms: `Atualizar da versão
  X para Y troca apenas o programa, mantém sua pasta de trabalho e permite
  voltar à versão anterior. Posso prosseguir?` Do not ask the user to repeat a
  plan ID unless the deterministic command requires it; the runtime still
  binds the confirmation to the exact plan.
- When a capability is unavailable, say what is waiting and the safe next
  action: `⚠️ A instalação está pronta, mas falta a aprovação do canal da
  empresa. Nada será instalado até ela existir.` Never show raw stack traces,
  provider URLs, tokens, filesystem paths or shell diagnostics by default.

## Completion

Return the requested outcome, active CLI and bundle versions, authentication
state, rollback availability and any release-environment approvals still
missing. Keep engineering evidence, corporate-device acceptance and pilot
readiness visibly distinct.
