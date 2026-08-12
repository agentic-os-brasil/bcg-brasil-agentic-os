---
name: workspace-agent-setup
description: Cria ou retoma o agente de um workspace de caso (briefing revisto, plano de pesquisa pública aprovado, evidências com fonte e snapshot econômico público opcional). Use ao iniciar um novo projeto de cliente ou refrescar o briefing de um existente.
---

# Workspace Agent Setup

Constrói contexto útil de projeto preservando o workspace como fronteira de
confidencialidade. O agente de caso é dono deste fluxo e nunca importa contexto
de outro workspace.

Todo o estado desta skill vive em arquivos locais sob
`${CLAUDE_PROJECT_DIR}/data/workspaces/<slug>/`. Não há binário, comando de
shell nem processo em segundo plano envolvido. A skill orienta, verifica, cria
arquivos com o Write e delega decisões críticas ao usuário.

## Interaction profile

Resolver `interaction-profile` antes de começar. Ele controla o nível de
detalhe técnico exibido durante o setup, jamais aprovação, classificação,
proveniência ou isolamento de workspace.

## Contrato de comunicação

- Uma pergunta por vez.
- Sem "você", "tu" ou "te". Preferir impessoal ou 3ª pessoa.
- Sem em-dash ("—") em texto externo.
- Nunca pedir para o usuário abrir terminal, editar JSON ou rodar script.
- Nunca invocar `bcgos` nem qualquer binário de instalador. Esse caminho foi
  encerrado.

## Layout em disco

Cada workspace vive em `${CLAUDE_PROJECT_DIR}/data/workspaces/<slug>/`, onde
`<slug>` é um nome curto, kebab-case, escolhido pelo dono do caso (ex.:
`hdi-ai-for-sales`, `natura-perfumaria-br`). Dentro do slug, esta skill mantém:

```
data/workspaces/<slug>/
  workspace.json         # metadados: slug, owner, criado_em, versão do briefing
  brief.md               # briefing consolidado revisto (imutável por versão)
  brief.json             # mesmo briefing em forma estruturada + provenance
  interventions.jsonl    # linhas append-only: kind, timestamp, run_id
  research/
    plan-<id>.json       # planos de pesquisa pública propostos
    approvals-<id>.json  # aprovação registrada (owner, timestamp, confirm)
    queries-<id>.jsonl   # slots imutáveis de query consumidos
    evidence.jsonl       # cada claim retido, uma linha
  economic/
    snapshot-<id>.json   # snapshot macro público, atestado, fora do workspace
```

Um `run_id` é uma string curta gerada pela skill (ex.: ISO8601 compacto +
sufixo de 4 letras). Serve como âncora de correlação entre etapas do mesmo
setup, mas não precisa ser exibido a um usuário não técnico.

## First useful result

1. Resolver o workspace ativo lendo `data/workspaces/`. Se o slug alvo ainda
   não existe, propor sua criação em uma frase e criar o diretório e o
   `workspace.json` inicial só após confirmação explícita. Parar se o workspace
   está ausente, ambíguo ou diferente do que o usuário acredita estar aberto.

2. Abrir um novo run: gerar `run_id` interno, registrar `run.json` em
   `data/workspaces/<slug>/runs/<run_id>.json` com `phase: "value"` e
   `state: "started"`. O run_id fica no fluxo, não é exposto ao usuário
   não técnico.

3. Conduzir a entrevista de forma conversacional. Fazer as seis perguntas
   mínimas, uma de cada vez:
   - decisão e horizonte;
   - audiência e restrições;
   - resultado útil esperado;
   - material autorizado;
   - hipóteses balanceadas;
   - próximo passo.
   Consolidar em um briefing e um plano de uma a três ações. Exibir a
   consolidação antes de gravar e pedir confirmação.

4. Após confirmação, gravar dois arquivos irmãos:
   - `brief.md` com o briefing consolidado (versão nova, nunca sobrescreve a
     anterior sem trilha).
   - `brief.json` com o mesmo conteúdo estruturado + `version`, `owner`,
     `classified_at`, `run_id` e um campo `handoff` compacto (só ponteiros e
     sinais operacionais, sem transcrição da entrevista).
   Atualizar `workspace.json` para apontar `current_brief_version`.

5. Em cada correção posterior, registrar somente a natureza da correção em
   `interventions.jsonl`, um objeto por linha:
   `{ "run_id": "...", "kind": "brief_correction" | "plan_correction" | "artifact_revision", "at": "<ISO8601>" }`.

