# Ensaio Windows — #431

Este pacote de desenvolvimento prepara o ensaio sintético no Windows
corporativo. F0 permanece **ABERTO**. Testes Go, compilação cruzada, parser
PowerShell e CI hospedado não encerram #431. Use uma sessão comum, sem
elevação, do PowerShell 5.1 ou 7, um checkout aprovado e Go já instalado.

## Ensaio direto

Execute da raiz do checkout. Escolha um caminho físico novo sob
`USERPROFILE\Developer`; o runner exercita espaços e Unicode.

```powershell
& ./dev/spikes/plugin429/windows-rehearsal.ps1 -Destination "$env:USERPROFILE/Developer/CodexWorkspace/Windows Plugin Spike"
```

O script executa somente os testes do spike, compila os dois helpers nativos,
verifica o manifesto de hooks, envia eventos sintéticos e grava
`windows-rehearsal.json` nesse destino. O ambiente do processo e o diretório
atual são restaurados ao terminar. Destinos existentes e ancestrais com
reparse points são rejeitados para destino, temporários e cache. Uma falha
preserva os arquivos parciais. Use outro destino para repetir o ensaio ou
trocar a edição do PowerShell.

Resultados esperados do ensaio direto:

- Os dois helpers em `versions/0.0.1/maestro-spike/scripts/` têm extensão `.exe`.
- As quatro tools de escrita negam o marker dedicado; outro nome é permitido.
  Este guard do marker é um spike e não substitui o isolamento entre casos.
- JSON inválido é negado; o log sintético de Agent/Task retém só `tool_name`.
- SessionStart direto encontra o runtime `.exe` e relata preflight
  `non_elevated`, `native_qualification=unattested`.
- O recibo registra commit, versões Go/PowerShell e hashes dos helpers;
  Code tab, aceite corporativo, pasta sincronizada e reload ficam `NOT_RUN`.

Um terminal elevado ou runner hospedado pode falhar no preflight existente.
Preserve o diagnóstico nativo e a revisão; repita em host aprovado sem
elevação. Bloqueios de execution policy, AppLocker/WDAC, antivírus, SmartScreen
ou acesso devem ser registrados. Preserve as proteções existentes.
O script não instala software, autentica, chama modelos, altera perfis de
plugins, migra dados nem escreve no state-home do produto.

O workflow manual `Plugin spike Windows rehearsal` executa o mesmo pacote em
Windows hospedado nas duas edições do PowerShell. Usa nenhum segredo e envia
somente o recibo JSON sintético. Falha de preflight mantém o job reprovado;
ausência de recibo não é sucesso. Ele não instala Claude nem observa um
dispositivo corporativo. Definição do workflow, execução hospedada e aceite
do host são evidências independentes.

## Pacote assistido no Code tab

Após o ensaio direto, registre versão/build do Windows, arquitetura, edição
PowerShell, versão do aplicativo, versão Claude Code e hashes do recibo.
Use somente workspace sintético, caminhos aprovados e a UI de plugins
suportada pelo host. Com o registro do plugin aprovado, execute os comandos
abaixo em um terminal separado, com diretório novo de configuração isolada:

```powershell
$env:CLAUDE_CONFIG_DIR = "$env:USERPROFILE/Developer/ToolData/maestro-spike-windows-cli"
claude --version
claude plugin validate '<marketplace-path>'
claude plugin validate '<marketplace-path>/versions/0.0.1/maestro-spike'
claude plugin marketplace add '<marketplace-path>'
claude plugin install maestro-spike@maestro-spike
```

