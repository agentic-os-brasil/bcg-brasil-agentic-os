$ErrorActionPreference = 'Stop'
try {
    . (Join-Path $PSScriptRoot 'lib/Maestro.Hooks.ps1')
    Invoke-MaestroCrossCaseGuard
} catch {
    [Console]::Error.WriteLine("Cross-case write blocked. The PowerShell guard failed safely: $($_.Exception.Message)")
    exit 2
}
exit 0
