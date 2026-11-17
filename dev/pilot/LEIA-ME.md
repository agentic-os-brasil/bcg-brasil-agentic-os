# Maestro MCP — preparar, conferir e testar

Este kit prepara um ensaio novo com produto, estado privado, workspace e perfil Code separados. Você não precisa de Go, Git, Python ou acesso ao repositório. Tenha instalado o cliente que vai testar. O roteiro de continuidade usa Code e Desktop; a conexão básica no Desktop não exige o comando Code. O perfil Code do ensaio pode pedir login pelo fluxo normal.

**É um artefato de engenharia sem assinatura de produção.** `ARTIFACT.txt` informa a revisão e os alvos; `KIT-ID.txt` e `SHA256SUMS` identificam os bytes. No kit básico, o servidor não tem versão de release e `0.0.0` é o rótulo dos pacotes. O kit completo contém `RUNTIME.json`: sua versão e sua autoridade experimental expiram na data indicada. Essa chave tem validade máxima de 14 dias e não substitui assinatura da empresa. Checksum não autentica o publicador.

## Começar

1. Extraia o ZIP inteiro para uma pasta local comum e crie uma pasta `Developer` dentro do seu perfil, caso ela ainda não exista. No Windows, baixe do SharePoint/OneDrive e extraia fora da pasta sincronizada; reparse points e Files On-Demand, mesmo hidratados, não fazem parte deste fluxo.
2. No Mac, abra **Preparar Maestro.command**. No Windows x64, abra **Preparar Maestro.cmd**. Leia os caminhos e digite `PREPARAR` se forem os pretendidos. O preparador confere o ensaio e indica o **RELATORIO.md**. Se houver bloqueio, envie esse Markdown ao time e preserve a pasta; a conferência não chama o modelo.
3. Abra **Testar Maestro** na pasta `launchers` indicada. Para uma conferência rápida e somente leitura, escolha **3 — Conferir conexão Code** ou **4 — Conferir conexão Desktop**. No Code, a opção escolhida abre a conversa com o texto correto, sem copiar ou colar. No Desktop, o texto vai para a área de transferência: cole uma vez em uma conversa nova. Para o ensaio completo de continuidade, use **1 — Ensaio completo no Code** e depois **2 — Retomar no Desktop**, seguindo o **ROTEIRO.md**.

**Kit completo com conversor no Mac Apple Silicon:** o conversor já acompanha o kit. A preparação mostra uma segunda revisão com o destino privado, o workspace, os identificadores do conversor e a validade. Leia esse registro e digite `ATIVAR CONVERSOR` somente se concordar. No Mac, essa etapa não exige instalar Python separadamente nem baixar componentes. Recusar preserva a preparação parcial e não ativa o conversor; não apague essa pasta. Solicite orientação ou escolha um destino novo. O roteiro completo já inclui a conversão de um DOCX sintético preparado no workspace e a leitura do resultado nos dois clientes. `content.status` informa os formatos efetivamente admitidos; o kit não promete formatos apenas porque existem no pacote upstream. O kit Windows x64 usa a aquisição guiada descrita abaixo, com aprovações separadas para download e colocação; a validação real no Windows continua separada. Não use documentos reais neste teste.

Quando a autoridade expirar, preserve seus dados e solicite outro kit revisado. O kit não troca a instalação existente, não corrige datas, não renova confiança e não configura atualizações de produção. Mac Intel e Windows continuam com os respectivos limites de validação; este ZIP completo de Mac não instala um runtime nesses sistemas.

**Sete hooks nativos do Code — opcional:** primeiro use a opção **3**, conclua o login normal no perfil do ensaio e confira a conexão. Feche essa conversa antiga do Code e volte a **Testar Maestro → 5 — Qualificar os sete hooks do Code**. Essa ação executa testes com modelo e pode gerar cobrança. O comando exige uma estimativa escolhida por você para registrar o plano; não calcula uma cotação de preço nem garante um teto de cobrança. Leia o plano completo mostrado pelo menu e digite `QUALIFICAR` somente se concordar. Vazio ou outra resposta cancela. Após sucesso, reabra pelo fluxo comum: **Testar Maestro → 1** para o exercício ou **Abrir Code** para uma conversa vazia; o launcher seleciona o pacote qualificado e as pastas automaticamente. Não continue na conversa antiga. Se houver recusa, preserve o resultado e a mensagem; não apague nem repita automaticamente.

