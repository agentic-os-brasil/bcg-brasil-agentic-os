$ErrorActionPreference = 'Stop'
try {
    . (Join-Path $PSScriptRoot 'lib/Maestro.Hooks.ps1')
    Invoke-MaestroFirstRunScaffold
} catch {
    $reason = $_.Exception.Message
    $project = $env:CLAUDE_PROJECT_DIR
    if ($project) {
        try {
            $breadcrumb = Join-Path $project 'FIRST-RUN-FAILED.txt'
            $message = "O Maestro nao conseguiu preparar data/. Rode /maestro-doctor. Motivo: $reason`r`n"
            [System.IO.File]::WriteAllText($breadcrumb, $message, (New-Object System.Text.UTF8Encoding($true)))
        } catch {}
    }
    [Console]::Error.WriteLine("maestro first-run-scaffold: $reason")
}
exit 0
