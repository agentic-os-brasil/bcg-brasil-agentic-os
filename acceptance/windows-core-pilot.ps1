[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$TestBinary,
    [Parameter(Mandatory = $true)][string]$Server,
    [Parameter(Mandatory = $true)][string]$OutputRoot,
    [string]$Kit,
    [string]$KitTestScript,
    [string]$MethodFixture,
    [string]$SdkWitness,
    [string]$AtlasTestBinary,
    [switch]$AuthoredBrain
)
$ErrorActionPreference = 'Stop'
$kitInputs = @($Kit, $KitTestScript, $MethodFixture, $SdkWitness)
$kitInputCount = @($kitInputs | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }).Count
if ($kitInputCount -ne 0 -and $kitInputCount -ne $kitInputs.Count) {
    throw 'Kit, KitTestScript, MethodFixture and SdkWitness must be supplied together.'
}
$withKit = $kitInputCount -eq $kitInputs.Count
if ($AtlasTestBinary -and -not (Test-Path -LiteralPath $AtlasTestBinary -PathType Leaf)) {
    throw 'The focused atlas publication witness is missing.'
}
if ($AuthoredBrain -and -not $AtlasTestBinary) {
    throw 'Public authored-brain qualification also requires the publication and journal witnesses.'
}
if ($withKit) {
    if (Test-Path -LiteralPath $OutputRoot) { throw 'Use a new qualification output; previous proof is preserved.' }
    foreach ($inputPath in @($Kit, ($Kit + '.zip'), ($Kit + '.zip.sha256'), $KitTestScript, $MethodFixture, $SdkWitness)) {
        if (-not (Test-Path -LiteralPath $inputPath)) { throw 'An exact-artifact qualification input is missing.' }
    }
    $kitServer = Join-Path $Kit 'bin/maestro-mcp-windows-amd64.exe'
    $serverHash = (Get-FileHash -LiteralPath $Server -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($serverHash -cne (Get-FileHash -LiteralPath $kitServer -Algorithm SHA256).Hash.ToLowerInvariant()) {
        throw 'Core must qualify the actual produced kit server.'
    }
    if ([IO.File]::ReadAllLines($MethodFixture).Count -ne 92) { throw 'The independent method fixture must contain 92 entries.' }
}
$name = 'mcp' + [Guid]::NewGuid().ToString('N').Substring(0, 10)
$password = [Guid]::NewGuid().ToString('N') + 'aA1!'
$secure = ConvertTo-SecureString $password -AsPlainText -Force
$credential = [PSCredential]::new("$env:COMPUTERNAME\$name", $secure)
$account = $null
try {
    $account = New-LocalUser -Name $name -Password $secure -AccountNeverExpires -PasswordNeverExpires -UserMayNotChangePassword
    $users = Get-LocalGroup -SID 'S-1-5-32-545'
    if (-not (Get-LocalGroupMember -Group $users | Where-Object { $_.SID -eq $account.SID })) {
        Add-LocalGroupMember -Group $users -Member $account
    }
    New-Item -ItemType Directory -Path $OutputRoot -Force | Out-Null
    $acl = [System.Security.AccessControl.DirectorySecurity]::new()
    $acl.SetAccessRuleProtection($true, $false)
    foreach ($sid in @($account.SID, [System.Security.Principal.SecurityIdentifier]::new('S-1-5-18'), [System.Security.Principal.SecurityIdentifier]::new('S-1-5-32-544'))) {
        $rule = [System.Security.AccessControl.FileSystemAccessRule]::new($sid, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow')
        $acl.AddAccessRule($rule)
    }
    Set-Acl -LiteralPath $OutputRoot -AclObject $acl
    $payload = Join-Path $OutputRoot 'payload'
    New-Item -ItemType Directory -Path $payload | Out-Null
    $nativeTest = Join-Path $payload 'windows-core.test.exe'
    $nativeServer = Join-Path $payload 'maestro-mcp.exe'
    Copy-Item -LiteralPath $TestBinary -Destination $nativeTest
    Copy-Item -LiteralPath $Server -Destination $nativeServer
    if ($AtlasTestBinary) {
        $nativeAtlasTest = Join-Path $payload 'windows-atlas.test.exe'
        Copy-Item -LiteralPath $AtlasTestBinary -Destination $nativeAtlasTest
    }
    if ($withKit) {
        $nativeKit = Join-Path $payload ([IO.Path]::GetFileName($Kit))
        Copy-Item -LiteralPath $Kit -Destination $nativeKit -Recurse
        Copy-Item -LiteralPath ($Kit + '.zip') -Destination ($nativeKit + '.zip')
        Copy-Item -LiteralPath ($Kit + '.zip.sha256') -Destination ($nativeKit + '.zip.sha256')
        $nativeKitTest = Join-Path $payload 'dev/pilot/test-windows-kit.ps1'
        $nativeMethods = Join-Path $payload 'adapters/mcp/packaging/testdata/method-ids.txt'
        foreach ($path in @($nativeKitTest, $nativeMethods)) {
            New-Item -ItemType Directory -Path ([IO.Path]::GetDirectoryName($path)) -Force | Out-Null
        }
        Copy-Item -LiteralPath $KitTestScript -Destination $nativeKitTest
        Copy-Item -LiteralPath $MethodFixture -Destination $nativeMethods
        $nativeSdk = Join-Path $payload 'maestro-pilot-sdk-witness.exe'
        Copy-Item -LiteralPath $SdkWitness -Destination $nativeSdk
        if ($serverHash -cne (Get-FileHash -LiteralPath $nativeServer -Algorithm SHA256).Hash.ToLowerInvariant() -or
            $serverHash -cne (Get-FileHash -LiteralPath (Join-Path $nativeKit 'bin/maestro-mcp-windows-amd64.exe') -Algorithm SHA256).Hash.ToLowerInvariant()) {
            throw 'The copied shipping PE differs from the produced kit.'
        }
        $zipHash = (Get-FileHash -LiteralPath ($Kit + '.zip') -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($zipHash -cne (Get-FileHash -LiteralPath ($nativeKit + '.zip') -Algorithm SHA256).Hash.ToLowerInvariant()) {
            throw 'The ordinary-user proof received a different generic ZIP.'
        }
        $expectedChecksum = $zipHash + '  ' + [IO.Path]::GetFileName($nativeKit) + '.zip'
        if ([IO.File]::ReadAllText($nativeKit + '.zip.sha256').Trim() -cne $expectedChecksum) {
            throw 'The generic ZIP checksum does not bind the actual archive.'
        }
    }
    $env:MAESTRO_WINDOWS_CORE_PROOF = '1'
    $env:MAESTRO_WINDOWS_CORE_SERVER = $nativeServer
    $env:MAESTRO_WINDOWS_CORE_PROOF_ROOT = $OutputRoot
    $stdout = Join-Path $OutputRoot 'native-test.stdout.log'
    $stderr = Join-Path $OutputRoot 'native-test.stderr.log'
    # LoadUserProfile does not replace the caller's inherited TEMP/TMP.
    # Create the synthetic temp inside the credentialed process so its actual
    # owner is the standard user; never grant that user access to runner data.
    $launcher = Join-Path $payload 'native-user-launcher.ps1'
    @'
param([string]$TestBinary, [string]$ProofRoot, [string]$Kit, [string]$KitTestScript, [string]$SdkWitness, [string]$AtlasTestBinary, [switch]$AuthoredBrain)
$ErrorActionPreference = 'Stop'
try {
    $temporary = Join-Path $ProofRoot 'user-temp'
    New-Item -ItemType Directory -Path $temporary | Out-Null
    $env:TEMP = $temporary
    $env:TMP = $temporary
    $families = @('Core', 'DirectoryDurability', 'PrivateState', 'OwnerReview')
    if ($AuthoredBrain) { $families += 'AuthoredBrain' }
    $pattern = '^TestWindows(' + ($families -join '|') + ')Pilot$'
    $coreOutput = @(& $TestBinary '-test.run' $pattern '-test.v' '-test.timeout=600s')
    $coreExit = $LASTEXITCODE
    $coreOutput | Write-Output
    if ($coreExit -ne 0) { throw "Native Core failed with exit $coreExit." }
    $coreLog = $coreOutput -join "`n"
    if ($coreLog -notmatch 'native_windows_environment private_current_user_temp=true' -or
        $coreLog -notmatch 'native_windows_token elevated=false admin_member=false' -or
        $coreLog -match '--- SKIP:') { throw 'Core did not attest the actual ordinary account and private temp without skips.' }
    foreach ($family in $families) {
        if ($coreLog -notmatch ('(?m)^--- PASS: TestWindows' + $family + 'Pilot ')) { throw 'Every requested native family must pass before qualification.' }
    }
    if ($AtlasTestBinary) {
        $atlasFamilies = @('Publication')
        if ($AuthoredBrain) { $atlasFamilies += 'Journal' }
        $atlasPattern = '^TestWindowsAtlas(' + ($atlasFamilies -join '|') + ')Kernel$'
        $atlasOutput = @(& $AtlasTestBinary '-test.run' $atlasPattern '-test.v' '-test.timeout=600s')
        $atlasExit = $LASTEXITCODE
        $atlasOutput | Write-Output
        $atlasLog = $atlasOutput -join "`n"
        if ($atlasExit -ne 0 -or $atlasLog -match '--- SKIP:') {
            throw 'The actual ordinary-account atlas publication kernel did not pass completely.'
        }
        foreach ($family in $atlasFamilies) {
            if ($atlasLog -notmatch ('(?m)^--- PASS: TestWindowsAtlas' + $family + 'Kernel ')) {
                throw 'Every requested atlas kernel must actually pass under the ordinary account.'
            }
        }
    }
    if ($Kit) {
        if ($PSVersionTable.PSEdition -ne 'Desktop' -or $PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion.Minor -ne 1) {
            throw 'Kit qualification requires stock Windows PowerShell5.1.'
        }
        $kitReport = Join-Path $ProofRoot 'native-kit-proof.json'
        & $KitTestScript -Kit $Kit -Report $kitReport
        $kitProof = Get-Content -LiteralPath $kitReport -Raw -Encoding UTF8 | ConvertFrom-Json
        $shippingHash = (Get-FileHash -LiteralPath (Join-Path $Kit 'bin/maestro-mcp-windows-amd64.exe') -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($kitProof.passed -ne $true -or $kitProof.actual_standard_user -ne $true -or $kitProof.shipping_server_sha256 -cne $shippingHash) {
            throw 'Public kit acceptance did not qualify the same shipping server.'
        }
        # Kit acceptance deliberately tampers its last managed-config fixture.
        # Prepare an independent fresh root through the actual public preparer.
        $developer = Join-Path ([Environment]::GetFolderPath([Environment+SpecialFolder]::UserProfile)) 'Developer'
        $sdkRoot = Join-Path $developer ('MaestroSdkProof-' + [Guid]::NewGuid().ToString('N'))
        $shell = Join-Path $env:SystemRoot 'System32/WindowsPowerShell/v1.0/powershell.exe'
        & $shell -NoLogo -NoProfile -NonInteractive -File (Join-Path $Kit 'Preparar Maestro.ps1') -Root $sdkRoot
        if ($LASTEXITCODE -ne 0) { throw 'Fresh SDK preparation failed.' }
        $initialized = Get-Content -LiteralPath (Join-Path $sdkRoot 'diagnostico-init.json') -Raw -Encoding UTF8 | ConvertFrom-Json
        $sdkServer = Join-Path $sdkRoot 'product/maestro-mcp.exe'
        if ((Get-FileHash -LiteralPath $sdkServer -Algorithm SHA256).Hash.ToLowerInvariant() -cne $shippingHash) {
            throw 'Public SDK preparation changed the exact kit PE.'
        }
        $sdkReport = Join-Path $ProofRoot 'native-kit-sdk-proof.json'
        & $SdkWitness -binary $sdkServer -root $sdkRoot -report $sdkReport
        if ($LASTEXITCODE -ne 0) { throw 'Actual shipping and restarted-packet SDK witness failed.' }
        $sdkProof = Get-Content -LiteralPath $sdkReport -Raw -Encoding UTF8 | ConvertFrom-Json
        if ($sdkProof.passed -ne $true -or $sdkProof.workspace_id -cne $initialized.workspace_id -or
            $sdkProof.paused.state_revision -ne 4 -or $sdkProof.resumed.state_revision -ne 5 -or
            $sdkProof.declined_owner_bytes_unchanged -ne $true -or $sdkProof.modern_identity_refused -ne $true) {
            throw 'SDK continuation/review proof differs from the actual prepared binding.'
        }
        $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
        $admin = New-Object Security.Principal.SecurityIdentifier('S-1-5-32-544')
        $principal = New-Object Security.Principal.WindowsPrincipal($identity)
        if ($principal.IsInRole($admin) -or @($identity.Groups | Where-Object { $_.Value -eq $admin.Value }).Count -ne 0) {
            throw 'Kit qualification did not retain an actual ordinary account.'
        }
        $proof = [ordered]@{
            passed=$true; actual_standard_user=$true; powershell_version=$PSVersionTable.PSVersion.ToString()
            shipping_server_sha256=$shippingHash
            generic_zip_sha256=(Get-FileHash -LiteralPath ($Kit + '.zip') -Algorithm SHA256).Hash.ToLowerInvariant()
            sdk_witness_sha256=(Get-FileHash -LiteralPath $SdkWitness -Algorithm SHA256).Hash.ToLowerInvariant()
            core_families_passed=4; core_skips=0; kit_acceptance=$true; fresh_prepared_sdk_binding=$true
            sdk_paused_revision=4; sdk_resumed_revision=5; stale_attempt_refused=$true
            reviewed_memory_restart=$true; declined_owner_bytes_unchanged=$true
            native_Code_Desktop='not_run'; mode='ordinary_unversioned_developer_rehearsal'
        }
        $utf8 = New-Object Text.UTF8Encoding($false)
        [IO.File]::WriteAllText((Join-Path $ProofRoot 'native-kit-qualification.json'), ($proof | ConvertTo-Json -Depth 5) + "`n", $utf8)
    }
    exit 0
} catch {
    Write-Error $_
    exit 1
}
'@ | Set-Content -LiteralPath $launcher -Encoding utf8
    $powershell = Join-Path $PSHOME 'pwsh.exe'
    $arguments = @('-NoLogo', '-NoProfile', '-NonInteractive', '-File', "`"$launcher`"", '-TestBinary', "`"$nativeTest`"", '-ProofRoot', "`"$OutputRoot`"")
    if ($AtlasTestBinary) { $arguments += @('-AtlasTestBinary', "`"$nativeAtlasTest`"") }
    if ($AuthoredBrain) { $arguments += '-AuthoredBrain' }
    if ($withKit) {
        $powershell = Join-Path $env:SystemRoot 'System32/WindowsPowerShell/v1.0/powershell.exe'
        $arguments += @('-Kit', "`"$nativeKit`"", '-KitTestScript', "`"$nativeKitTest`"", '-SdkWitness', "`"$nativeSdk`"")
    }
    $hadModulePath = Test-Path Env:PSModulePath
    $priorModulePath = $env:PSModulePath
    try {
        # Stock Windows PowerShell must resolve its own modules, not inherited PowerShell7 modules.
        if ($withKit) { Remove-Item Env:PSModulePath -ErrorAction SilentlyContinue }
        $process = Start-Process -FilePath $powershell -ArgumentList $arguments -Credential $credential -LoadUserProfile -WorkingDirectory $OutputRoot -RedirectStandardOutput $stdout -RedirectStandardError $stderr -Wait -PassThru
    } finally {
        if ($hadModulePath) { $env:PSModulePath = $priorModulePath }
        else { Remove-Item Env:PSModulePath -ErrorAction SilentlyContinue }
    }
    if ($withKit -or $AuthoredBrain -or $AtlasTestBinary) {
        foreach ($path in @($stdout, $stderr)) {
            if ((Get-Item -LiteralPath $path).Length -gt 2097152) { throw 'Native proof output exceeded its bounded synthetic log contract.' }
        }
    }
    Get-Content -LiteralPath $stdout
    if (Test-Path -LiteralPath $stderr) { Get-Content -LiteralPath $stderr }
    if ($process.ExitCode -ne 0) { throw "Native core test failed with exit $($process.ExitCode); retain the actual log." }
    $log = Get-Content -LiteralPath $stdout -Raw
    if ($log -notmatch 'native_windows_environment private_current_user_temp=true' -or $log -notmatch 'native_windows_token elevated=false admin_member=false' -or $log -notmatch '--- PASS: TestWindowsCorePilot' -or $log -notmatch '--- PASS: TestWindowsDirectoryDurabilityPilot' -or $log -notmatch '--- PASS: TestWindowsPrivateStatePilot' -or $log -notmatch '--- PASS: TestWindowsOwnerReviewPilot' -or $log -match '--- SKIP:') {
        throw 'The standard-user native proof did not run completely.'
    }
    if ($AuthoredBrain -and $log -notmatch '(?m)^--- PASS: TestWindowsAuthoredBrainPilot ') {
        throw 'The authored-brain native family did not pass under the ordinary account.'
    }
    if ($AtlasTestBinary -and $log -notmatch '(?m)^--- PASS: TestWindowsAtlasPublicationKernel ') {
        throw 'The atlas publication kernel did not pass under the ordinary account.'
    }
    if ($AuthoredBrain -and $log -notmatch '(?m)^--- PASS: TestWindowsAtlasJournalKernel ') {
        throw 'The atlas journal kernel did not pass under the ordinary account.'
    }
    if ($withKit) {
        foreach ($proofName in @('native-core-proof.json', 'native-kit-proof.json', 'native-kit-sdk-proof.json', 'native-kit-qualification.json')) {
            $path = Join-Path $OutputRoot $proofName
            if ((Get-Item -LiteralPath $path).Length -gt 2097152) { throw 'Native JSON proof exceeded its bounded contract.' }
            $proof = Get-Content -LiteralPath $path -Raw -Encoding UTF8 | ConvertFrom-Json
            if ($proofName -eq 'native-core-proof.json') {
                if ($proof.server_sha256 -cne $serverHash) { throw 'Core proof does not bind the produced kit PE.' }
            } elseif ($proof.passed -ne $true) { throw 'Exact-artifact proof is incomplete.' }
        }
        $finalProof = Get-Content -LiteralPath (Join-Path $OutputRoot 'native-kit-qualification.json') -Raw -Encoding UTF8 | ConvertFrom-Json
        if ($finalProof.shipping_server_sha256 -cne $serverHash -or $finalProof.generic_zip_sha256 -cne $zipHash) {
            throw 'Final proof does not bind the produced PE and generic ZIP.'
        }
    }
} finally {
    Remove-Item Env:MAESTRO_WINDOWS_CORE_PROOF -ErrorAction SilentlyContinue
    Remove-Item Env:MAESTRO_WINDOWS_CORE_SERVER -ErrorAction SilentlyContinue
    Remove-Item Env:MAESTRO_WINDOWS_CORE_PROOF_ROOT -ErrorAction SilentlyContinue
    if ($account) { Remove-LocalUser -SID $account.SID -ErrorAction Continue }
    $password = $null
    $secure = $null
    $credential = $null
}
