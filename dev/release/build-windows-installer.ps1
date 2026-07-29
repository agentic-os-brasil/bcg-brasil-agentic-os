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

Require-AbsolutePath $Icon "Icon"
Require-AbsolutePath $WizardDir "WizardDir"
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
$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
$succeeded = $false

try {
    @"
1 ICON "$($iconItem.FullName)"
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

    Copy-Item -LiteralPath $wizardItem.FullName -Destination (Join-Path $packageOutput "wizard") -Recurse
    Copy-Item -LiteralPath $iconItem.FullName -Destination (Join-Path $packageOutput "maestro-app-icon.ico")

    $resourceDigest = (Get-FileHash -LiteralPath $sysoPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $provenance = [ordered]@{
        product = "maestro-installer"
        version = $Version
        target_os = "windows"
        target_arch = "amd64"
        icon = $iconItem.Name
        icon_sha256 = $iconDigest
        wizard_root = "wizard"
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
    if (-not $succeeded -and $outputCreated) {
        Remove-Item -LiteralPath $packageOutput -Recurse -Force -ErrorAction SilentlyContinue
    }
    if ($null -eq $previousGOOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $previousGOOS }
    if ($null -eq $previousGOARCH) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $previousGOARCH }
}
