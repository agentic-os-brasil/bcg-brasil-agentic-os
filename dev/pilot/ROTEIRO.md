# Roteiro curto — um ensaio, dois clientes

Use somente material sintético. O preparador não faz chamadas de modelo; os prompts abaixo usam sua conta normal no cliente.

## 1. Identificar o MCP e iniciar o onboarding

Abra o Code pelo launcher indicado em COMECE-AQUI.md. Informe os campos Owner directory e Workspace quando o plugin pedir. Copie o workspace_id de preparation.json.

> Use somente o Maestro deste ensaio. Chame maestro_context, sem alterar nada, e mostre o workspace_id. Compare com o valor que vou informar. Se não conseguir identificar o conector ou houver divergência, pare antes de escrever.

Esperado: o ID coincide. Ferramenta listada ou extensão ativada ainda não prova uma chamada funcionando.

> Usando a skill maestro-onboarding, leia o estado real deste ensaio e conduza a próxima etapa, uma pergunta por vez. Use um perfil profissional sintético “dev do piloto”. Antes de salvar qualquer dado, apresente a revisão e peça a confirmação pelo fluxo suportado. Se uma dependência estiver indisponível, explique e pare nessa etapa.

Esperado: o onboarding segue o estado persistido; reiniciar a conversa não reinicia etapas já confirmadas. Login do cliente é feito pelo usuário, nunca por uma ferramenta do Maestro.

## 2. Usar uma skill e uma página

> Descubra o método Pocock de code review deste Maestro e leia suas instruções. Faça uma revisão curta deste exemplo sintético: uma função divide um total pela quantidade sem verificar quantidade zero. Depois proponha criar uma página no segundo cérebro com título “Ensaio de revisão” e o marcador literal MAESTRO_PAGE_READBACK. Peça confirmação antes de criar. Não use conectores externos.

Confirme pelo fluxo apresentado. Peça a leitura da página pelo identificador real retornado. Esperado: o marcador está no readback. Se o cliente não oferecer a confirmação necessária, registre indisponibilidade; não aprove por argumentos inventados.

## 3. Pausar no Code

> Crie um único trabalho sintético chamado “Ensaio de continuidade do Maestro”, com próximo passo “Retomar no Desktop”. Inicie, registre um checkpoint com resumo exato MAESTRO_PILOT_CONTINUITY e pause. Mostre item_id, revisão e estado. Se esse trabalho já existir, leia-o e não crie outro.

Esperado: o item fica paused. Anote item_id e revisão; confirme a página/memória somente quando o fluxo pedir.

## 4. Retomar no Desktop

Abra o MCPB pelo launcher e preencha os mesmos Owner directory e Workspace. Se já houver outra extensão maestro-product, pare para preservar seu outro ensaio.

> Use somente a extensão Maestro deste ensaio. Chame maestro_context e confira o workspace_id que informarei. Leia o item_id que informarei. Se estiver paused, na revisão esperada e com resumo MAESTRO_PILOT_CONTINUITY, retome uma única vez usando essa revisão. Se algum dado divergir ou o conector for ambíguo, pare e explique.

Esperado: o mesmo item passa a running com revisão sucessora e nova tentativa; o checkpoint permanece. Não repetir resume automaticamente.

Feche e reabra a conexão, depois peça a leitura do mesmo item e da página. Esperado: os mesmos IDs e conteúdo persistem.

## 5. Memória aceita e recusada

> Proponha salvar a memória sintética “No ensaio do Maestro, o próximo passo permanece explícito”. Mostre a revisão para minha confirmação antes de salvar.

Aceite uma vez. Reinicie a conexão e peça a leitura pelo identificador retornado. Em seguida proponha outra captura e **recuse**.

Esperado: a aceita pode ser lida; a recusada não aparece. Uma confirmação textual em argumento do modelo não substitui a confirmação suportada.

## Retorno mínimo

- Kit/OS/arquitetura e versões dos clientes.
- Preparação: pronta / recusada / parcial.
- Code: plugin carregado / MCP respondeu / método produziu resultado.
- Desktop: pacote importado / MCP respondeu / mesmo workspace.
- Continuidade e página após reinício: passou / falhou / não testado.
- Memória aceita e recusada: resultado real.
- Mensagem de erro e etapa, sem pastas privadas ou documentos reais.

Instalação, importação, conexão e operação são observações separadas. O roteiro não atesta assinatura de produção, atualização automática, execução recorrente em segundo plano, guarda nativa ou paridade de capacidades indisponíveis.
