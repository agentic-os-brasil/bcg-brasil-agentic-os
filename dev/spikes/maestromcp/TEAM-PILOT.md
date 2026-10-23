# Piloto de equipe: publicação sincronizada, instalação local

Este piloto distribui o experimento **sintético** do Maestro por uma pasta
local, que pode ser entregue aos colegas pela sincronização de uma biblioteca
SharePoint/OneDrive. O instalador é um executável nativo: os participantes não
precisam instalar Go, Python ou Node. O piloto não migra dados existentes e não
autoriza uso de casos reais, produção ou distribuição corporativa irrestrita.

Ainda é necessário escolher a biblioteca e qualificar sincronização, políticas
da organização, aceitação dos binários e instalação real em cada plataforma.
Nenhuma biblioteca SharePoint foi criada ou escolhida por estes comandos.

## Experiência do participante

O publicador entrega um instalador com a chave pública do piloto embutida,
por um canal de bootstrap cuja origem possa ser conferida. A pessoa sincroniza
a pasta da sua plataforma, mantém os arquivos disponíveis localmente e executa
**Instalar Maestro.command** (macOS) ou **Instalar Maestro.cmd** (Windows).
Também pode executar o instalador diretamente. O caminho `CANAL_LOCAL` abaixo é o caminho da pasta sincronizada
na própria máquina, nunca uma URL do navegador:

```sh
maestro-team-pilot install --feed "CANAL_LOCAL" --activate-code --open-desktop
```

O instalador verifica a publicação assinada, copia os artefatos para uma pasta
local separada e prepara plugin, extensão e estado sintético. A pasta padrão
é `~/Developer/Maestro Team Pilot`; `--home` permite escolher outro destino
admitido pelas validações do piloto. O estado fica fora dos pacotes e da pasta
compartilhada. O instalador recusa publicações inválidas ou incompletas antes
de chamar os clientes.

`--activate-code` chama os comandos oficiais `claude plugin marketplace add`
e `claude plugin install` na instalação inicial; em `update`, chama
`claude plugin update` no marketplace já registrado. Usa argumentos separados,
sem editar arquivos
internos do Claude nem alterar suas políticas. Requer o CLI `claude` disponível
no PATH e as permissões normais do ambiente. `CODE_INSTALL_COMMANDS_SUCCEEDED` significa que os
comandos retornaram sucesso; confirme o carregamento em nova sessão ou reload
e execute uma ferramenta MCP antes de considerar o cliente qualificado.

`--open-desktop` abre o `.mcpb` no Claude em macOS, ou pelo aplicativo associado
no Windows. `DESKTOP_CONFIRMATION_REQUIRED` significa que **a pessoa ainda
precisa confirmar a instalação na interface**. Abrir um arquivo não comprova
instalação, aceitação pela política ou chamada de ferramenta. Não há troca
silenciosa de extensões nem manipulação do diretório interno do Desktop.

É possível omitir os dois flags para somente preparar arquivos verificados,
sem executar comandos dos clientes. Se um comando do cliente falhar, os
arquivos preparados continuam disponíveis e o resultado registra a falha.

Depois que uma publicação nova terminar de sincronizar:

```sh
maestro-team-pilot update --feed "CANAL_LOCAL" --activate-code --open-desktop
maestro-team-pilot status
```

O launcher local **Atualizar Maestro.command** (macOS) ou **Atualizar Maestro.cmd**
(Windows), gerado na pasta de instalação, mantém o caminho do canal. Repetir uma
publicação já aplicada é idempotente quanto aos artefatos. Os flags de ativação
são explícitos e podem voltar a abrir o cliente. Feche ou recarregue a sessão
do Code para verificar a versão carregada; confirme a atualização no Desktop.
`status` descreve arquivos locais, sem consultar contas ou atestar atividade
dos clientes; o resultado mantém `HOST_RUNTIME_UNATTESTED`. **Não há polling automático nem promessa de atualização em uma
conversa já aberta.**

## Preparação pelo publicador

Use uma chave exclusiva deste piloto. Guarde a chave privada em uma pasta local
em Developer fora do canal sincronizado. `keygen` cria arquivos novos com modo
0600, nunca sobrescreve e nunca imprime a chave privada:

```sh
team-release keygen --private-key "PRIVATE_LOCAL/pilot-private.hex" --public-key "PRIVATE_LOCAL/pilot-public.hex"
```

Os diretórios de chaves devem existir e os caminhos devem ser absolutos. O
publicador compila `cmd/team-install` para cada plataforma com a chave pública
embutida em `main.TrustedKeyHex`, além do servidor nativo em `cmd/server` e do
publicador `cmd/team-release`. As ferramentas de compilação são necessárias
somente nessa etapa de engenharia. Não coloque a chave privada no repositório,
em comandos que a imprimam, nos pacotes ou no SharePoint.

Cada plataforma tem seu canal. Gere primeiro a publicação em uma pasta de
staging fisicamente sob Developer, fora da pasta de chaves. Publique versão e
sequência crescentes:

```sh
team-release publish --key "PRIVATE_LOCAL/pilot-private.hex" --feed "CANAL_LOCAL" --version 0.0.1 --sequence 1 --target darwin-arm64 --server "BIN_LOCAL/maestro-mcp-spike" --installer "BIN_LOCAL/maestro-team-pilot"
```

Depois copie ou envie o conteúdo do canal de staging para a biblioteca
SharePoint escolhida, mantendo as versões anteriores. Envie os artefatos
completos antes do novo `feed.json`, que é o ponteiro de promoção assinado.
O publicador deste experimento exige destino físico sob Developer e não altera
serviços ou pastas sincronizadas do sistema. A sincronização com os colegas é
feita pelo OneDrive após essa publicação. A conclusão
do comando **não comprova** que outro computador recebeu os arquivos. Libere o
teste aos colegas depois de conferir a sincronização e ensaiar a instalação
com a cópia que eles recebem. Validação incompleta deve produzir falha, sem
ativar uma versão parcialmente recebida.

## Confiança e limites

A assinatura Ed25519 detecta modificações no feed com base na chave já embutida
no instalador. O instalador nunca aprende uma chave nova da pasta compartilhada.
Isso depende de um bootstrap confiável: copiar um executável e uma chave da
mesma pasta sem conferir sua origem não resolve essa etapa.

Esses binários experimentais não possuem, por esta implementação, assinatura
Apple Developer ID, notarização ou Authenticode corporativo. A assinatura do
manifesto não substitui assinatura de sistema operacional. Respeite os prompts
e bloqueios normais; não remova quarentena, não desative Gatekeeper/Defender,
não altere allowlists e não use esta chave de piloto como autoridade de produção.

A qualificação precisa separar: publicação local; sincronização; verificação;
arquivos aplicados; instalação dos clientes; versão carregada; chamada MCP;
continuidade do estado. O experimento anterior já comprovou continuidade local
Code → Desktop → rejeição de escrita antiga; isso não atesta este novo canal
SharePoint nem o ambiente de cada participante. Registre os resultados do
piloto sem dados reais, credenciais ou conversas privadas.

Referências primárias: [marketplace local e instalação](https://code.claude.com/docs/en/plugin-marketplaces),
[comando de atualização e reload](https://code.claude.com/docs/en/discover-plugins#keep-plugins-updated).
