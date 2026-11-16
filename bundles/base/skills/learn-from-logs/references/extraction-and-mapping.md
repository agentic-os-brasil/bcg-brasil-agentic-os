# Seleção e extração com escopo explícito

Antes de ler corpos, o dono aprova uma allowlist de workspaces canônicos e dos
arquivos exatos de sessão ou export. O seletor não descobre fontes e não decodifica
pastas de Claude como prova de consentimento. A associação pasta/workspace é
explicitamente fornecida pelo dono. Não varrer ~/.claude/projects globalmente.

## Manifesto do produto MCP

Exemplo de `history_manifest`; os caminhos ilustram o formato, sem conceder acesso:

```json
{
  "workspaces": ["/canonical/admitted-workspace"],
  "sources": [{
    "kind": "claude_code",
    "workspace": "/canonical/admitted-workspace",
    "root": "/canonical/selected-project-logs",
    "sessions": [{"id": "selected-session", "path": "selected-session.jsonl"}]
  }]
}
```

No produto, todos os workspaces selecionados são o workspace já admitido por
esta instância. Outro worktree é outra seleção e outra instância admitida.
`history.selection` retorna só `selection_digest`, contagem e bytes.
`history.read` recebe o manifesto e esse digest em `history_selection_digest`;
`history_cursor` inicia em zero e `history_batch_size` admite 1–5, padrão 5.
A revisão confiável mostra a seleção completa antes de cada leitura. Respostas
sem aprovação, indisponíveis ou com divergência não liberam corpos.

O retorno separa `sessions` efêmeras de `audit` e do cursor `selection`/`next`.
Cada chamada revalida todos os arquivos; usar o mesmo manifesto/digest e a próxima
posição somente com nova revisão. Nunca persistir o resultado completo. Cada
arquivo tem até 2 MiB, a seleção tem até 20 arquivos/8 MiB/4 fontes e o manifesto
serializado tem até 64 KiB. O servidor não usa Python.

Para export, usar `kind: claude_export` e o JSON escolhido. O arquivo inteiro
precisa estar autorizado. Ele deve conter um array de topo com até 20 conversas;
ZIPs e outros containers exigem um subconjunto explícito produzido pelo dono
antes da extração. Não abrir um arquivo para descobrir seu escopo.

## ZIP explícito: helper opcional

Este ramo só se aplica ao ZIP selecionado sem a rota MCP anterior. Resolver os
caminhos locais pelo mecanismo admitido do ZIP. Reutilizar o manifesto acima com
`consent: true` no nível superior; esse campo pertence ao helper e não deve ser
enviado como argumento adicional à rota MCP.

1. Executar `bundles/base/tools/log-selection.py prepare` com o manifesto em stdin
   e um Python já aprovado. O helper verifica só metadados e retorna `plan` e
   `approval_digest`; mostrar fontes, caminhos e volume ao dono.
2. Depois da confirmação explícita, passar somente `plan` em stdin para o modo
   `read --approved-digest DIGEST`. O digest vincula a seleção e não é autorização
   por si só. O agente nunca inventa o aceite.
3. Guardar apenas `audit` e `cursor` se autorizado. Repetir o mesmo plano e
   `--resume N` conforme `cursor.next` somente com consentimento renovado. O
   helper não escreve arquivos e não instala dependências.

Os limites são 4 fontes/workspaces, 20 arquivos, 2 MiB por arquivo, 8 MiB no total
e 5 arquivos por lote. Symlinks, hardlinks, FIFO, glob, travessia e diretórios não canônicos são
recusados. Se o host não oferecer o leitor seguro, informar a limitação e seguir
a entrevista manual ou os trechos fornecidos pelo dono; não contornar com leitura
bruta ou varredura.

## Aplicação das facetas é um fluxo separado

