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
if (Test-Path -LiteralPath $Root) {
  if (-not (Test-Path -LiteralPath $marker -PathType Leaf) -or (Get-Content -LiteralPath $marker -Raw).Trim() -cne $KitID) { Fail 'Pasta estrangeira preservada; escolha uma pasta nova.' }
  if (-not (Test-Path -LiteralPath (Join-Path $Root '.maestro-pilot-complete')) -or -not (Test-Path -LiteralPath (Join-Path $Root 'managed-files.sha256'))) { Fail 'Preparação parcial preservada; use uma pasta nova para recuperar.' }
  if ((Get-Content -LiteralPath (Join-Path $Root 'managed-files.sha256') -Raw) -cne (Inventory)) { Fail 'Material gerenciado mudou; os arquivos foram preservados.' }
  & $bin inspect @common
  if ($LASTEXITCODE -ne 0) { Fail 'Diagnóstico recusou o vínculo; preserve o ensaio.' }
  Write-Host "Ensaio já preparado. Abra $Root\COMECE-AQUI.md"
  return
}
New-Item -ItemType Directory -Path $Root | Out-Null
Write-New $marker ($KitID + $nl)
foreach ($folder in @('product','owner','exports','launchers','client','client/code')) { New-Item -ItemType Directory -Path (Join-Path $Root $folder) | Out-Null }
[IO.File]::Copy($binary, $bin, $false)
$binaryLine = @(Get-Content -LiteralPath (Join-Path $Kit 'SHA256SUMS') | Where-Object { $_ -cmatch '^[0-9a-f]{64}  \./bin/maestro-mcp-windows-amd64\.exe$' })
if ($binaryLine.Count -ne 1 -or (Get-FileHash -LiteralPath $bin -Algorithm SHA256).Hash.ToLowerInvariant() -cne $binaryLine[0].Substring(0,64)) { Fail 'O executável copiado mudou; preserve o ensaio.' }
$result = & $bin init @common
if ($LASTEXITCODE -ne 0) { Fail 'Inicialização parcial preservada; selecione uma pasta nova para recuperar.' }
Write-New (Join-Path $Root 'preparation.json') (($result -join $nl) + $nl)
& $bin inspect @common
if ($LASTEXITCODE -ne 0) { Fail 'O diagnóstico recusou; preserve os arquivos.' }
$extra = @()
switch ((Get-Content -LiteralPath (Join-Path $Kit 'LIFECYCLE-CONTEXT.txt') -Raw).Trim()) {
  'enabled' { $extra = @('--lifecycle-context') }
  'disabled' { }
  default { Fail 'Seleção de contexto desconhecida.' }
}
& $bin plugin @common --server $bin --destination (Join-Path $Root 'exports/code') --version 0.0.0 @extra
if ($LASTEXITCODE -ne 0) { Fail 'Pacote Code parcial preservado.' }
& $bin mcpb @common --server $bin --destination (Join-Path $Root 'exports/maestro-product.mcpb') --version 0.0.0
if ($LASTEXITCODE -ne 0) { Fail 'Pacote Desktop parcial preservado.' }
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
Write-Host 'Confirme somente se não substituir outro ensaio Maestro. Use os campos de COMECE-AQUI.md.'
Start-Process -FilePath (Join-Path $root 'exports/maestro-product.mcpb')
'@
Write-New (Join-Path $Root 'launchers/Abrir Code.ps1') $code
Write-New (Join-Path $Root 'launchers/Abrir Desktop.ps1') $desktop
Write-New (Join-Path $Root 'launchers/Abrir Code.cmd') ('@echo off' + [char]13 + $nl + 'powershell.exe -NoLogo -NoProfile -File "%~dp0Abrir Code.ps1"' + [char]13 + $nl)
Write-New (Join-Path $Root 'launchers/Abrir Desktop.cmd') ('@echo off' + [char]13 + $nl + 'powershell.exe -NoLogo -NoProfile -File "%~dp0Abrir Desktop.ps1"' + [char]13 + $nl)
$start = @"
# Maestro — ensaio local

Abra launchers/Abrir Code.cmd. O perfil é separado e pode pedir login normal.
Ao habilitar o plugin, preencha estes valores literais:

Owner directory:

    $Root\owner

Workspace:

    $Root\workspace

Depois abra launchers/Abrir Desktop.cmd e confirme os mesmos campos.
Se existir outra extensão maestro-product, pare antes de substituir.

workspace_id da criação real:

$($result -join $nl)

Use o ROTEIRO.md do kit. Servidor de engenharia sem versão de release;
0.0.0 é o rótulo dos pacotes. ARTIFACT.txt e SHA256SUMS identificam os bytes.
"@
Write-New (Join-Path $Root 'COMECE-AQUI.md') $start
Write-New (Join-Path $Root 'managed-files.sha256') (Inventory)
Write-New (Join-Path $Root '.maestro-pilot-complete') ($KitID + $nl)
Write-Host "Preparado. Abra $Root\COMECE-AQUI.md"
