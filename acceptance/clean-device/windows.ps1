[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("install", "update", "rollback")]
    [string]$Phase,
    [Parameter(Mandatory = $true)]
    [string]$RunID,
    [Parameter(Mandatory = $true)]
    [string]$DeviceIDHash,
    [Parameter(Mandatory = $true)]
    [string]$Version,
    [Parameter(Mandatory = $true)]
    [string]$ProviderReleaseID,
    [Parameter(Mandatory = $true)]
    [string]$ReleaseTag,
    [Parameter(Mandatory = $true)]
    [string]$ManifestSHA256,
    [Parameter(Mandatory = $true)]
    [string]$ExpectedSignerID,
    [Parameter(Mandatory = $true)]
    [string]$ManagedRoot,
    [Parameter(Mandatory = $true)]
    [string]$DataRoot,
    [Parameter(Mandatory = $true)]
    [string]$Workspace,
    [string]$SignedRelease = "",
    [string]$PlanID = "",
    [string]$ActivationReceipt = "",
    [Parameter(Mandatory = $true)]
    [string]$Sentinel,
    [Parameter(Mandatory = $true)]
    [string]$SentinelSHA256,
    [Parameter(Mandatory = $true)]
    [string]$Output
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ($RunID -notmatch '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$' -or
    $DeviceIDHash -notmatch '^[a-f0-9]{64}$' -or
    $Version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' -or
    $ProviderReleaseID -notmatch '^[1-9][0-9]{0,19}$' -or
    $ReleaseTag -ne "maestro-v$Version" -or
    $ManifestSHA256 -notmatch '^[a-f0-9]{64}$' -or
    $ExpectedSignerID -notmatch '^(?:[a-fA-F0-9]{40}|[a-fA-F0-9]{64})$' -or
    $SentinelSHA256 -notmatch '^[a-f0-9]{64}$') {
    throw "Acceptance identity is invalid."
}
foreach ($path in @($ManagedRoot, $DataRoot, $Workspace, $Sentinel, $Output)) {
    if (-not [IO.Path]::IsPathRooted($path)) {
        throw "Acceptance paths must be absolute."
    }
}
if (Test-Path -LiteralPath $Output) {
    throw "Receipt output already exists."
}
if ($env:PROCESSOR_ARCHITECTURE -ne "AMD64") {
    throw "Clean-device acceptance supports Windows amd64 only."
}

$bootstrapper = Join-Path $ManagedRoot "bcgos-bootstrap.exe"
$registry = Join-Path $ManagedRoot "trust\release-authority-registry.json"
$activeCLI = Join-Path $ManagedRoot "bin\bcgos.exe"
foreach ($path in @($bootstrapper, $registry, $Sentinel)) {
    $item = Get-Item -LiteralPath $path
    if (-not $item.PSIsContainer -and
        -not ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        continue
    }
    throw "Acceptance input must be a regular non-reparse file: $path"
}

function Get-ApprovedSignerID([string]$Path) {
    $signature = Get-AuthenticodeSignature -FilePath $Path
    if ($signature.Status -ne "Valid" -or $null -eq $signature.SignerCertificate) {
        throw "Authenticode signature is invalid: $Path"
    }
    $signerID = $signature.SignerCertificate.Thumbprint.ToLowerInvariant()
    if ($signerID -ne $ExpectedSignerID.ToLowerInvariant()) {
        throw "Native signer is not approved: $Path"
    }
    return $signerID
}

$nativeSignerID = Get-ApprovedSignerID $bootstrapper
if ([string]::IsNullOrWhiteSpace($nativeSignerID)) {
    throw "Approved bootstrapper Authenticode signature is invalid."
}
$bootstrapperDigest = (Get-FileHash -LiteralPath $bootstrapper -Algorithm SHA256).Hash.ToLowerInvariant()
$seedStatus = (& $bootstrapper seed-status | ConvertFrom-Json)
$registryDigest = (Get-FileHash -LiteralPath $registry -Algorithm SHA256).Hash.ToLowerInvariant()
if ($seedStatus.product -ne "maestro" -or
    $seedStatus.authority_registry_sha256 -ne $registryDigest) {
    throw "Bootstrapper and authority registry are not the same approved seed."
}
$beforeSentinel = (Get-FileHash -LiteralPath $Sentinel -Algorithm SHA256).Hash.ToLowerInvariant()
if ($beforeSentinel -ne $SentinelSHA256) {
    throw "Owner-data sentinel changed before the acceptance phase."
}

