# Teste guiado do Maestro MCP

Use somente dados sintéticos. Você precisa colar **um texto no Code e outro no Desktop**; o launcher preenche o workspace e encontra o trabalho automaticamente.

1. Abra **Testar Maestro** na pasta `launchers` do ensaio. O verificador testa a conexão MCP do pacote, confere os recursos e abre `RELATORIO.md`. Nenhuma chamada ao modelo é feita nessa etapa.
2. Se o relatório estiver bloqueado, envie esse Markdown ao time e pare. Preserve a pasta; o relatório distingue preparação, pacote e conexão. Se o comando `claude` estiver ausente, disponibilize o Code pelo fluxo normal antes de escolhê-lo; isso não comprova ausência do Desktop.
3. Escolha **1 — Code**. O texto do ensaio vai para a área de transferência e o perfil isolado abre. Faça login normalmente se solicitado e cole o texto uma vez. Ele verifica o ID, inicia o onboarding, descobre uma skill, propõe uma página sintética e pausa um trabalho. Confirme somente pelo fluxo apresentado pelo cliente.
4. Abra **Testar Maestro** novamente e escolha **2 — Desktop**. Confirme o MCPB e os campos `Owner directory` e `Workspace` de `COMECE-AQUI.md`. Se existir outra extensão `maestro-product`, pare antes de substituir. Cole o texto já copiado: ele confere o mesmo workspace, localiza exatamente um trabalho do ensaio e só retoma se estiver pausado. Não precisa copiar IDs ou revisões.
5. Feche e reabra a conversa/conexão. Peça a leitura do mesmo trabalho e da página criada. Registre se o conteúdo persistiu. Proponha uma memória sintética para aceitar e outra para recusar; verifique os resultados pelo fluxo real de confirmação.

O relatório automático mantém **Code, Desktop, login, confirmações e hooks como não testados**. A conexão feita pelo verificador não atesta que o cliente carregou o plugin. Anote o que você observou nesses clientes.

Para devolver o teste, envie somente o `RELATORIO.md` mais recente, com este complemento curto:

- Versões Code/Desktop e sistema/arquitetura.
- Code: abriu / MCP respondeu / skill funcionou / não testado.
- Desktop: importou / MCP respondeu / retomou o mesmo trabalho / não testado.
- Página e trabalho após reinício: passou / falhou / não testado.
- Memória aceita e recusada, hooks e mensagem de erro: resultado real.

Confira o Markdown antes de compartilhar. Preserve `owner`, `workspace`, perfil Code, prompts e pacotes locais na sua máquina. O ZIP genérico é o material de distribuição; o ensaio preparado contém dados e caminhos locais.
