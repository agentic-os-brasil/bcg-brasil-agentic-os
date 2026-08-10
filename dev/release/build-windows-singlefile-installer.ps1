[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$')]
    [string]$Version,
    [Parameter(Mandatory = $true)]
    [string]$Icon,
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[a-f0-9]{64}$')]
    [string]$IconSHA256,
    [Parameter(Mandatory = $true)]
    [string]$WizardDir,
    [Parameter(Mandatory = $true)]
    [string]$ReleaseDirectory,
    [Parameter(Mandatory = $true)]
    [string]$AuthorityRegistry,
    [Parameter(Mandatory = $true)]
    [string]$Bootstrapper,
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[a-f0-9]{64}$')]
    [string]$ResourceCompilerSHA256,
    [Parameter(Mandatory = $true)]
    [string]$OutputFile,
    [string]$ResourceCompiler = "windres",
    [switch]$CanarySimple,
    [switch]$LocalBeta,
    [string]$LocalBetaIssuer = "",
    [string]$LocalBetaKeyID = "",
    [string]$LocalBetaAuthorityRegistrySHA256 = "",
    [string]$LocalBetaBootstrapperSHA256 = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$localBetaValues = @(
    $LocalBetaIssuer,
    $LocalBetaKeyID,
    $LocalBetaAuthorityRegistrySHA256,
    $LocalBetaBootstrapperSHA256
)
if ($LocalBeta) {
    if ([string]::IsNullOrWhiteSpace($LocalBetaIssuer) -or
        [string]::IsNullOrWhiteSpace($LocalBetaKeyID) -or
        [string]::IsNullOrWhiteSpace($LocalBetaAuthorityRegistrySHA256) -or
        [string]::IsNullOrWhiteSpace($LocalBetaBootstrapperSHA256)) {
        throw "LocalBeta requires issuer, key ID, authority-registry SHA-256 and bootstrapper SHA-256 pins."
    }
}
elseif (@($localBetaValues | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }).Count -ne 0) {
    throw "LocalBeta issuer, key ID and digest pins are valid only with -LocalBeta."
}

function Require-AbsolutePath([string]$Path, [string]$Label) {
    if (-not [IO.Path]::IsPathRooted($Path)) {
        throw "$Label must be an absolute path."
    }
}

Require-AbsolutePath $Icon "Icon"
Require-AbsolutePath $WizardDir "WizardDir"
Require-AbsolutePath $ReleaseDirectory "ReleaseDirectory"
Require-AbsolutePath $AuthorityRegistry "AuthorityRegistry"
Require-AbsolutePath $Bootstrapper "Bootstrapper"
Require-AbsolutePath $OutputFile "OutputFile"

