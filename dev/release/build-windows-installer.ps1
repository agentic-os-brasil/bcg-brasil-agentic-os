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
    [string]$OutputDirectory,
    [string]$ArchiveOutput = "",
    [string]$ResourceCompiler = "windres",
    [switch]$Windowed,
    [switch]$CanarySimple,
    [switch]$LocalBeta,
    [string]$LocalBetaIssuer = "",
    [string]$LocalBetaKeyID = "",
    [string]$LocalBetaAuthorityRegistrySHA256 = "",
    [string]$LocalBetaBootstrapperSHA256 = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "windows-native-signature.ps1")

$localBetaIdentityPattern = '^[a-z0-9][a-z0-9._-]{0,127}$'
$sha256Pattern = '^[a-f0-9]{64}$'
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
    if ($LocalBetaIssuer -notmatch $localBetaIdentityPattern -or $LocalBetaKeyID -notmatch $localBetaIdentityPattern) {
        throw "LocalBeta issuer and key ID must be bounded lowercase identifiers."
    }
    if ($LocalBetaAuthorityRegistrySHA256 -notmatch $sha256Pattern -or $LocalBetaBootstrapperSHA256 -notmatch $sha256Pattern) {
        throw "LocalBeta authority-registry and bootstrapper pins must be lowercase SHA-256 values."
    }
}
if ($CanarySimple -and $LocalBeta) {
    throw "CanarySimple and LocalBeta cannot be combined."
}
elseif (-not $LocalBeta -and @($localBetaValues | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }).Count -ne 0) {
    throw "LocalBeta issuer, key ID and digest pins are valid only with -LocalBeta."
}

function Require-AbsolutePath([string]$Path, [string]$Label) {
    if (-not [IO.Path]::IsPathRooted($Path)) {
        throw "$Label must be an absolute path."
    }
}

