$ErrorActionPreference = 'Stop'
try {
    . (Join-Path $PSScriptRoot 'lib/Maestro.Hooks.ps1')
    Invoke-MaestroSessionStartMemoryInject
} catch {
    [Console]::Error.WriteLine("maestro session-start: $($_.Exception.Message)")
}
exit 0
