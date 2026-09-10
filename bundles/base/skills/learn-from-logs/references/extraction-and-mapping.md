# Extração e mapeamento

## Fonte A — sessões locais do Claude Code

- Caminho: `~/.claude/projects/**/*.jsonl` (encoding do path: `/Users/foo/bar` vira `-Users-foo-bar` no nome da pasta — isso é só para reconhecer a pasta, não precisa decodificar para extrair o conteúdo).
- Janela padrão: **120 dias**, cap de **500 sessões** por passada. Ordenar por `mtime` ascendente antes de cortar no cap, para não descartar sistematicamente as sessões mais antigas da janela.
- Descartar sessão com <20 linhas ou sem nenhum turno do dono (só ferramentas rodando, por exemplo).
- **Projeto ≠ worktree.** O `cwd` de uma sessão pode ser `<repo>/.claude/worktrees/<nome>`; nesse caso o projeto real é `<repo>`, não o worktree efêmero. Regra: se `cwd` contém `/.claude/worktrees/`, cortar tudo a partir daí antes de derivar qualquer identificação de projeto. Isso só importa aqui para decidir se uma sessão é ruído de infraestrutura (ex.: uma sessão inteira dentro de um worktree de PR já mesclado) — esta skill não mantém mais um arquivo por projeto, então não precisa do slug em si.
- Extrair: turnos do dono (texto), sugestões do assistente que o turno seguinte do dono não contesta (candidato a `[ACEITO]`), e qualquer trecho colado que cite um terceiro (`[TERCEIRO]`).
- Sessões já processadas são identificadas por sha256 do arquivo e guardadas no cursor — nunca reprocessar a mesma sessão duas vezes na mesma faceta.

## Fonte B — export do claude.ai

- O dono baixa em claude.ai → Configurações → Conta → Exportar dados; chega por email como um zip, geralmente com um arquivo do tipo `conversations.json` dentro.
- **Não presumir o schema.** Formatos de export mudam entre versões do produto. Antes de extrair, ler uma amostra do JSON e inspecionar as chaves reais (é comum haver um array de conversas, cada uma com um array de mensagens, cada mensagem com remetente e texto — mas os nomes exatos dos campos podem variar). Se a estrutura não bater com o esperado, diga isso ao dono em vez de forçar uma extração errada silenciosamente.
- Não há cap de sessões: o export é finito por natureza. Ainda vale a janela de 120 dias como padrão sugerido (não obrigatório) para não misturar um perfil de anos atrás com o presente — perguntar ao dono se quer ajustar a janela quando o export for muito extenso.
- Mesmo descarte de conversa muito curta ou sem turno do dono que a fonte A usa.

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

## Cursor do modo avulso

`brain/memory/learn-from-logs/_cursor.json` — criar se ausente:

```json
{
  "schema_version": 1,
  "last_run": null,
  "sources": {
    "claude_code": { "processed_sha256": [] },
    "claude_export": { "last_export_timestamp": null }
  },
  "pattern_counters": {}
}
```

- `processed_sha256`: lista de sessões já varridas na fonte A — nunca reprocessar.
- `last_export_timestamp`: data do export mais recente já lido da fonte B, para avisar o dono quando ele trouxer um export mais velho que o já processado.
- `pattern_counters`: sinal que já apareceu uma vez ([OP]/[TERCEIRO]) mas ainda não teve uma segunda ocorrência independente — guardado aqui até cruzar a barra de confirmação ou ser descartado por obsolescência (a critério do dono, nunca automaticamente).

Este arquivo não é uma página do brain (`brain/`) e não carrega o frontmatter de `bundles/base/brain-contract.md` — é estado de máquina puro, no mesmo espírito de `brain/memory/sharepoint-config.json`.
