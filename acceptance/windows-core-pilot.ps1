[CmdletBinding(DefaultParameterSetName = 'Pilot')]
param(
    [Parameter(Mandatory = $true, ParameterSetName = 'Pilot')][string]$TestBinary,
    [Parameter(Mandatory = $true, ParameterSetName = 'Pilot')][string]$Server,
    [Parameter(Mandatory = $true)][string]$OutputRoot,
    [Parameter(ParameterSetName = 'Pilot')][string]$Kit,
    [Parameter(ParameterSetName = 'Pilot')][string]$KitTestScript,
    [Parameter(ParameterSetName = 'Pilot')][string]$MethodFixture,
    [Parameter(ParameterSetName = 'Pilot')][string]$SdkWitness,
    [Parameter(ParameterSetName = 'Pilot')][string]$AtlasTestBinary,
    [Parameter(ParameterSetName = 'Pilot')][string]$DoclingTestBinary,
    [Parameter(ParameterSetName = 'Pilot')][string]$CanonicalManifest,
    [Parameter(ParameterSetName = 'Pilot')][switch]$AuthoredBrain,
    [Parameter(ParameterSetName = 'Pilot')][switch]$PrivateWiki,
    [Parameter(Mandatory = $true, ParameterSetName = 'Inspection')][string]$InspectionManifest
)
$ErrorActionPreference = 'Stop'
$kitInputs = @($Kit, $KitTestScript, $MethodFixture, $SdkWitness)
$kitInputCount = @($kitInputs | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }).Count
if ($kitInputCount -ne 0 -and $kitInputCount -ne $kitInputs.Count) {
    throw 'Kit, KitTestScript, MethodFixture and SdkWitness must be supplied together.'
}
$withKit = $kitInputCount -eq $kitInputs.Count
$withCanonical = -not [string]::IsNullOrWhiteSpace($CanonicalManifest)
$withInspection = $PSCmdlet.ParameterSetName -eq 'Inspection'
$withManifest = $withCanonical -or $withInspection
if ($withManifest) {
    $manifestPath = if ($withInspection) { $InspectionManifest } else { $CanonicalManifest }
    if ($withInspection) {
        $manifestFile = Get-Item -LiteralPath $manifestPath -Force
        if ($manifestFile -isnot [IO.FileInfo] -or ($manifestFile.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $manifestFile.Length -gt 2097152) {
            throw 'Inspection manifest must be a bounded ordinary file.'
        }
        if (Test-Path -LiteralPath $OutputRoot) { throw 'Use a new inspection output; previous proof is preserved.' }
    }
    $canonical = Get-Content -LiteralPath $manifestPath -Raw -Encoding UTF8 | ConvertFrom-Json
    $expectedPackages = @('execution', 'datahome', 'ownerctx', 'memory', 'privatewiki', 'processidentity')
    if ($withInspection) { $expectedPackages = @('datahome', 'scheduler', 'zipruntime', 'maestro-runtime') }
    $actualPackages = @($canonical.packages | ForEach-Object { $_.name })
    if ($canonical.schema_version -ne 1 -or $actualPackages.Count -ne $expectedPackages.Count -or
        @((Compare-Object ($expectedPackages | Sort-Object) ($actualPackages | Sort-Object))).Count -ne 0) {
        if ($withInspection) { throw 'Inspection qualification requires exactly the four owning packages.' }
        throw 'Canonical qualification requires exactly the six owning packages.'
    }
    foreach ($path in @($canonical.source_root, $canonical.sdk_root, $canonical.sdk_go)) {
        if (-not [IO.Path]::IsPathRooted($path) -or -not (Test-Path -LiteralPath $path)) {
            throw 'Canonical public source and SDK inputs must be existing absolute paths.'
        }
    }
    if ((Get-FileHash -LiteralPath $canonical.sdk_go -Algorithm SHA256).Hash.ToLowerInvariant() -cne $canonical.sdk_go_sha256) {
        throw 'Canonical pinned SDK hash differs from its manifest.'
    }
    foreach ($entry in $canonical.packages) {
        $relative = 'internal/' + $entry.name
        if ($withInspection -and $entry.name -eq 'maestro-runtime') { $relative = 'cmd/maestro-runtime' }
        if ($entry.package -cne ('./' + $relative) -or
            $entry.working_directory -cne (Join-Path $canonical.source_root $relative) -or
            @($entry.tests).Count -eq 0 -or @($entry.tests | Sort-Object -Unique).Count -ne @($entry.tests).Count -or
            @($entry.tests | Where-Object { $_ -cnotmatch '^(Test|Example)[A-Za-z0-9_]*$' }).Count -ne 0) {
            throw 'Canonical package identity, working directory or closed test inventory is invalid.'
        }
        if ((Get-FileHash -LiteralPath $entry.binary -Algorithm SHA256).Hash.ToLowerInvariant() -cne $entry.sha256) {
            throw 'Canonical test executable differs from its manifest.'
        }
        if ($withInspection) {
            $policies = @($entry.skip_policy)
            if (@($policies | Where-Object { ($_.test -split '/')[0] -cnotin @($entry.tests) -or
                    $_.test -cnotmatch '^(Test|Example)[A-Za-z0-9_]*(/[^\s/]+)*$' -or [string]::IsNullOrWhiteSpace($_.reason) }).Count -ne 0 -or
                @($policies.test | Sort-Object -Unique).Count -ne $policies.Count) {
                throw 'Inspection skips require unique existing tests and explicit platform reasons.'
            }
            if ($entry.name -eq 'zipruntime') {
                foreach ($required in @('TestCodexLifecycleRequiresExplicitMigration', 'TestHookPreflightPreservesCommittedLegacyAndCaseAliases')) {
                    if ($required -cnotin @($entry.tests) -or @($policies | Where-Object { ($_.test -split '/')[0] -ceq $required }).Count -ne 0) {
                        throw 'Both guarded migration contracts must run without skips.'
                    }
                }
            }
        }
    }
    if ($withInspection) {
        if ($canonical.source_revision -cnotmatch '^[0-9a-f]{40}$' -or $canonical.source_archive_sha256 -cnotmatch '^[0-9a-f]{64}$' -or
            @($canonical.vendor_files).Count -eq 0 -or @($canonical.vendor_files.path | Sort-Object -Unique).Count -ne @($canonical.vendor_files).Count) {
            throw 'Inspection source and vendored dependencies require a sealed inventory.'
        }
        foreach ($file in $canonical.vendor_files) {
            if ([IO.Path]::IsPathRooted($file.path) -or $file.path -match '(^|/)\.\.?(/|$)|[\\:\x00\r\n]' -or
                $file.sha256 -cnotmatch '^[0-9a-f]{64}$') { throw 'Invalid sealed vendor path or hash.' }
            $inputFile = Get-Item -LiteralPath (Join-Path $canonical.source_root ('vendor/' + $file.path)) -Force
            if ($inputFile -isnot [IO.FileInfo] -or ($inputFile.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or
                (Get-FileHash -LiteralPath $inputFile.FullName -Algorithm SHA256).Hash.ToLowerInvariant() -cne $file.sha256) {
                throw 'Inspection vendored input differs from its sealed ordinary file.'
            }
        }
    }
}
if ($AtlasTestBinary -and -not (Test-Path -LiteralPath $AtlasTestBinary -PathType Leaf)) {
    throw 'The focused atlas publication witness is missing.'
}
if ($DoclingTestBinary) {
    $doclingEntry = Get-Item -LiteralPath $DoclingTestBinary -Force
    if ($doclingEntry -isnot [IO.FileInfo] -or ($doclingEntry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'The focused Docling placement witness must be an ordinary file.'
    }
    $doclingHash = (Get-FileHash -LiteralPath $DoclingTestBinary -Algorithm SHA256).Hash.ToLowerInvariant()
}
if ($AuthoredBrain -and -not $AtlasTestBinary) {
    throw 'Public authored-brain qualification also requires the publication and journal witnesses.'
}
if ($PrivateWiki -and -not $AuthoredBrain) {
    throw 'Private wiki qualification also requires the held-publication authored-brain profile.'
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
    if (-not $withInspection) {
        Copy-Item -LiteralPath $TestBinary -Destination $nativeTest
        Copy-Item -LiteralPath $Server -Destination $nativeServer
    }
    if ($AtlasTestBinary) {
        $nativeAtlasTest = Join-Path $payload 'windows-atlas.test.exe'
        Copy-Item -LiteralPath $AtlasTestBinary -Destination $nativeAtlasTest
    }
    if ($DoclingTestBinary) {
        $nativeDoclingTest = Join-Path $payload 'windows-docling.test.exe'
        Copy-Item -LiteralPath $DoclingTestBinary -Destination $nativeDoclingTest
        if ((Get-FileHash -LiteralPath $nativeDoclingTest -Algorithm SHA256).Hash.ToLowerInvariant() -cne $doclingHash) {
            throw 'The copied Docling placement witness differs from the compiled executable.'
        }
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
    if ($withManifest) {
        foreach ($entry in $canonical.packages) {
            $copied = Join-Path $payload ($entry.name + '.test.exe')
            Copy-Item -LiteralPath $entry.binary -Destination $copied
            if ((Get-FileHash -LiteralPath $copied -Algorithm SHA256).Hash.ToLowerInvariant() -cne $entry.sha256) {
                throw 'Copied canonical test executable differs from its sealed source.'
            }
            $entry.binary = $copied
        }
        $manifestName = if ($withInspection) { 'inspection-contract-manifest.json' } else { 'canonical-contract-manifest.json' }
        $nativeCanonical = Join-Path $payload $manifestName
        $canonical | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $nativeCanonical -Encoding utf8
    }
    if (-not $withInspection) {
        $env:MAESTRO_WINDOWS_CORE_PROOF = '1'
        $env:MAESTRO_WINDOWS_CORE_SERVER = $nativeServer
        $env:MAESTRO_WINDOWS_CORE_PROOF_ROOT = $OutputRoot
    }
    $stdout = Join-Path $OutputRoot 'native-test.stdout.log'
    $stderr = Join-Path $OutputRoot 'native-test.stderr.log'
    # LoadUserProfile does not replace the caller's inherited TEMP/TMP.
    # Create the synthetic temp inside the credentialed process so its actual
    # owner is the standard user; never grant that user access to runner data.
    $launcher = Join-Path $payload 'native-user-launcher.ps1'
    @'
param([string]$TestBinary, [string]$ProofRoot, [string]$Kit, [string]$KitTestScript, [string]$SdkWitness, [string]$AtlasTestBinary, [string]$DoclingTestBinary, [string]$DoclingTestSHA256, [switch]$AuthoredBrain, [switch]$PrivateWiki, [string]$CanonicalManifest, [switch]$Inspection, [string]$ExpectedUserSID)
$ErrorActionPreference = 'Stop'
try {
    $temporary = Join-Path $ProofRoot 'user-temp'
    New-Item -ItemType Directory -Path $temporary | Out-Null
    $env:TEMP = $temporary
    $env:TMP = $temporary
    if ($Inspection) {
        $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
        $admin = [Security.Principal.SecurityIdentifier]::new('S-1-5-32-544')
        $principal = [Security.Principal.WindowsPrincipal]::new($identity)
        Add-Type -TypeDefinition 'using System; using System.Runtime.InteropServices; public static class InspectionToken { [DllImport("advapi32.dll", SetLastError=true)] public static extern bool GetTokenInformation(IntPtr token, int kind, out uint value, uint length, out uint returned); }'
        [uint32]$elevation = 0; [uint32]$returned = 0
        if (-not [InspectionToken]::GetTokenInformation($identity.Token, 20, [ref]$elevation, 4, [ref]$returned) -or
            $returned -ne 4 -or $elevation -ne 0 -or $identity.User.Value -cne $ExpectedUserSID -or
            $principal.IsInRole($admin) -or @($identity.Groups | Where-Object { $_.Value -eq $admin.Value }).Count -ne 0) {
            throw 'Inspection requires the actual intended non-elevated ordinary account.'
        }
        $userHomeFixture = Join-Path $ProofRoot 'inspection-user-home'
        New-Item -ItemType Directory -Path $userHomeFixture | Out-Null
        foreach ($privateFixture in @($temporary, $userHomeFixture)) {
            $fixtureEntry = Get-Item -LiteralPath $privateFixture -Force
            $fixtureAcl = Get-Acl -LiteralPath $privateFixture
            if ($fixtureEntry -isnot [IO.DirectoryInfo] -or ($fixtureEntry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or
                $fixtureAcl.GetOwner([Security.Principal.SecurityIdentifier]).Value -cne $ExpectedUserSID -or
                @($fixtureAcl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier]) | Where-Object {
                    $_.IdentityReference.Value -cnotin @($ExpectedUserSID, 'S-1-5-18', 'S-1-5-32-544') -or $_.AccessControlType -ne 'Allow'
                }).Count -ne 0) { throw 'Inspection private fixtures must be ordinary child-owned directories with only private principals.' }
        }
        $previousUserProfile = $env:USERPROFILE
        $previousHome = $env:HOME
        $env:USERPROFILE = $userHomeFixture
        $env:HOME = $userHomeFixture
        $inspectionProfile = [ordered]@{
            profile_resolution='inspection_private_fixture'
            inherited_userprofile_matches_fixture=[string]::Equals($previousUserProfile, $userHomeFixture, [StringComparison]::OrdinalIgnoreCase)
            inherited_home_matches_fixture=[string]::Equals($previousHome, $userHomeFixture, [StringComparison]::OrdinalIgnoreCase)
            private_fixture_roots_admitted=$true
        }
        Write-Output 'inspection_profile elevated=false admin_member=false private_current_user_temp=true'
    }
    if (-not $Inspection) {
    $families = @('Core', 'DirectoryDurability', 'PrivateState', 'OwnerReview')
    if ($AuthoredBrain) { $families += @('AuthoredBrain', 'AuthoredBrainPartialPreparation') }
    if ($PrivateWiki) { $families += 'PrivateWiki' }
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
    if ($DoclingTestBinary) {
        if ((Get-FileHash -LiteralPath $DoclingTestBinary -Algorithm SHA256).Hash.ToLowerInvariant() -cne $DoclingTestSHA256) {
            throw 'The ordinary-account Docling placement witness changed.'
        }
        $doclingTests = @('TestPlacementWindowsSealsAndFlushesHeldRoot', 'TestPlacementWindowsPreservesOccupiedDestination')
        $doclingPattern = '^(' + ($doclingTests -join '|') + ')$'
        $doclingOutput = @(& $DoclingTestBinary '-test.run' $doclingPattern '-test.v' '-test.timeout=600s' 2>&1 | ForEach-Object { "$_" })
        $doclingExit = $LASTEXITCODE
        $doclingLog = $doclingOutput -join "`n"
        if ([Text.Encoding]::UTF8.GetByteCount($doclingLog) -gt 2097152) { throw 'Docling placement output exceeds the bounded log contract.' }
        $doclingOutput | Write-Output
        if ($doclingExit -ne 0 -or $doclingLog -match '--- (FAIL|SKIP):') {
            throw 'The actual ordinary-account Docling placement tests did not pass completely.'
        }
        foreach ($test in $doclingTests) {
            if ($doclingLog -notmatch ('(?m)^--- PASS: ' + $test + ' ')) {
                throw 'Every requested Docling placement test must actually pass under the ordinary account.'
            }
        }
        Write-Output ('native_windows_docling placement_tests_passed=' + $doclingTests.Count + ' binary_sha256=' + $DoclingTestSHA256)
    }
    }
    if ($CanonicalManifest) {
        $manifest = Get-Content -LiteralPath $CanonicalManifest -Raw -Encoding UTF8 | ConvertFrom-Json
        if ($env:GOROOT) { throw 'Canonical evidence refuses caller-supplied GOROOT.' }
        foreach ($root in @($manifest.source_root, $manifest.sdk_root)) {
            $probe = Join-Path $root ('ordinary-write-probe-' + [Guid]::NewGuid().ToString('N'))
            $wrote = $false
            try { [IO.File]::WriteAllText($probe, 'unexpected-write'); $wrote = $true } catch [UnauthorizedAccessException] { }
            if ($wrote) { Remove-Item -LiteralPath $probe; throw 'Canonical public input is writable by the ordinary account.' }
        }
        if ((Get-FileHash -LiteralPath $manifest.sdk_go -Algorithm SHA256).Hash.ToLowerInvariant() -cne $manifest.sdk_go_sha256) {
            throw 'Ordinary-account SDK differs from the pinned executable.'
        }
        $env:PATH = (Split-Path -Parent $manifest.sdk_go) + ';' + $env:PATH
        $env:GOCACHE = Join-Path $temporary 'go-build'
        $env:GOPATH = Join-Path $temporary 'go-path'
        $env:GOMODCACHE = Join-Path $env:GOPATH 'pkg/mod'
        $env:GOENV = 'off'
        $env:GOTOOLCHAIN = 'local'
        if ($Inspection) {
            $env:GOTMPDIR = $temporary
            $env:GOWORK = 'off'
            $env:GOFLAGS = '-mod=vendor -buildvcs=false'
            $env:GOPROXY = 'off'
            $env:GOSUMDB = 'off'
            foreach ($file in $manifest.vendor_files) {
                $path = Join-Path $manifest.source_root ('vendor/' + $file.path)
                if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -cne $file.sha256) {
                    throw 'Ordinary-account vendor input differs from its sealed source.'
                }
            }
        }
        $selectedRoot = (& $manifest.sdk_go env GOROOT).Trim()
        if ($LASTEXITCODE -ne 0 -or $selectedRoot -cne $manifest.sdk_root) {
            throw 'Ordinary-account SDK did not retain the compiled public root.'
        }
        $results = @()
        foreach ($entry in $manifest.packages) {
            if ((Get-FileHash -LiteralPath $entry.binary -Algorithm SHA256).Hash.ToLowerInvariant() -cne $entry.sha256) {
                throw 'Ordinary-account canonical executable changed.'
            }
            $listed = @(& $entry.binary '-test.list' '^(Test|Example)')
            if ($LASTEXITCODE -ne 0 -or @((Compare-Object ($entry.tests | Sort-Object) ($listed | Sort-Object))).Count -ne 0) {
                throw 'The actual canonical binary test inventory differs from its closed manifest.'
            }
            Push-Location -LiteralPath $entry.working_directory
            try {
                $output = @(& $entry.binary '-test.v' '-test.timeout=300s' 2>&1 | ForEach-Object { "$_" })
                $code = $LASTEXITCODE
            } finally { Pop-Location }
            $body = $output -join "`n"
            if ([Text.Encoding]::UTF8.GetByteCount($body) -gt 2097152) { throw 'Canonical package output exceeds the bounded log contract.' }
            $prefix = if ($Inspection) { 'inspection' } else { 'canonical' }
            $packageLog = Join-Path $ProofRoot ($prefix + '-' + $entry.name + '.log')
            [IO.File]::WriteAllText($packageLog, $body + "`n", (New-Object Text.UTF8Encoding($false)))
            $outcomes = @([regex]::Matches($body, '(?m)^--- (PASS|FAIL|SKIP): ((?:Test|Example)[A-Za-z0-9_]*) ') | ForEach-Object {
                [ordered]@{test=$_.Groups[2].Value; outcome=$_.Groups[1].Value}
            })
            $observed = @($outcomes | ForEach-Object { $_.test })
            $complete = @((Compare-Object ($entry.tests | Sort-Object) ($observed | Sort-Object))).Count -eq 0
            $skipped = @($outcomes | Where-Object { $_.outcome -eq 'SKIP' } | ForEach-Object { $_.test })
            $allSkips = @([regex]::Matches($body, '(?m)^\s*--- SKIP: ((?:Test|Example)\S*) ') | ForEach-Object { $_.Groups[1].Value })
            $unclassified = @()
            $requiredPassed = $true
            if ($Inspection) {
                $unclassified = @($allSkips | Where-Object { $_ -cnotin @($entry.skip_policy.test) })
                if ($entry.name -eq 'zipruntime') {
                    foreach ($test in @('TestCodexLifecycleRequiresExplicitMigration', 'TestHookPreflightPreservesCommittedLegacyAndCaseAliases')) {
                        if (@($outcomes | Where-Object { $_.test -ceq $test -and $_.outcome -ceq 'PASS' }).Count -ne 1 -or
                            @($allSkips | Where-Object { ($_ -split '/')[0] -ceq $test }).Count -ne 0) { $requiredPassed = $false }
                    }
                }
            }
            $results += [ordered]@{
                name=$entry.name; package=$entry.package; sha256=$entry.sha256; exit_code=$code
                expected_tests=@($entry.tests); outcomes=$outcomes; inventory_complete=$complete
                skipped_tests=$skipped; log=[IO.Path]::GetFileName($packageLog)
                log_sha256=(Get-FileHash -LiteralPath $packageLog -Algorithm SHA256).Hash.ToLowerInvariant()
                owner_mismatch_diagnostics=([regex]::Matches($body, 'private object belongs to a different Windows owner')).Count
            }
            if ($Inspection) {
                $results[-1].all_skipped_tests = $allSkips
                $results[-1].skip_policy = @($entry.skip_policy)
                $results[-1].unclassified_skips = $unclassified
                $results[-1].required_migration_tests_passed = $requiredPassed
            }
            Write-Output ($prefix + '_result package=' + $entry.name + ' exit=' + $code + ' inventory_complete=' + $complete + ' skips=' + $skipped.Count)
        }
        $qualificationComplete = @($results | Where-Object { $_.exit_code -ne 0 -or -not $_.inventory_complete -or $_.skipped_tests.Count -ne 0 }).Count -eq 0
        if ($Inspection) {
            $qualificationComplete = @($results | Where-Object { $_.exit_code -ne 0 -or -not $_.inventory_complete -or
                $_.unclassified_skips.Count -ne 0 -or -not $_.required_migration_tests_passed }).Count -eq 0
        }
        $report = [ordered]@{
            schema_version=1; source_revision=$manifest.source_revision; actual_standard_user=$true
            private_current_user_temp=$true; public_source_and_sdk_read_only=$true
            sdk_go_sha256=$manifest.sdk_go_sha256; sdk_root=$selectedRoot
            packages=$results; qualification_complete=$qualificationComplete
            platform_nonapplicability='unclassified_existing_skips_are_not_passing_coverage'
        }
        $reportName = if ($Inspection) { 'inspection-contract-results.json' } else { 'canonical-contract-results.json' }
        if ($Inspection) {
            $report.elevated = $false; $report.admin_member = $false
            $report.profile_environment = $inspectionProfile
            $report.platform_nonapplicability = 'classified_existing_skips_are_not_passing_coverage'
            $report.observed_skips_count = @($results | ForEach-Object { $_.all_skipped_tests }).Count
            $report.unclassified_skips_count = @($results | ForEach-Object { $_.unclassified_skips }).Count
            $report.classified_skips_count = $report.observed_skips_count - $report.unclassified_skips_count
            $report.all_contracts_qualified = $qualificationComplete -and $report.observed_skips_count -eq 0
            $report.native_coverage = 'Four inspection package binaries only; classified skips are missing coverage, and native Code/Desktop is not qualified.'
        }
        $report | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath (Join-Path $ProofRoot $reportName) -Encoding utf8
        if (-not $qualificationComplete) {
            if ($Inspection) { throw 'Inspection contracts remain incomplete; retain all four package logs and classified outcomes.' }
            throw 'Canonical contracts remain incomplete; retain all six package logs and classified outcomes.'
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
            core_families_passed=$families.Count; core_skips=0; kit_acceptance=$true; fresh_prepared_sdk_binding=$true
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
    $arguments = @('-NoLogo', '-NoProfile', '-NonInteractive', '-File', "`"$launcher`"", '-ProofRoot', "`"$OutputRoot`"")
    if ($withInspection) { $arguments += @('-Inspection', '-ExpectedUserSID', $account.SID.Value) }
    else { $arguments += @('-TestBinary', "`"$nativeTest`"") }
    if ($AtlasTestBinary) { $arguments += @('-AtlasTestBinary', "`"$nativeAtlasTest`"") }
    if ($DoclingTestBinary) { $arguments += @('-DoclingTestBinary', "`"$nativeDoclingTest`"", '-DoclingTestSHA256', $doclingHash) }
    if ($AuthoredBrain) { $arguments += '-AuthoredBrain' }
    if ($PrivateWiki) { $arguments += '-PrivateWiki' }
    if ($withManifest) { $arguments += @('-CanonicalManifest', "`"$nativeCanonical`"") }
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
    if ($withKit -or $AuthoredBrain -or $AtlasTestBinary -or $DoclingTestBinary -or $withInspection) {
        foreach ($path in @($stdout, $stderr)) {
            if ((Get-Item -LiteralPath $path).Length -gt 2097152) { throw 'Native proof output exceeded its bounded synthetic log contract.' }
        }
    }
    Get-Content -LiteralPath $stdout
    if (Test-Path -LiteralPath $stderr) { Get-Content -LiteralPath $stderr }
    if ($process.ExitCode -ne 0) { throw "Native core test failed with exit $($process.ExitCode); retain the actual log." }
    $log = Get-Content -LiteralPath $stdout -Raw
    if ($withInspection) {
        $reportPath = Join-Path $OutputRoot 'inspection-contract-results.json'
        $reportFile = Get-Item -LiteralPath $reportPath -Force
        if ($reportFile -isnot [IO.FileInfo] -or ($reportFile.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $reportFile.Length -gt 2097152) {
            throw 'Inspection proof must be a bounded ordinary file.'
        }
        $report = Get-Content -LiteralPath $reportPath -Raw -Encoding UTF8 | ConvertFrom-Json
        if (-not $report.qualification_complete -or -not $report.actual_standard_user -or
            $log -notmatch 'inspection_profile elevated=false admin_member=false private_current_user_temp=true') {
            throw 'The standard-user inspection proof did not run completely.'
        }
    } else {
    if ($log -notmatch 'native_windows_environment private_current_user_temp=true' -or $log -notmatch 'native_windows_token elevated=false admin_member=false' -or $log -notmatch '--- PASS: TestWindowsCorePilot' -or $log -notmatch '--- PASS: TestWindowsDirectoryDurabilityPilot' -or $log -notmatch '--- PASS: TestWindowsPrivateStatePilot' -or $log -notmatch '--- PASS: TestWindowsOwnerReviewPilot' -or $log -match '--- SKIP:') {
        throw 'The standard-user native proof did not run completely.'
    }
    if ($AuthoredBrain -and $log -notmatch '(?m)^--- PASS: TestWindowsAuthoredBrainPilot ') {
        throw 'The authored-brain native family did not pass under the ordinary account.'
    }
    if ($PrivateWiki -and $log -notmatch '(?m)^--- PASS: TestWindowsPrivateWikiPilot ') {
        throw 'The private wiki native family did not pass under the ordinary account.'
    }
    if ($AtlasTestBinary -and $log -notmatch '(?m)^--- PASS: TestWindowsAtlasPublicationKernel ') {
        throw 'The atlas publication kernel did not pass under the ordinary account.'
    }
    if ($AuthoredBrain -and $log -notmatch '(?m)^--- PASS: TestWindowsAtlasJournalKernel ') {
        throw 'The atlas journal kernel did not pass under the ordinary account.'
    }
    if ($DoclingTestBinary -and $log -notmatch ('(?m)^' + [regex]::Escape('native_windows_docling placement_tests_passed=2 binary_sha256=' + $doclingHash))) {
        throw 'The Docling placement proof does not bind both passing tests to the compiled executable.'
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
