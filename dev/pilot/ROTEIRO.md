# Teste guiado do Maestro MCP

Use somente dados sintéticos. Para conferir cada cliente de forma independente e somente leitura, escolha **3 — Conferir conexão Code** ou **4 — Conferir conexão Desktop**. Para o ensaio completo de continuidade, escolha **1 — Ensaio completo no Code** e depois **2 — Retomar no Desktop**: um texto em cada cliente, com workspace e trabalho preenchidos automaticamente.

Para conferir apenas a conexão no Code, abra **Testar Maestro** e escolha **3 — Conferir conexão Code (somente leitura)**. O perfil separado pode pedir login: conclua o fluxo normal nessa janela e depois cole uma vez o texto copiado. A resposta deve mostrar o workspace esperado e o estado `ready`, obtidos por uma única leitura do MCP declarado pelo plugin. O estado `ready` descreve a saúde da conexão, não o estado dos trabalhos. Você pode repetir essa consulta preservando trabalhos, páginas, onboarding e memória. Se o login, o plugin ou a leitura falhar, envie a mensagem real; abrir o cliente ou ter login em outro perfil não comprova esta conexão.

Para conferir apenas a conexão no Desktop, escolha **4 — Conferir conexão Desktop (somente leitura)**. Confirme a instalação da extensão **Maestro MCP — Ensaio …** indicada, abra uma conversa nova e cole uma vez o texto copiado. Ele faz uma única leitura de contexto e mostra o workspace esperado e a saúde da conexão, sem consultar ou retomar trabalhos. O Desktop não exige o comando Code nem um trabalho criado no Code para essa conferência. Os pacotes já usam as pastas aprovadas; não há diretórios para copiar. Preserve outras extensões e pare se o cliente propuser substituir uma de outro ensaio.

1. Abra **Testar Maestro** na pasta `launchers` do ensaio. O verificador testa a conexão MCP do pacote, confere os recursos e abre `RELATORIO.md`. Nenhuma chamada ao modelo é feita nessa etapa.
2. Se o relatório estiver bloqueado, envie esse Markdown ao time e pare. Preserve a pasta; o relatório distingue preparação, pacote e conexão. Se o comando `claude` estiver ausente, disponibilize o Code pelo fluxo normal antes de escolhê-lo; isso não comprova ausência do Desktop.
3. Escolha **1 — Ensaio completo no Code**. O texto do ensaio vai para a área de transferência e o perfil isolado abre. Faça login normalmente se solicitado e cole o texto uma vez. Ele verifica o ID, inicia o onboarding, descobre uma skill, propõe uma página sintética e pausa um trabalho. Confirme somente pelo fluxo apresentado pelo cliente.
4. Abra **Testar Maestro** novamente e escolha **2 — Retomar no Desktop**. Confirme a instalação da extensão **Maestro MCP — Ensaio …** indicada no relatório; ela já usa as pastas preparadas, sem campos de caminho para salvar. Preserve outras extensões Maestro e pare se o cliente propuser substituir outra extensão. Abra uma conversa nova e cole o texto já copiado: ele precisa responder pelo mesmo workspace, localizar exatamente um trabalho do ensaio e só retomar se estiver pausado. Se a extensão estiver desativada ou não aparecer, confira **Configurações → Extensões → Configurar → Ativado** nessa extensão. A chave ativada, sozinha, não confirma a conexão. Não precisa copiar IDs ou revisões.
5. Feche e reabra a conversa/conexão. Peça a leitura do mesmo trabalho e da página criada. Registre se o conteúdo persistiu. Proponha uma memória sintética para aceitar e outra para recusar; verifique os resultados pelo fluxo real de confirmação.

O relatório automático mantém **Code, Desktop, login, confirmações e hooks como não testados**. A conexão feita pelo verificador não atesta que o cliente carregou o plugin. Anote o que você observou nesses clientes.

Para devolver o teste, envie somente o `RELATORIO.md` mais recente, com este complemento curto:

- Versões Code/Desktop e sistema/arquitetura.
- Code: abriu / MCP respondeu / skill funcionou / não testado.
- Desktop: importou / MCP respondeu / retomou o mesmo trabalho / não testado.
- Página e trabalho após reinício: passou / falhou / não testado.
- Memória aceita e recusada, hooks e mensagem de erro: resultado real.

Confira o Markdown antes de compartilhar. Preserve `owner`, `workspace`, perfil Code, prompts e pacotes locais na sua máquina. O ZIP genérico é o material de distribuição; o ensaio preparado contém dados e caminhos locais.
