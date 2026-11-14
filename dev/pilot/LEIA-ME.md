# Maestro MCP — preparar, conferir e testar

Este kit prepara um ensaio novo com produto, estado privado, workspace e perfil Code separados. Você não precisa de Go, Git, Python ou acesso ao repositório. Precisa de Code e Desktop instalados; o perfil Code do ensaio pode pedir login pelo fluxo normal.

**É um artefato de engenharia sem assinatura de produção.** `ARTIFACT.txt` informa a revisão e os alvos; `KIT-ID.txt` e `SHA256SUMS` identificam os bytes. O servidor informa versão de release indisponível; `0.0.0` é o rótulo dos pacotes. Checksum não autentica o publicador.

## Começar

1. Extraia o ZIP inteiro para uma pasta local comum. No Windows, baixe do SharePoint/OneDrive e extraia fora da pasta sincronizada; reparse points e Files On-Demand, mesmo hidratados, não fazem parte deste fluxo.
2. Crie uma pasta local `Developer` dentro do seu perfil, caso ela ainda não exista.
3. No Mac, abra **Preparar Maestro.command**. No Windows x64, abra **Preparar Maestro.cmd**. Leia os caminhos apresentados e digite `PREPARAR` se forem os pretendidos.
4. O preparador confere automaticamente o ensaio e indica o **RELATORIO.md**. Se houver bloqueio, envie esse Markdown ao time e preserve a pasta; nenhuma chamada ao modelo é feita pela conferência.
5. Abra **Testar Maestro** na pasta `launchers` indicada. Escolha **Code** ou **Desktop**: o texto correto vai para a área de transferência. Cole uma vez na conversa do cliente. Comece pelo Code e depois retome no Desktop, como explica o **ROTEIRO.md**.

O relatório automático registra a conexão MCP que ele próprio testou. Login, carregamento e permissões dos clientes, confirmação de gravações e execução real dos hooks continuam sendo observações suas. **COMECE-AQUI.md** guarda os valores de Owner directory e Workspace quando o cliente pedir.

O Code abre com `--plugin-dir` e um `CLAUDE_CONFIG_DIR` específico deste ensaio. O preparador não instala plugins no seu perfil normal. Ao habilitar o plugin, preencha os campos pedidos com os valores de COMECE-AQUI.md.

No Desktop, confirme a importação local do MCPB e preencha os mesmos campos. **Se já houver outra extensão maestro-product, pare antes de substituir.** O preparador não altera o Desktop nem escolhe uma extensão existente por você.

Se o sistema, a política PowerShell ou o cliente recusar o arquivo, preserve a mensagem e pare. O kit não muda políticas, remove quarentena, faz login ou desativa controles. Autentique somente na interface normal do cliente.

## O que testar

- Descoberta das skills, incluindo os métodos Pocock, e onboarding profissional guiado.
- Criação de um trabalho, checkpoint, pausa, retomada no outro cliente e leitura após reinício.
- Memória aceita e memória recusada pelo fluxo real de confirmação.
- Criação e leitura de uma página sintética do segundo cérebro pelo MCP.

Os pacotes contêm os recursos canônicos; recursos listados não provam que suas dependências estejam disponíveis. O Code comum inclui as projeções de agentes. Hooks de contexto passivo só entram quando o publicador selecionou essa opção em LIFECYCLE-CONTEXT.txt; isso não qualifica bloqueio de ferramentas, delegação ou execução nativa. Desktop carrega recursos MCP, sem instalar hooks/agentes nativos do Code ou Desktop Skills.

Conversão sem runtime offline admitido, instalação/atualização assinada sem autoridade organizacional, capacidades nativas sem qualificação e jobs Darwin no Windows permanecem indisponíveis. Operações que não garantem preservar alterações concorrentes retornam pendência/manual; este ensaio não dispensa essa garantia.

## Repetir e recuperar

Abrir novamente o mesmo preparador confere os arquivos gerenciados e gera outro relatório; memória, páginas e checkpoints permanecem no estado privado. Você também pode repetir **Testar Maestro** diretamente. Cada execução guarda um novo relatório em `diagnostics`, sem sobrescrever o anterior.

Outra identidade de kit usa outra pasta. Destino estrangeiro e material alterado são preservados e recusados. Se a inicialização falhar após copiar o executável, o preparador guarda o diagnóstico e deixa **Testar Maestro** disponível na pasta parcial. Envie o relatório antes de tentar outra pasta; preserve a anterior. O kit não corrige permissões automaticamente.

Compartilhe somente o ZIP genérico. A pasta preparada contém estado e caminhos locais; não redistribua owner, workspace, perfil Code, MCPB ou plugin gerados.

Retorne o **RELATORIO.md**, após conferir seu conteúdo, e o resultado real nos clientes conforme o roteiro. Use “não testado” ou “indisponível” quando couber. Não envie credenciais, memória ou documentos reais.
