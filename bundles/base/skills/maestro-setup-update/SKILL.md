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
- Se o usuário pedir "voltar versão", ser transparente: o Maestro atual não faz rollback automático. Se o ZIP anterior foi guardado, reinstalar a versão antiga seguindo o `README-INSTALL.md` resolve. Caso contrário, pedir o link ao time BCG Brasil AI.
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

Contexto: o time BCG Brasil AI envia um email com o link do ZIP novo. O usuário baixa e segue o ritual do `README-INSTALL.md` na raiz da pasta Maestro, que é a fonte única desse processo. Esta skill entra depois disso, para verificar.

Antes da verificação longa, leia `UPDATE-RUNBOOK.md`. O contrato exige um
receipt versionado sob `data/canary/`, retomada idempotente e dois despachos
reais e sequenciais: Darwin para saúde do sistema, depois Yoda para o veredito
final. Não simule os retornos e não encerre em plano ou progresso parcial.

**Nunca repita os passos do ritual nesta skill.** Qualquer resumo diverge do original e vira instrução destrutiva. Em particular, nunca oriente a extrair o ZIP por cima da pasta atual: isso deixa arquivos de versões diferentes misturados. O `README-INSTALL.md` manda renomear a pasta antiga e **copiar** a `data/` para a instalação nova — é ele que o usuário deve seguir.

1. **Perguntar a versão esperada.** Uma frase apenas: "qual versão o email do time BCG Brasil AI pediu para instalar?"

2. **Ler `VERSION` local.** Comparar com a versão informada.
   - **Match:** "instalado v<X.Y.Z>, igual à versão do email. O núcleo novo está ativo. Agora vou verificar a preservação da workspace pelo manifesto criado antes da troca." A igualdade de `VERSION` nunca prova preservação de `data/`; concluir somente depois de validar o baseline descrito no kit.
   - **Mismatch (local abaixo do esperado):** "a versão instalada é v<X.Y.Z>, abaixo da que o email pediu. Feche o Claude Code inteiro e siga o passo a passo do `README-INSTALL.md` que está na raiz da pasta Maestro — ele preserva sua `data/`. Quando reabrir, é só dizer 'confere versão' que eu verifico." Não listar os passos aqui.
   - **Mismatch (local acima do esperado):** raro, mas possível. Informar: "a versão instalada é mais nova que a informada. Confirme com o time BCG Brasil AI qual é a versão correta antes de qualquer ação."

3. **Abrir ou retomar o receipt.** Derivar a versão de destino do arquivo
   `VERSION` e usar `data/canary/update-<versão>.json`. Se não existir, criar
   um JSON com `schema_version: 1`, `contract_id:
   maestro-update-long-run-v1`, um `attempt_id` UUID opaco, versões de
   origem/destino, plataforma, `target_release_sha256` do ZIP exato validado
   contra seu sidecar, `target_core_sha256` agregado de todos os arquivos fora
   de `data/`, `baseline_manifest_sha256`, `installation_root_sha256` do
   caminho canônico, `status: in_progress`, timestamps, checks nomeados e a
   lista `required_agents: [darwin, yoda]`. Checks e agentes ainda não
   executados não carregam `state`; ausência de estado significa pendente. O
   receipt é somente metadado:
   nunca incluir prompts, conteúdo de arquivos, material de cliente ou dados
   pessoais. Se existir, validar schema e todas essas bindings antes de
   reutilizar qualquer `PASS`. Campo ausente, ZIP/core/baseline/raiz divergente
   ou tentativa de outra plataforma torna o receipt anterior stale: preservá-lo
   como `update-<versão>-stale-<attempt_id>.json`, gerar novo `attempt_id` e
   começar outra tentativa. Só retomar do primeiro item não terminal quando
   todas as bindings coincidirem.

4. **Provar o update.** Registrar separadamente como `PASS`, `FAIL` ou
   `UNAVAILABLE`: versão; baseline SHA-256 de cada arquivo preexistente;
   preservação de `data/`; runtime da plataforma; `/status`; seis handlers em
   `/hooks`; execução observada de SessionStart e UserPromptSubmit; projeções
   de agentes. Configurado, carregado e executado são estados diferentes.

5. **Sanidade pós-atualização.** Rodar `maestro-doctor`. Se surgir dúvida
   (arquivo faltando, hook não roda), seguir a prescrição dele antes de
   continuar.

6. **Darwin obrigatório.** Usar a ferramenta Agent com `subagent_type: darwin`.
   Enviar somente o pacote fechado de saúde do sistema: versões,
   checks, caminhos do core, diagnósticos e marcadores relevantes. Não enviar
   conteúdo de `data/`. Aguardar o retorno real, registrar o veredito no
   receipt e endereçar achados seguros e reversíveis. Se Agent estiver
   indisponível, registrar `UNAVAILABLE`; não imitar Darwin.

