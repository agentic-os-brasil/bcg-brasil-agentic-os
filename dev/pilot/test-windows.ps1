param([switch]$CheckOnly)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$root = Split-Path -Parent $PSScriptRoot
function Fail([string]$Message) { throw "Maestro: $Message" }
function Write-New([string]$Path, [string]$Body) {
  $bytes = [Text.UTF8Encoding]::new($false).GetBytes($Body)
  $stream = [IO.File]::Open($Path, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
  try { $stream.Write($bytes, 0, $bytes.Length) } finally { $stream.Dispose() }
}
$walk = $root
while ($walk) {
  $item = Get-Item -LiteralPath $walk -Force
  if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { Fail 'Caminho com reparse/link preservado; use a pasta local preparada.' }
  $parent = Split-Path -Parent $walk
  if ($parent -ceq $walk) { break }
  $walk = $parent
}
if (-not (Test-Path -LiteralPath (Join-Path $root '.maestro-pilot-owned') -PathType Leaf)) { Fail 'Este launcher requer uma pasta preparada pelo kit.' }
foreach ($name in @('.maestro-pilot-owned','pilot-binary.sha256','managed-files.sha256','.maestro-pilot-complete')) {
  $path = Join-Path $root $name
  if ((Test-Path -LiteralPath $path) -and (((Get-Item -LiteralPath $path -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0)) { Fail 'Metadado com reparse/link; preserve a preparação.' }
}
foreach ($folder in @('product','exports','launchers','diagnostics')) {
  $path = Join-Path $root $folder
  if (Test-Path -LiteralPath $path) {
    foreach ($entry in @((Get-Item -LiteralPath $path -Force)) + @(Get-ChildItem -LiteralPath $path -Recurse -Force)) {
      if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { Fail 'Material gerenciado ou diagnóstico com reparse/link; preserve a preparação.' }
    }
  }
}
$identity = Get-Content -LiteralPath (Join-Path $root 'pilot-binary.sha256') -Raw
$bin = Join-Path $root 'product/maestro-mcp.exe'
if ($identity.Trim() -cnotmatch '^([0-9a-f]{64})  product/maestro-mcp\.exe$') { Fail 'Identidade do executável inválida.' }
if ((Get-FileHash -LiteralPath $bin -Algorithm SHA256).Hash.ToLowerInvariant() -cne $Matches[1]) { Fail 'O executável mudou; preserve a preparação.' }
if (Test-Path -LiteralPath (Join-Path $root '.maestro-pilot-complete')) {
  $rows = @()
  foreach ($folder in @('product','exports','launchers')) {
    foreach ($item in Get-ChildItem -LiteralPath (Join-Path $root $folder) -Recurse -Force -File) {
      $rows += ((Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + $item.FullName.Substring($root.Length + 1).Replace('\','/'))
    }
  }
  $actual = (($rows | Sort-Object -CaseSensitive) -join "`n") + "`n"
  if ((Get-Content -LiteralPath (Join-Path $root 'managed-files.sha256') -Raw) -cne $actual) { Fail 'Material gerenciado mudou; preserve a preparação.' }
}
$diagnostics = Join-Path $root 'diagnostics'
if (-not (Test-Path -LiteralPath $diagnostics)) { New-Item -ItemType Directory -Path $diagnostics | Out-Null }
$run = Join-Path $diagnostics ('teste-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $run | Out-Null
$common = @('--product-root',(Join-Path $root 'product'),'--user-home',(Join-Path $root 'owner'),'--workspace',(Join-Path $root 'workspace'))
$previousPreference = $ErrorActionPreference
$json = @()
$status = 1
try {
  $ErrorActionPreference = 'Continue'
  $json = & $bin pilot-check @common 2>$null
  $status = $LASTEXITCODE
} finally { $ErrorActionPreference = $previousPreference }
Write-New (Join-Path $run 'report.json') (($json -join "`n") + "`n")
try {
  $report = ($json -join "`n") | ConvertFrom-Json
  if ($report.schema_version -ne 1 -or $report.status -notin @('ready','partial','blocked')) { throw 'schema' }
  Write-New (Join-Path $run 'RELATORIO.md') ([string]$report.markdown)
  Write-New (Join-Path $run 'CODE.txt') ([string]$report.code_prompt)
  Write-New (Join-Path $run 'DESKTOP.txt') ([string]$report.desktop_prompt)
} catch {
  Write-New (Join-Path $run 'FALHA.md') "# Maestro MCP — diagnóstico incompleto`n`nO verificador não produziu um relatório reconhecido. Preserve esta pasta e envie este arquivo ao time. Nenhum cliente foi aberto.`n"
  Fail "Relatório incompleto em $run\FALHA.md. Preserve os arquivos."
}
Write-Host "Relatório: $run\RELATORIO.md"
if ($CheckOnly) { if ($status -ne 0) { Fail 'Diagnóstico bloqueado; consulte o relatório.' }; return }
Start-Process -FilePath (Join-Path $env:WINDIR 'System32/notepad.exe') -ArgumentList ('"' + (Join-Path $run 'RELATORIO.md') + '"')
if ($status -ne 0) { Fail 'Preparação bloqueada. Envie RELATORIO.md ao time; os arquivos foram preservados.' }
Write-Host "1 — Testar no Code`n2 — Testar no Desktop`nEnter — Sair"
$choice = Read-Host 'Escolha'
switch ($choice) {
  '1' {
    if (-not $report.code_prompt) { Fail 'O relatório ainda não liberou o ensaio no cliente.' }
    if (-not (Get-Command claude -ErrorAction SilentlyContinue)) { Fail 'Disponibilize o comando claude pelo fluxo normal do Code e abra Testar Maestro novamente.' }
    Set-Clipboard -Value $report.code_prompt
    Write-Host 'Prompt copiado. Cole uma vez na conversa do Code.'
    & (Join-Path $root 'launchers/Abrir Code.ps1')
  }
  '2' {
    if (-not $report.desktop_prompt) { Fail 'O relatório ainda não liberou o ensaio no cliente.' }
    Set-Clipboard -Value $report.desktop_prompt
    Write-Host 'Prompt copiado. Confirme a extensão deste ensaio e cole uma vez em uma conversa do Desktop.'
    & (Join-Path $root 'launchers/Abrir Desktop.ps1')
  }
  '' { }
  default { Fail 'Escolha 1, 2 ou Enter.' }
}
