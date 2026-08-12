---
name: maestro-setup-update
description: Guia conversacional para instalar, atualizar ou reparar o Maestro a partir do ZIP distribuído pelo time BCG Brasil AI. Use sempre que o pedido envolver install, primeira instalação, update, atualização, upgrade, reparo, recuperação, "voltar versão" ou rollback do Maestro.
---

# Maestro Setup and Update

Guia conversacional para três desfechos: primeira instalação, atualização, reparo. O trabalho mecânico é sempre a extração de um ZIP pelo próprio usuário. Esta skill orienta, verifica e diagnostica, não executa instalador. Nunca peça terminal, edit de arquivo, script ou permissão.

## Interaction profile

Resolver `interaction-profile` se disponível. Ajustar vocabulário e ritmo, jamais o envelope de segurança (uma pergunta por vez, sem terminal, sem edit manual).

## Contrato de comunicação

- Uma pergunta por vez.
- Sem "você", "tu" ou "te". Preferir impessoal ou 3ª pessoa.
- Sem em-dash ("—") em texto externo. Usar vírgula, dois pontos ou parênteses.
- Sem jargão de shell, JSON ou chmod.
- Se o usuário pedir "voltar versão", ser transparente: o Maestro atual não faz rollback automático. Se o ZIP anterior foi guardado, extrair por cima da pasta resolve. Caso contrário, pedir o link ao time BCG Brasil AI.
- Nunca mencionar `bcgos`, `bcgos doctor`, `bcgos update` ou qualquer binário de instalador. Esse caminho foi encerrado.

## Roteamento inicial

Perguntar apenas qual desfecho o usuário quer, em uma frase:

> "É primeira instalação, atualização ou reparo?"

Se o pedido for rollback, tratar como caso de "reparo com ZIP anterior" (ver seção Rollback abaixo).

## Fluxo: primeira instalação

1. **Confirmar posição.** Verificar se `${CLAUDE_PROJECT_DIR}/VERSION` existe.
   - Se existir, informar: "a pasta Maestro já está aberta aqui, versão v<X.Y.Z>. Podemos seguir com o onboarding."
   - Se não existir, orientar: "abra no Claude Code a pasta Maestro que foi extraída do ZIP. Feche esta janela e reabra pela pasta correta."

2. **Confirmar workspace inicial.** Verificar se `data/` e `data/.initialized` existem.
   - Se ausentes, orientar: "feche a pasta no Claude Code e reabra. Na próxima abertura o Maestro cria a workspace pessoal automaticamente."
   - Se presentes, seguir.

3. **Delegar identidade.** Encaminhar para a skill `maestro-onboarding` sem duplicar o trabalho dela. Frase-ponte sugerida: "com a pasta pronta, vamos à apresentação e captura de identidade. Ativando o onboarding."

## Fluxo: atualização

Contexto: o time BCG Brasil AI envia um email com o link do ZIP novo. O usuário baixa, extrai por cima da pasta Maestro atual (mantendo `data/`), reabre no Claude Code. Esta skill entra depois disso, para verificar.

1. **Perguntar a versão esperada.** Uma frase apenas: "qual versão o email do time BCG Brasil AI pediu para instalar?"

2. **Ler `VERSION` local.** Comparar com a versão informada.
   - **Match:** "instalado v<X.Y.Z>, igual à versão do email. Atualização concluída. Sua workspace `data/` foi preservada."
   - **Mismatch (local abaixo do esperado):** orientar sequência sem terminal.
     1. Fechar o Claude Code inteiro.
     2. Extrair o ZIP novo por cima da pasta Maestro. Todos os arquivos do ZIP substituem os existentes. A pasta `data/` fica intacta porque não está no ZIP.
     3. Reabrir a pasta Maestro no Claude Code.
     4. Perguntar de novo o status: "quando reabrir, é só dizer 'confere versão' que faço a verificação."
   - **Mismatch (local acima do esperado):** raro, mas possível. Informar: "a versão instalada é mais nova que a informada. Confirme com o time BCG Brasil AI qual é a versão correta antes de qualquer ação."

3. **Sanidade pós-atualização.** Se surgir dúvida (arquivo faltando, hook não roda), delegar para `maestro-doctor` e seguir a prescrição dele.

