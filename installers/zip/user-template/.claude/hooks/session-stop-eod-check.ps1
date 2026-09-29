$ErrorActionPreference = 'Stop'
try {
    . (Join-Path $PSScriptRoot 'lib/Maestro.Hooks.ps1')
    Invoke-MaestroSessionStopEodCheck
} catch {
    [Console]::Error.WriteLine("maestro session-stop-eod-check: $($_.Exception.Message)")
}
exit 0
