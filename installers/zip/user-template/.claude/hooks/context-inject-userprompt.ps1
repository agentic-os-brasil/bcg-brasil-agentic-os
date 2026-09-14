$ErrorActionPreference = 'Stop'
try {
    . (Join-Path $PSScriptRoot 'lib/Maestro.Hooks.ps1')
    Invoke-MaestroContextInject
} catch {
    [Console]::Error.WriteLine("maestro context-inject: $($_.Exception.Message)")
}
exit 0