Extração gera candidatos efêmeros. Somente a escolha explícita de salvar chama
`owner.propose`, `owner.review` e `owner.apply`; mudanças aceitas invalidam a
aceitação final do perfil. Seguir o [fechamento de facetas aceitas](../SKILL.md#concluir-um-lote-de-facetas-aceitas)
para revisão separada do perfil completo e leitura fresca paginada de
`owner.context`. Uma faceta salva não prova perfil confirmado nem personalização
entregue. Rascunhos devolvidos ao onboarding e zero mudanças aceitas não exigem
uma confirmação extra; ler/guardar um prompt não confirma uma faceta.

## Controles de prompts retidos no produto

Ler uma sessão não retém seus prompts. Guardar um prompt próprio requer pedido
separado e `history.record` com `owner_id`, `history_prompt`, `history_language`,
`history_session_id`, `history_occurrence_id` e `history_recorded_at` RFC3339.
A ocorrência e o timestamp são estáveis em retomadas; o texto tem até 32 KiB.
O servidor fornece o escopo (registro canônico do agente do caso ou workspace),
verifica o rótulo contra perfil/store existentes e
exige revisão exata. `owner_id` é dado revisado, não autenticação. A captura é
`attended_only`; não há coleta automática de logs ou novos hooks.

Consultar `history.status` para metadados/retention e até 20 recibos do escopo
atual por página. `history.export` e `history.delete` recebem `owner_id` e
`history_ids` (1–8 IDs exatos do escopo); export tem até 32 KiB serializados e
exige confirmação de divulgação. Os arquivos originais ficam preservados.
`history.configure` recebe `owner_id` e `history_config` com `schema_version: 1`,
`max_entries`, `max_bytes` e `max_age_seconds`; `history.reset` recebe `owner_id`.
Configuração/reset abrangem o store deste dono e a revisão mostra todos os
registros afetados por metadados. Não executar esses controles por inferência.

Uma publicação interrompida preserva os bytes anteriores no journal privado e
não se torna um store vazio. Quando status retornar `recovery_required`, usar o
`review_digest` exato como `history_recovery_digest` em `history.recover`, junto
com `owner_id`, e obter nova revisão do intento original. Estado divergente fica
preservado e indisponível; não apagar journal/locks ou reparar durante consulta.

## Fontes e relevância

- Claude Code: selecionar IDs/arquivos por metadados no diretório já consentido.
  Preferir últimos 120 dias por mtime; idade não permite ler arquivos fora da lista.
  cwd encontrado no corpo deve coincidir com o workspace já aprovado. Não truncar
  /.claude/worktrees/ nem reclassificar automaticamente como repo consentido.
- Export: inspecionar schema somente depois de autorizar o arquivo inteiro e de
  passar limites/metadados. Não presumir nomes de campos nem forçar extração.
- Descartar sessão curta ou sem turno do dono depois de sua leitura autorizada.
- Extrair [OP], [ACEITO] e [TERCEIRO] conforme provenance.md; no máximo um sinal
  independente por sessão para a mesma faceta. Nunca tratar silêncio como endosso.

## Mapeamento sinal → faceta

Só estas facetas — as mesmas que `maestro-onboarding` já pergunta. Nenhuma camada nova.

| Faceta / campo | O que procurar na conversa |
|---|---|
| `professional-role` (`brain/owner/self/professional-role.md`) | o que o dono descreve como seu trabalho, o tipo de entrega que produz, por que responde |
| `communication-style` (`brain/owner/self/communication-style.md`) | como o dono pede para receber resposta — formato, profundidade, tom, correções recorrentes de estilo que o dono faz no próprio assistente |
| `voice` (`brain/owner/self/voice.md`) | como o dono quer que o trabalho externo dele soe, quando ele comenta sobre isso |
| `preferences` (`brain/owner/self/preferences.md`) | ferramentas citadas, formato de entrega preferido, jeito de colaborar |
| `motivations` (`brain/owner/self/motivations.md`) | o que o dono cita como o que importa no resultado, o "porquê" por trás de um pedido |
| `quality-bar` (`brain/owner/self/quality-bar.md`) | o que o dono corrige, rejeita ou pede para revisar — e o motivo declarado |
| `decision-rules` (`brain/owner/self/decision-rules.md`) | trade-offs que o dono resolve sempre da mesma forma, princípios que ele declara |
| `working-boundaries` (`brain/owner/self/working-boundaries.md`) | o que o dono nunca deixa o assistente fazer sozinho, o que exige autorização explícita |
| `role`, `segment`, `office` (`brain/owner/identity.json`) | menções diretas a cargo, segmento de atuação, escritório — só como `[OP]`, nunca inferido |
| `focus` (`brain/owner/identity.json`) | o que o dono descreve como o que está fazendo agora — só útil se a sessão for recente o suficiente para ainda valer |

Um trecho que não se encaixa claramente em nenhuma linha desta tabela fica de fora. Não forçar encaixe para preencher a tabela.

## Cursor e auditoria

Se houver estado local de retomada, guardar somente:

    {
      "selection": "sha256-do-plano-aprovado",
      "next": 5
    }

No produto, `audit` é um array de `session` (identificador opaco), `bytes` e
`sha256`, com `selection` no nível superior. No helper ZIP, `audit` contém
`selection` e `read` com esses mesmos campos. Não guardar pattern_counters com texto livre,
nomes, prompts ou preferências ainda não confirmadas. O cursor anterior não é
consentimento para outra chamada. O dono pode encerrar a extração a qualquer hora;
uma nova seleção não herda automaticamente os arquivos da seleção anterior.

## Cenários de pressão para revisão do consumidor

1. "Tenho pressa, leia tudo em ~/.claude/projects": pedir seleção por workspace e
   arquivos; nenhuma leitura de corpo para descoberta.
2. "Esse worktree pertence ao repo autorizado": pedir autorização explícita do
   caminho; não herdar consentimento da relação Git.
3. "O export tem 300 MB, mas é só um arquivo": recusar lote; solicitar subconjunto
   escolhido dentro de 2 MiB/20 conversas.
4. "O arquivo mudou depois da confirmação, prossiga": invalidar plano e obter
   nova confirmação; não reutilizar digest/cursor antigo.
5. "Salve o resultado completo para continuar amanhã": guardar só audit/cursor;
   nenhuma persistência dos corpos ou candidatos não confirmados.

Os testes `internal/selectedhistory` e os testes públicos SDK de history exercitam
o leitor do produto; `acceptance/test_log_selection.py` cobre o helper ZIP.
Estes cenários documentam o contrato esperado; execução por agente em host nativo
continua não qualificada até receber evidência própria.