O percurso **1 → 2** verifica o estado e a continuidade do trabalho entre clientes; ele não qualifica os hooks nativos. Preparar, abrir Code normalmente e conferir a conexão também não executam a qualificação da opção 5. O Desktop continua sendo uma extensão MCP com recursos, sem instalar os sete hooks nativos do Code.

O relatório automático registra a conexão MCP que ele próprio testou. Login, carregamento e permissões dos clientes, confirmação de gravações e execução real dos hooks continuam sendo observações suas. **COMECE-AQUI.md** resume os percursos disponíveis. A conferência rápida mostra o workspace e o estado de saúde da conexão; não consulta nem altera o estado dos trabalhos.

**Code no Windows:** as opções automáticas 1 e 3 e a qualificação da opção 5 exigem a instalação nativa `claude.exe` no PATH, pelo fluxo normal autorizado. Instalações que só oferecem `.cmd`, `.bat` ou `.ps1` não recebem o prompt automático nem executam a opção 5. Como alternativa manual, abra **Abrir Code.cmd** sem argumentos e cole o texto `CODE.txt` (opção 1) ou `CODE-CONEXAO.txt` (opção 3) salvo no diagnóstico atual. A abertura comum continua disponível; o kit não instala nem altera o cliente.

O perfil Code deste ensaio é separado. Ao criar um perfil novo, o preparador
desativa somente nele a memória automática do cliente e os plugins sincronizados
da conta; a memória do Maestro continua disponível. Essa escolha aparece antes
de PREPARAR. Repetir a preparação preserva suas configurações e login existentes.

O Code abre com `--plugin-dir` e um `CLAUDE_CONFIG_DIR` específico deste ensaio. O preparador não instala plugins no seu perfil normal. O plugin já usa as pastas aprovadas na preparação, sem diretórios para preencher. Se aparecer uma solicitação de configurar outros caminhos, preserve a mensagem e pare.

Ao escolher Desktop em **Testar Maestro**, confirme a instalação da extensão **Maestro MCP — Ensaio …** indicada no relatório. Ela já usa as pastas aprovadas na preparação; não há caminhos para copiar ou campos de diretório para salvar. Antes de colar, abra **Configurações → Extensões → a extensão deste ensaio** e, somente na linha **Maestro context** (`maestro_context`), selecione **✓ / Sempre permitir**. Preserve as demais permissões. ✓ / Sempre permitir libera só a leitura; gravações de conteúdo exigem revisão exata no formulário nativo ou no navegador local selecionado neste ensaio. Abra uma conversa nova e cole o texto copiado. Se a extensão estiver desativada ou não aparecer, confira **Configurações → Extensões → Configurar → Ativado** nessa extensão. Preserve outras extensões Maestro e pare se o cliente propuser substituir uma de outro ensaio. A etapa termina quando o prompt responder com o workspace esperado, não apenas quando a chave aparecer ativada.

**Revisão do dono:** O Desktop deste ensaio e o Code do kit completo selecionam explicitamente a revisão local no navegador para gravações protegidas. Uma solicitação interativa abre o registro completo no navegador deste computador, mesmo quando o cliente oferece formulário nativo. Leia o conteúdo, marque a caixa inicialmente desmarcada e escolha Aprovar alteração exata ou Recusar. Fechar a aba, cancelar ou ultrapassar 150 segundos não aprova. Este canal local não qualifica formulários MCP, hooks ou bloqueio nativo. Se receber `confirmation_unavailable`, deixe a ação pendente, preserve a mensagem e envie o RELATORIO.md ao time. Não repita a chamada, não contorne o bloqueio nem use um sim no chat como aprovação. A permissão de leitura não aprova gravações.

