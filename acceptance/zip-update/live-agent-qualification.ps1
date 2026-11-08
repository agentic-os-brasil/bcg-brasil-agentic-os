param(
    [Parameter(Mandatory = $true)][string]$Zip,
    [Parameter(Mandatory = $true)][string]$Trace,
    [Parameter(Mandatory = $true)][string]$Receipt,
    [Parameter(Mandatory = $true)][Alias('Profile')][string]$TestProfile
)

$ErrorActionPreference = 'Stop'
if (-not [System.IO.Path]::IsPathRooted($TestProfile)) { throw 'Test profile must be an absolute real directory' }
$selectedProfile = (Resolve-Path -LiteralPath $TestProfile -ErrorAction Stop).Path.TrimEnd([char[]]@('\','/'))
if ([System.IO.Path]::GetFullPath($TestProfile).TrimEnd([char[]]@('\','/')) -ne $selectedProfile) { throw 'Test profile must not contain path aliases' }
$profileDirectory = Get-Item -LiteralPath $selectedProfile -Force
if (-not $profileDirectory.PSIsContainer) { throw 'Test profile must be a real directory' }
for ($directory = $profileDirectory; $null -ne $directory; $directory = $directory.Parent) {
    if (($directory.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Test profile must not contain directory aliases' }
}
$operatorProfile = $(if ($env:OS -eq 'Windows_NT') { $env:USERPROFILE } else { $env:HOME })
if ([System.IO.Path]::GetFullPath($operatorProfile).TrimEnd([char[]]@('\','/')) -eq $selectedProfile) { throw 'Use a dedicated test profile, not the operator profile' }
$profileConfig = Join-Path $selectedProfile '.claude'
if ($env:CLAUDE_CONFIG_DIR -and [System.IO.Path]::GetFullPath($env:CLAUDE_CONFIG_DIR).TrimEnd([char[]]@('\','/')) -ne $profileConfig) { throw 'CLAUDE_CONFIG_DIR conflicts with the selected test profile' }
$configItem = Get-Item -LiteralPath $profileConfig -Force -ErrorAction SilentlyContinue
if ($null -ne $configItem) {
    if (-not $configItem.PSIsContainer -or ($configItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Test profile Claude configuration must be a real directory' }
}
if ($env:OS -ne 'Windows_NT') { throw 'This live qualification must run on native Windows PowerShell.' }
if (Test-Path -LiteralPath $Trace) { throw "Trace already exists: $Trace" }
if (Test-Path -LiteralPath $Receipt) { throw "Receipt already exists: $Receipt" }
if (-not (Get-Command claude -ErrorAction SilentlyContinue)) { throw 'Claude Code is unavailable' }
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$sourceCommit = ((& git -C $repo rev-parse HEAD 2>$null) | Out-String).Trim()
$sourceStatus = ((& git -C $repo status --porcelain 2>$null) | Out-String).Trim()
if (-not $sourceCommit) { throw 'Source commit is unavailable' }
if ($sourceStatus) { throw 'Source tree is dirty; commit and freeze the candidate before issuing a receipt' }

. (Join-Path $PSScriptRoot 'lib/LiveAgentTrace.ps1')

$zipPath = (Resolve-Path -LiteralPath $Zip).Path
$tracePath = [System.IO.Path]::GetFullPath($Trace)
$receiptPath = [System.IO.Path]::GetFullPath($Receipt)
$scratch = Join-Path ([System.IO.Path]::GetTempPath()) ("maestro-live-agent-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $scratch -Force | Out-Null
$savedEnvironment = @{}
foreach ($name in @('HOME','USERPROFILE','MAESTRO_HOME','CLAUDE_CONFIG_DIR','CLAUDE_PROJECT_DIR')) { $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
try {
    Expand-Archive -LiteralPath $zipPath -DestinationPath $scratch -Force
    $product = Join-Path $scratch 'Maestro'
    $env:HOME = $selectedProfile
    $env:USERPROFILE = $selectedProfile
    $env:MAESTRO_HOME = Join-Path $selectedProfile '.maestro'
    $env:CLAUDE_PROJECT_DIR = $product
    Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue
    . (Join-Path $repo 'installers/zip/user-template/.claude/hooks/lib/maestro-runtime.ps1')
    $homeResult = Invoke-MaestroRuntime -Root $product -RuntimeArgs @('home','init','--product-root',$product,'--user-home',$selectedProfile)
    $home = ($homeResult -join "`n") | ConvertFrom-Json
    if (-not $home.writable) { throw 'Test profile HOME admission is unavailable; no model was started and runtime is NOT_QUALIFIED' }
    $prompt = 'Executa este teste de qualificacao de release com exatamente uma chamada real da ferramenta Agent. Chama Yoda e pede apenas QUALIFICACAO_YODA_OK. Nao simule nem substitua a chamada. Responde QUALIFICACAO_MAESTRO_OK somente depois de receber o retorno real.'
    Push-Location (Join-Path $scratch 'Maestro')
    try {
        & claude -p `
            --setting-sources project `
            --output-format stream-json `
            --verbose `
            --include-hook-events `
            --forward-subagent-text `
            --no-session-persistence `
            --model opus `
            --effort xhigh `
            --permission-mode dontAsk `
            --allowedTools=Agent `
            -- $prompt | Set-Content -LiteralPath $tracePath -Encoding UTF8
        $claudeRc = $LASTEXITCODE
    } finally {
        Pop-Location
    }

    $events = @()
    foreach ($line in Get-Content -LiteralPath $tracePath -Encoding UTF8) {
        try { $events += ($line | ConvertFrom-Json) } catch {}
    }

    $evaluation = Get-MaestroAgentTraceEvaluation -Events $events -ClaudeExitCode $claudeRc
    $checks = $evaluation.Checks
    $agentIds = $evaluation.AgentToolUseIds
    $allPassed = $evaluation.Passed
    $receiptObject = [ordered]@{
        schema_version = 1
        evidence_kind = 'maestro_claude_agent_release_qualification'
        platform = 'Windows'
        architecture = $env:PROCESSOR_ARCHITECTURE
        powershell_version = $PSVersionTable.PSVersion.ToString()
        release_sha256 = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
        source_commit = $sourceCommit
        claude_code = ((& claude --version 2>$null | Out-String).Trim())
        agent_tool_use_ids = $agentIds
        checks = $checks
        verdict = $(if ($allPassed) { 'PASS' } else { 'FAIL' })
        limits = @('controlled qualification prompt', 'not release publication or signing evidence')
    }
    $receiptObject | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $receiptPath -Encoding UTF8
    Write-Host $receiptObject.verdict
    if (-not $allPassed) { exit 1 }
} finally {
    foreach ($name in $savedEnvironment.Keys) { [Environment]::SetEnvironmentVariable($name, $savedEnvironment[$name], 'Process') }
    if (Test-Path -LiteralPath $scratch) { Remove-Item -LiteralPath $scratch -Recurse -Force }
}
