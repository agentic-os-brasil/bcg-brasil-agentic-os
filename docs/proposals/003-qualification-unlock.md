# Proposta 003 — Qualificação vira evento de produto, não corrida de obstáculos do usuário

**Status:** pedido de input — proposta arquitetural, não altera código nesta PR.
**Origem:** Canary v0.1.22 executado em 2026-08-09.
**Defeitos endereçados:** cadeia de qualificação que nunca fecha (raiz de Darwin dormente, hooks sem `native_qualified`, work items não persistentes, checkpoint `unavailable`).
**Severidade percebida:** alta — bloqueia Darwin, memória e continuidade em toda instalação nova.

---

## 1. Conclusão

Hoje o Maestro instala travado. Toda instalação nova nasce com hooks `configured: true` mas
`native_qualified: false`, e o sistema espera que a evidência de conformidade apareça sozinha
durante o uso. Ela não aparece. O resultado é que Darwin, housekeeping, memória e checkpoint
ficam permanentemente `unavailable` em um sistema que está, de fato, funcionando.

Este PR inverte a lógica: a qualificação passa a ser um evento de produto, executado pelo time
interno via Canary, com o resultado assinado dentro do bundle distribuído. Usuário beta instala
um bundle já qualificado e termina o setup com tudo liberado. Gates sobrevivem apenas onde
existe risco real: ação destrutiva, acesso a dado externo e mudança estrutural.

---

## 2. Problema observado

Ao final de um Canary completo de 12 tarefas, com 63 receipts `post_action_observe` gravados e
um `stop_finalize` atravessando fronteira de sessão, o CLI ainda reportava:

```
session_start        configured: true   adapter_observed: false   native_qualified: false
pre_action_guard     configured: true   adapter_observed: false   native_qualified: false
post_action_observe  configured: true   adapter_observed: false   native_qualified: false
stop_finalize        configured: true   adapter_observed: false   native_qualified: false
context_injection    configured: true   adapter_observed: false   native_qualified: false
```

Motivo declarado em todos: `"qualifying native conformance evidence is pending"`.

Os hooks executaram. As receipts provam que executaram. O modelo de evidência não reconheceu
a própria evidência que ele mesmo gravou.

---

## 3. Cadeia de bloqueio

```
LaunchAgent com.bcg.maestro.maintenance dispara a cada 900s
  └─ maintenance wake  →  BLOQUEADO: exige autoridade attended ou preauthorized
       └─ exige: native_scheduler_qualification_pending
            └─ exige: native_session_qualification_pending (runtime claude)
                 └─ exige: evidência de conformance vinda do SessionStart
                      └─ bounded_signals.attested_capture_files = 0   ← nunca sai de zero
```

O elo final nunca fecha. A escada de qualificação (`configured` → `adapter_observed` →
`native_qualified`) tem o primeiro degrau instalado e o segundo inalcançável, então os dois
degraus seguintes ficam mortos por construção.

O LaunchAgent está correto ao não passar `--attended`: um daemon de fundo não deve se
auto-conceder autoridade attended. O erro não está no daemon. Está em exigir que a autoridade
apareça por inferência posterior em vez de ser estabelecida no setup.

---

## 4. Os dois defeitos de produto

### DEF-A: setup nunca emite evidência de qualificação

O instalador registra bindings e escreve `configured: true`, mas não executa nenhuma verificação
de conformance nem grava um atestado de qualificação. O sistema fica dependente de inferência
post-hoc sobre receipts acumuladas, e essa inferência nunca conclui positivamente.

**Consequência:** nenhuma instalação nova jamais atinge `native_qualified` sem intervenção manual
não documentada.

### DEF-B: janela de diagnóstico de 64 entradas é hostil ao uso real

`bcgos doctor` reporta:

> `lifecycle receipt history exceeds the 64-entry diagnostic window; receipts remain local and
> no native qualification was inferred.`

Uma sessão de trabalho normal ultrapassa 64 receipts com facilidade. O Canary produziu mais de 70.
Quanto mais o usuário trabalha, mais garantido fica que a qualificação falha. O mecanismo é
anti-correlacionado com o uso que ele deveria medir.

**Consequência:** mesmo que DEF-A fosse corrigido, a inferência quebraria de novo no primeiro dia
de uso intenso.

---

## 5. Arquitetura alvo

```
  ANTES                                    DEPOIS

  cada usuário precisa provar              time interno prova uma vez, via Canary
  conformance no próprio runtime                        │
             │                                          ▼
             ▼                             Canary PASS grava atestado assinado
  inferência post-hoc sobre receipts                    │
             │                                          ▼
             ▼                             atestado embarcado no bundle distribuído
  nunca conclui                                         │
             │                                          ▼
             ▼                             setup valida assinatura e libera tudo
  Darwin/memória/hooks unavailable                      │
  para sempre                                           ▼
                                           usuário termina o setup operacional
```

Princípio: **o Canary qualifica o produto, não o usuário.** A pergunta "este runtime executa hooks
corretamente?" é uma propriedade da build, respondida uma vez pelo time, não uma prova que cada
consultor precisa refazer no próprio notebook.

---

## 6. Mudanças propostas

### 6.1 Atestado de qualificação no bundle

Adicionar ao bundle distribuído um `qualification-attestation.json` assinado:

```json
{
  "schema_version": 1,
  "bundle_version": "0.1.23",
  "runtime": "claude",
  "qualified_capabilities": [
    "session_start", "pre_action_guard", "post_action_observe",
    "stop_finalize", "context_injection",
    "scheduler_invocation", "darwin_housekeeping"
  ],
  "qualified_by": "canary",
  "canary_run_id": "<id>",
  "qualified_at": "<iso8601>",
  "signature": "<assinatura do bundle>"
}
```