$checks = [Collections.Generic.List[string]]::new()
$checks.Add("native-bootstrapper-signature")
$checks.Add("authority-seed-bound")
$checks.Add("owner-data-sentinel-preserved")
$fromVersion = ""
$activationReceiptDigest = ""
$statePath = Join-Path $DataRoot "config\install-state.json"

switch ($Phase) {
    "install" {
        if ([string]::IsNullOrWhiteSpace($SignedRelease) -or
            -not [IO.Path]::IsPathRooted($SignedRelease)) {
            throw "Install phase requires an absolute signed-release directory."
        }
        if ((Test-Path -LiteralPath $activeCLI) -or
            (Test-Path -LiteralPath (Join-Path $DataRoot "config\install-state.json"))) {
            throw "Install phase requires a device with no existing Maestro state."
        }
        $manifest = Join-Path $SignedRelease "release-manifest.json"
        $actualManifest = (Get-FileHash -LiteralPath $manifest -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actualManifest -ne $ManifestSHA256) {
            throw "Signed release manifest does not match the approved acceptance identity."
        }
        $releaseCLI = Join-Path $SignedRelease "bcgos_$($Version)_windows_amd64.exe"
        [void](Get-ApprovedSignerID $releaseCLI)
        & $bootstrapper install --verified-directory $SignedRelease --data-root $DataRoot
        if ($LASTEXITCODE -ne 0) { throw "First installation failed." }
        & $activeCLI init $Workspace | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Sanitized fixture workspace initialization failed." }
        $checks.Add("signed-release-verified")
        $checks.Add("first-install-activated")
        $checks.Add("provider-release-operator-asserted")
    }
    "update" {
        if ($PlanID -notmatch '^[a-f0-9]{32}$') {
            throw "Update phase requires the exact user-confirmed plan ID."
        }
        $fromVersion = (Get-Content -Raw -LiteralPath $statePath | ConvertFrom-Json).release
        $pendingPath = Join-Path $DataRoot "config\pending-update.json"
        $pending = Get-Content -Raw -LiteralPath $pendingPath | ConvertFrom-Json
        if ($pending.plan.id -ne $PlanID -or
            $pending.plan.manifest_sha256 -ne $ManifestSHA256 -or
            $pending.plan.provider_release_id.ToString() -ne $ProviderReleaseID -or
            $pending.plan.to_release -ne $Version) {
            throw "Pending update does not bind the approved plan, provider release and manifest."
        }
        $ActivationReceipt = Join-Path (Split-Path -Parent $pending.activation_plan_path) "activation-receipt.json"
        $confirmation = (& $activeCLI update --confirm $PlanID | ConvertFrom-Json)
        if ($LASTEXITCODE -ne 0 -or
            $confirmation.plan_id -ne $PlanID -or
            $confirmation.state -ne "activation_started") {
            throw "CLI did not start the exact confirmed update."
        }
        $updated = $false
        for ($attempt = 0; $attempt -lt 120; $attempt++) {
            if (Test-Path -LiteralPath $statePath) {
                $state = Get-Content -Raw -LiteralPath $statePath | ConvertFrom-Json
                if ($state.release -eq $Version) {
                    $updated = $true
                    break
                }
            }
            Start-Sleep -Seconds 1
        }
        if (-not $updated) { throw "Confirmed update did not reach the expected release." }
        $activationItem = Get-Item -LiteralPath $ActivationReceipt
        if ($activationItem.PSIsContainer -or
            ($activationItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
            throw "Update activation receipt is not a regular file."
        }
        $activation = Get-Content -Raw -LiteralPath $ActivationReceipt | ConvertFrom-Json
        if ($activation.confirmation_plan_id -ne $PlanID -or
            $activation.release -ne $Version -or
            $activation.manifest_sha256 -ne $ManifestSHA256) {
            throw "Update activation receipt does not bind the confirmed release."
        }
        $activationReceiptDigest = (Get-FileHash -LiteralPath $ActivationReceipt -Algorithm SHA256).Hash.ToLowerInvariant()
        $checks.Add("activation-receipt-bound")
        $checks.Add("exact-plan-confirmed")
        $checks.Add("pending-provider-bound")
        $checks.Add("signed-update-activated")
    }
    "rollback" {
        if ([string]::IsNullOrWhiteSpace($ActivationReceipt) -or
            -not [IO.Path]::IsPathRooted($ActivationReceipt)) {
            throw "Rollback phase requires the update activation receipt."
        }
        $fromVersion = (Get-Content -Raw -LiteralPath $statePath | ConvertFrom-Json).release
        $activationItem = Get-Item -LiteralPath $ActivationReceipt
        if ($activationItem.PSIsContainer -or
            ($activationItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
            throw "Rollback activation receipt is not a regular file."
        }
        $activation = Get-Content -Raw -LiteralPath $ActivationReceipt | ConvertFrom-Json
        if ($activation.release -ne $fromVersion) {
            throw "Rollback evidence is not bound to the active release."
        }
        $activationReceiptDigest = (Get-FileHash -LiteralPath $ActivationReceipt -Algorithm SHA256).Hash.ToLowerInvariant()
        & $bootstrapper rollback --data-root $DataRoot
        if ($LASTEXITCODE -ne 0) { throw "Last-known-good rollback failed." }
        $checks.Add("activation-receipt-bound")
        $checks.Add("last-known-good-restored")
        $checks.Add("provider-release-operator-asserted")
    }
}

$activeItem = Get-Item -LiteralPath $activeCLI
if ($activeItem.Attributes -band [IO.FileAttributes]::ReparsePoint) {
    throw "Active CLI is a reparse point."
}
[void](Get-ApprovedSignerID $activeCLI)
$versionOutput = (& $activeCLI version | Out-String).Trim()
if ($LASTEXITCODE -ne 0 -or $versionOutput -ne "bcgos $Version") {
    throw "Active CLI version self-check failed."
}
$status = (& $activeCLI status $Workspace | ConvertFrom-Json)
if ($LASTEXITCODE -ne 0 -or
    $status.workspace.state -ne "ready" -or
    $status.capabilities.private_release_auth -ne "configured" -or
    $status.capabilities.updates -ne "configured") {
    throw "Maestro status is not ready for private release acceptance."
}
$doctor = (& $activeCLI doctor $Workspace | ConvertFrom-Json)
if ($LASTEXITCODE -ne 0 -or $doctor.state -ne "ready") {
    throw "Maestro doctor is not ready."
}
$afterSentinel = (Get-FileHash -LiteralPath $Sentinel -Algorithm SHA256).Hash.ToLowerInvariant()
if ($afterSentinel -ne $SentinelSHA256) {
    throw "Owner-data sentinel changed during the acceptance phase."
}
$checks.Add("native-cli-signature")
$checks.Add("active-cli-self-check")
$checks.Add("status-verified")
$checks.Add("doctor-verified")

$receipt = [ordered]@{
    schema_version = 1
    run_id = $RunID
    device_id_hash = $DeviceIDHash
    platform = "windows"
    phase = $Phase
    from_version = $fromVersion
    to_version = $Version
    provider_release_id = $ProviderReleaseID
    release_tag = $ReleaseTag
    manifest_sha256 = $ManifestSHA256
    bootstrapper_sha256 = $bootstrapperDigest
    authority_registry_sha256 = $registryDigest
    native_signer_id = $nativeSignerID
    activation_receipt_sha256 = $activationReceiptDigest
    state = "pass"
    recorded_at = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
    checks = @($checks)
}
$parent = Split-Path -Parent $Output
if (-not [string]::IsNullOrWhiteSpace($parent)) {
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
}
$receiptJSON = ($receipt | ConvertTo-Json -Depth 4) + [Environment]::NewLine
$stream = [IO.File]::Open(
    $Output,
    [IO.FileMode]::CreateNew,
    [IO.FileAccess]::Write,
    [IO.FileShare]::None
)
try {
    $writer = [IO.StreamWriter]::new($stream, [Text.UTF8Encoding]::new($false))
    try {
        $writer.Write($receiptJSON)
        $writer.Flush()
        $stream.Flush($true)
    }
    finally {
        $writer.Dispose()
    }
}
finally {
    $stream.Dispose()
}
Write-Host "Sanitized Windows $Phase receipt written to $Output"
