param([Parameter(Mandatory = $true)][ValidateSet('session-start', 'guard', 'delegation')][string]$Mode)

# Inherit raw standard handles; Framework redirected stdin writes an encoding preamble.
$ErrorActionPreference = 'Stop'
[Console]::Error.WriteLine("MAESTRO_SPIKE_HANDLER=powershell/$Mode")
$start = New-Object Diagnostics.ProcessStartInfo
$start.FileName = Join-Path $PSScriptRoot 'maestro-spike-hook.exe'
$start.Arguments = $Mode
$start.UseShellExecute = $false
$process = [Diagnostics.Process]::Start($start)
try {
    $process.WaitForExit()
    exit $process.ExitCode
} finally { $process.Dispose() }
