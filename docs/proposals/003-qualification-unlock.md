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

  cada usuário precisa provar              Canary é gate de RELEASE
  conformance no próprio runtime           (o time decide se a build sai)
             │                                          │
             ▼                                          ▼
  inferência post-hoc sobre receipts       Canary PASS = build habilitada a distribuir
             │                                          │
             ▼                                          ▼
  nunca conclui                            build sai da fábrica com tudo LIGADO
             │                                          │
             ▼                                          ▼
  Darwin/memória/hooks unavailable         usuário instala e usa
  para sempre                              não existe gate de runtime a atravessar
```

Princípio: **o Canary é gate de release, não de runtime.** A pergunta "este build executa hooks
corretamente?" é respondida uma vez pelo time interno antes de distribuir. Uma vez respondida, ela
está respondida. O cliente não a repete, o cliente não a valida, o cliente não depende dela para
funcionar.

**Por default, tudo liberado.** Instalação nova nasce com Darwin ativo, hooks qualificados, memória
disponível, checkpoint funcionando. Não há caminho de código no cliente que possa deixar essas
capabilities em `unavailable` por falta de "evidência". A evidência mora no processo de release,
não no runtime do usuário.

---

## 6. Mudanças propostas

### 6.1 Runtime abre em modo operacional

Remover do runtime toda lógica que decide entre `configured`, `adapter_observed`, `native_qualified`
e `unavailable` com base em inferência de receipts. A distinção sobrevive apenas como telemetria
descritiva (o que aconteceu), não como gate (o que pode acontecer).

Estado inicial de uma instalação, sem nenhuma sessão ainda registrada:

```
session_start        state: operational
pre_action_guard     state: operational
post_action_observe  state: operational
stop_finalize        state: operational
context_injection    state: operational
scheduler_invocation state: operational
darwin_housekeeping  state: operational
```

Não há caminho para `unavailable` por "evidência pendente". `unavailable` fica reservado para
condição real de indisponibilidade (binário ausente, permissão negada, disco cheio) — não para
ausência de prova.

### 6.2 Canary vira gate de release, não de runtime

O Canary continua existindo e continua sendo obrigatório — mas do lado do fabricante:

- Time interno roda o Canary em máquinas de teste antes de aprovar uma versão.
- Canary PASS é pré-requisito para publicar a build no canal de distribuição.
- O resultado do Canary vai para o registro de release (changelog, release notes), não para
  dentro do binário como atestado a ser verificado em runtime.

A certeza que o Canary dá é: "esta build, no runtime alvo, executa os hooks corretamente." Essa
certeza é do time. O usuário não precisa reproduzir a prova.

### 6.3 Autoridade de manutenção padrão-ligada no setup

O setup registra, como parte do fluxo padrão, a autorização local para o LaunchAgent de manutenção,
escopada ao workspace instalado. Não é passo opcional, não é comando manual, não é `--attended`
digitado pelo usuário. É parte do que "instalar o Maestro" significa.

`maintenance canary` do lado do cliente deixa de existir como conceito. O usuário não roda Canary.
O Canary é do time.

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
   `state: operational`, sem comando manual adicional e sem depender de evidência de runtime.
2. `bcgos maintenance status` reporta Darwin operacional imediatamente após o setup.
3. Sessão com mais de 500 receipts não degrada nenhum estado de capability.
4. `bcgos doctor` não emite warning por histórico de receipts exceder janela.
5. Job `memory-checkpoint` executa e persiste sem exigir autoridade attended.
6. Work item criado em uma sessão continua visível e resumível na sessão seguinte
   (fecha DEF-01 e DEF-02 pela raiz).
7. Build que não passou pelo Canary interno não entra no canal de distribuição. Gate vive no
   pipeline de release, não no runtime do usuário.
8. Tentativa de escrita cross-workspace continua bloqueada.

---

## 9. Migração e compatibilidade

Instalações existentes não precisam de migração de dados. A primeira execução da versão nova
sobe todas as capabilities para `state: operational` sem exigir reinstalação, atestado ou
verificação em runtime. Estado antigo `unavailable` por "evidência pendente" é reescrito para
`operational` na inicialização.

Fail-closed continua onde ainda faz sentido: indisponibilidade real (binário ausente, permissão
negada, disco cheio) mantém `unavailable`. O que muda é que ausência de prova deixa de ser
condição de indisponibilidade — porque a prova é do time, feita antes da build sair.

---

## 10. Riscos

**Risco:** build ruim escapa do Canary interno e chega ao usuário com hooks quebrados.
**Mitigação:** Canary é bloqueante para publicação no canal de distribuição. Sem Canary PASS,
sem release. Rollback de versão é o caminho quando um problema chega em campo, não gate de
runtime que sempre falha fechado.

**Risco:** reduzir gates aumenta superfície de ação automática.
**Mitigação:** a tabela da seção 7 mantém gate em tudo que é destrutivo, externo ou estrutural.
O que foi liberado é local, idempotente e reversível.

**Risco:** autorização de manutenção concedida no setup é ampla demais.
**Mitigação:** escopar ao `workspace_id` gravado no setup. Autorização não transfere entre
workspaces e não se propaga para operações destrutivas ou externas, que continuam com gate.

---

## 11. Evidência anexa

- `canary/MAESTRO-CANARY-REPORT.md`, matriz de 12 tarefas e tabela de 8 defeitos
- Receipts em `~/Library/Application Support/BCGOS/runtime/receipts/913479cfd98f88e22fe7f645efe42047/`,
  63 `post_action_observe` mais 1 `stop_finalize` de sessão anterior
- `~/Library/LaunchAgents/com.bcg.maestro.maintenance.plist`, invocação sem `--attended`
- Saída de `bcgos doctor`, warning de janela de 64 entradas e reasons de qualificação pendente

---

> Artefato do Canary v0.1.22. Nenhum cliente, dado real ou stakeholder envolvido.