7. **Yoda obrigatório.** Depois de fechar os checks e o retorno de Darwin,
   usar a ferramenta Agent com `subagent_type: yoda`. Enviar pedido literal,
   resumo de evidência, consequência de erro, reversibilidade e gaps. Aguardar
   o retorno real e registrar o veredito. Se Agent estiver indisponível,
   registrar `UNAVAILABLE`; não imitar Yoda.

8. **Terminalidade.** Continuar enquanto houver ação segura e autorizada.
   Finalizar o receipt apenas quando todo check e ambos os agentes estiverem em
   `PASS`, `FAIL` ou `UNAVAILABLE`. `status: pass` exige todos em `PASS`;
   qualquer `FAIL` produz `status: fail`; sem falha mas com prova impossível,
   `status: unavailable`. Um bloqueio corporativo é terminal honesto, não
   convite para contornar política. O schema é fechado: `status` aceita apenas
   `in_progress`, `pass`, `fail` ou `unavailable` em minúsculas; cada
   `checks[*].state` e `agents.<id>.state` aceita apenas `PASS`, `FAIL` ou
   `UNAVAILABLE` em maiúsculas.

9. **Fechar o ciclo (obrigatório).** Somente quando o receipt terminar em
   `pass`, reconciliar os marcadores em `data/`:
   - Ler `${CLAUDE_PROJECT_DIR}/VERSION` (versão em execução) e `${CLAUDE_PROJECT_DIR}/data/.maestro-version` (versão instalada anteriormente).
   - Se diferentes e a verificação confirmou o novo ZIP no lugar, atualizar `data/.maestro-version` para o novo valor via Write ou Edit.
   - Se existir `data/.upgrade-pending`, apagar o arquivo. Ele foi escrito pelo hook `first-run-scaffold.sh` e serviu de gatilho; sem essa limpeza o SessionStart repete o alerta.
   - Se a migração falhou ou ficou `unavailable`, preservar o marcador e
     informar honestamente que o upgrade não fechou.

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

   - **Arquivos core ausentes** (`VERSION`, `CLAUDE.md`, `.claude/`, `bundles/`): "a instalação está incompleta. Baixe o ZIP mais recente indicado no último email do time BCG Brasil AI e siga o `README-INSTALL.md`. A workspace `data/` é preservada pelo ritual."
   - **`data/` ausente, core presente:** "feche o Claude Code e reabra a pasta Maestro. Na próxima abertura a workspace é recriada automaticamente."
   - **Hooks presentes mas sem permissão de execução (Mac/Linux):** "reinstale seguindo o `README-INSTALL.md`. A extração de uma pasta nova restaura as permissões corretas."
   - **`data/` corrompida ou com conteúdo perdido:** ser direto. "o Maestro não guarda backup automático da sua workspace. Se há uma cópia manual (Time Machine, backup em nuvem pessoal, cópia do OneDrive), restaure por cima da `data/` atual. Sem backup, o conteúdo perdido não é recuperável pelo Maestro."
   - **`VERSION` presente mas fora do formato `X.Y.Z`:** tratar como install corrompido, apontar para o `README-INSTALL.md`.

3. **Confirmar recuperação.** Após qualquer ação, sugerir rodar `maestro-doctor` de novo para confirmar veredicto "Tudo funcionando".

## Rollback

Não há rollback automático. Se o usuário pediu para voltar a uma versão anterior:

1. Perguntar: "o ZIP da versão anterior foi guardado localmente?"
2. **Sim:** orientar a mesma sequência do fluxo de atualização, usando o ZIP antigo no lugar do novo. Fechar Claude Code e seguir o `README-INSTALL.md` usando o ZIP antigo, depois reabrir. A `data/` é preservada pelo ritual.
3. **Não:** informar honestamente que o Maestro atual não faz rollback automático e sugerir pedir o link do ZIP anterior ao time BCG Brasil AI pelo canal oficial.

## O que esta skill nunca faz

- Não sugere abrir terminal, rodar script ou editar JSON.
- Não invoca `bcgos` nem qualquer binário de instalador (esse caminho foi encerrado).
- Não promete rollback automático.
- Não altera conteúdo autoral em `data/`. Depois de uma atualização validada,
  pode reconciliar somente os marcadores de lifecycle explicitados neste
  contrato (`.maestro-version` e `.upgrade-pending`).
- Não repete o trabalho de `maestro-onboarding` (identidade) nem de `maestro-doctor` (diagnóstico). Delega.

## Encerramento

Ao terminar, resumir em uma linha o desfecho, a versão ativa (lida em `VERSION`) e o caminho absoluto da workspace (`${CLAUDE_PROJECT_DIR}/data/`). Se houver ação pendente do lado do time BCG Brasil AI (email com link, versão a confirmar), deixar explícito.