Registre se o Code tab realmente usa esse perfil; instalação CLI isolada não
comprova ativação no desktop. Se o host corporativo não puder selecionar o
plugin/perfil de ensaio, registre o bloqueio e encerre essa etapa.
Siga o [ensaio assistido do README](README.md#ensaio-assistido-no-code-tab)
e registre a raiz/revisão ativa em cada observação: SessionStart, skill com
namespace, Write negado e Write de controle permitido, nome real Agent/Task,
e revisão da skill em uma segunda sessão nova sem update. Ausência do arquivo
sozinha não comprova negação despachada. Registre pasta sincronizada e
controles de execução somente em caminho real aprovado; uma pasta local sob
Developer não comprova essas propriedades. Preserve arquivos do owner e de
clientes; remova o plugin pelo fluxo normal ao concluir o ensaio.

## Comparação literal Bash/PowerShell — somente no artefato de ensaio

O builder nativo Windows também grava `hooks/compare-shells.json` e as twins
`scripts/compare-shells.sh` / `scripts/compare-shells.ps1`. O manifesto padrão
`hooks/hooks.json` continua com execução nativa direta. O arquivo alternativo
declara cada modo duas vezes, com `shell: bash` e `shell: powershell`, sem
`args`, para comparar os shells de fato. O campo `shell` seleciona o shell do
handler; ele não seleciona o sistema operacional. A [referência oficial dos
campos de hooks](https://code.claude.com/docs/en/hooks#command-hook-fields),
verificada em 2026-10-21, especifica esses dois valores e informa que `args`
faria o host ignorar `shell`.

Construa outro destino novo com o runner; este pacote será exclusivo da
comparação assistida. Git Bash já disponível é requisito dessa comparação,
não do manifesto nativo padrão. Registre a falta de um shell ou bloqueio de
policy como resultado; preserve as proteções e instalações do host.

```powershell
& ./dev/spikes/plugin429/windows-rehearsal.ps1 -Destination "$env:USERPROFILE/Developer/CodexWorkspace/Windows Plugin Comparison"
```

Ative a variante somente na cópia recém-gerada, com as sessões do ensaio
fechadas. O exemplo abaixo usa um caminho fixo sob Developer: ajuste o nome
apenas para o destino novo que acabou de construir. Preserve o terminal para
conferir o hash na restauração.

```powershell
$plugin = Join-Path $env:USERPROFILE 'Developer/CodexWorkspace/Windows Plugin Comparison/versions/0.0.1/maestro-spike'
$active = Join-Path $plugin 'hooks/hooks.json'
$backup = Join-Path $plugin 'hooks/native-exec.saved.json'
if (Test-Path -LiteralPath $backup) { throw 'Backup já existe; preserve e inspecione esse ensaio antes de repetir.' }
$nativeHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $active).Hash
Copy-Item -LiteralPath $active -Destination $backup
Copy-Item -LiteralPath (Join-Path $plugin 'hooks/compare-shells.json') -Destination $active -Force
```

Registre esse marketplace novo no perfil isolado aprovado, conforme o pacote
assistido acima. Confira a raiz que o host realmente selecionou; se selecionar
uma cópia em cache antiga, o resultado fica inconclusivo até o fluxo suportado
carregar o artefato correto. Abra uma sessão nova e examine `/hooks` e os
diagnósticos do host. Para cada evento observado, registre as duas ocorrências
e seus exit codes. As wrappers emitem IDs fixos no stderr, separados do stdout
do helper:

| Modo | Handler Bash | Handler PowerShell |
|---|---|---|
| SessionStart | `MAESTRO_SPIKE_HANDLER=bash/session-start` | `MAESTRO_SPIKE_HANDLER=powershell/session-start` |
| Escrita | `MAESTRO_SPIKE_HANDLER=bash/guard` | `MAESTRO_SPIKE_HANDLER=powershell/guard` |
| Delegação | `MAESTRO_SPIKE_HANDLER=bash/delegation` | `MAESTRO_SPIKE_HANDLER=powershell/delegation` |

Espere ambos os handlers de SessionStart ao iniciar; repita a tentativa de
Write no marker, o Write de controle e a delegação inofensiva. O helper deve
manter os mesmos exit codes e conteúdo; os IDs identificam a wrapper que
iniciou, enquanto exit code e saída do helper comprovam a conclusão. Um ID
sozinho não comprova stdin correto, negação ou sucesso do helper. Não presuma
ordem de execução: os handlers podem rodar em paralelo, e os dois handlers de
delegação podem registrar a mesma tool. Capture ambos com diagnósticos nativos
limitados ao workspace sintético; não colete prompts nem conteúdo de clientes.

Depois do ensaio, feche suas sessões e restaure **o arquivo do artefato de
comparação**, conferindo o hash. Preserve o backup e as evidências:

```powershell
Copy-Item -LiteralPath $backup -Destination $active -Force
if ((Get-FileHash -Algorithm SHA256 -LiteralPath $active).Hash -cne $nativeHash) { throw 'Hash divergente; preserve os arquivos e inspecione antes de reabrir.' }
```

O teste local das wrappers usa um helper de teste que ecoa bytes e termina
com 0 ou 2, em caminhos com espaços e Unicode. Isso verifica o transporte e
os IDs. Um probe adicional no CLI 2.1.233/macOS despachou ambos os handlers
de SessionStart com `shell` explícito, IDs distintos, stdout do helper
preservado e exit 0; depois falhou por falta de autenticação, com custo zero.
Esse probe usou uma fixture separada com helper macOS nativo copiado para o
nome `.exe`, sem modificar artefatos registrados. A comparação dupla real no
Windows, PowerShell 5.1 e Code tab continua `NOT_RUN` até a observação assistida.

## Contrato de launch e limites

O pacote Windows é gerado no Windows e recebe um manifesto nativo separado.
Cada handler usa `command` apontando ao `.exe` empacotado e `args: [mode]`.
O executável recebe stdin JSON e preserva stdout/exit code; dispensa wrappers
de hook em Bash, Python, Node ou PowerShell. PowerShell orquestra este ensaio.

A [referência oficial de hooks](https://code.claude.com/docs/en/hooks#exec-form-and-shell-form),
verificada em 2026-10-20, especifica execução direta com `args`, substituição
literal de placeholders e executáveis reais no Windows. O pacote usa apenas
campos documentados. A versão corporativa instalada ainda precisa demonstrar
esse contrato; a documentação não certifica versão mínima nem sessão nativa.
O macOS mantém seu manifesto shell-form existente.

Uma fixture macOS isolada no CLI instalado 2.1.233 despachou corretamente
`args: ["session-start"]`, antes de a autenticação do modelo falhar com custo
zero. Isso reduz a incerteza de versão somente no macOS. A fixture Windows
passou em `plugin validate`, cuja validação de manifesto não comprova parsing
dos hooks nem execução Windows. Consulte [RESULTS.md](RESULTS.md).
