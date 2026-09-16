param(
    [Parameter(Mandatory = $true)]
    [string]$Zip,

    [string]$Output = 'windows-powershell-offline.json',

    [switch]$AllowNonWindowsContractTest
)

$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$sourceCommit = ((& git -C $repo rev-parse HEAD 2>$null) | Out-String).Trim()
$sourceStatus = ((& git -C $repo status --porcelain 2>$null) | Out-String).Trim()
if (-not $sourceCommit) { throw 'Source commit is unavailable' }
if ($sourceStatus) { throw 'Source tree is dirty; commit and freeze the candidate before issuing a receipt' }
$zipPath = (Resolve-Path $Zip).Path
$nativeWindows = ($env:OS -eq 'Windows_NT')
if (-not $nativeWindows -and -not $AllowNonWindowsContractTest) {
    throw 'Native Windows qualification must run on Windows. Use -AllowNonWindowsContractTest only for local parser/contract development.'
}
$sidecar = [System.IO.Path]::ChangeExtension($zipPath, '.sha256')
if (-not (Test-Path -LiteralPath $sidecar -PathType Leaf)) { throw "Missing checksum sidecar: $sidecar" }

$expected = ((Get-Content -LiteralPath $sidecar -Raw).Trim() -split '\s+')[0].ToLowerInvariant()
$actual = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
if ($expected -ne $actual) { throw "SHA-256 mismatch: expected $expected, got $actual" }

$scratch = Join-Path ([System.IO.Path]::GetTempPath()) ("maestro-native-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $scratch -Force | Out-Null
try {
    Expand-Archive -LiteralPath $zipPath -DestinationPath $scratch -Force
    $maestro = Join-Path $scratch 'Maestro'
    if (-not (Test-Path -LiteralPath (Join-Path $maestro 'VERSION') -PathType Leaf)) { throw 'Extracted ZIP has no Maestro/VERSION' }
    if (Test-Path -LiteralPath (Join-Path $maestro 'data')) { throw 'Release ZIP must not contain data/' }
    $runbookPath = Join-Path $maestro 'UPDATE-RUNBOOK.md'
    if (-not (Test-Path -LiteralPath $runbookPath -PathType Leaf)) { throw 'Release ZIP has no UPDATE-RUNBOOK.md' }
    $runbook = Get-Content -LiteralPath $runbookPath -Raw -Encoding UTF8
    foreach ($required in @(
        'contract_id: maestro-update-long-run-v1',
        'model_family: opus',
        'preferred_effort: xhigh',
        '  - yoda',
        '/goal',
        'receipt_bindings:',
        '  - target_release_sha256',
        '  - target_core_sha256',
        '  - baseline_manifest_sha256',
        '  - installation_root_sha256'
    )) {
        if (-not $runbook.Contains($required)) { throw "Long-running update contract is missing: $required" }
    }
    if ($runbook.Contains('  - darwin')) { throw 'Long-running update contract must require Yoda only' }
    Write-Host 'PASS  long-running update contract binds Opus/xhigh, goal continuity, install identity and Yoda'

    $smokeOutput = @(& (Join-Path $repo 'acceptance/zip-update/powershell-hook-smoke.ps1') `
        -MaestroRoot $maestro `
        -SettingsPath (Join-Path $maestro '.claude/settings.json'))
    $smokeExit = $LASTEXITCODE
    $smokeOutput | ForEach-Object { Write-Host $_ }
    if ($smokeExit -ne 0) { throw "PowerShell hook smoke failed with exit $smokeExit" }
    $summary = (($smokeOutput | Where-Object { $_ -match '^Summary:' } | Select-Object -Last 1 | Out-String).Trim())
    if ($summary -notmatch 'Summary:\s+(\d+) pass,\s+(\d+) fail') { throw 'PowerShell hook smoke summary is unavailable' }
    $passed = [int]$Matches[1]
    $failed = [int]$Matches[2]
    $claudeVersion = 'unavailable'
    if (Get-Command claude -ErrorAction SilentlyContinue) { $claudeVersion = ((& claude --version 2>$null | Out-String).Trim()) }

    $receipt = [ordered]@{
        schema_version = 1
        verdict = $(if ($nativeWindows) { 'PASS' } else { 'CONTRACT_PASS_NON_WINDOWS' })
        platform = $(if ($nativeWindows) { 'windows-powershell-native' } else { 'powershell-contract-non-windows' })
        powershell_edition = $PSVersionTable.PSEdition
        powershell_version = $PSVersionTable.PSVersion.ToString()
        architecture = $env:PROCESSOR_ARCHITECTURE
        claude_code = $claudeVersion
        release_version = (Get-Content -LiteralPath (Join-Path $maestro 'VERSION') -Raw).Trim()
        release_sha256 = $actual
        source_commit = $sourceCommit
        zip_name = [System.IO.Path]::GetFileName($zipPath)
        checks = [ordered]@{
            passed = $passed
            failed = $failed
            skipped = 0
            checksum_matched_sidecar = $true
            release_contains_no_data = $true
            powershell_wiring_and_runtime_passed = $true
            long_running_update_contract_present = $true
        }
        limitations = @(
            'offline hook qualification only',
            'does not prove a live Claude Agent tool call',
            'does not publish or sign the release'
        )
    }
    $receipt | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $Output -Encoding UTF8
    Write-Host "PASS  Windows PowerShell native receipt: $Output"
} finally {
    if (Test-Path -LiteralPath $scratch) { Remove-Item -LiteralPath $scratch -Recurse -Force }
}
