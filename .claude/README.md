# Guia do Claude para contribuidores

Claude Code e a experiencia principal de desenvolvimento deste repositorio. O arquivo `.claude/skill-routing.json` define, de forma verificavel, qual skill nativa o Claude deve usar para cada tipo de trabalho. O harness registra a ativacao das skills por sessao e bloqueia mutacoes sem o workflow correto. O onboarding verifica a versao minima do Claude Code necessaria para esses hooks.

Voce nao precisa saber Git para contribuir. Diga ao Claude, em linguagem normal, o que quer fazer.

## Primeira vez

Escreva: **"Use start-contributing e me guie passo a passo."**

O Claude vai verificar a instalacao, explicar qualquer problema e instalar os mecanismos de protecao deste clone. Ele deve dar uma unica proxima acao por vez.

Se o repositorio ainda nao foi clonado em um computador Windows, use primeiro o prompt compartilhavel em `docs/onboarding/windows-contributor-prompt.md`. As skills descobertas nativamente pelo Claude em `.claude/skills/` sao projecoes finas; os contratos canonicos continuam em `dev/skills/`.

## Fluxo normal

1. **Comecar:** "Use start-work. Quero alterar ..."
2. **Desenvolver:** o Claude usa `develop-change`, testes e o harness.
   Para desenvolver memoria, dreaming, rollups ou injecao de contexto, ele deve usar `evolve-memory`. A skill `dream-memory` pertence ao produto distribuido e nao substitui o workflow de desenvolvimento.
3. **Documentar visualmente:** em mudancas materiais, diga "Use visualize-change para atualizar os READMEs e diagramas Mermaid com o que foi implementado."
4. **Entregar:** "Use prepare-pr para preparar isso para revisao."
5. **Se algo parecer estranho:** "Use recover-work. Estou perdido."

Em termos simples: uma **branch** separa seu trabalho; um **commit** salva um checkpoint; um **push** envia a branch ao GitHub; um **PR** pede revisao humana.

## Regras de seguranca

- O Claude nunca deve apagar seu trabalho para "arrumar" o Git.
- Trabalho nao acontece diretamente em `main`.
- Push e criacao de PR pedem sua confirmacao; merge e sempre humano.
- Segredos, credenciais e possiveis arquivos de cliente nao entram no repositorio.
- Se um hook bloquear algo, ele deve explicar o motivo, confirmar que nada foi perdido e indicar um unico comando seguro.

As politicas canonicas ficam em `AGENTS.md`, `specs/005-development-harness.md` e `dev/skills/`. Esta pasta e a adaptacao nativa e principal do Claude; ela nao duplica a politica canonica.
