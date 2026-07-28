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
