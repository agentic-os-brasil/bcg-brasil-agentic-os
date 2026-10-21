param([Parameter(Mandatory = $true)][string]$Destination)

# Development-only direct probes; Code tab qualification remains attended.
$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT') { throw 'Run this rehearsal on Windows.' }
$developer = [IO.Path]::GetFullPath((Join-Path $env:USERPROFILE 'Developer'))
$destinationPath = [IO.Path]::GetFullPath($Destination)
if (-not $destinationPath.StartsWith($developer + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Choose a new destination physically under USERPROFILE\Developer.'
}
if (Test-Path -LiteralPath $destinationPath) { throw 'Destination exists; nothing overwritten. Choose a new destination.' }
# Refuse redirected ancestors instead of following a junction into a cloud/profile location.
$scratch = Join-Path $developer 'tmp'
$cache = Join-Path $developer 'ToolData/go-build'
foreach ($writePath in @($destinationPath, $scratch, $cache)) {
    $ancestor = $writePath
    while ($ancestor) {
        if ((Test-Path -LiteralPath $ancestor) -and ((Get-Item -Force -LiteralPath $ancestor).Attributes -band [IO.FileAttributes]::ReparsePoint)) {
            throw 'Write path ancestor is a reparse point; choose physical Developer paths.'
        }
        $ancestor = Split-Path -Parent $ancestor
    }
}
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
Push-Location $repo
$saved = @{}
foreach ($key in @('TEMP', 'TMP', 'GOTMPDIR', 'GOCACHE', 'CLAUDE_PLUGIN_ROOT', 'CLAUDE_PLUGIN_DATA')) {
    $saved[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
}
try {
    New-Item -ItemType Directory -Force -Path $scratch, $cache, (Split-Path -Parent $destinationPath) | Out-Null
    $env:TEMP = $scratch; $env:TMP = $scratch; $env:GOTMPDIR = $scratch; $env:GOCACHE = $cache
    go test ./dev/spikes/plugin429 -count=1 -v
    if ($LASTEXITCODE -ne 0) { throw 'Focused Go tests failed.' }
    go run ./dev/spikes/plugin429 build $destinationPath
    if ($LASTEXITCODE -ne 0) { throw 'Build failed; partial output retained for inspection.' }
    $env:CLAUDE_PLUGIN_ROOT = Join-Path $destinationPath 'versions/0.0.1/maestro-spike'
    $env:CLAUDE_PLUGIN_DATA = Join-Path $destinationPath 'synthetic-data'
    $sandbox = Join-Path $destinationPath 'work space ç'
    New-Item -ItemType Directory -Path $sandbox | Out-Null
    $hook = Join-Path $env:CLAUDE_PLUGIN_ROOT 'scripts/maestro-spike-hook.exe'
    $config = Get-Content -Raw -LiteralPath (Join-Path $env:CLAUDE_PLUGIN_ROOT 'hooks/hooks.json') | ConvertFrom-Json
    foreach ($group in @($config.hooks.SessionStart) + @($config.hooks.PreToolUse)) {
        foreach ($handler in $group.hooks) {
            if ($handler.command -cne '${CLAUDE_PLUGIN_ROOT}/scripts/maestro-spike-hook.exe' -or @($handler.args).Count -ne 1) {
                throw 'Expected Windows native exec-form hooks.'
            }
        }
    }
    $comparison = Get-Content -Raw -LiteralPath (Join-Path $env:CLAUDE_PLUGIN_ROOT 'hooks/compare-shells.json') | ConvertFrom-Json
    foreach ($group in @($comparison.hooks.SessionStart) + @($comparison.hooks.PreToolUse)) {
        if (@($group.hooks).Count -ne 2 -or $group.hooks[0].shell -cne 'bash' -or $group.hooks[1].shell -cne 'powershell') {
            throw 'Expected separate Bash/PowerShell comparison handlers.'
        }
    }
    foreach ($name in @('compare-shells.sh', 'compare-shells.ps1')) {
        if (-not (Test-Path -LiteralPath (Join-Path $env:CLAUDE_PLUGIN_ROOT "scripts/$name") -PathType Leaf)) {
            throw 'Missing shell comparison wrapper.'
        }
    }
    function Invoke-Probe([string]$Mode, [string]$InputJson, [int]$Expected) {
        $start = New-Object Diagnostics.ProcessStartInfo
        $start.FileName = $hook; $start.Arguments = $Mode
        $start.UseShellExecute = $false; $start.CreateNoWindow = $true
        $start.RedirectStandardInput = $true; $start.RedirectStandardOutput = $true; $start.RedirectStandardError = $true
        $process = [Diagnostics.Process]::Start($start)
        try {
            # .NET Framework in PowerShell 5.1 has no StandardInputEncoding property.
            $inputBytes = [Text.Encoding]::UTF8.GetBytes($InputJson)
            $process.StandardInput.BaseStream.Write($inputBytes, 0, $inputBytes.Length)
            $process.StandardInput.BaseStream.Close()
            $stdout = $process.StandardOutput.ReadToEnd()
            $stderr = $process.StandardError.ReadToEnd()
            $process.WaitForExit()
            if ($process.ExitCode -ne $Expected) { throw "Probe $Mode expected $Expected, got $($process.ExitCode): $stderr" }
            return $stdout
        } finally { $process.Dispose() }
    }
    foreach ($tool in @('Write', 'Edit', 'MultiEdit', 'NotebookEdit')) {
        $payload = @{ hook_event_name = 'PreToolUse'; cwd = $sandbox; tool_name = $tool; tool_input = @{ file_path = './maestro-spike-denied.txt'; notebook_path = './maestro-spike-denied.txt' } } | ConvertTo-Json -Depth 4 -Compress
        $null = Invoke-Probe 'guard' $payload 2
    }
    if (Test-Path -LiteralPath (Join-Path $sandbox 'maestro-spike-denied.txt')) { throw 'Guard created the marker.' }
    $payload = @{ hook_event_name = 'PreToolUse'; cwd = $sandbox; tool_name = 'Write'; tool_input = @{ file_path = 'allowed.txt' } } | ConvertTo-Json -Depth 4 -Compress
    $null = Invoke-Probe 'guard' $payload 0
    $null = Invoke-Probe 'guard' '{' 2
    foreach ($tool in @('Agent', 'Task')) {
        $payload = @{ hook_event_name = 'PreToolUse'; tool_name = $tool; tool_input = @{ prompt = 'synthetic content must not be retained' } } | ConvertTo-Json -Depth 4 -Compress
        $null = Invoke-Probe 'delegation' $payload 0
    }
    $log = Get-Content -LiteralPath (Join-Path $env:CLAUDE_PLUGIN_DATA 'delegation.jsonl')
    if (@($log).Count -ne 2 -or $log[0] -cne '{"tool_name":"Agent"}' -or $log[1] -cne '{"tool_name":"Task"}') { throw 'Unexpected delegation log.' }
    $session = Invoke-Probe 'session-start' '{}' 0
    if ($session -notmatch '^MAESTRO_SPIKE revision=0\.0\.1;') { throw 'Missing SessionStart probe output.' }
    $packet = [ordered]@{
        evidence = 'direct-synthetic-only'; issue = 431; source_commit = (git rev-parse HEAD)
        powershell = $PSVersionTable.PSVersion.ToString(); go = (go version)
        helper_sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $hook).Hash
        runtime_sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $env:CLAUDE_PLUGIN_ROOT 'scripts/maestro-runtime.exe')).Hash
        guard = 'PASS'; delegation_metadata = 'PASS'; session_start_direct = 'PASS'
        shell_comparison = 'NOT_RUN'; code_tab = 'NOT_RUN'; corporate_windows = 'NOT_RUN'; synced_folder = 'NOT_RUN'; source_reload = 'NOT_RUN'; f0 = 'OPEN'
    }
    $packetPath = Join-Path $destinationPath 'windows-rehearsal.json'
    [IO.File]::WriteAllText($packetPath, ($packet | ConvertTo-Json -Depth 4), (New-Object Text.UTF8Encoding($false)))
    Write-Output "Direct synthetic checks passed. F0 OPEN. Evidence: $packetPath"
} finally {
    foreach ($key in $saved.Keys) { [Environment]::SetEnvironmentVariable($key, $saved[$key], 'Process') }
    Pop-Location
}
