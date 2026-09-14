$ErrorActionPreference = 'Stop'
try {
    . (Join-Path $PSScriptRoot 'lib/Maestro.Hooks.ps1')
    Invoke-MaestroAgentAnnouncement
} catch {
    [Console]::Error.WriteLine("maestro agent-announcement: $($_.Exception.Message)")
}
exit 0
