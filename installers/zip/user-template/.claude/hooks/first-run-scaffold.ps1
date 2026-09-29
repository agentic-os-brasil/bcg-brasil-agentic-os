$ErrorActionPreference = 'Stop'
try {
    . (Join-Path $PSScriptRoot 'lib/Maestro.Hooks.ps1')
    Invoke-MaestroFirstRunScaffold
} catch {
    $reason = $_.Exception.Message
    [Console]::Out.WriteLine('Scaffold unavailable; initialization is incomplete. Run /maestro-doctor.')
    [Console]::Error.WriteLine("maestro first-run-scaffold: $reason")
}
exit 0