Se o sistema, a política PowerShell ou o cliente recusar o arquivo, preserve a mensagem e pare. O kit não muda políticas, remove quarentena, faz login ou desativa controles. Autentique somente na interface normal do cliente.

## O que testar

- Descoberta das skills, incluindo os métodos Pocock, e onboarding profissional guiado.
- Criação de um trabalho, checkpoint, pausa, retomada no outro cliente e leitura após reinício.
- Memória aceita e memória recusada pelo fluxo real de confirmação.
- Criação e leitura de uma página sintética do segundo cérebro pelo MCP.

Os pacotes contêm os recursos canônicos; recursos listados não provam que suas dependências estejam disponíveis. O canon de agentes está disponível como recurso MCP; ativação nativa depende de qualificação separada. Hooks de contexto passivo só entram quando o publicador selecionou essa opção em LIFECYCLE-CONTEXT.txt; isso não qualifica bloqueio de ferramentas, delegação ou execução nativa. Desktop carrega recursos MCP, sem instalar hooks/agentes nativos do Code ou Desktop Skills.

Conversão sem runtime offline admitido, instalação/atualização assinada sem autoridade organizacional, capacidades nativas sem qualificação e jobs Darwin no Windows permanecem indisponíveis. Operações que não garantem preservar alterações concorrentes retornam pendência/manual; este ensaio não dispensa essa garantia.

## Repetir e recuperar

Abrir novamente o mesmo preparador confere os arquivos gerenciados e gera outro relatório; memória, páginas e checkpoints permanecem no estado privado. Você também pode repetir **Testar Maestro** diretamente. Cada execução guarda um novo relatório em `diagnostics`, sem sobrescrever o anterior.

Outra identidade de kit usa outra pasta. Destino estrangeiro e material alterado são preservados e recusados. Se a inicialização falhar após copiar o executável, o preparador guarda o diagnóstico e deixa **Testar Maestro** disponível na pasta parcial. Envie o relatório antes de tentar outra pasta; preserve a anterior. O kit não corrige permissões automaticamente.

Compartilhe somente o ZIP genérico. A pasta preparada contém estado e caminhos locais; não redistribua owner, workspace, perfil Code, MCPB ou plugin gerados.

Retorne o **RELATORIO.md**, após conferir seu conteúdo, e o resultado real nos clientes conforme o roteiro. Use “não testado” ou “indisponível” quando couber. Não envie credenciais, memória ou documentos reais.

## Kit Windows com aquisição guiada

Quando RUNTIME.json indicar Windows, o ZIP contém o servidor e os recursos do
Maestro, o registro público e uma receita assinada. Python, wheels, DLLs de
terceiros e o archive do conversor não são distribuídos nesse ZIP. O preparador
baixa as entradas públicas diretamente dos fornecedores somente após você ler
a revisão e digitar **BAIXAR CONVERSOR**. A revisão mostra URLs, hashes,
quantidades, bytes de entrada, reserva máxima, cache retido, tentativa e validade.
Não exige credenciais dos fornecedores e não aceita licenças em seu nome.

As tentativas são preservadas, limitadas a três, com retenção mais reserva de no
máximo 2 GiB. A revisão expira em até 30 minutos. Falha não dispara repetição nem
limpeza automática. Após a aquisição, uma segunda revisão completa pede
**ATIVAR CONVERSOR** para colocar o runtime e verificar os controles locais do
Windows. Recusar qualquer etapa preserva o estado e não autoriza a próxima.
A aquisição não executa código baixado; a colocação/qualificação é separada.
Só um resultado real no Windows comprova essa etapa; compilação e testes no Mac
não a atestam. Instalação e atualização corporativas continuam fora deste piloto.

No kit completo, mudanças protegidas do Code usam o navegador local escolhido
na preparação, mesmo quando o cliente anuncia formulários nativos. Leia o
registro atual/proposto, marque a caixa inicialmente vazia e aprove somente a
alteração exata. Fechar, recusar, cancelar ou deixar expirar não aprova nada.