$iconItem = Get-Item -LiteralPath $Icon -ErrorAction Stop
if ($iconItem.PSIsContainer -or ($iconItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    throw "Icon must be a regular non-reparse file."
}
if ((Get-FileHash -LiteralPath $iconItem.FullName -Algorithm SHA256).Hash.ToLowerInvariant() -ne $IconSHA256) {
    throw "Icon SHA-256 does not match the approved input."
}
$outputPath = [IO.Path]::GetFullPath($OutputFile)
if ([IO.Path]::GetExtension($outputPath) -ne ".exe") {
    throw "OutputFile must have the .exe extension."
}
if (Test-Path -LiteralPath $outputPath) {
    throw "OutputFile already exists: $outputPath"
}
if ($LocalBeta) {
    $expectedOutputName = "Maestro-Installer-$Version-windows-amd64-local-beta-unsigned.exe"
    if ([IO.Path]::GetFileName($outputPath) -ne $expectedOutputName) {
        throw "LocalBeta OutputFile must be named $expectedOutputName."
    }
}
$outputParent = [IO.Path]::GetDirectoryName($outputPath)
if ([string]::IsNullOrWhiteSpace($outputParent) -or -not (Test-Path -LiteralPath $outputParent)) {
    throw "OutputFile parent directory must already exist: $outputParent"
}

$compiler = Get-Command $ResourceCompiler -CommandType Application -ErrorAction Stop
$compilerItem = Get-Item -LiteralPath $compiler.Source -ErrorAction Stop
if ($compilerItem.PSIsContainer -or ($compilerItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    throw "Resource compiler must be a regular non-reparse file."
}
$compilerDigest = (Get-FileHash -LiteralPath $compiler.Source -Algorithm SHA256).Hash.ToLowerInvariant()
if ($compilerDigest -ne $ResourceCompilerSHA256) {
    throw "Resource compiler SHA-256 is not the approved fingerprint."
}

$root = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$factory = Join-Path $PSScriptRoot "build-windows-installer.ps1"
$workingRoot = Join-Path ([IO.Path]::GetTempPath()) ("maestro-singlefile-" + [Guid]::NewGuid().ToString("N"))
$packageDirectory = Join-Path $workingRoot "package"
$wrapperBase = Join-Path $workingRoot "maestro-installer-wrapper.exe"
$nonce = [Guid]::NewGuid().ToString("N")
$rcPath = Join-Path ([IO.Path]::GetTempPath()) ("maestro-installer-singlefile-$nonce.rc")
$outerPackageDir = Join-Path $root "cmd\maestro-installer-singlefile"
$sysoPath = Join-Path $outerPackageDir ("maestro-installer-singlefile-$nonce.syso")
$go = Get-Command "go" -CommandType Application -ErrorAction Stop
$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
$succeeded = $false

try {
    New-Item -ItemType Directory -Path $workingRoot -Force | Out-Null

    & $factory `
        -Version $Version `
        -Icon $Icon `
        -IconSHA256 $IconSHA256 `
        -WizardDir $WizardDir `
        -ReleaseDirectory $ReleaseDirectory `
        -AuthorityRegistry $AuthorityRegistry `
        -Bootstrapper $Bootstrapper `
        -ResourceCompilerSHA256 $ResourceCompilerSHA256 `
        -ResourceCompiler $ResourceCompiler `
        -Windowed `
    -OutputDirectory $packageDirectory `
    -CanarySimple:$CanarySimple `
    -LocalBeta:$LocalBeta `
        -LocalBetaIssuer $LocalBetaIssuer `
        -LocalBetaKeyID $LocalBetaKeyID `
        -LocalBetaAuthorityRegistrySHA256 $LocalBetaAuthorityRegistrySHA256 `
        -LocalBetaBootstrapperSHA256 $LocalBetaBootstrapperSHA256
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath (Join-Path $packageDirectory "maestro-installer.exe"))) {
        throw "complete Windows installer package could not be built."
    }

    @"
1 ICON "$($iconItem.FullName)"
"@ | Set-Content -LiteralPath $rcPath -Encoding ascii
    & $compiler.Source --target=pe-x86-64 -i $rcPath -o $sysoPath
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $sysoPath)) {
        throw "windres did not create the self-contained installer resource object."
    }

    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    Push-Location $root
    try {
        & $go.Source build -mod=readonly -buildvcs=false -trimpath -ldflags "-H=windowsgui" -o $wrapperBase ./cmd/maestro-installer-singlefile
        if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $wrapperBase)) {
            throw "self-contained Windows installer wrapper build failed."
        }
    }
    finally {
        Pop-Location
    }

    if ($null -eq $previousGOOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $previousGOOS }
    if ($null -eq $previousGOARCH) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $previousGOARCH }

    Push-Location $root
    try {
        & $go.Source run ./dev/release self-contained --base $wrapperBase --source $packageDirectory --output $outputPath
        if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $outputPath)) {
            throw "self-contained installer payload append failed."
        }
    }
    finally {
        Pop-Location
    }

    $installerDigest = (Get-FileHash -LiteralPath $outputPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $innerProvenancePath = Join-Path $packageDirectory "maestro-installer.exe.provenance.json"
    $innerProvenance = Get-Content -LiteralPath $innerProvenancePath -Raw -ErrorAction Stop | ConvertFrom-Json
    $checksumPath = "$outputPath.sha256"
    if (Test-Path -LiteralPath $checksumPath) {
        throw "Self-contained installer checksum already exists: $checksumPath"
    }
    "$installerDigest  $([IO.Path]::GetFileName($outputPath))" | Set-Content -LiteralPath $checksumPath -Encoding ascii
    $distributionProfile = "strict"
    $nativeSignature = "pending"
    if ($LocalBeta) {
        $distributionProfile = "windows-local-beta"
        $nativeSignature = "not-signed-controlled-canary"
    }
    $provenance = [ordered]@{
        product = "maestro-installer-singlefile"
        version = $Version
        target_os = "windows"
        target_arch = "amd64"
        distribution_profile = $distributionProfile
        release_channel = [string]$innerProvenance.release_channel
        release_issuer = [string]$innerProvenance.release_issuer
        release_key_id = [string]$innerProvenance.release_key_id
        authority_registry_sha256 = [string]$innerProvenance.authority_registry_sha256
        bootstrapper_sha256 = [string]$innerProvenance.bootstrapper_sha256
        bootstrapper_authenticode_status = $(if ($LocalBeta) { "NotSigned" } else { "not-evaluated-by-factory" })
        installer_sha256 = $installerDigest
        icon = $iconItem.Name
        icon_sha256 = $IconSHA256
        embedded_entrypoint = "maestro-installer.exe"
        embedded_package = "validated-directory-payload"
        native_signature = $nativeSignature
        status = "unsigned-candidate"
    }
    $provenance | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath "$outputPath.provenance.json" -Encoding utf8
    $succeeded = $true
    if ($LocalBeta) {
        Write-Output "unsigned controlled local-beta self-contained Windows installer candidate: $outputPath"
    }
    else {
        Write-Output "unsigned self-contained Windows installer candidate: $outputPath"
    }
    Write-Output "provenance: $outputPath.provenance.json"
    Write-Output "sha256: $checksumPath"
}
finally {
    if ($null -eq $previousGOOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $previousGOOS }
    if ($null -eq $previousGOARCH) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $previousGOARCH }
    Remove-Item -LiteralPath $rcPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $sysoPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $workingRoot -Recurse -Force -ErrorAction SilentlyContinue
    if (-not $succeeded) {
        Remove-Item -LiteralPath $outputPath -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath "$outputPath.provenance.json" -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath "$outputPath.sha256" -Force -ErrorAction SilentlyContinue
    }
}
