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
    [string]$ResourceCompiler = "windres"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

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
$registryItem = Get-Item -LiteralPath $AuthorityRegistry -ErrorAction Stop
if ($registryItem.PSIsContainer -or ($registryItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    throw "AuthorityRegistry must be a regular non-reparse file."
}
$registryDigest = (Get-FileHash -LiteralPath $registryItem.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
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

$root = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
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
    & $go.Source build -mod=readonly -buildvcs=false -trimpath -o $outputFull ./cmd/maestro-installer
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
    $provenance = [ordered]@{
        product = "maestro-installer"
        version = $Version
        target_os = "windows"
        target_arch = "amd64"
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
        installable_inputs = "bundled"
        rehearsal_launcher = "Run-Maestro-Rehearsal.cmd"
        resource_compiler = $compilerPath
        resource_compiler_sha256 = $compilerDigest
        resource_compiler_approved_sha256 = $ResourceCompilerSHA256
        resource_object_sha256 = $resourceDigest
        native_signature = "pending"
        status = "unsigned-candidate"
    }
    $provenancePath = "$outputFull.provenance.json"
    $provenance | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $provenancePath -Encoding utf8
    @"
Maestro installer candidate — unsigned

Abra Run-Maestro-Rehearsal.cmd para iniciar o ensaio técnico. Ele chama o
wizard com --simulate, usa apenas o seu perfil de usuário e não pede
administrador. Esta versão ainda não possui Authenticode; use somente para
ensaio técnico.

O executável também carrega o release/, o authority-registry.json e o
bootstrapper nativo que foram fornecidos ao empacotador. O caminho real só
prossegue quando esses bytes passarem pela verificação de assinatura; este
candidato continua sem Authenticode e não é uma release apta para piloto.
"@ | Set-Content -LiteralPath (Join-Path $packageOutput "README-UNSIGNED.md") -Encoding utf8
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
    $succeeded = $true
    Write-Output "unsigned Windows installer candidate: $outputFull"
    Write-Output "provenance: $provenancePath"
}
finally {
    Remove-Item -LiteralPath $rcPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $sysoPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $stagingRoot -Recurse -Force -ErrorAction SilentlyContinue
    if (-not $succeeded -and $outputCreated) {
        Remove-Item -LiteralPath $packageOutput -Recurse -Force -ErrorAction SilentlyContinue
    }
    if ($null -eq $previousGOOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $previousGOOS }
    if ($null -eq $previousGOARCH) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $previousGOARCH }
}
