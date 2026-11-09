# Retain only the exact fifteen bounded synthetic proof files.
$ErrorActionPreference = 'Stop'
$evidence = Join-Path $env:RUNNER_TEMP 'native-engine-evidence'
New-Item -ItemType Directory -Path $evidence -ErrorAction Stop | Out-Null
$sources = @([pscustomobject]@{root=(Join-Path $env:RUNNER_TEMP 'native-engine-proof'); names=@('native-test.stdout.log', 'native-test.stderr.log', 'native-core-proof.json', 'engine-contract-results.json', 'engine-execution.log', 'engine-datahome.log', 'engine-ownerctx.log', 'engine-memory.log', 'engine-scheduler.log', 'engine-processidentity.log', 'engine-winprivate.log', 'engine-zipmigration.log', 'adapter-mcp.log', 'adapter-mcp-command.log')})
if ($env:MAESTRO_ENGINE_PUBLIC_ROOT) {
  $sources += [pscustomobject]@{root=$env:MAESTRO_ENGINE_PUBLIC_ROOT; names=@('engine-build-manifest.json')}
}
foreach ($source in $sources) {
  if (-not (Test-Path -LiteralPath $source.root)) { continue }
  $directory = Get-Item -LiteralPath $source.root -Force
  if ($directory -isnot [IO.DirectoryInfo] -or ($directory.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Evidence root must be an ordinary directory.' }
  foreach ($name in $source.names) {
    $path = Join-Path $source.root $name
    if (-not (Test-Path -LiteralPath $path)) { continue }
    $file = Get-Item -LiteralPath $path -Force
    if ($file -isnot [IO.FileInfo] -or ($file.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $file.Length -gt 2097152) { throw 'Evidence must be a bounded ordinary synthetic file.' }
    $destination = Join-Path $evidence $name
    Copy-Item -LiteralPath $path -Destination $destination
    if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -cne (Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash) { throw 'Evidence copy changed bytes.' }
  }
}
