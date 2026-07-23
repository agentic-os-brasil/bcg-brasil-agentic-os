[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)] [string]$ArtifactPath,
    [Parameter(Mandatory = $true)] [string]$ChecksumPath,
    [string]$InstallRoot = $(Join-Path $env:LOCALAPPDATA "BCGOS\trial"),
    [switch]$AllowUnsignedTrial
)

$ErrorActionPreference = "Stop"

if (-not $AllowUnsignedTrial) { throw "-AllowUnsignedTrial is required. This local trial is not a signed release installer." }
if (-not (Test-Path -LiteralPath $ArtifactPath -PathType Leaf)) { throw "Artifact was not found: $ArtifactPath" }
if (-not (Test-Path -LiteralPath $ChecksumPath -PathType Leaf)) { throw "Checksum file was not found: $ChecksumPath" }
if (Test-Path -LiteralPath $InstallRoot) { throw "Trial installation already exists at $InstallRoot; it was not replaced." }

$checksumLine = (Get-Content -LiteralPath $ChecksumPath -TotalCount 1).Trim()
if ($checksumLine -notmatch '^(?<hash>[A-Fa-f0-9]{64})\s+\*?.+$') { throw "Checksum file does not begin with a SHA-256 digest and filename." }
$expectedHash = $Matches.hash.ToLower()
$actualHash = (Get-FileHash -LiteralPath $ArtifactPath -Algorithm SHA256).Hash.ToLower()
if ($actualHash -ne $expectedHash) { throw "Artifact checksum does not match; nothing was installed." }

$parent = Split-Path -Parent $InstallRoot
New-Item -ItemType Directory -Force -Path $parent | Out-Null
$stage = Join-Path $parent (".bcgos-trial-stage-" + [guid]::NewGuid())

try {
    New-Item -ItemType Directory -Force -Path (Join-Path $stage "bin") | Out-Null
    $target = Join-Path $stage "bin\bcgos.exe"
    Copy-Item -LiteralPath $ArtifactPath -Destination $target
    & $target version | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Artifact did not pass its version self-check; nothing was installed." }
    Move-Item -LiteralPath $stage -Destination $InstallRoot
    $stage = $null
    Write-Output "BCGOS trial installed at $(Join-Path $InstallRoot 'bin\bcgos.exe')"
    Write-Output "This unsigned local trial does not configure PATH or automatic updates."
} finally {
    if ($null -ne $stage -and (Test-Path -LiteralPath $stage)) { Remove-Item -Recurse -Force -LiteralPath $stage }
}
