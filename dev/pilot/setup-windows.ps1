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
        $rows += ((Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + $relative)
      }
    }
  }
  return (($rows | Sort-Object -CaseSensitive) -join $nl) + $nl
}
$marker = Join-Path $Root '.maestro-pilot-owned'
$bin = Join-Path $Root 'product/maestro-mcp.exe'
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
[IO.File]::Copy($binary, $bin, $false)
$binaryLine = @(Get-Content -LiteralPath (Join-Path $Kit 'SHA256SUMS') | Where-Object { $_ -cmatch '^[0-9a-f]{64}  \./bin/maestro-mcp-windows-amd64\.exe$' })
if ($binaryLine.Count -ne 1 -or (Get-FileHash -LiteralPath $bin -Algorithm SHA256).Hash.ToLowerInvariant() -cne $binaryLine[0].Substring(0,64)) { Fail 'O executável copiado mudou; preserve o ensaio.' }
Write-New (Join-Path $Root 'pilot-binary.sha256') ($binaryLine[0].Substring(0,64) + '  product/maestro-mcp.exe' + $nl)
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
$extra = @()
switch ((Get-Content -LiteralPath (Join-Path $Kit 'LIFECYCLE-CONTEXT.txt') -Raw).Trim()) {
  'enabled' { $extra = @('--lifecycle-context') }
  'disabled' { }
  default { Fail 'Seleção de contexto desconhecida.' }
}
& $bin plugin @common --server $bin --destination (Join-Path $Root 'exports/code') --version 0.0.0 @extra
if ($LASTEXITCODE -ne 0) { Diagnose-Failure 'O pacote Code não foi concluído.' }
& $bin mcpb --desktop-pilot @common --server $bin --destination (Join-Path $Root 'exports/maestro-product.mcpb') --version 0.0.0
if ($LASTEXITCODE -ne 0) { Diagnose-Failure 'O pacote Desktop não foi concluído.' }
$code = @'
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
& (Join-Path $root 'exports/code/scripts/maestro-mcp.exe') inspect --packet --product-root (Join-Path $root 'exports/code') --user-home (Join-Path $root 'owner') --workspace (Join-Path $root 'workspace')
if ($LASTEXITCODE -ne 0) { throw 'Vínculo recusado; preserve os arquivos.' }
$command = Get-Command claude -ErrorAction Stop
$old = $env:CLAUDE_CONFIG_DIR
try {
  $env:CLAUDE_CONFIG_DIR = Join-Path $root 'client/code'
  Push-Location -LiteralPath (Join-Path $root 'workspace')
  try { & $command.Source --plugin-dir (Join-Path $root 'exports/code') } finally { Pop-Location }
} finally {
  if ($null -eq $old) { Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue } else { $env:CLAUDE_CONFIG_DIR = $old }
}
'@
$desktop = @'
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Write-Host 'Confirme a extensão Maestro MCP — Ensaio indicada no relatório e seus caminhos preenchidos. Preserve outras extensões Maestro.'
Start-Process -FilePath (Join-Path $root 'exports/maestro-product.mcpb')
'@
Write-New (Join-Path $Root 'launchers/Abrir Code.ps1') $code
Write-New (Join-Path $Root 'launchers/Abrir Desktop.ps1') $desktop
Write-New (Join-Path $Root 'launchers/Abrir Code.cmd') ('@echo off' + [char]13 + $nl + 'setlocal' + [char]13 + $nl + 'set "PSModulePath="' + [char]13 + $nl + 'powershell.exe -NoLogo -NoProfile -File "%~dp0Abrir Code.ps1"' + [char]13 + $nl)
Write-New (Join-Path $Root 'launchers/Abrir Desktop.cmd') ('@echo off' + [char]13 + $nl + 'setlocal' + [char]13 + $nl + 'set "PSModulePath="' + [char]13 + $nl + 'powershell.exe -NoLogo -NoProfile -File "%~dp0Abrir Desktop.ps1"' + [char]13 + $nl)
$start = @"
# Maestro MCP — teste guiado

Abra **launchers/Testar Maestro.cmd**. Ele confere o ensaio, abre o relatório e
permite escolher Code ou Desktop. O texto com o ID correto é copiado automaticamente;
cole uma vez na conversa escolhida. O perfil Code é separado e pode pedir login normal.
Ao habilitar o plugin, preencha estes valores literais:

Owner directory:

    $Root\owner

Workspace:

    $Root\workspace

Depois abra launchers/Abrir Desktop.cmd e confirme a extensão Maestro MCP — Ensaio
indicada no relatório. Os caminhos já vêm preenchidos para este ensaio.
Se o cliente propuser substituir outra extensão, pare.

workspace_id da criação real:

$($result -join $nl)

Os relatórios ficam em diagnostics/, fora dos dados do Maestro. Compartilhe somente
RELATORIO.md após conferir seu conteúdo. Login, confirmações, chamadas reais nos
clientes e hooks continuam sendo verificados por você. Servidor de engenharia sem
versão de release; 0.0.0 é o rótulo dos pacotes.
"@
Write-New (Join-Path $Root 'COMECE-AQUI.md') $start
Write-New (Join-Path $Root 'managed-files.sha256') (Inventory)
Write-New (Join-Path $Root '.maestro-pilot-complete') ($KitID + $nl)
& (Join-Path $Root 'launchers/Testar Maestro.ps1') -CheckOnly
Write-Host "Preparado e conferido. Abra $Root\launchers\Testar Maestro.cmd para testar nos clientes."
