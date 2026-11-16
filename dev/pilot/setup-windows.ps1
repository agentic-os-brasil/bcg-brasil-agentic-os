param([string]$Root, [switch]$Review, [string]$ConfirmDigest)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'Maestro: este preparador requer Windows.' }
$nl = [string][char]10
function Fail([string]$Message) { throw "Maestro: $Message" }
function Write-New([string]$Path, [string]$Body) {
  $encoding = [Text.UTF8Encoding]::new($Path.EndsWith('.ps1', [StringComparison]::OrdinalIgnoreCase))
  $bytes = $encoding.GetPreamble() + $encoding.GetBytes($Body)
  $stream = [IO.File]::Open($Path, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
  try { $stream.Write($bytes, 0, $bytes.Length) } finally { $stream.Dispose() }
}
function Hash-Text([string]$Body) {
  $sha = [Security.Cryptography.SHA256]::Create()
  try { return ([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($Body)))).Replace('-', '').ToLowerInvariant() }
  finally { $sha.Dispose() }
}
$Kit = $PSScriptRoot
if ($env:MAESTRO_HOME) { Fail 'MAESTRO_HOME já está definido. O redirecionamento existente foi preservado.' }
function Verify-Kit {
  $script:KitID = (Get-Content -LiteralPath (Join-Path $Kit 'KIT-ID.txt') -Raw).Trim()
  $sums = Join-Path $Kit 'SHA256SUMS'
  if ($script:KitID -cnotmatch '^[0-9a-f]{64}$' -or (Get-FileHash -LiteralPath $sums -Algorithm SHA256).Hash.ToLowerInvariant() -cne $script:KitID) { Fail 'Identidade do kit divergente.' }
  foreach ($entry in Get-ChildItem -LiteralPath $Kit -Recurse -Force) {
    if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { Fail 'O kit contém reparse/cloud/link; extraia para uma pasta local comum.' }
  }
  foreach ($line in Get-Content -LiteralPath $sums) {
    if ($line -cnotmatch '^([0-9a-f]{64})  \./([^\\:]+)$') { Fail 'Inventário inválido.' }
    $expected = $Matches[1]; $relative = $Matches[2]
    if ($relative -match '(^|/)\.\.?(/|$)|[\x00-\x1f]') { Fail 'Caminho inválido no inventário.' }
    $path = Join-Path $Kit $relative
    if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -cne $expected) { Fail 'Conteúdo do kit mudou; preserve os arquivos.' }
  }
}
Verify-Kit
$ReviewedKitID = $KitID
$RuntimeMode = $false
$PilotVersion = '0.0.0'
$BinaryRelative = 'product/maestro-mcp.exe'
if (Test-Path -LiteralPath (Join-Path $Kit 'RUNTIME.json')) {
  $runtime = Get-Content -LiteralPath (Join-Path $Kit 'RUNTIME.json') -Raw | ConvertFrom-Json
  if ($runtime.schema_version -ne 1 -or $runtime.profile -cne 'engineering-runtime-pilot-v1' -or $runtime.target -cne 'windows-amd64' -or $runtime.acquisition -ne $true -or $runtime.version -cnotmatch '^[0-9]+\.[0-9]+\.[0-9]+$') { Fail 'Este kit não oferece aquisição Windows autenticada; nenhum ensaio foi criado.' }
  $RuntimeMode = $true
  $PilotVersion = $runtime.version
  $BinaryRelative = 'product/server/maestro-mcp.exe'
}
$dev = [IO.Path]::GetFullPath((Join-Path $env:USERPROFILE 'Developer'))
if (-not (Test-Path -LiteralPath $dev -PathType Container)) { Fail 'Crie uma pasta local Developer no seu perfil antes de preparar.' }
if (-not $Root) { $Root = Join-Path $dev ('MaestroPilot-' + $KitID.Substring(0,12)) }
if (-not [IO.Path]::IsPathRooted($Root) -or $Root -match '[\x00-\x1f]') { Fail 'Escolha um caminho absoluto sem controles.' }
$Root = [IO.Path]::GetFullPath($Root)
if (-not $Root.StartsWith($dev + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { Fail 'O ensaio fica somente na pasta local Developer do seu perfil.' }
$walk = $Root
while ($walk) {
  if (Test-Path -LiteralPath $walk) {
    $item = Get-Item -LiteralPath $walk -Force
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { Fail 'O caminho contém reparse/cloud/link; nada foi redirecionado.' }
  }
  $parent = Split-Path -Parent $walk
  if ($parent -ceq $walk) { break }
  $walk = $parent
}
if (-not (Test-Path -LiteralPath (Split-Path -Parent $Root) -PathType Container)) { Fail 'A pasta pai deve existir.' }
$Target = 'windows-amd64'
if ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture -ne [Runtime.InteropServices.Architecture]::X64) { Fail 'Este preparador cobre Windows x64; outro alvo permanece indisponível.' }
$binary = Join-Path $Kit 'bin/maestro-mcp-windows-amd64.exe'
if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) { Fail 'Este artefato não contém o alvo Windows x64.' }
$digest = Hash-Text ('ordinary-engineering-rehearsal-v1' + $nl + $KitID + $nl + $Target + $nl + $Root + $nl)
Write-Host 'Ensaio local de engenharia; sem instalação/atualização assinada.'
Write-Host ("Produto: $Root\product" + $nl + "Estado privado: $Root\owner" + $nl + "Workspace: $Root\workspace" + $nl + "Perfil Code isolado: $Root\client\code" + $nl + "review_digest=$digest")
Write-Host 'Somente ao criar este perfil Code novo: autoMemoryEnabled:false e syncClaudeAiPlugins:false desativam a memória automática do cliente e o download/carregamento de plugins sincronizados da conta. Perfis existentes e suas escolhas são preservados.'
Write-Host 'O pacote Desktop usará revisão local no navegador quando não houver formulário nativo; cada alteração exige sua aprovação exata e expira em 150 segundos.'
if ($RuntimeMode) { Write-Host 'No Code deste ensaio, gravações protegidas usam o navegador local explicitamente selecionado, mesmo com formulários nativos. Leia o registro completo atual/proposto; a caixa começa desmarcada. Recusar, fechar, cancelar ou deixar expirar nunca aprova.'; Write-Host ('Conversor experimental via aquisição direta dos fornecedores: versão ' + $PilotVersion + '; validade ' + $runtime.expires_at + '. O ZIP não contém Python, wheels, DLLs ou archive do runtime. Após inicializar, você verá duas revisões separadas: download/cache e colocação/qualificação local. Nenhuma delas é aprovada por PREPARAR.') }
if ($Review) { return }
if ($ConfirmDigest) {
  if ($ConfirmDigest -cne $digest) { Fail 'Digest diferente; leia a revisão atual.' }
} elseif ((Read-Host 'Para criar ou conferir este ensaio, digite PREPARAR') -cne 'PREPARAR') { Fail 'Preparação recusada; nada foi criado.' }
Verify-Kit
if ($KitID -cne $ReviewedKitID) { Fail 'O kit mudou após a revisão; leia uma revisão nova.' }
function Inventory {
  $rows = @()
  foreach ($folder in @('product','exports','launchers')) {
    foreach ($item in Get-ChildItem -LiteralPath (Join-Path $Root $folder) -Recurse -Force) {
      if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { Fail 'Material gerenciado contém reparse/link; preservado.' }
      if (-not $item.PSIsContainer) {
        $relative = $item.FullName.Substring($Root.Length + 1).Replace('\','/')
        # This one optional export has its own canonical inventory and binding check.
        if ($relative.StartsWith('exports/code-native-qualified/', [StringComparison]::OrdinalIgnoreCase)) { continue }
        $rows += ((Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + $relative)
      }
    }
  }
  return (($rows | Sort-Object -CaseSensitive) -join $nl) + $nl
}
$marker = Join-Path $Root '.maestro-pilot-owned'
$bin = Join-Path $Root $BinaryRelative
$common = @('--product-root',(Join-Path $Root 'product'),'--user-home',(Join-Path $Root 'owner'),'--workspace',(Join-Path $Root 'workspace'))
function Diagnose-Failure([string]$Message) {
  try { & (Join-Path $Root 'launchers/Testar Maestro.ps1') -CheckOnly } catch { Write-Warning 'O relatório do teste está na pasta diagnostics deste ensaio.' }
  Fail ($Message + ' Preparação preservada; consulte o relatório e não apague a pasta.')
}
if (-not (Get-Command claude -ErrorAction SilentlyContinue)) { Write-Warning 'Code: comando claude indisponível. O relatório continuará; disponibilize-o pelo fluxo normal do cliente para testar Code.' }
if (Test-Path -LiteralPath $Root) {
  if (-not (Test-Path -LiteralPath $marker -PathType Leaf) -or (Get-Content -LiteralPath $marker -Raw).Trim() -cne $KitID) { Fail 'Pasta estrangeira preservada; escolha uma pasta nova.' }
  if (-not (Test-Path -LiteralPath (Join-Path $Root '.maestro-pilot-complete')) -or -not (Test-Path -LiteralPath (Join-Path $Root 'managed-files.sha256'))) { Fail 'Preparação parcial preservada; abra launchers/Testar Maestro.cmd para obter o diagnóstico ou use outro destino.' }
  if ((Get-Content -LiteralPath (Join-Path $Root 'managed-files.sha256') -Raw) -cne (Inventory)) { Fail 'Material gerenciado mudou; os arquivos foram preservados.' }
  & (Join-Path $Root 'launchers/Testar Maestro.ps1') -CheckOnly
  Write-Host "Ensaio conferido. Para continuar, abra $Root\launchers\Testar Maestro.cmd"
  return
}
New-Item -ItemType Directory -Path $Root | Out-Null
Write-New $marker ($KitID + $nl)
foreach ($folder in @('product','owner','exports','launchers','client','client/code')) { New-Item -ItemType Directory -Path (Join-Path $Root $folder) | Out-Null }
Write-New (Join-Path $Root 'client/code/settings.json') ('{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false}' + $nl)
if ($RuntimeMode) {
  New-Item -ItemType Directory -Path (Join-Path $Root 'product/server') | Out-Null
  New-Item -ItemType Directory -Path (Join-Path $Root 'product/managed/trust') -Force | Out-Null
  $registry = Join-Path $Root 'product/managed/trust/release-authority-registry.json'
  [IO.File]::Copy((Join-Path $Kit 'runtime-registry.json'), $registry, $false)
  if ((Get-FileHash -LiteralPath $registry -Algorithm SHA256).Hash.ToLowerInvariant() -cne $runtime.registry_sha256) { Fail 'Registro público divergente; preserve o ensaio.' }
}
[IO.File]::Copy($binary, $bin, $false)
$binaryLine = @(Get-Content -LiteralPath (Join-Path $Kit 'SHA256SUMS') | Where-Object { $_ -cmatch '^[0-9a-f]{64}  \./bin/maestro-mcp-windows-amd64\.exe$' })
if ($binaryLine.Count -ne 1 -or (Get-FileHash -LiteralPath $bin -Algorithm SHA256).Hash.ToLowerInvariant() -cne $binaryLine[0].Substring(0,64)) { Fail 'O executável copiado mudou; preserve o ensaio.' }
Write-New (Join-Path $Root 'pilot-binary.sha256') ($binaryLine[0].Substring(0,64) + '  ' + $BinaryRelative + $nl)
foreach ($pair in @(@('test-windows.ps1','Testar Maestro.ps1'),@('test-windows.cmd','Testar Maestro.cmd'))) {
  $source = Join-Path $Kit ('templates/' + $pair[0])
  $target = Join-Path $Root ('launchers/' + $pair[1])
  [IO.File]::Copy($source, $target, $false)
  if ((Get-FileHash -LiteralPath $source -Algorithm SHA256).Hash -cne (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash) { Fail 'Launcher ocupado ou alterado; preserve o ensaio.' }
}
$priorPreference = $ErrorActionPreference
try {
  # Windows PowerShell may terminate on native stderr before checking the exit code.
  $ErrorActionPreference = 'Continue'
  $result = & $bin init @common 2>$null
  $initExit = $LASTEXITCODE
} finally { $ErrorActionPreference = $priorPreference }
if ($initExit -ne 0) { Diagnose-Failure 'A inicialização não foi concluída.' }
Write-New (Join-Path $Root 'preparation.json') (($result -join $nl) + $nl)
& $bin inspect @common
if ($LASTEXITCODE -ne 0) { Diagnose-Failure 'O diagnóstico recusou o vínculo.' }
if ($RuntimeMode) {
  [IO.File]::Copy((Join-Path $Kit 'test-documents/maestro-pilot.docx'), (Join-Path $Root 'workspace/maestro-pilot.docx'), $false)
  $carrier = Join-Path $Kit 'runtime-release'
  $acquisitionReviewFile = Join-Path $Root 'acquisition-review.json'
  $review = & $bin pilot-acquisition-review @common --release-directory $carrier
  if ($LASTEXITCODE -ne 0) { Diagnose-Failure 'A revisão autenticada de aquisição foi recusada.' }
  Write-New $acquisitionReviewFile (($review -join $nl) + $nl)
  Write-Host ($review -join $nl)
  $record = ($review -join $nl) | ConvertFrom-Json
  Write-Host ('Entradas: ' + $record.plan.recipe.input_count + '; bytes de entrada: ' + $record.plan.recipe.total_input_bytes + '; reserva adicional máxima: ' + $record.plan.maximum_additional_bytes + '; retidos: ' + $record.plan.retained_bytes + '; tentativa: ' + $record.plan.attempt + '; revisão válida até: ' + $record.plan.valid_until)
  if ((Read-Host 'Leia toda a revisão acima. Para baixar somente essas entradas públicas e manter o cache privado, digite BAIXAR CONVERSOR') -cne 'BAIXAR CONVERSOR') { Diagnose-Failure 'Aquisição recusada; nenhum download foi iniciado.' }
  $acquired = & $bin pilot-acquisition-apply @common --release-directory $carrier --acquisition-review-file $acquisitionReviewFile --review-digest $record.review_sha256 --confirm
  if ($LASTEXITCODE -ne 0) { Diagnose-Failure 'Aquisição incompleta; cache e tentativas foram preservados. Não repita automaticamente.' }
  Write-New (Join-Path $Root 'acquisition-result.json') (($acquired -join $nl) + $nl)
  $complete = (($acquired -join $nl) | ConvertFrom-Json).release_directory
  $placement = & $bin pilot-runtime-review @common --release-directory $complete
  if ($LASTEXITCODE -ne 0) { Diagnose-Failure 'A revisão do release completo foi recusada.' }
  Write-New (Join-Path $Root 'runtime-review.json') (($placement -join $nl) + $nl)
  Write-Host ($placement -join $nl)
  $placementRecord = ($placement -join $nl) | ConvertFrom-Json
  if ((Read-Host 'Esta etapa coloca o conversor e testa os controles Windows locais descritos. Digite ATIVAR CONVERSOR somente após ler a revisão completa') -cne 'ATIVAR CONVERSOR') { Diagnose-Failure 'Colocação recusada; entradas adquiridas foram preservadas.' }
  $placed = & $bin pilot-runtime-apply @common --release-directory $complete --review-digest $placementRecord.review_sha256 --confirm
  if ($LASTEXITCODE -ne 0) { Diagnose-Failure 'Colocação ou qualificação local recusada; preserve os recibos e arquivos.' }
  Write-New (Join-Path $Root 'runtime-placement.json') (($placed -join $nl) + $nl)
}
$extra = @()
switch ((Get-Content -LiteralPath (Join-Path $Kit 'LIFECYCLE-CONTEXT.txt') -Raw).Trim()) {
  'enabled' { $extra = @('--lifecycle-context') }
  'disabled' { }
  default { Fail 'Seleção de contexto desconhecida.' }
}
if ($RuntimeMode) { $extra += @('--code-pilot','--owner-review-channel','local-browser') }
& $bin plugin @common --server $bin --destination (Join-Path $Root 'exports/code') --version $PilotVersion @extra
if ($LASTEXITCODE -ne 0) { Diagnose-Failure 'O pacote Code não foi concluído.' }
$desktopExtra = @()
if ($RuntimeMode) { $desktopExtra = @('--engineering-runtime-pilot') }
& $bin mcpb --desktop-pilot --owner-review-channel local-browser @common --server $bin --destination (Join-Path $Root 'exports/maestro-product.mcpb') --version $PilotVersion @desktopExtra
if ($LASTEXITCODE -ne 0) { Diagnose-Failure 'O pacote Desktop não foi concluído.' }
$code = @'
param([string]$InitialPrompt)
$ErrorActionPreference = 'Stop'
if ($args.Count -ne 0 -or ($PSBoundParameters.ContainsKey('InitialPrompt') -and [string]::IsNullOrEmpty($InitialPrompt))) { throw 'Use zero argumentos ou um único prompt.' }
$root = Split-Path -Parent $PSScriptRoot
$identity = (Get-Content -LiteralPath (Join-Path $root 'pilot-binary.sha256') -Raw).Trim()
if ($identity -cnotmatch '^([0-9a-f]{64})  (product/(server/)?maestro-mcp\.exe)$') { throw 'Identidade do executável inválida.' }
$expected = $Matches[1]
$bin = Join-Path $root $Matches[2]
if ((Get-FileHash -LiteralPath $bin -Algorithm SHA256).Hash.ToLowerInvariant() -cne $expected) { throw 'O executável mudou; preserve a preparação.' }
$packet = Join-Path $root 'exports/code'
$native = Join-Path $root 'exports/code-native-qualified'
$nativeItem = Get-Item -LiteralPath $native -Force -ErrorAction SilentlyContinue
if ($null -ne $nativeItem) {
  if (($nativeItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Export nativo com reparse/link; preserve o ensaio.' }
  $inspection = & $bin inspect --packet --product-root $native --user-home (Join-Path $root 'owner') --workspace (Join-Path $root 'workspace')
  if ($LASTEXITCODE -ne 0) { throw 'Export nativo recusado; preserve os arquivos.' }
  $nativeStatus = ($inspection -join "`n") | ConvertFrom-Json
  if ($nativeStatus.packet_server_sha256 -cne $expected) { throw 'O export nativo usa outro executável; preserve o ensaio.' }
  switch ($nativeStatus.packet_mode) {
    'native' { $packet = $native }
    'mcp_only' { }
    default { throw 'Export nativo não qualificado; preserve o ensaio.' }
  }
}
$inspection = & $bin inspect --packet --product-root $packet --user-home (Join-Path $root 'owner') --workspace (Join-Path $root 'workspace')
if ($LASTEXITCODE -ne 0) { throw 'Vínculo recusado; preserve os arquivos.' }
$selected = ($inspection -join "`n") | ConvertFrom-Json
if ($selected.packet_server_sha256 -cne $expected) { throw 'O pacote selecionado usa outro executável; preserve o ensaio.' }
function Assert-LocalCodeProfile {
  foreach ($directory in @((Join-Path $root 'client'),(Join-Path $root 'client/code'))) {
    $item = Get-Item -LiteralPath $directory -Force -ErrorAction Stop
    if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Perfil Code ausente ou com reparse/link; preserve a preparação e o destino existente.' }
  }
}
if ($PSBoundParameters.ContainsKey('InitialPrompt') -or ($selected.packet_mode -ceq 'native')) {
  $command = Get-Command claude.exe -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($null -eq $command -or [IO.Path]::GetExtension($command.Source) -ine '.exe') {
    throw 'O prompt automático e o pacote nativo requerem a instalação nativa do Claude Code (claude.exe). Disponibilize-a pelo fluxo normal autorizado. Para um pacote MCP comum, abra Abrir Code sem argumentos e use o prompt salvo em diagnostics deste teste.'
  }
} else {
  $command = Get-Command claude -ErrorAction Stop
}
$promptArgs = @()
if ($PSBoundParameters.ContainsKey('InitialPrompt')) {
  $literal = $InitialPrompt
  if ($PSVersionTable.PSVersion -lt [version]'7.3') {
    # The legacy native binder consumes quotes; preserve the prompt's literal argv.
    $literal = $literal -replace '(\\*)"', '$1$1\"'
  }
  $promptArgs = @('--', $literal)
}
$old = $env:CLAUDE_CONFIG_DIR
try {
  $env:CLAUDE_CONFIG_DIR = Join-Path $root 'client/code'
  $selectionArgs = @()
  if ($selected.packet_mode -ceq 'native') {
    Assert-LocalCodeProfile
    $selection = Join-Path $env:CLAUDE_CONFIG_DIR ('native-session.' + [Guid]::NewGuid().ToString('N') + '.json')
    $body = @{pluginConfigs=@{'maestro-product@inline'=@{options=@{user_home=(Join-Path $root 'owner');workspace=(Join-Path $root 'workspace')}}}} | ConvertTo-Json -Depth 5 -Compress
    $stream = [IO.File]::Open($selection, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
    try { $bytes = [Text.UTF8Encoding]::new($false).GetBytes($body); $stream.Write($bytes, 0, $bytes.Length) } finally { $stream.Dispose() }
    $selectionArgs = @('--settings', $selection)
  }
  Push-Location -LiteralPath (Join-Path $root 'workspace')
  try { Assert-LocalCodeProfile; & $command.Source --plugin-dir $packet @selectionArgs @promptArgs } finally { Pop-Location }
} finally {
  if ($null -eq $old) { Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue } else { $env:CLAUDE_CONFIG_DIR = $old }
}
'@
$desktop = @'
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Write-Host 'Confirme a instalação da extensão Maestro MCP — Ensaio indicada no relatório. Ela já usa as pastas aprovadas, sem campos de caminho. Antes de colar, abra Configurações > Extensões > a extensão deste ensaio e, somente na linha Maestro context (maestro_context), selecione ✓ / Sempre permitir. Preserve as demais permissões. ✓ / Sempre permitir libera só a leitura; gravações de conteúdo exigem revisão exata no formulário nativo ou no navegador local selecionado neste ensaio. Abra uma conversa nova e cole o texto copiado por Testar Maestro. Se a extensão estiver desativada ou ausente, confira Configurações > Extensões > Configurar > Ativado. Preserve outras extensões Maestro.'
Start-Process -FilePath (Join-Path $root 'exports/maestro-product.mcpb')
'@
Write-New (Join-Path $Root 'launchers/Abrir Code.ps1') $code
Write-New (Join-Path $Root 'launchers/Abrir Desktop.ps1') $desktop
Write-New (Join-Path $Root 'launchers/Abrir Code.cmd') ('@echo off' + [char]13 + $nl + 'setlocal' + [char]13 + $nl + 'set "PSModulePath="' + [char]13 + $nl + 'powershell.exe -NoLogo -NoProfile -File "%~dp0Abrir Code.ps1"' + [char]13 + $nl)
Write-New (Join-Path $Root 'launchers/Abrir Desktop.cmd') ('@echo off' + [char]13 + $nl + 'setlocal' + [char]13 + $nl + 'set "PSModulePath="' + [char]13 + $nl + 'powershell.exe -NoLogo -NoProfile -File "%~dp0Abrir Desktop.ps1"' + [char]13 + $nl)
$start = @"
# Maestro MCP — teste guiado

Abra **launchers/Testar Maestro.cmd**. Para conferir a conexão sem alterar dados,
escolha **3 — Conferir conexão Code** ou **4 — Conferir conexão Desktop**. O texto
abre automaticamente no Code, sem copiar ou colar. No Desktop, o texto é copiado:
cole uma vez em uma conversa nova. O estado mostrado é a saúde da conexão, não o estado dos trabalhos.

Para o ensaio completo, escolha **1 — Ensaio completo no Code** e depois
**2 — Retomar no Desktop**. O Code recebe o prompt inicial automaticamente; no Desktop,
cole o texto copiado. Cada etapa preserva as confirmações normais.

Para testar os sete hooks nativos, primeiro use a opção 3 e conclua o login normal
no perfil Code. Depois escolha **5 — Qualificar os sete hooks do Code**. Essa ação
separada usa modelo e pode gerar cobrança: escolha sua estimativa por sessão,
leia o plano completo e confirme QUALIFICAR somente se concordar. Preparar,
abrir Code e conferir conexão não executam a qualificação. Após sucesso, o
launcher seleciona o pacote qualificado e as duas pastas automaticamente.
Resultado existente é preservado, sem repetição automática. Desktop continua
como extensão MCP e não instala os sete hooks nativos do Code.

No Windows, o prompt automático das opções 1 e 3 requer a instalação nativa
claude.exe no PATH, obtida pelo fluxo normal autorizado. Com somente um shim
.cmd/.bat/.ps1, o teste para antes de enviar o prompt. Para uso manual, abra
Abrir Code sem argumentos e copie o texto CODE.txt ou CODE-CONEXAO.txt do
relatório atual em diagnostics para a conversa. O kit não instala o cliente.

O perfil Code é separado e pode pedir login normal nessa janela. Ao criar um perfil
novo, este ensaio desativa a memória automática do cliente e os plugins sincronizados
da conta somente nesse perfil. Repetir a preparação preserva suas escolhas existentes.
O plugin e a extensão já usam as pastas aprovadas; não há diretórios para copiar ou preencher.
Se o cliente pedir configurar outros caminhos, preserve a mensagem e pare.

Para Desktop, escolha 4 para conferir somente a conexão ou 2 para retomar o ensaio
completo em launchers/Testar Maestro.cmd e confirme a
instalação da extensão Maestro MCP — Ensaio indicada no relatório. Ela já usa as
pastas aprovadas; não há campos de caminho para salvar.
Antes de colar, abra Configurações > Extensões > a extensão deste ensaio e,
somente na linha Maestro context (maestro_context), selecione ✓ / Sempre permitir.
Preserve as demais permissões. ✓ / Sempre permitir libera só a leitura; gravações
de conteúdo exigem revisão exata no formulário nativo ou no navegador local selecionado neste ensaio. Abra uma conversa nova e cole o texto copiado. Se a extensão estiver desativada ou ausente, confira
Configurações > Extensões > Configurar > Ativado nessa extensão.
Se o cliente propuser substituir outra extensão, pare.

Revisão do dono no Desktop: este ensaio seleciona explicitamente a revisão local
no navegador para gravações protegidas. Sem formulário nativo, uma solicitação
interativa abre o registro completo no navegador deste computador. Leia tudo,
marque a caixa inicialmente desmarcada e escolha Aprovar alteração exata ou
Recusar. Fechar, cancelar ou ultrapassar 150 segundos não aprova. Formulários
nativos continuam tendo prioridade. Este canal não qualifica hooks nem bloqueio
nativo. Se receber confirmation_unavailable, deixe a ação pendente e confira o
navegador ou use o Code com sua confirmação nativa. Um sim no chat e a permissão
de leitura não aprovam gravações.

workspace_id da criação real:

$($result -join $nl)

Os relatórios ficam em diagnostics/, fora dos dados do Maestro. Compartilhe somente
RELATORIO.md após conferir seu conteúdo. Login, confirmações, chamadas reais nos
clientes e hooks continuam sendo verificados por você. No kit básico, 0.0.0 é o
rótulo dos pacotes. No kit completo, RUNTIME.json informa a versão e a validade
experimental; o diagnóstico verifica a disponibilidade, sem converter documentos.
"@
Write-New (Join-Path $Root 'COMECE-AQUI.md') $start
Write-New (Join-Path $Root 'managed-files.sha256') (Inventory)
Write-New (Join-Path $Root '.maestro-pilot-complete') ($KitID + $nl)
& (Join-Path $Root 'launchers/Testar Maestro.ps1') -CheckOnly
Write-Host "Preparado e conferido. Abra $Root\launchers\Testar Maestro.cmd para testar nos clientes."
