---
contract_id: maestro-update-long-run-v2
schema_version: 2
machine_contract: UPDATE-CONTRACT.json
minimum_effort: high
permission_mode: host-native-and-user-selected
required_subagents:
  - yoda
progress_receipt: brain/.maestro/updates/update-0.2.0.json
---

# Atualização 0.2.0 — executar, verificar e retomar

Este runbook e `UPDATE-CONTRACT.json` são o contrato de execução do Maestro.
O pacote substitui receitas, agentes, hooks e ferramentas gerenciadas; não
substitui o Maestro pessoal do dono, nem inclui sua memória ou seus casos.

## Preparação da sessão

Abra a raiz da nova instalação no host escolhido, Claude Code ou Codex.
Revise a confiança dos hooks dessa pasta; não altere políticas corporativas.
Use um modelo de raciocínio forte disponível na organização e esforço alto
ou o maior disponível. No Claude, se o BCG oferecer Opus 4.7, ele pode ser
selecionado; confirme a versão efetiva na UI, sem presumir que está liberada.
No Codex, use um modelo Codex disponível: não tente selecionar Opus.

Auto mode, quando oferecido pelo host e escolhido pelo usuário, é modo de
permissão; não é modelo, nem licença para ignorar políticas. Se indisponível,
use o modo normal. Use goal/continuidade nativa quando o host oferecer esse
recurso; caso contrário execute o mesmo pedido com checkpoints em disco.
Não invente comandos de um host no outro.

## Máquina de execução

1. Leia o contrato JSON e o receipt existente. Confirme origem 0.1.11 ou
   0.1.12, destino 0.2.0, plataforma e host.
2. Antes de reutilizar um PASS, valide todas as bindings. O hash do núcleo
   exclui `data/`, `brain/` e os receipts de qualificação, mas inclui os
   arquivos gerenciados e a configuração efetiva do host. Hash do ZIP é
   conferido com o sidecar entregue independentemente. Hash não prova
   identidade do publicador nem substitui assinatura corporativa.
3. Receipt stale é preservado com seu attempt_id; gere nova tentativa.
   Retome pelo primeiro check não PASS da tentativa válida. Uma mudança de
   arquivo gerenciado, ZIP, origem, host ou raiz invalida a evidência anterior.
4. Compare TODOS os arquivos da `data/` original e copiada com o baseline:
   caminhos, tamanho e SHA-256, inclusive arquivos ocultos. Na 0.2.0 a origem
   fica intacta: não há exclusão genérica de arquivos modificados.
5. Consulte `maestro-runtime migration --project <raiz> --status` usando o
   wrapper verificado da plataforma. Exija `committed` para migração legada.
   Confira source_fingerprint, target_fingerprint, plan_sha256 e receipt.
   Uma árvore dupla sem plano válido, falha parcial ou receipt adulterado não é PASS.
   Se verification=target_evolved e current=false, preserve a edição autoral: a
   migração histórica continua válida, mas a nova qualificação precisa verificar
   o estado final e o runtime de novo. Não force recópia nem reescreva o recibo.
   Topologia insegura, symlinks/escapes e erros de leitura continuam bloqueantes.
6. Prove leitura de cada namespace preservado, inclusive agentes e workspaces
   legados, pelo mapa validado da migração. Arquivo preservado mas invisível
   para seus consumidores é atualização incompleta.
7. Reconcile personalizações fora de data/brain: agentes próprios, hooks,
   instruções e integrações locais. Preserve a cópia antiga e faça comparação
   explícita; nunca copie o núcleo antigo por cima do novo nem descarte
   personalizações silenciosamente. Não copie credenciais para o pacote.
8. Confira integridade do runtime nativo e rode doctor. Windows usa PowerShell
   5.1/7 e o helper incluído; não requer Bash, Python ou Go instalado. Mac usa
   Bash e o helper incluído; ferramentas Python são diagnosticadas separadamente.
9. Compare todos os handlers da configuração efetiva com o manifesto do
   pacote. Claude e Codex têm configurações e formatos nativos diferentes.
   Colete prova da sessão real de início, prompt, proteção de escrita,
   despacho e término. Executar um script diretamente prova só
   `adapter_command`, não invocação pelo host.
10. Faça uma chamada real ao Yoda pelo mecanismo nativo de subagents do host,
    envie um ReviewPacket com `review_type: delivery`, aguarde o retorno tipado
    e registre o vínculo à tentativa. `VERDICT: approved` é necessário para
    yoda_review=PASS; refine-and-return/missing-the-mark pedem correção, hold
    mantém o bloqueio. Um intent/approve não substitui revisão de entrega. Arquivo de agente,
    TOML/YAML, leitura da persona ou resposta simulada não contam.
11. Encaminhe ao Yoda o pacote final de evidências. Corrija achados acionáveis,
    repita os testes afetados e obtenha nova revisão quando o conteúdo mudar.
12. Registre os checks obrigatórios e opcionais separadamente. caseOS depende
    de URL oficial, acesso e autenticação; sua ausência não impede a atualização
    local. A skill caseos-connect conduz a instalação e o uso ativo autorizado.
    Python adicional é instalado somente quando uma tarefa o exige, pelo fluxo
    gerenciado e consentido; nunca como dependência oculta dos hooks Windows.

## Continuidade e encerramento

Persista checkpoint após cada etapa e antes de delegar: tentativa, bindings,
checks concluídos, evidências, próximo passo e bloqueio objetivo. Continue
enquanto houver trabalho seguro e acionável; um plano ou resumo de progresso
não é conclusão. Use subagents com tarefas independentes e contexto mínimo;
o Maestro continua responsável pela integração e pela conclusão.

O receipt usa `status: in_progress|pass|fail|unavailable` e cada check usa
`PASS|FAIL|UNAVAILABLE`. PASS exige todos os checks obrigatórios aprovados
na mesma tentativa válida, retorno real do Yoda e provas do host/plataforma
selecionados. Registre evidência local, fixture, adapter_command e sessão
nativa com rótulos distintos. Nunca promova um teste simulado a nativo.

Interrupção, compactação ou limite do provedor não apagam o checkpoint:
reabra a mesma pasta e peça para retomar este contrato. Nenhum Markdown pode
obrigar o provedor a ultrapassar limites, permissões ou disponibilidade.
Uma ação externa necessária gera UNAVAILABLE com o próximo passo exato,
não PASS inventado, loop infinito ou instalação improvisada.

## Reversão sem perda

Não apague a instalação anterior. Rollback da tentativa revoga sua ativação,
preserva as duas árvores e declara `restored_runtime: false`; a restauração
do runtime exige reabrir a instalação anterior e validá-la. Alterações feitas
na pasta nova depois da cópia também devem ser preservadas. Mantenha o backup
por pelo menos sete dias após PASS. Apagar backup nunca faz parte automática
do update.
