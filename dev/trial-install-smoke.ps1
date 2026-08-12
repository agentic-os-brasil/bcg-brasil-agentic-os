# DEPRECATED: This script builds `cmd/bcgos`, which PR #302 deleted as part
# of the surgical Go delete / ZIP-only distribution pivot. It cannot be run
# as-is and will fail closed until it is either repointed at the ZIP
# installer path (`installers/zip/`) or removed.
Write-Error "DEPRECATED: dev/trial-install-smoke.ps1 depends on cmd/bcgos, which was removed in PR #302 (ZIP-only pivot). Repoint at installers/zip/ or delete before use."
exit 1
# Runs a real Windows trial install in an isolated temporary home.
$ErrorActionPreference = "Stop"

$ProjectRoot = Split-Path -Parent $PSScriptRoot
$TrialRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("bcgos-trial-" + [guid]::NewGuid())
$ArtifactDir = Join-Path $TrialRoot "artifact"
$InstallRoot = Join-Path $TrialRoot "install"
$Workspace = Join-Path $TrialRoot "workspace"
$GoCache = Join-Path $TrialRoot "go-cache"
$GoModCache = Join-Path $TrialRoot "go-mod-cache"

try {
    New-Item -ItemType Directory -Force -Path $ArtifactDir | Out-Null
    $env:GOCACHE = $GoCache
    $env:GOMODCACHE = $GoModCache
    go build -trimpath -o (Join-Path $ArtifactDir "bcgos.exe") (Join-Path $ProjectRoot "cmd/bcgos")
    if ($LASTEXITCODE -ne 0) { throw "could not build trial artifact" }

    $hash = (Get-FileHash -Algorithm SHA256 (Join-Path $ArtifactDir "bcgos.exe")).Hash.ToLower()
    "$hash  bcgos.exe" | Set-Content -NoNewline (Join-Path $ArtifactDir "bcgos.exe.sha256")
    (("0" * 64) -join "") + "  bcgos.exe" | Set-Content -NoNewline (Join-Path $ArtifactDir "invalid.sha256")

    try {
        & (Join-Path $ProjectRoot "installers/trial/install.ps1") `
            -ArtifactPath (Join-Path $ArtifactDir "bcgos.exe") `
            -ChecksumPath (Join-Path $ArtifactDir "invalid.sha256") `
            -InstallRoot (Join-Path $TrialRoot "rejected-install") `
            -AllowUnsignedTrial
        throw "installer accepted an invalid checksum"
    } catch {
        if ($_.Exception.Message -match "installer accepted") { throw }
    }
    if (Test-Path -LiteralPath (Join-Path $TrialRoot "rejected-install")) { throw "installer left an activation directory after checksum rejection" }

    & (Join-Path $ProjectRoot "installers/trial/install.ps1") `
        -ArtifactPath (Join-Path $ArtifactDir "bcgos.exe") `
        -ChecksumPath (Join-Path $ArtifactDir "bcgos.exe.sha256") `
        -InstallRoot $InstallRoot `
        -AllowUnsignedTrial

    & (Join-Path $InstallRoot "bin/bcgos.exe") version | Select-String -Quiet '^bcgos '
    if (-not $?) { throw "installed binary did not report its version" }
    try {
        & (Join-Path $ProjectRoot "installers/trial/install.ps1") `
            -ArtifactPath (Join-Path $ArtifactDir "bcgos.exe") `
            -ChecksumPath (Join-Path $ArtifactDir "bcgos.exe.sha256") `
            -InstallRoot $InstallRoot `
            -AllowUnsignedTrial
        throw "installer replaced an existing trial installation"
    } catch {
        if ($_.Exception.Message -match "installer replaced") { throw }
    }
    & (Join-Path $InstallRoot "bin/bcgos.exe") version | Select-String -Quiet '^bcgos '
    if (-not $?) { throw "existing binary stopped working after refused replacement" }

    $env:LOCALAPPDATA = Join-Path $TrialRoot "local-app-data"
    & (Join-Path $InstallRoot "bin/bcgos.exe") init $Workspace | Select-String -Quiet '"state": "initialized"'
    if (-not $?) { throw "installed binary did not initialize a workspace" }
    & (Join-Path $InstallRoot "bin/bcgos.exe") doctor $Workspace | Select-String -Quiet '"state": '
    if (-not $?) { throw "installed binary did not run doctor" }

    Write-Output "[ok] Windows trial installation"
} finally {
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $TrialRoot
}
