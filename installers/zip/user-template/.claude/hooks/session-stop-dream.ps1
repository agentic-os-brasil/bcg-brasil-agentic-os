$ErrorActionPreference = 'Stop'
try {
    . (Join-Path $PSScriptRoot 'lib/Maestro.Hooks.ps1')
    Invoke-MaestroSessionStopDream
} catch {
    [Console]::Error.WriteLine("maestro session-stop: $($_.Exception.Message)")
}
exit 0
