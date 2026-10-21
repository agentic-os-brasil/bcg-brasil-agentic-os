param([Parameter(Mandatory = $true)][ValidateSet('session-start', 'guard', 'delegation')][string]$Mode)

# Attended Windows comparison only; copy raw stdin for PowerShell 5.1/7.
$ErrorActionPreference = 'Stop'
[Console]::Error.WriteLine("MAESTRO_SPIKE_HANDLER=powershell/$Mode")
$start = New-Object Diagnostics.ProcessStartInfo
$start.FileName = Join-Path $PSScriptRoot 'maestro-spike-hook.exe'
$start.Arguments = $Mode
$start.UseShellExecute = $false
$start.RedirectStandardInput = $true
$process = [Diagnostics.Process]::Start($start)
try {
    [Console]::OpenStandardInput().CopyTo($process.StandardInput.BaseStream)
    $process.StandardInput.BaseStream.Close()
    $process.WaitForExit()
    exit $process.ExitCode
} finally { $process.Dispose() }
