# Guia do Claude para contribuidores

Voce nao precisa saber Git para contribuir. Diga ao Claude, em linguagem normal, o que quer fazer.

## Primeira vez

Escreva: **"Use start-contributing e me guie passo a passo."**

O Claude vai verificar a instalacao, explicar qualquer problema e instalar os mecanismos de protecao deste clone. Ele deve dar uma unica proxima acao por vez.

Se o repositorio ainda nao foi clonado em um computador Windows, use primeiro o prompt compartilhavel em `docs/onboarding/windows-contributor-prompt.md`. As skills descobertas nativamente pelo Claude em `.claude/skills/` sao projecoes finas; os contratos canonicos continuam em `dev/skills/`.

## Fluxo normal

1. **Comecar:** "Use start-work. Quero alterar ..."
2. **Desenvolver:** o Claude usa `develop-change`, testes e o harness.
3. **Entregar:** "Use prepare-pr para preparar isso para revisao."
4. **Se algo parecer estranho:** "Use recover-work. Estou perdido."

Em termos simples: uma **branch** separa seu trabalho; um **commit** salva um checkpoint; um **push** envia a branch ao GitHub; um **PR** pede revisao humana.

## Regras de seguranca

- O Claude nunca deve apagar seu trabalho para "arrumar" o Git.
- Trabalho nao acontece diretamente em `main`.
- Push e criacao de PR pedem sua confirmacao; merge e sempre humano.
- Segredos, credenciais e possiveis arquivos de cliente nao entram no repositorio.
- Se um hook bloquear algo, ele deve explicar o motivo, confirmar que nada foi perdido e indicar um unico comando seguro.

As politicas canonicas ficam em `AGENTS.md`, `specs/005-development-harness.md` e `dev/skills/`. Esta pasta e apenas a adaptacao do Claude.
