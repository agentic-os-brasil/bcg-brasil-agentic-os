$ErrorActionPreference = 'Stop'
try {
    . (Join-Path $PSScriptRoot 'lib/Maestro.Hooks.ps1')
    Invoke-MaestroSessionStopBrainIndex
} catch {
    [Console]::Error.WriteLine("maestro session-stop-brain-index: $($_.Exception.Message)")
}
exit 0
