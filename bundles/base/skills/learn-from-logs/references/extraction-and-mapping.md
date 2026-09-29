# Seleção e extração com escopo explícito

Antes de ler corpos, o dono aprova uma allowlist de workspaces canônicos e dos
arquivos exatos de sessão ou export. O seletor não descobre fontes e não decodifica
pastas de Claude como prova de consentimento. A associação pasta/workspace é
explicitamente fornecida pelo dono. Não varrer ~/.claude/projects globalmente.

## Manifesto de seleção

Formato local ilustrativo; os caminhos são exemplos, não fontes autorizadas:

    {
      "consent": true,
      "workspaces": ["/canonical/approved-workspace"],
      "sources": [{
        "kind": "claude_code",
        "workspace": "/canonical/approved-workspace",
        "root": "/canonical/selected-project-logs",
        "sessions": [{"id": "selected-session", "path": "selected-session.jsonl"}]
      }]
    }

Para export usar kind claude_export e o JSON escolhido. A aprovação inclui o
arquivo inteiro, não só os trechos que uma leitura posterior selecionaria. Se o
arquivo contém material não autorizado, pedir um subconjunto produzido pelo dono.
ZIP não é suportado. O leitor aceita arrays de topo com até 20 conversas;
containers desconhecidos não são emitidos. Para formatos alternativos, o dono
fornece um subconjunto explícito nesse formato antes da extração. Não presumir
que objeto de topo significa uma única conversa nem contornar a recusa.

1. Rodar python3 bundles/base/tools/log-selection.py prepare com o manifesto em stdin.
   Em Windows, usar apenas um Python já aprovado; nunca instalar como requisito
   de uso do Maestro. O helper pode declarar leitura segura indisponível no host.
2. Guardar o plano retornado somente como controle local; ele contém os caminhos
   escolhidos e metadados, mas nenhum corpo. Mostrar a seleção/volume ao dono.
3. Depois da confirmação explícita, fornecer somente plan em stdin para
   python3 bundles/base/tools/log-selection.py read --approved-digest DIGEST.
   DIGEST é o valor retornado por prepare e aprovado, nunca um valor inventado.
4. O resultado tem sessions (corpos efêmeros), audit (metadados) e cursor.
   Persistir apenas audit/cursor, caso necessário; não salvar o resultado completo.
5. Para próximo lote, passar o mesmo plano e --resume N conforme cursor.next.
   O cursor está vinculado ao digest; não amplia a seleção nem a autorização.

Até 4 fontes/workspaces; 20 arquivos selecionados; 2 MiB por arquivo; 8 MiB total;
5 arquivos por lote. Os mesmos limites valem para JSONL e export. Dados acima do
limite requerem um subconjunto explícito em nova chamada, nunca aumento silencioso
do teto. Arquivos alterados/repostos depois do plano invalidam o consumo. Symlinks,
travessia, globs e diretórios não canônicos são recusados. A leitura usa descritores
sem seguir aliases; hosts sem esse mecanismo permanecem indisponíveis.

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

O audit separado contém apenas selection e read com session (identificador
opaco derivado), bytes e sha256. Não guardar pattern_counters com texto livre,
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

Os testes acceptance/test_log_selection.py exercitam o seletor determinístico.
Estes cenários documentam o contrato esperado; execução por agente em host nativo
continua não qualificada até receber evidência própria.