6. Para retomar depois, ler `workspace.json` + `brief.json` mais recente e
   continuar do handoff. Não repetir a entrevista nem injetar a transcrição.

O fluxo de first-value não navega, não ingere documentos, não consulta wiki,
não sonha memória, não refresca dados econômicos e não cria outro agente.
Pesquisa externa é um fluxo separado, descrito abaixo, e só ocorre com
aprovação explícita.

## Pesquisa pública opcional

1. Propor um plano de pesquisa pública minimizado. Usar apenas hostnames de
   uma allowlist e nunca incluir nomes confidenciais de projeto, stakeholders,
   estratégia não publicada ou fatos fornecidos pelo cliente nas query themes.

2. Persistir a proposta em
   `data/workspaces/<slug>/research/plan-<id>.json` com campos: `id`,
   `purpose`, `query_themes`, `source_allowlist`, `budget`, `proposed_at`,
   `run_id`. Exibir purpose, themes e allowlist ao dono e só seguir após
   aprovação explícita.

3. Registrar a aprovação em
   `data/workspaces/<slug>/research/approvals-<id>.json` com
   `plan_id`, `approved_by`, `approved_at`, `confirm: true`. Sem esse
   arquivo, nada é executado.

4. Executar pesquisa externa apenas quando o runtime expõe ao mesmo tempo uma
   ferramenta de busca ou browser aprovada e um guard efetivo de workspace ou
   pré-ação. Caso contrário, reportar a capacidade como indisponível. Nunca
   substituir por API, credencial ou provedor não aprovado, e nunca afirmar
   isolamento forte.

5. Imediatamente antes de cada query externa, consumir um slot imutável de
   budget: acrescentar uma linha em
   `data/workspaces/<slug>/research/queries-<id>.jsonl` com `plan_id`,
   `query_text_exato`, `at`. Parar quando o budget aprovado se esgotar.

6. Para cada claim retido, acrescentar uma linha em
   `data/workspaces/<slug>/research/evidence.jsonl` contendo `plan_id`,
   `query`, `source_url` (HTTPS), `retrieved_at`, `claim`,
   `evidence_strength`, `validity_date`, `classification: "public"`.
   Rejeitar planos ou evidências expirados, e queries ou fontes fora do plano
   aprovado.

7. Snapshot macro público pode ser importado para
   `data/workspaces/<slug>/economic/snapshot-<id>.json` só com os campos
   `attested_public: true`, `attested_by`, `confirm_no_workspace_derivation: true`.
   Cada claim precisa ser público e apontar uma fonte declarada. A atestação
   humana é fronteira de governança, não detecção automática; nunca usar
   queries do workspace, metadados ou síntese derivada do cliente.

8. Encerrar retornando: workspace, versão do briefing, plano aprovado,
   evidências com fonte, versão do snapshot econômico, lacunas de frescor e
   capacidades indisponíveis.

## Refresh

Refrescar somente quando pedido, quando uma data de decisão se aproxima, quando
uma fonte fica desatualizada ou quando um evento material cria lacuna de
evidência. Uma nova query fora dos temas ou domínios aprovados exige plano
novo e aprovação nova.

## Invariantes

- O handoff em `brief.json` contém ponteiros e sinais operacionais, não corpos
  de pesquisa nem transcrições.
- Briefings, planos, aprovações e evidências são artefatos imutáveis com
  proveniência. Nova versão nunca sobrescreve a anterior; sobrescrita só via
  entrada explícita em `interventions.jsonl`.
- Nenhuma consulta cross-workspace, nenhuma promoção automática para contexto
  de Client Account.
- Snapshots econômicos atestados vivem fora de cada workspace só em referência
  por ID imutável. Cada claim aponta fontes públicas declaradas e registra
  quem atestou que nada derivado do workspace foi usado.
- Isolamento de filesystem em runtime é fail-closed: se o runtime não pode
  aplicar a raiz declarada do workspace, declarar essa limitação explícita e
  não afirmar isolamento forte.

## O que esta skill nunca faz

- Nunca invoca `bcgos` nem binário externo.
- Nunca pede ao usuário para abrir terminal, editar JSON ou rodar script.
- Nunca importa contexto de outro workspace.
- Nunca sobrescreve briefing anterior sem entrada em `interventions.jsonl`.
- Nunca executa pesquisa externa sem `approvals-<id>.json` gravado.
