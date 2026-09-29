$ErrorActionPreference = 'Stop'
try {
    . (Join-Path $PSScriptRoot 'lib/Maestro.Hooks.ps1')
    Invoke-MaestroSessionStopAgentCheck
} catch {
    [Console]::Error.WriteLine("maestro session-stop-agent-check: $($_.Exception.Message)")
}
exit 0