4. **Fechar o ciclo (obrigatório).** Ao concluir a verificação (match ou orientação de reextração aceita), reconciliar os marcadores em `data/`:
   - Ler `${CLAUDE_PROJECT_DIR}/VERSION` (versão em execução) e `${CLAUDE_PROJECT_DIR}/data/.maestro-version` (versão instalada anteriormente).
   - Se diferentes e a verificação confirmou o novo ZIP no lugar, atualizar `data/.maestro-version` para o novo valor via Write ou Edit.
   - Se existir `data/.upgrade-pending`, apagar o arquivo. Ele foi escrito pelo hook `first-run-scaffold.sh` e serviu de gatilho; sem essa limpeza o SessionStart repete o alerta.
   - Se a migração falhou (não conseguiu extrair, VERSION continua diferente), preservar o marcador e informar honestamente que o upgrade não fechou.

## Migração incremental de schema

O bundle carrega dois marcadores de schema em `data/`:

- `data/.maestro-version` — versão do bundle instalado.
- `data/memory/.schema-version` — schema efetivo da árvore de memória.

Quando um upgrade muda o schema de memória, o release notes do time BCG Brasil AI indica explicitamente. Nessa situação, além do fluxo de atualização acima:

1. Confirmar que o release notes menciona mudança de schema de memória.
2. Delegar para `dream-memory` a validação — a skill lê `data/memory/.schema-version` e recusa qualquer escrita se o schema esperado não bater. Não migrar manualmente.
3. Se o schema exigir atualização, o release notes explicita o novo valor. Só então atualizar `data/memory/.schema-version` via Edit para o valor indicado. Sem release notes explícito, não tocar.

## Fluxo: reparo

1. **Delegar diagnóstico.** Ativar `maestro-doctor`. Aguardar o veredicto de uma linha e a lista de pontos.

2. **Mapear cada achado à ação certa.** O `maestro-doctor` reporta em linguagem simples; a tabela mental abaixo traduz cada caso para a orientação ao usuário.

   - **Arquivos core ausentes** (`VERSION`, `CLAUDE.md`, `.claude/`, `bundles/`): "a instalação está incompleta. Baixe o ZIP mais recente indicado no último email do time BCG Brasil AI e extraia por cima da pasta atual. A workspace `data/` é preservada."
   - **`data/` ausente, core presente:** "feche o Claude Code e reabra a pasta Maestro. Na próxima abertura a workspace é recriada automaticamente."
   - **Hooks presentes mas sem permissão de execução (Mac/Linux):** "reextraia o ZIP por cima da pasta Maestro. A extração restaura as permissões corretas."
   - **`data/` corrompida ou com conteúdo perdido:** ser direto. "o Maestro não guarda backup automático da sua workspace. Se há uma cópia manual (Time Machine, backup em nuvem pessoal, cópia do OneDrive), restaure por cima da `data/` atual. Sem backup, o conteúdo perdido não é recuperável pelo Maestro."
   - **`VERSION` presente mas fora do formato `X.Y.Z`:** tratar como install corrompido, orientar reextração.

3. **Confirmar recuperação.** Após qualquer ação, sugerir rodar `maestro-doctor` de novo para confirmar veredicto "Tudo funcionando".

## Rollback

Não há rollback automático. Se o usuário pediu para voltar a uma versão anterior:

1. Perguntar: "o ZIP da versão anterior foi guardado localmente?"
2. **Sim:** orientar a mesma sequência do fluxo de atualização, usando o ZIP antigo no lugar do novo. Fechar Claude Code, extrair por cima, reabrir. A `data/` é preservada.
3. **Não:** informar honestamente que o Maestro atual não faz rollback automático e sugerir pedir o link do ZIP anterior ao time BCG Brasil AI pelo canal oficial.

## O que esta skill nunca faz

- Não sugere abrir terminal, rodar script ou editar JSON.
- Não invoca `bcgos` nem qualquer binário de instalador (esse caminho foi encerrado).
- Não promete rollback automático.
- Não toca em `data/`. Essa pasta pertence ao usuário.
- Não repete o trabalho de `maestro-onboarding` (identidade) nem de `maestro-doctor` (diagnóstico). Delega.

## Encerramento

Ao terminar, resumir em uma linha o desfecho, a versão ativa (lida em `VERSION`) e o caminho absoluto da workspace (`${CLAUDE_PROJECT_DIR}/data/`). Se houver ação pendente do lado do time BCG Brasil AI (email com link, versão a confirmar), deixar explícito.
