#Requires -Version 5.1
[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$Output)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$Utf8 = New-Object Text.UTF8Encoding($false)
if ($env:OS -ne 'Windows_NT' -or $PSVersionTable.PSEdition -ne 'Desktop' -or -not [Environment]::Is64BitProcess) { throw 'Use native Windows x64 and stock Windows PowerShell5.1.' }
$processors = @(Get-CimInstance -ClassName Win32_Processor -Property Architecture)
if ($processors.Count -lt 1 -or @($processors | Where-Object { $_.Architecture -ne 9 }).Count -ne 0) { throw 'The producer target requires native Windows x64.' }
$source = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
Push-Location (Join-Path $source 'adapters/mcp')
try {
    # Go's effective values include persisted GOENV settings, not just process variables.
    $goEnvironment = @(& go env -json GOOS GOARCH GOFLAGS)
    if ($LASTEXITCODE -ne 0) { throw 'Effective Go environment lookup failed; no artifact output was created.' }
    $goEnvironmentJson = $goEnvironment -join "`n"
    if ($goEnvironmentJson.Length -gt 16384 -or -not $goEnvironmentJson.TrimStart().StartsWith('{')) { throw 'Effective Go environment output is not a bounded JSON object.' }
    $effectiveGo = $goEnvironmentJson | ConvertFrom-Json
    $fields = @($effectiveGo.PSObject.Properties.Name)
    if ($fields.Count -ne 3 -or @($fields | Where-Object { @('GOOS', 'GOARCH', 'GOFLAGS') -cnotcontains $_ }).Count -ne 0) { throw 'Effective Go environment fields differ from the closed producer contract.' }
    foreach ($field in $fields) {
        if ($effectiveGo.$field -isnot [string]) { throw 'Effective Go environment values must be strings.' }
    }
    if ($effectiveGo.GOOS -cne 'windows' -or $effectiveGo.GOARCH -cne 'amd64' -or $effectiveGo.GOFLAGS.Length -ne 0) { throw 'The ordinary producer requires effective windows/amd64 with empty GOFLAGS; existing Go settings were preserved.' }
} finally { Pop-Location }
if ($Output -notmatch '^[A-Za-z]:\\' -or $Output -notmatch '\\Developer\\' -or [IO.Path]::GetFullPath($Output) -cne $Output -or (Test-Path -LiteralPath $Output) -or (Test-Path -LiteralPath ($Output + '.zip'))) { throw 'Use a new absolute Developer-local output; previous artifacts remain unchanged.' }
$parent = [IO.Path]::GetDirectoryName($Output)
if (-not (Test-Path -LiteralPath $parent)) { throw 'Create the Developer-local output parent before producing this kit.' }
for ($walk = Get-Item -LiteralPath $parent -Force; $null -ne $walk; $walk = $walk.Parent) {
    if (([int]$walk.Attributes -band 0x00441400) -ne 0) { throw 'Producer output cannot traverse a reparse or offline directory.' }
}
[void][IO.Directory]::CreateDirectory((Join-Path $Output 'bin'))
$binary = Join-Path $Output 'bin/maestro-mcp-windows-amd64.exe'
Push-Location (Join-Path $source 'adapters/mcp')
try {
    & go build -trimpath -o $binary ./cmd/maestro-mcp
    if ($LASTEXITCODE -ne 0) { throw 'Ordinary server build failed.' }
} finally { Pop-Location }
foreach ($pair in @(
    @('dev/pilot/setup-windows.ps1', 'Preparar Maestro.ps1'),
    @('dev/pilot/start-windows.cmd', 'Preparar Maestro.cmd'),
    @('dev/pilot/LEIA-ME-WINDOWS.md', 'LEIA-ME.md'),
    @('dev/pilot/ROTEIRO-WINDOWS.md', 'ROTEIRO.md'),
    @('LICENSE.md', 'LICENSE.md')
)) { [IO.File]::Copy((Join-Path $source $pair[0]), (Join-Path $Output $pair[1]), $false) }
$base = & git -C $source rev-parse HEAD
if ($LASTEXITCODE -ne 0) { throw 'Source commit is unavailable.' }
$metadata = @"
mode=ordinary-unversioned-developer-rehearsal
binary_source_base_commit=$base
source_state=development_snapshot_native_sha256_identifies_exact_bytes
server_version=unavailable
packet_label=0.0.0
platforms=windows-amd64
native_client_loading=not-yet-observed
windows_amd64_evidence=requires_exact_artifact_standard_user_preparation_and_sdk_proof
signed_install_update=unavailable
docling_unseeded=unavailable
native_agents_hooks=unavailable
"@
[IO.File]::WriteAllText((Join-Path $Output 'ARTIFACT.txt'), $metadata + "`n", $Utf8)
$files = @(Get-ChildItem -LiteralPath $Output -File -Recurse | ForEach-Object {
    $relative = $_.FullName.Substring($Output.Length + 1).Replace('\', '/')
    '{0}  ./{1}' -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(), $relative
})
[Array]::Sort($files, [StringComparer]::Ordinal)
$content = $Utf8.GetBytes(($files -join "`n") + "`n")
$hasher = [Security.Cryptography.SHA256]::Create()
try { $id = ([BitConverter]::ToString($hasher.ComputeHash($content))).Replace('-', '').ToLowerInvariant() } finally { $hasher.Dispose() }
[IO.File]::WriteAllText((Join-Path $Output 'KIT-ID.txt'), ('maestro-windows-rehearsal-20261105-' + $id + "`n"), $Utf8)
$files += '{0}  ./KIT-ID.txt' -f (Get-FileHash -LiteralPath (Join-Path $Output 'KIT-ID.txt') -Algorithm SHA256).Hash.ToLowerInvariant()
[Array]::Sort($files, [StringComparer]::Ordinal)
[IO.File]::WriteAllText((Join-Path $Output 'SHA256SUMS'), ($files -join "`n") + "`n", $Utf8)
# Windows PowerShell targets older .NET Framework defaults; emit portable ZIP separators.
[AppContext]::SetSwitch('Switch.System.IO.Compression.ZipFile.UseBackslash', $false)
Add-Type -AssemblyName System.IO.Compression.FileSystem
[IO.Compression.ZipFile]::CreateFromDirectory($Output, $Output + '.zip', [IO.Compression.CompressionLevel]::Optimal, $true)
$zipHash = (Get-FileHash -LiteralPath ($Output + '.zip') -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::WriteAllText($Output + '.zip.sha256', ($zipHash + '  ' + [IO.Path]::GetFileName($Output) + ".zip`n"), $Utf8)
Write-Output ('Kit: ' + $Output + '.zip')
Write-Output ('SHA256: ' + $zipHash)
