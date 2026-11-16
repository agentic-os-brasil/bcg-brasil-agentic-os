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
      if ($item.FullName.Substring($root.Length + 1).Replace('\','/').StartsWith('exports/code-native-qualified/', [StringComparison]::OrdinalIgnoreCase)) { continue }
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
$start = [Diagnostics.ProcessStartInfo]::new()
$start.FileName = $bin
$start.UseShellExecute = $false
$start.CreateNoWindow = $true
$start.RedirectStandardOutput = $true
$start.RedirectStandardError = $true
$start.StandardOutputEncoding = [Text.UTF8Encoding]::new($false)
# Fixed flags and Windows leaf paths avoid shell parsing and trailing-slash quoting.
foreach ($argument in $common) { if ($argument -match '["\x00-\x1f]') { Fail 'Argumento inválido; preserve a preparação.' } }
$start.Arguments = 'pilot-check ' + (($common | ForEach-Object { '"' + $_ + '"' }) -join ' ')
$process = [Diagnostics.Process]::new()
$process.StartInfo = $start
$json = ''
$status = 1
try {
  if (-not $process.Start()) { throw 'start' }
  $stdout = $process.StandardOutput.ReadToEndAsync()
  $stderr = $process.StandardError.ReadToEndAsync()
  if (-not $process.WaitForExit(60000)) {
    $process.Kill()
    $process.WaitForExit()
    throw 'timeout'
  }
  $json = $stdout.GetAwaiter().GetResult()
  $null = $stderr.GetAwaiter().GetResult()
  $status = $process.ExitCode
} catch { $json = ''; $status = 1 } finally { $process.Dispose() }
Write-New (Join-Path $run 'report.json') (($json -join "`n") + "`n")
try {
  $report = ($json -join "`n") | ConvertFrom-Json
  if ($report.schema_version -ne 1 -or $report.status -notin @('ready','partial','blocked')) { throw 'schema' }
  Write-New (Join-Path $run 'RELATORIO.md') ([string]$report.markdown)
  Write-New (Join-Path $run 'CODE.txt') ([string]$report.code_prompt)
  Write-New (Join-Path $run 'DESKTOP.txt') ([string]$report.desktop_prompt)
  Write-New (Join-Path $run 'CODE-CONEXAO.txt') ([string]$report.code_ready_prompt)
  Write-New (Join-Path $run 'DESKTOP-CONEXAO.txt') ([string]$report.desktop_ready_prompt)
} catch {
  Write-New (Join-Path $run 'FALHA.md') "# Maestro MCP — diagnóstico incompleto`n`nO verificador não produziu um relatório reconhecido. Preserve esta pasta e envie este arquivo ao time. Nenhum cliente foi aberto.`n"
  Fail "Relatório incompleto em $run\FALHA.md. Preserve os arquivos."
}
Write-Host "Relatório: $run\RELATORIO.md"
if ($CheckOnly) { if ($status -ne 0) { Fail 'Diagnóstico bloqueado; consulte o relatório.' }; return }
Start-Process -FilePath (Join-Path $env:WINDIR 'System32/notepad.exe') -ArgumentList ('"' + (Join-Path $run 'RELATORIO.md') + '"')
if ($status -ne 0) { Fail 'Preparação bloqueada. Envie RELATORIO.md ao time; os arquivos foram preservados.' }
function Qualify-Code {
  $destination = Join-Path $root 'exports/code-native-qualified'
  if ($null -ne (Get-Item -LiteralPath $destination -Force -ErrorAction SilentlyContinue)) { Fail 'Já existe um resultado de qualificação. Ele foi preservado; abra Code para usar um resultado nativo válido ou envie o relatório ao time. Não há repetição automática.' }
  $code = Get-Command claude.exe -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($null -eq $code -or [IO.Path]::GetExtension($code.Source) -ine '.exe') { Fail 'A qualificação requer a instalação nativa claude.exe pelo fluxo normal autorizado.' }
  Write-Host 'Esta ação executa testes sintéticos com modelo no Code e pode gerar cobrança. Preparar e abrir Code normalmente não executam estes testes.'
  Write-Host 'Até 3 sessões CLI verificam ações permitidas, bloqueios e falhas/timeout dos hooks, com 8 turnos de pai e 120 segundos por sessão. O comando exige sua estimativa escolhida por sessão (maior que zero e no máximo US$ 1) para registrar o plano que você vai aprovar; não calcula uma cotação de preço. O plano mostra a soma estimada das sessões. Estimativas e limites do provedor não são tetos rígidos de cobrança; agentes filhos podem fazer outras requisições. Leia o plano completo antes de confirmar.'
  $budget = Read-Host 'Sua estimativa por sessão em USD (ex.: 0.25; vazio cancela)'
  if ([string]::IsNullOrEmpty($budget)) { return }
  $scratch = Join-Path $root 'owner/native-scratch'
  if (-not (Test-Path -LiteralPath $scratch)) { New-Item -ItemType Directory -Path $scratch | Out-Null }
  $arguments = @('--product-root',(Join-Path $root 'product'),'--user-home',(Join-Path $root 'owner'),'--workspace',(Join-Path $root 'workspace'),'--server',$bin,'--destination',$destination,'--version','0.0.0','--native-executable',$code.Source,'--native-scratch-root',$scratch,'--native-challenge-directory',(Join-Path $root 'workspace/native-qualification-controls'),'--max-budget-usd',$budget)
  $old = $env:CLAUDE_CONFIG_DIR
  try {
    $env:CLAUDE_CONFIG_DIR = Join-Path $root 'client/code'
    $review = & $bin native-review @arguments
    if ($LASTEXITCODE -ne 0) { Fail 'A revisão recusou a qualificação. Nenhum teste com modelo foi iniciado; preserve o diagnóstico privado e confira o login normal do perfil Code.' }
    Write-New (Join-Path $run 'native-review.json') (($review -join "`n") + "`n")
    Write-Host ($review -join "`n")
    $plan = ($review -join "`n") | ConvertFrom-Json
    if ($plan.plan.state -cne 'reviewable' -or $plan.review_digest -cnotmatch '^[0-9a-f]{64}$') { Fail 'O plano está indisponível. Nenhuma qualificação foi iniciada; preserve o diagnóstico privado.' }
    if ((Read-Host 'Li o plano completo e autorizo estes testes com modelo. Para executar, digite QUALIFICAR (qualquer outra resposta cancela)') -cne 'QUALIFICAR') { return }
    $result = & $bin native-qualify-and-build @arguments --review-digest $plan.review_digest --confirm
    $exit = $LASTEXITCODE
    Write-New (Join-Path $run 'native-result.json') (($result -join "`n") + "`n")
    if ($exit -ne 0) { Fail 'A qualificação não terminou. Preserve os diagnósticos privados e confira o relatório antes de voltar ao ensaio MCP comum.' }
    if ((($result -join "`n") | ConvertFrom-Json).state -cne 'qualified') { Fail 'Os testes não qualificaram os sete hooks. O resultado foi preservado; o ensaio MCP comum continua disponível. Compartilhe somente RELATORIO.md após conferir seu conteúdo.' }
    $inspection = & $bin inspect --packet --product-root $destination --user-home (Join-Path $root 'owner') --workspace (Join-Path $root 'workspace')
    if ($LASTEXITCODE -ne 0 -or (($inspection -join "`n") | ConvertFrom-Json).packet_mode -cne 'native') { Fail 'O pacote resultante não é nativo qualificado; nenhum cliente foi aberto. Preserve os diagnósticos privados.' }
    Write-Host 'Qualificação concluída. O launcher verificará o pacote e abrirá o Code com os sete hooks. Os diagnósticos native-*.json são privados; não os compartilhe com o relatório.'
    & (Join-Path $root 'launchers/Abrir Code.ps1')
  } finally {
    if ($null -eq $old) { Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue } else { $env:CLAUDE_CONFIG_DIR = $old }
  }
}
Write-Host "1 — Ensaio completo no Code`n2 — Retomar no Desktop`n3 — Conferir conexão Code (somente leitura)`n4 — Conferir conexão Desktop (somente leitura)`n5 — Qualificar os sete hooks do Code (testes com modelo; confirmação separada)`nEnter — Sair"
$choice = Read-Host 'Escolha'
switch ($choice) {
  '1' {
    if (-not $report.code_prompt) { Fail 'O relatório ainda não liberou o ensaio no cliente.' }
    if (-not (Get-Command claude -ErrorAction SilentlyContinue)) { Fail 'Disponibilize o comando claude pelo fluxo normal do Code e abra Testar Maestro novamente.' }
    Write-Host 'Abrindo o Code com o ensaio completo. Conclua o login normal se solicitado.'
    & (Join-Path $root 'launchers/Abrir Code.ps1') -InitialPrompt ([string]$report.code_prompt)
  }
  '2' {
    if (-not $report.desktop_prompt) { Fail 'O relatório ainda não liberou o ensaio no cliente.' }
    Set-Clipboard -Value $report.desktop_prompt
    Write-Host 'Prompt copiado. Confirme a instalação da extensão deste ensaio. Antes de colar, abra Configurações > Extensões > a extensão deste ensaio e, somente na linha Maestro context (maestro_context), selecione ✓ / Sempre permitir. Preserve as demais permissões. ✓ / Sempre permitir libera só a leitura; gravações de conteúdo exigem revisão exata no formulário nativo ou no navegador local selecionado neste ensaio. Depois cole uma vez em uma conversa nova do Desktop.'
    & (Join-Path $root 'launchers/Abrir Desktop.ps1')
  }
  '3' {
    if (-not $report.code_ready_prompt) { Fail 'O relatório ainda não liberou a conferência no Code.' }
    if (-not (Get-Command claude -ErrorAction SilentlyContinue)) { Fail 'Disponibilize o comando claude pelo fluxo normal do Code e abra Testar Maestro novamente.' }
    Write-Host 'Abrindo o Code com a conferência de conexão. Conclua o login normal se solicitado.'
    & (Join-Path $root 'launchers/Abrir Code.ps1') -InitialPrompt ([string]$report.code_ready_prompt)
  }
  '4' {
    if (-not $report.desktop_ready_prompt) { Fail 'O relatório ainda não liberou a conferência no Desktop.' }
    Set-Clipboard -Value $report.desktop_ready_prompt
    Write-Host 'Conferência copiada. Confirme a instalação da extensão deste ensaio. Antes de colar, abra Configurações > Extensões > a extensão deste ensaio e, somente na linha Maestro context (maestro_context), selecione ✓ / Sempre permitir. Preserve as demais permissões. ✓ / Sempre permitir libera só a leitura; gravações de conteúdo exigem revisão exata no formulário nativo ou no navegador local selecionado neste ensaio. Depois cole uma vez em uma conversa nova do Desktop.'
    & (Join-Path $root 'launchers/Abrir Desktop.ps1')
  }
  '5' { Qualify-Code }
  '' { }
  default { Fail 'Escolha 1, 2, 3, 4, 5 ou Enter.' }
}
