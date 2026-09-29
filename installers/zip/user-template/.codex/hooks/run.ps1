param([Parameter(Mandatory=$true)][string]$Event)
$ErrorActionPreference = 'Stop'
try {
  [Console]::InputEncoding = New-Object System.Text.UTF8Encoding($false)
  $root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
  . (Join-Path $root '.claude/hooks/lib/maestro-runtime.ps1')
  Invoke-MaestroRuntime -Root $root -RuntimeArgs @('codex-hook', $Event, '--root', $root) -Payload ([Console]::In.ReadToEnd())
} catch {
  [Console]::Error.WriteLine('Maestro native helper unavailable. Verify this installation before retrying.')
  exit 2
}