function Get-SafeTreeDigest([string]$Root) {
    $rootItem = Get-Item -LiteralPath $Root -ErrorAction Stop
    if (-not $rootItem.PSIsContainer -or ($rootItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        throw "Tree root must be a regular non-reparse directory."
    }
    $lines = [Collections.Generic.List[string]]::new()
    foreach ($entry in @(Get-ChildItem -LiteralPath $rootItem.FullName -Recurse -Force | Sort-Object FullName)) {
        if ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) {
            throw "Tree contains a reparse point: $($entry.FullName)"
        }
        $relative = $entry.FullName.Substring($rootItem.FullName.Length).TrimStart([char[]]@('\', '/'))
        if ($entry.PSIsContainer) {
            $lines.Add("D|$relative")
            continue
        }
        $digest = (Get-FileHash -LiteralPath $entry.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        $lines.Add("F|$relative|$($entry.Length)|$digest")
    }
    $payload = [Text.Encoding]::UTF8.GetBytes(($lines -join "`n"))
    $sha = [Security.Cryptography.SHA256]::Create()
    try {
        return ([BitConverter]::ToString($sha.ComputeHash($payload)).Replace('-', '')).ToLowerInvariant()
    }
    finally {
        $sha.Dispose()
    }
}

Require-AbsolutePath $Icon "Icon"
Require-AbsolutePath $WizardDir "WizardDir"
Require-AbsolutePath $ReleaseDirectory "ReleaseDirectory"
Require-AbsolutePath $AuthorityRegistry "AuthorityRegistry"
Require-AbsolutePath $Bootstrapper "Bootstrapper"
Require-AbsolutePath $OutputDirectory "OutputDirectory"

$iconItem = Get-Item -LiteralPath $Icon -ErrorAction Stop
if ($iconItem.PSIsContainer -or ($iconItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    throw "Icon must be a regular non-reparse file."
}
$iconDigest = (Get-FileHash -LiteralPath $iconItem.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
if ($iconDigest -ne $IconSHA256) {
    throw "Icon SHA-256 does not match the approved input."
}
$wizardItem = Get-Item -LiteralPath $WizardDir -ErrorAction Stop
if (-not $wizardItem.PSIsContainer -or ($wizardItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    throw "WizardDir must be a regular non-reparse directory."
}
$wizardReparse = Get-ChildItem -LiteralPath $wizardItem.FullName -Recurse -Force |
    Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }
if ($null -ne $wizardReparse -and @($wizardReparse).Count -gt 0) {
    throw "WizardDir contains a reparse point."
}
$initialWizardDigest = Get-SafeTreeDigest $wizardItem.FullName
$releaseItem = Get-Item -LiteralPath $ReleaseDirectory -ErrorAction Stop
$initialReleaseDigest = Get-SafeTreeDigest $releaseItem.FullName
$releaseManifestPath = Join-Path $releaseItem.FullName "release-manifest.json"
$releaseManifestItem = Get-Item -LiteralPath $releaseManifestPath -ErrorAction Stop
if ($releaseManifestItem.PSIsContainer -or ($releaseManifestItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    throw "Release manifest must be a regular non-reparse file."
}
if ($releaseManifestItem.Length -gt 1MB) {
    throw "Release manifest exceeds the 1 MiB packaging bound."
}
try {
    $releaseManifest = Get-Content -LiteralPath $releaseManifestPath -Raw -ErrorAction Stop | ConvertFrom-Json
}
catch {
    throw "Release manifest is not valid JSON: $($_.Exception.Message)"
}
if ($releaseManifest.product -ne "maestro" -or $releaseManifest.release -ne $Version) {
    throw "Release manifest identity does not match -Version $Version."
}
if ($LocalBeta) {
    if ($releaseManifest.channel -ne "canary") {
        throw "LocalBeta requires a canary release manifest."
    }
    if ($releaseManifest.issuer.id -ne $LocalBetaIssuer -or $releaseManifest.issuer.key_id -ne $LocalBetaKeyID) {
        throw "LocalBeta release manifest issuer does not match the pinned test-only authority."
    }
}
$registryItem = Get-Item -LiteralPath $AuthorityRegistry -ErrorAction Stop
if ($registryItem.PSIsContainer -or ($registryItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    throw "AuthorityRegistry must be a regular non-reparse file."
}
if ($registryItem.Length -gt 1MB) {
    throw "AuthorityRegistry exceeds the 1 MiB packaging bound."
}
$registryDigest = (Get-FileHash -LiteralPath $registryItem.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
if ($LocalBeta) {
    if ($registryDigest -ne $LocalBetaAuthorityRegistrySHA256) {
        throw "LocalBeta authority-registry SHA-256 does not match the approved pin."
    }
    try {
        $registryDocument = Get-Content -LiteralPath $registryItem.FullName -Raw -ErrorAction Stop | ConvertFrom-Json
    }
    catch {
        throw "LocalBeta authority registry is not valid JSON: $($_.Exception.Message)"
    }
    $matchingAuthorities = @($registryDocument.authorities | Where-Object {
        $_.issuer -eq $LocalBetaIssuer -and $_.key_id -eq $LocalBetaKeyID
    })
    if ($registryDocument.product -ne "maestro" -or $matchingAuthorities.Count -ne 1 -or $matchingAuthorities[0].status -ne "active") {
        throw "LocalBeta authority registry must contain the exact active test-only issuer/key identity."
    }
}
$bootstrapperItem = Get-Item -LiteralPath $Bootstrapper -ErrorAction Stop
if ($bootstrapperItem.PSIsContainer -or ($bootstrapperItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    throw "Bootstrapper must be a regular non-reparse file."
}
$bootstrapperVersionMatch = [regex]::Match($bootstrapperItem.Name, '^bcgos-bootstrap_(?<version>(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*))_windows_amd64\.exe$')
if (-not $bootstrapperVersionMatch.Success) {
    throw "Bootstrapper must use the versioned bcgos-bootstrap_<version>_windows_amd64.exe package name."
}
if ($bootstrapperVersionMatch.Groups["version"].Value -ne $Version) {
    throw "Bootstrapper version does not match -Version $Version."
}
$bootstrapperDigest = (Get-FileHash -LiteralPath $bootstrapperItem.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
if ($LocalBeta -and $bootstrapperDigest -ne $LocalBetaBootstrapperSHA256) {
    throw "LocalBeta bootstrapper SHA-256 does not match the approved pin."
}
$bootstrapperAuthenticodeStatus = "not-evaluated-by-factory"
if ($LocalBeta) {
    $bootstrapperAuthenticodeStatus = Get-MaestroAuthenticodeStatus $bootstrapperItem.FullName
    if ($bootstrapperAuthenticodeStatus -ne "NotSigned") {
        throw "LocalBeta bootstrapper Authenticode status must be exactly NotSigned; got $bootstrapperAuthenticodeStatus."
    }
}

$root = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$requiredBundleFiles = @(
    "skills/maestro-onboarding/SKILL.md",
    "skills/maestro-onboarding/agents/openai.yaml",
    "skills/interaction-profile/SKILL.md",
    "skills/interaction-profile/agents/openai.yaml",
    "skills/maestro-setup-update/SKILL.md",
    "skills/maestro-setup-update/agents/openai.yaml"
)
$requiredBundleProvenance = [Collections.Generic.List[object]]::new()
foreach ($relativePath in $requiredBundleFiles) {
    $sourcePath = Join-Path $root (Join-Path "bundles\base" ($relativePath -replace '/', '\'))
    $sourceItem = Get-Item -LiteralPath $sourcePath -ErrorAction Stop
    if ($sourceItem.PSIsContainer -or ($sourceItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        throw "Required installer skill input must be a regular non-reparse file: $relativePath"
    }
    $requiredBundleProvenance.Add([ordered]@{
        path = $relativePath
        source_sha256 = (Get-FileHash -LiteralPath $sourceItem.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    })
}
$bundleArtifacts = @($releaseManifest.artifacts | Where-Object {
    $_.kind -eq "bundle" -and $_.os -eq "any" -and $_.arch -eq "any"
})
if ($bundleArtifacts.Count -ne 1) {
    throw "Release manifest must contain exactly one platform-neutral bundle artifact."
}
$bundleName = [string]$bundleArtifacts[0].name
if ($bundleName -ne "maestro-base_$Version.tar.gz" -or
    [IO.Path]::GetFileName($bundleName) -ne $bundleName -or
    $bundleName -match '[\\/]') {
    throw "Release bundle artifact must be the versioned basename maestro-base_$Version.tar.gz."
}
$declaredBundleDigest = [string]$bundleArtifacts[0].sha256
if ($declaredBundleDigest -notmatch '^[a-f0-9]{64}$') {
    throw "Release bundle artifact must declare a lowercase SHA-256 digest."
}
$bundlePath = Join-Path $releaseItem.FullName $bundleName
$bundleItem = Get-Item -LiteralPath $bundlePath -ErrorAction Stop
if ($bundleItem.PSIsContainer -or ($bundleItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    throw "Release bundle must be a regular non-reparse file."
}
$actualBundleDigest = (Get-FileHash -LiteralPath $bundleItem.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actualBundleDigest -ne $declaredBundleDigest) {
    throw "Release bundle digest does not match the manifest."
}
$tar = Get-Command "tar" -CommandType Application -ErrorAction Stop
$skillCheckRoot = Join-Path ([IO.Path]::GetTempPath()) ("maestro-installer-skill-check-" + [Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Path $skillCheckRoot -Force | Out-Null
    foreach ($entry in $requiredBundleProvenance) {
        $relativePath = [string]$entry.path
        & $tar.Source -xzf $bundleItem.FullName -C $skillCheckRoot $relativePath 2>$null
        if ($LASTEXITCODE -ne 0) {
            throw "Release bundle is missing the required installer skill file: $relativePath"
        }
        $extractedPath = Join-Path $skillCheckRoot ($relativePath -replace '/', '\')
        $extractedItem = Get-Item -LiteralPath $extractedPath -ErrorAction Stop
        if ($extractedItem.PSIsContainer -or ($extractedItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
            throw "Required installer skill entry is not a regular file: $relativePath"
        }
        $bundleSkillDigest = (Get-FileHash -LiteralPath $extractedItem.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($bundleSkillDigest -ne [string]$entry.source_sha256) {
            throw "Release bundle skill bytes do not match the approved source: $relativePath"
        }
        $entry["bundle_sha256"] = $bundleSkillDigest
    }
}
finally {
    Remove-Item -LiteralPath $skillCheckRoot -Recurse -Force -ErrorAction SilentlyContinue
}
$buildPackageDir = Join-Path $root "cmd\maestro-installer"
$go = Get-Command "go" -CommandType Application -ErrorAction Stop
$compiler = Get-Command $ResourceCompiler -CommandType Application -ErrorAction Stop
$compilerPath = $compiler.Source
$compilerItem = Get-Item -LiteralPath $compilerPath -ErrorAction Stop
if ($compilerItem.PSIsContainer -or ($compilerItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    throw "Resource compiler must be a regular non-reparse file."
}
$compilerDigest = (Get-FileHash -LiteralPath $compilerPath -Algorithm SHA256).Hash.ToLowerInvariant()
if ($compilerDigest -ne $ResourceCompilerSHA256) {
    throw "Resource compiler SHA-256 is not the approved fingerprint."
}
$packageOutput = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $packageOutput) {
    throw "Output directory already exists: $packageOutput"
}
$packageParent = Split-Path -Parent $packageOutput
if ([string]::IsNullOrWhiteSpace($ArchiveOutput)) {
    $archiveName = "Maestro-Installer-$Version-windows-amd64-portable-unsigned.zip"
    if ($CanarySimple) {
        $archiveName = "Maestro-Installer-$Version-windows-amd64-portable-canary-simple-unsigned.zip"
    }
    elseif ($LocalBeta) {
        $archiveName = "Maestro-Installer-$Version-windows-amd64-portable-local-beta-unsigned.zip"
    }
    $ArchiveOutput = Join-Path $packageParent $archiveName
}
$archiveOutput = [IO.Path]::GetFullPath($ArchiveOutput)
if (Test-Path -LiteralPath $archiveOutput) {
    throw "Portable archive already exists: $archiveOutput"
}
New-Item -ItemType Directory -Path $packageOutput -Force | Out-Null
$outputCreated = $true
$outputFull = Join-Path $packageOutput "maestro-installer.exe"

$nonce = [Guid]::NewGuid().ToString("N")
$rcPath = Join-Path ([IO.Path]::GetTempPath()) "maestro-installer-$nonce.rc"
$sysoPath = Join-Path $buildPackageDir "maestro-installer-$nonce.syso"
$stagingRoot = Join-Path ([IO.Path]::GetTempPath()) "maestro-installer-input-$nonce"
$stagedIconPath = Join-Path $stagingRoot "maestro-app-icon.ico"
$stagedWizardPath = Join-Path $stagingRoot "wizard"
$stagedReleasePath = Join-Path $stagingRoot "release"
$stagedRegistryPath = Join-Path $stagingRoot "authority-registry.json"
$stagedBootstrapperPath = Join-Path $stagingRoot $bootstrapperItem.Name
$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
$succeeded = $false

try {
	New-Item -ItemType Directory -Path $stagingRoot -Force | Out-Null
	Copy-Item -LiteralPath $iconItem.FullName -Destination $stagedIconPath
	Copy-Item -LiteralPath $wizardItem.FullName -Destination $stagedWizardPath -Recurse
	Copy-Item -LiteralPath $releaseItem.FullName -Destination $stagedReleasePath -Recurse
	Copy-Item -LiteralPath $registryItem.FullName -Destination $stagedRegistryPath
	Copy-Item -LiteralPath $bootstrapperItem.FullName -Destination $stagedBootstrapperPath
	$stagedIconItem = Get-Item -LiteralPath $stagedIconPath -ErrorAction Stop
	if ($stagedIconItem.PSIsContainer -or ($stagedIconItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
		throw "Staged icon must be a regular non-reparse file."
	}
	if ((Get-FileHash -LiteralPath $stagedIconPath -Algorithm SHA256).Hash.ToLowerInvariant() -ne $IconSHA256) {
		throw "Staged icon SHA-256 does not match the approved input."
	}
	$stagedWizardDigest = Get-SafeTreeDigest $stagedWizardPath
	if ($stagedWizardDigest -ne $initialWizardDigest) {
		throw "Wizard changed while it was staged."
	}
	$stagedReleaseDigest = Get-SafeTreeDigest $stagedReleasePath
	if ($stagedReleaseDigest -ne $initialReleaseDigest) {
		throw "Release changed while it was staged."
	}
	if ((Get-FileHash -LiteralPath $stagedRegistryPath -Algorithm SHA256).Hash.ToLowerInvariant() -ne $registryDigest) {
		throw "Authority registry changed while it was staged."
	}
	if ((Get-FileHash -LiteralPath $stagedBootstrapperPath -Algorithm SHA256).Hash.ToLowerInvariant() -ne $bootstrapperDigest) {
		throw "Bootstrapper changed while it was staged."
	}
	if ((Get-FileHash -LiteralPath $iconItem.FullName -Algorithm SHA256).Hash.ToLowerInvariant() -ne $IconSHA256) {
		throw "Icon changed while it was staged."
	}

    @"
1 ICON "$($stagedIconItem.FullName)"
"@ | Set-Content -LiteralPath $rcPath -Encoding ascii

    & $compilerPath --target=pe-x86-64 -i $rcPath -o $sysoPath
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $sysoPath)) {
        throw "windres did not create a Windows resource object."
    }

    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    Push-Location $root
    try {
        $goBuildArguments = @("build", "-mod=readonly", "-buildvcs=false", "-trimpath")
        $linkerFlags = [Collections.Generic.List[string]]::new()
        if ($Windowed) {
            $linkerFlags.Add("-H=windowsgui")
        }
        if ($CanarySimple) {
            $linkerFlags.Add("-X main.BuildTrustProfile=canary-simple")
        }
        elseif ($LocalBeta) {
            $linkerFlags.Add("-X main.BuildTrustProfile=windows-local-beta")
            $linkerFlags.Add("-X main.BuildLocalBetaIssuer=$LocalBetaIssuer")
            $linkerFlags.Add("-X main.BuildLocalBetaKeyID=$LocalBetaKeyID")
            $linkerFlags.Add("-X main.BuildLocalBetaRegistrySHA256=$LocalBetaAuthorityRegistrySHA256")
            $linkerFlags.Add("-X main.BuildLocalBetaBootstrapperSHA256=$LocalBetaBootstrapperSHA256")
        }
        if ($linkerFlags.Count -gt 0) {
            $goBuildArguments += @("-ldflags", ($linkerFlags -join " "))
        }
        $goBuildArguments += @("-o", $outputFull, "./cmd/maestro-installer")
        & $go.Source @goBuildArguments
        if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $outputFull)) {
            throw "Windows Maestro installer build failed."
        }
    }
    finally {
        Pop-Location
    }

    $installerDigest = (Get-FileHash -LiteralPath $outputFull -Algorithm SHA256).Hash.ToLowerInvariant()

    if ((Get-FileHash -LiteralPath $iconItem.FullName -Algorithm SHA256).Hash.ToLowerInvariant() -ne $IconSHA256) {
        throw "Icon changed during packaging."
    }
	if ((Get-SafeTreeDigest $wizardItem.FullName) -ne $initialWizardDigest) {
		throw "Wizard changed during packaging."
	}
	if ((Get-SafeTreeDigest $releaseItem.FullName) -ne $initialReleaseDigest) {
		throw "Release changed during packaging."
	}
	if ((Get-FileHash -LiteralPath $registryItem.FullName -Algorithm SHA256).Hash.ToLowerInvariant() -ne $registryDigest) {
		throw "Authority registry changed during packaging."
	}
	if ((Get-FileHash -LiteralPath $bootstrapperItem.FullName -Algorithm SHA256).Hash.ToLowerInvariant() -ne $bootstrapperDigest) {
		throw "Bootstrapper changed during packaging."
	}
	Copy-Item -LiteralPath $stagedWizardPath -Destination (Join-Path $packageOutput "wizard") -Recurse
	Copy-Item -LiteralPath $stagedReleasePath -Destination (Join-Path $packageOutput "release") -Recurse
	Copy-Item -LiteralPath $stagedRegistryPath -Destination (Join-Path $packageOutput "authority-registry.json")
	Copy-Item -LiteralPath $stagedBootstrapperPath -Destination (Join-Path $packageOutput $bootstrapperItem.Name)
	Copy-Item -LiteralPath $stagedIconPath -Destination (Join-Path $packageOutput "maestro-app-icon.ico")
	if ((Get-SafeTreeDigest (Join-Path $packageOutput "wizard")) -ne $initialWizardDigest) {
		throw "Packaged wizard changed while it was copied."
	}
	if ((Get-SafeTreeDigest (Join-Path $packageOutput "release")) -ne $initialReleaseDigest) {
		throw "Packaged release changed while it was copied."
	}
	if ((Get-FileHash -LiteralPath (Join-Path $packageOutput "authority-registry.json") -Algorithm SHA256).Hash.ToLowerInvariant() -ne $registryDigest) {
		throw "Packaged authority registry changed while it was copied."
	}
	if ((Get-FileHash -LiteralPath (Join-Path $packageOutput $bootstrapperItem.Name) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $bootstrapperDigest) {
		throw "Packaged bootstrapper changed while it was copied."
	}

    $resourceDigest = (Get-FileHash -LiteralPath $sysoPath -Algorithm SHA256).Hash.ToLowerInvariant()
$distributionProfile = "strict"
$nativeSignature = "pending"
if ($CanarySimple) {
    $distributionProfile = "canary-simple"
    $nativeSignature = "not-signed-controlled-canary"
}
elseif ($LocalBeta) {
        $distributionProfile = "windows-local-beta"
        $nativeSignature = "not-signed-controlled-canary"
    }
    $provenance = [ordered]@{
        product = "maestro-installer"
        version = $Version
        target_os = "windows"
        target_arch = "amd64"
        distribution_profile = $distributionProfile
        release_channel = [string]$releaseManifest.channel
        release_issuer = [string]$releaseManifest.issuer.id
        release_key_id = [string]$releaseManifest.issuer.key_id
        bridge_sha256 = $installerDigest
        icon = $iconItem.Name
        icon_sha256 = $iconDigest
        wizard_root = "wizard"
        wizard_tree_sha256 = $initialWizardDigest
        release_root = "release"
        release_tree_sha256 = $initialReleaseDigest
        authority_registry = "authority-registry.json"
        authority_registry_sha256 = $registryDigest
        bootstrapper = $bootstrapperItem.Name
        bootstrapper_sha256 = $bootstrapperDigest
        bootstrapper_authenticode_status = $bootstrapperAuthenticodeStatus
        local_beta_authority_registry_sha256 = $(if ($LocalBeta) { $LocalBetaAuthorityRegistrySHA256 } else { "" })
        local_beta_bootstrapper_sha256 = $(if ($LocalBeta) { $LocalBetaBootstrapperSHA256 } else { "" })
        required_bundle_files = $requiredBundleProvenance
        installable_inputs = "bundled"
        rehearsal_launcher = "Run-Maestro-Rehearsal.cmd"
        resource_compiler = $compilerPath
        resource_compiler_sha256 = $compilerDigest
        resource_compiler_approved_sha256 = $ResourceCompilerSHA256
        resource_object_sha256 = $resourceDigest
        native_signature = $nativeSignature
        status = "unsigned-candidate"
    }
    $provenancePath = "$outputFull.provenance.json"
    $provenance | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $provenancePath -Encoding utf8
    $readmeTitle = "Maestro Windows installer candidate - unsigned"
    $readmeTrust = @"
Esta versão ainda não possui Authenticode; use somente como candidato técnico.
"@
    if ($LocalBeta) {
        $readmeTitle = "Maestro Windows Canary controlado - beta local sem Authenticode"
        $readmeTrust = @"
Este pacote $Version usa o perfil compilado windows-local-beta. O release e seus
artefatos continuam autenticados pela autoridade Ed25519 beta fixada, e o
bootstrapper precisa corresponder aos hashes registrados no pacote. Somente o
status Authenticode NotSigned e aceito; uma assinatura invalida continua sendo
bloqueada.

O Windows SmartScreen, WDAC ou AppLocker pode impedir a abertura antes que o
Maestro execute. Distribua este EXE somente ao grupo Canary controlado e
confira o SHA-256 publicado por um canal independente.
"@
    }
    @"
$readmeTitle

Abra maestro-installer.exe para iniciar a instalação visual. O pacote precisa
ser mantido completo: wizard/, release/, authority-registry.json e o
bcgos-bootstrap_<versao>_windows_amd64.exe devem permanecer ao lado do
executável. Run-Maestro-Rehearsal.cmd inicia apenas um ensaio técnico com
--simulate, usa somente o perfil do usuário e não pede administrador.

$readmeTrust
O arquivo bcgos_<versao>_windows_amd64.exe dentro de release/ é o runtime CLI,
não um instalador e não deve ser enviado separadamente.

O executável também carrega o release/, o authority-registry.json e o
bootstrapper nativo que foram fornecidos ao empacotador. O caminho real só
prossegue quando esses bytes passarem pela verificação de assinatura; este
candidato continua sem Authenticode e não é uma release apta para piloto.
"@ | Set-Content -LiteralPath (Join-Path $packageOutput "README-UNSIGNED.md") -Encoding utf8
    @"
@echo off
setlocal
set "ROOT=%~dp0"
"%ROOT%maestro-installer.exe" --wizard-dir "%ROOT%wizard" --release-dir "%ROOT%release" --authority-registry "%ROOT%authority-registry.json" --bootstrapper "%ROOT%$($bootstrapperItem.Name)"
if errorlevel 1 (
  echo.
  echo A instalacao nao foi concluida. Consulte a mensagem acima.
  pause
  exit /b 1
)
"@ | Set-Content -LiteralPath (Join-Path $packageOutput "Install-Maestro.cmd") -Encoding ascii
    @"
@echo off
setlocal
set "SIMULATION_ROOT=%TEMP%\Maestro-Install-Rehearsal-%RANDOM%"
"%~dp0maestro-installer.exe" --simulate --wizard-dir "%~dp0wizard" --simulation-root "%SIMULATION_ROOT%"
if errorlevel 1 (
  echo.
  echo O ensaio nao foi concluido. Consulte a mensagem acima.
  pause
  exit /b 1
)
echo.
echo Sandbox criada em: %SIMULATION_ROOT%
pause
"@ | Set-Content -LiteralPath (Join-Path $packageOutput "Run-Maestro-Rehearsal.cmd") -Encoding ascii
    Compress-Archive -LiteralPath $packageOutput -DestinationPath $archiveOutput -CompressionLevel Optimal
    if (-not (Test-Path -LiteralPath $archiveOutput)) {
        throw "Portable Windows archive was not created."
    }
    $archiveDigest = (Get-FileHash -LiteralPath $archiveOutput -Algorithm SHA256).Hash.ToLowerInvariant()
    $succeeded = $true
    if ($LocalBeta) {
        Write-Output "unsigned controlled local-beta Windows installer candidate: $outputFull"
    }
    else {
        Write-Output "unsigned Windows installer candidate: $outputFull"
    }
    Write-Output "portable archive: $archiveOutput"
    Write-Output "portable archive sha256: $archiveDigest"
    Write-Output "provenance: $provenancePath"
}
finally {
    Remove-Item -LiteralPath $rcPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $sysoPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $stagingRoot -Recurse -Force -ErrorAction SilentlyContinue
    if (-not $succeeded -and $outputCreated) {
        Remove-Item -LiteralPath $packageOutput -Recurse -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $archiveOutput -Force -ErrorAction SilentlyContinue
    }
    if ($null -eq $previousGOOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $previousGOOS }
    if ($null -eq $previousGOARCH) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $previousGOARCH }
}