Regra de leitura: se a assinatura confere e `runtime` bate com o runtime instalado, as capabilities
listadas iniciam em `native_qualified: true`. Sem inferência, sem espera.

### 6.2 Setup emite evidência em vez de esperar por ela

No fim de `maestro-setup-update`, executar uma verificação de conformance ativa e curta:
dispara um ciclo sintético de hook (SessionStart, PreToolUse, PostToolUse, Stop), confere que as
receipts saíram com o shape esperado e grava o resultado como evidência local de primeira classe.

Isso substitui a inferência passiva por um teste determinístico de alguns segundos, executado uma
vez, no momento em que o usuário está presente e o contexto é conhecido.

### 6.3 Autoridade de manutenção concedida no setup

O setup passa a registrar autorização `preauthorized` para o LaunchAgent de manutenção, escopada ao
workspace instalado. O daemon continua sem `--attended`, como deve ser, mas encontra a autorização
já gravada em vez de bater numa porta fechada a cada 15 minutos.

`maintenance canary` sai de `catalog_only` direto para o estado operacional, sem exigir o
`maintenance wake --attended` manual que hoje não está documentado em lugar nenhum.

### 6.4 Janela de diagnóstico corrigida

Três mudanças em `bcgos doctor`:

1. A janela de 64 entradas passa a ler as **N mais recentes** em vez de falhar quando o histórico
   excede o limite. Exceder o histórico não pode ser condição de erro.
2. O limite vira configurável, com default significativamente maior (sugestão: 512).
3. Qualificação deixa de depender dessa janela. A janela serve para diagnóstico e apresentação,
   não para decidir estado de capability. Estado de capability vem do atestado (6.1) ou da
   verificação de setup (6.2).

### 6.5 Housekeeping e memória saem do escopo de gate

`memory-checkpoint`, `memory-light-dream`, `darwin-housekeeping-daily` e a gravação de receipts
`post_action_observe` passam a rodar sem gate de qualificação. São operações locais, idempotentes,
reversíveis e sem alcance externo. Bloqueá-las não protege nada e quebra a continuidade que é a
proposta central do produto.

---

## 7. O que continua com gate

O PR não remove governança. Reduz o gate ao que justifica um gate:

| Categoria | Continua bloqueado | Racional |
|---|---|---|
| Ação destrutiva | Sim | Remoção e sobrescrita irreversível de conteúdo do usuário |
| Acesso a dado externo | Sim | SharePoint, rede, qualquer fonte fora do workspace autorizado |
| Mudança estrutural | Sim, como proposta apenas | Alteração de `control-plane/`, agentes, políticas |
| Escrita cross-workspace | Sim | Isolamento de workspace é invariante do produto |
| Housekeeping local | Não | Idempotente, reversível, sem alcance externo |
| Memória e checkpoint | Não | É a função do produto, não um privilégio |
| Receipts de observação | Não | Metadado local, já é o mecanismo de evidência |

---

## 8. Critérios de aceite

1. Instalação limpa em máquina nova termina o setup com todas as capabilities de hook em
   `native_qualified: true`, sem comando manual adicional.
2. `bcgos maintenance status` reporta Darwin operacional imediatamente após o setup.
3. Sessão com mais de 500 receipts não degrada nenhum estado de capability.
4. `bcgos doctor` não emite warning por histórico de receipts exceder janela.
5. Job `memory-checkpoint` executa e persiste sem exigir autoridade attended.
6. Work item criado em uma sessão continua visível e resumível na sessão seguinte
   (fecha DEF-01 e DEF-02 pela raiz).
7. Bundle sem atestado válido, ou com assinatura divergente, mantém o comportamento
   conservador atual e falha fechado.
8. Tentativa de escrita cross-workspace continua bloqueada.

---

## 9. Migração e compatibilidade

Instalações existentes não têm atestado no bundle. Para elas, o caminho é o item 6.2: a primeira
execução da versão nova roda a verificação de conformance e grava a evidência local, promovendo as
capabilities sem exigir reinstalação.

O fail-closed é preservado: ausência de atestado e falha na verificação levam ao estado
conservador atual, não a uma liberação otimista. A mudança é sobre existir um caminho que conclui,
não sobre remover a checagem.

---

## 10. Riscos

**Risco:** atestado assinado no bundle pode mascarar um runtime que de fato não executa hooks.
**Mitigação:** o atestado é escopado por `runtime` e `bundle_version`. Runtime divergente do
atestado não recebe qualificação. A verificação de setup (6.2) roda de qualquer forma e pode
rebaixar o estado se o ciclo sintético falhar.

**Risco:** reduzir gates aumenta superfície de ação automática.
**Mitigação:** a tabela da seção 7 mantém gate em tudo que é destrutivo, externo ou estrutural.
O que foi liberado é local, idempotente e reversível.

**Risco:** autorização `preauthorized` concedida no setup é ampla demais.
**Mitigação:** escopar ao `workspace_id` gravado no setup. Autorização não transfere entre
workspaces.

---

## 11. Evidência anexa

- `canary/MAESTRO-CANARY-REPORT.md`, matriz de 12 tarefas e tabela de 8 defeitos
- Receipts em `~/Library/Application Support/BCGOS/runtime/receipts/913479cfd98f88e22fe7f645efe42047/`,
  63 `post_action_observe` mais 1 `stop_finalize` de sessão anterior
- `~/Library/LaunchAgents/com.bcg.maestro.maintenance.plist`, invocação sem `--attended`
- Saída de `bcgos doctor`, warning de janela de 64 entradas e reasons de qualificação pendente

---

> Artefato do Canary v0.1.22. Nenhum cliente, dado real ou stakeholder envolvido.
