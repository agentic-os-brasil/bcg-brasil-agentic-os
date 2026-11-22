# Typed helpers for the existing windows-core-pilot.ps1 credentialed launcher.
# No account creation, generic command execution or publisher authority lives here.
$script:DoclingRuntimeQualificationProfile = 'windows_docling_registry_read_r17'
$script:DoclingRuntimeTests = @('TestWindowsNativeConfinement', 'TestWindowsNativeConfinementDeadline', 'TestWindowsSharedImplementationIdentityAcrossEntryModules', 'TestWindowsRealAuthenticatedDocling')
function Read-DoclingRuntimeManifest([string]$Path) {
    $entry = Get-Item -LiteralPath $Path -Force
    if ($entry -isnot [IO.FileInfo] -or ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $entry.Length -gt 65536) { throw 'Docling manifest must be a bounded ordinary file.' }
    $manifest = Get-Content -LiteralPath $Path -Raw -Encoding UTF8 | ConvertFrom-Json
    $fields = @('schema_version','qualification_profile','runtime_profile','runtime_lock_sha256','source_revision','source_tree','public_root','source_root','sdk_go','module_cache','raw_inputs','test_binary','inventory_sha256','required_tests')
    if (@($manifest.PSObject.Properties.Name).Count -ne $fields.Count -or @($manifest.PSObject.Properties.Name | Where-Object { $_ -cnotin $fields }).Count -ne 0 -or
        $manifest.schema_version -ne 1 -or $manifest.qualification_profile -cne $script:DoclingRuntimeQualificationProfile -or $manifest.runtime_profile -cne 'model-free-documents-v1' -or $manifest.runtime_lock_sha256 -cne '38b1a0257c3feec8a5b4b97856e7f2096cdb72ed2bd1734d8e61567bf4757643' -or $manifest.source_revision -cnotmatch '^[a-f0-9]{40}$' -or
        $manifest.source_tree -cnotmatch '^[a-f0-9]{40}$' -or $manifest.inventory_sha256 -cnotmatch '^[a-f0-9]{64}$' -or @($manifest.required_tests).Count -ne $script:DoclingRuntimeTests.Count -or
        @((Compare-Object ($script:DoclingRuntimeTests | Sort-Object -CaseSensitive) (@($manifest.required_tests) | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0) { throw 'Docling requires the exact closed native runtime profile.' }
    if ($manifest.public_root -cnotmatch '^[A-Za-z]:\\MaestroNativeEngine-[a-f0-9]{32}$' -or
        -not [string]::Equals([IO.Path]::GetFullPath($manifest.public_root), $manifest.public_root, [StringComparison]::OrdinalIgnoreCase)) { throw 'Docling public input root is not the fresh sealed engine root.' }
    foreach ($binding in @{source_root='source'; sdk_go='sdk/bin/go.exe'; module_cache='modules'; raw_inputs='raw'; test_binary='bin/docling.test.exe'}.GetEnumerator()) {
        if (-not [string]::Equals($manifest.($binding.Key), (Join-Path $manifest.public_root $binding.Value), [StringComparison]::OrdinalIgnoreCase)) { throw 'Docling selected input escapes its fixed public root.' }
    }
    if (-not [string]::Equals($entry.FullName, (Join-Path $manifest.public_root 'docling-runtime-manifest.json'), [StringComparison]::OrdinalIgnoreCase)) { throw 'Docling manifest must occupy its one canonical public leaf.' }
    $inventoryPath = Join-Path $manifest.public_root 'docling-input-inventory.jsonl'
    $inventoryFile = Get-Item -LiteralPath $inventoryPath -Force
    if ($inventoryFile -isnot [IO.FileInfo] -or ($inventoryFile.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $inventoryFile.Length -gt 67108864 -or
        (Get-FileHash -LiteralPath $inventoryPath -Algorithm SHA256).Hash.ToLowerInvariant() -cne $manifest.inventory_sha256) { throw 'Docling sealed public inventory differs.' }
    $listed = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    [long]$expanded = 0
    foreach ($line in [IO.File]::ReadLines($inventoryPath)) {
        $file = $line | ConvertFrom-Json
        if (@($file.PSObject.Properties.Name).Count -ne 3 -or @($file.PSObject.Properties.Name | Where-Object { $_ -cnotin @('path','size','sha256') }).Count -ne 0 -or
            [IO.Path]::IsPathRooted($file.path) -or $file.path -match '(^|/)\.\.?(/|$)|[\\:\x00-\x1f]' -or $file.path -match '(^|/)[^/]*[. ](/|$)' -or
            $file.path -cin @('docling-runtime-manifest.json','docling-input-inventory.jsonl') -or $file.sha256 -cnotmatch '^[a-f0-9]{64}$' -or
            $file.size -isnot [long] -and $file.size -isnot [int] -or $file.size -lt 0 -or $file.size -gt 536870912 -or -not $listed.Add($file.path) -or $listed.Count -gt 100000) { throw 'Docling public inventory contains an unsafe or duplicate input.' }
        $selected = Get-Item -LiteralPath (Join-Path $manifest.public_root $file.path) -Force
        if ($selected -isnot [IO.FileInfo] -or ($selected.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $selected.Length -ne $file.size -or
            (Get-FileHash -LiteralPath $selected.FullName -Algorithm SHA256).Hash.ToLowerInvariant() -cne $file.sha256) { throw 'Docling exact public source, SDK, dependency, raw input or test image changed.' }
        $expanded += $file.size
        if ($expanded -gt 4294967296) { throw 'Docling public input closure exceeds its size bound.' }
    }
    foreach ($selected in @((Get-Item -LiteralPath $manifest.public_root -Force)) + @(Get-ChildItem -LiteralPath $manifest.public_root -Recurse -Force)) {
        if (($selected.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Docling public input closure contains a reparse object.' }
        if ($selected -is [IO.FileInfo]) {
            $relative = $selected.FullName.Substring($manifest.public_root.Length + 1).Replace('\','/')
            if ($relative -cnotin @('docling-runtime-manifest.json','docling-input-inventory.jsonl') -and -not $listed.Remove($relative)) { throw 'Docling public input closure has an unlisted file.' }
        }
    }
    if ($listed.Count -ne 0) { throw 'Docling public input closure is incomplete.' }
    $runtimeInputs = @(Get-Content -LiteralPath (Join-Path $manifest.source_root 'internal/ingest/docling/runtime/documents/windows-amd64/inputs.json') -Raw -Encoding UTF8 | ConvertFrom-Json)
    if ($runtimeInputs.Count -ne 56 -or @($runtimeInputs.filename | Sort-Object -Unique).Count -ne 56 -or @($runtimeInputs | Measure-Object size -Sum).Sum -ne 85851562) { throw 'Docling Windows wheel closure differs.' }
    if ((Get-FileHash -LiteralPath (Join-Path $manifest.source_root 'internal/ingest/docling/runtime/documents/windows-amd64/pylock.toml') -Algorithm SHA256).Hash.ToLowerInvariant() -cne $manifest.runtime_lock_sha256) { throw 'Full document profile lock differs.' }
    $pythonName = 'cpython-3.12.16+20261026-x86_64-pc-windows-msvc-install_only.tar.gz'
    $raw = @(Get-ChildItem -LiteralPath $manifest.raw_inputs -Force)
    if ($raw.Count -ne 57 -or @($raw | Where-Object { $_ -isnot [IO.FileInfo] }).Count -ne 0) { throw 'Docling raw input closure must contain exactly 57 ordinary files.' }
    foreach ($inputFile in $runtimeInputs) {
        $path = Join-Path $manifest.raw_inputs $inputFile.filename
        if ((Get-Item -LiteralPath $path).Length -ne $inputFile.size -or (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -cne $inputFile.sha256) { throw 'Docling pinned wheel differs.' }
    }
    $pythonPath = Join-Path $manifest.raw_inputs $pythonName
    if ((Get-Item -LiteralPath $pythonPath).Length -ne 47113364 -or (Get-FileHash -LiteralPath $pythonPath -Algorithm SHA256).Hash.ToLowerInvariant() -cne '590dfcbe3a34c07301e6332e13e2188b98655cd072b08afa45a7998dc4c22bc5') { throw 'Docling pinned Windows interpreter differs.' }
    return $manifest
}

# Copy only a bounded observation of a private raw capture. Publish the fixed
# upload leaf only after its fresh staging file is closed and hashable.
function Copy-DoclingFailureSnapshot([string]$Raw, [string]$Retained, [string]$PrivateStage, [long]$Limit, [bool]$ProducerExited) {
    if($Limit -lt 0 -or $Limit -gt 4194304 -or (Test-Path -LiteralPath $Retained)){throw 'Snapshot requires a fresh leaf and finite bound.'}
    $stage=Join-Path $PrivateStage ('.bounded-'+[Guid]::NewGuid().ToString('N'))
    $source=$null;$target=$null;[long]$copied=0
    try {
        $source=[IO.File]::Open($Raw,[IO.FileMode]::Open,[IO.FileAccess]::Read,([IO.FileShare]::ReadWrite -bor [IO.FileShare]::Delete))
        [long]$observed=$source.Length;[long]$wanted=[Math]::Min($observed,$Limit)
        $target=[IO.File]::Open($stage,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
        $buffer=New-Object byte[] 65536
        while($copied -lt $wanted){
            $count=$source.Read($buffer,0,[int][Math]::Min($buffer.Length,$wanted-$copied))
            if($count -eq 0){throw 'Raw capture ended before its observed snapshot.'}
            $target.Write($buffer,0,$count);$copied+=$count
        }
        $target.Flush($true);[long]$after=$source.Length
    } finally {if($null -ne $target){$target.Dispose()};if($null -ne $source){$source.Dispose()}}
    if((Get-Item -LiteralPath $stage -Force).Length -ne $copied -or $copied -gt $Limit){throw 'Closed snapshot differs from its bound.'}
    $hash=(Get-FileHash -LiteralPath $stage -Algorithm SHA256).Hash.ToLowerInvariant()
    $writerClosed=$false
    try {$probe=[IO.File]::Open($Raw,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read);$after=$probe.Length;$probe.Dispose();$writerClosed=$true}catch{}
    # The two-argument move refuses any occupied retained destination.
    [IO.File]::Move($stage,$Retained)
    return @{name=[IO.Path]::GetFileName($Retained);raw_length_observed=$observed;raw_length_after_observed=$after
        retained_bytes=$copied;sha256=$hash;producer_exited=$ProducerExited;writer_closed_observed=$writerClosed
        snapshot_complete=($ProducerExited -and $writerClosed -and $copied -eq $observed -and $after -eq $observed)}
}

# Failure-only observation inside the already-admitted ordinary account. Debugger
# results never qualify conversion, overwrite the baseline log, or authorize export.
function Invoke-DoclingFailureTrace($Manifest, [string]$ProofRoot, [string]$ExpectedImageSHA256, [int]$BaselineExit) {
    $receiptPath=Join-Path $ProofRoot 'docling-loader-trace.json'
    if(Test-Path -LiteralPath $receiptPath){Write-Output 'docling_failure_trace diagnostic_only=true status=existing_receipt_preserved';return}
    $receipt = [ordered]@{schema_version=1; diagnostic_only=$true; qualification=$false; baseline_exit_code=$BaselineExit
        baseline_log_sha256=(Get-FileHash -LiteralPath (Join-Path $ProofRoot 'docling-native-tests.log') -Algorithm SHA256).Hash.ToLowerInvariant()
        source_revision=$Manifest.source_revision; test_binary_sha256=$ExpectedImageSHA256
        status='not_started'; timeout_seconds=60; retained_log_limit_bytes=4194304; no_registry_changes=$true; debuggee_kill_on_debugger_exit=$true
        public_symbol_prefetch_configured=$true; public_symbol_modules=@('ntdll.dll','KERNELBASE.dll'); public_symbol_store='https://msdl.microsoft.com/download/symbols'; symbol_network_phase='stopped_parent_before_test_only'
        replay_test='TestWindowsRealAuthenticatedDocling'; module_load_scope='successful_loads_are_not_failed_dll_attribution'}
    $process=$null; $outputsAdmitted=$false; $traceOut=Join-Path $ProofRoot 'docling-loader-trace.stdout.log'; $traceErr=Join-Path $ProofRoot 'docling-loader-trace.stderr.log'
    try {
        if ($env:GITHUB_ACTIONS -cne 'true' -or $env:RUNNER_ENVIRONMENT -cne 'github-hosted' -or [Environment]::OSVersion.Platform -ne 'Win32NT') {
            $receipt.status='not_run_outside_disposable_hosted_windows'; return
        }
        $debugger='C:\Program Files (x86)\Windows Kits\10\Debuggers\x64\cdb.exe'
        $tool=Get-Item -LiteralPath $debugger -Force
        if ($tool -isnot [IO.FileInfo] -or ($tool.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Stock CDB is not an ordinary file.' }
        if ((Get-FileHash -LiteralPath $Manifest.test_binary -Algorithm SHA256).Hash.ToLowerInvariant() -cne $ExpectedImageSHA256) { throw 'Sealed test image changed before diagnostic replay.' }
        $receipt.debugger_sha256=(Get-FileHash -LiteralPath $debugger -Algorithm SHA256).Hash.ToLowerInvariant()
        $commands=Join-Path $ProofRoot 'docling-loader-commands.txt'
        $stdin=Join-Path $ProofRoot 'docling-loader-stdin.txt'; $symbols=Join-Path $ProofRoot 'docling-loader-symbols'
        $rawRoot=Join-Path $ProofRoot ('.docling-loader-raw-'+[Guid]::NewGuid().ToString('N'))
        $rawOut=Join-Path $rawRoot 'stdout.raw';$rawErr=Join-Path $rawRoot 'stderr.raw'
        foreach($path in @($traceOut,$traceErr,$commands,$stdin,$symbols,$rawRoot)) { if(Test-Path -LiteralPath $path){throw 'Diagnostic output must be fresh.'} }
        # Inherit the already-private ProofRoot ACL; RAW names are never upload inputs.
        New-Item -ItemType Directory -Path $rawRoot|Out-Null
        $outputsAdmitted=$true
        # Fetch only matching stock NTDLL/KERNELBASE metadata before the test.
        # The replay follows the real LPAC Python child without changing its flags.
        # Restore local symbols before execution; successful loads do not name a failed DLL.
        if($symbols -match '[;"\r\n]'){throw 'Diagnostic symbol cache path is ambiguous.'}
        $scriptText=@'
.echo MAESTRO_PUBLIC_NTDLL_PREFETCH
.sympath srv*__MAESTRO_SYMBOL_CACHE__*https://msdl.microsoft.com/download/symbols
.reload /f ntdll.dll
lmvm ntdll
!lmi ntdll
.reload /f KERNELBASE.dll
lmvm KERNELBASE
!lmi KERNELBASE
.sympath srv*__MAESTRO_SYMBOL_CACHE__
.echo MAESTRO_SYMBOLS_LOCAL_ONLY
sxe -c ".echo MAESTRO_PYTHON_CREATE;.lastevent;|.;g" cpr:python.exe
sxe -c ".echo MAESTRO_MODULE_LOAD_SUCCESS;.lastevent;|.;g" ld
sxe -c ".lastevent;k;g" bpe
sxe -c ".echo MAESTRO_ACCESS_DENIED_EXCEPTION;.lastevent;|.;k;gn" 0xc0000022
sxe -c ".echo MAESTRO_PYTHON_EXIT;.lastevent;|.;g" epr:python.exe
g
'@
        $scriptText=$scriptText.Replace('__MAESTRO_SYMBOL_CACHE__',$symbols)
        $receipt.symbol_cache=$symbols
        [IO.File]::WriteAllText($commands,$scriptText,[Text.UTF8Encoding]::new($false))
        [IO.File]::WriteAllText($stdin,'',[Text.UTF8Encoding]::new($false))
        [IO.Directory]::CreateDirectory($symbols)|Out-Null
        $arguments=@('-o','-G','-hd','-sins','-noshell','-nosqm','-y',('"'+$symbols+'"'),'-cf',('"'+$commands+'"'),('"'+$Manifest.test_binary+'"'),'-test.run','^TestWindowsRealAuthenticatedDocling$','-test.v','-test.timeout=2400s')
        $receipt.debugger_arguments=$arguments; $receipt.commands_sha256=(Get-FileHash -LiteralPath $commands -Algorithm SHA256).Hash.ToLowerInvariant()
        $watch=[Diagnostics.Stopwatch]::StartNew()
        $process=Start-Process -FilePath $debugger -ArgumentList $arguments -WorkingDirectory $Manifest.source_root -NoNewWindow -RedirectStandardInput $stdin -RedirectStandardOutput $rawOut -RedirectStandardError $rawErr -PassThru
        $null=$process.Handle # Retain this launched process, never look it up by name.
        $receipt.status='running'
        while(-not $process.WaitForExit(200)) {
            [long]$bytes=0;foreach($path in @($rawOut,$rawErr)){if(Test-Path -LiteralPath $path){$bytes+=(Get-Item -LiteralPath $path).Length}}
            if($bytes -gt 4194304){$receipt.status='output_limit';break}
            if($watch.Elapsed.TotalSeconds -ge 60){$receipt.status='timeout';break}
        }
        if($process.HasExited){if($receipt.status -ceq 'running'){$receipt.status='completed'};$receipt.debugger_exit_code=$process.ExitCode}
        $receipt.elapsed_seconds=$watch.Elapsed.TotalSeconds
        $receipt.test_image_unchanged=((Get-FileHash -LiteralPath $Manifest.test_binary -Algorithm SHA256).Hash.ToLowerInvariant() -ceq $ExpectedImageSHA256)
    } catch {
        $receipt.status='diagnostic_error';$receipt.error_type=$_.Exception.GetType().FullName
    } finally {
        if($null -ne $process){
            try {
                if(-not $process.HasExited){$process.Kill();$receipt.owned_debugger_terminated=$true}
                $receipt.owned_debugger_exited=$process.WaitForExit(5000)
            } catch {$receipt.cleanup_error_type=$_.Exception.GetType().FullName}
            finally {$process.Dispose()}
        }
        # Only closed, verified bounded copies may occupy the fixed upload names.
        # An uncertain producer exit withholds them; raw capture remains private.
        [long]$remaining=4194304;$logs=@();$receipt.retention_status='not_started'
        if($outputsAdmitted){
            if($receipt.Contains('owned_debugger_exited') -and $receipt.owned_debugger_exited){
                try {
                    foreach($capture in @(@{raw=$rawOut;retained=$traceOut},@{raw=$rawErr;retained=$traceErr})){
                        if(Test-Path -LiteralPath $capture.raw){
                            $snapshot=Copy-DoclingFailureSnapshot $capture.raw $capture.retained $rawRoot $remaining $true
                            $remaining-=$snapshot.retained_bytes;$logs+=$snapshot
                        }
                    }
                    $receipt.retention_status='bounded_snapshots_closed'
                } catch {$receipt.retention_status='snapshot_failure_unsafe_paths_withheld';$receipt.snapshot_error_type=$_.Exception.GetType().FullName}
            } else {$receipt.retention_status='withheld_producer_exit_unconfirmed'}
        }
        $receipt.logs=$logs
        $receipt | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $receiptPath -Encoding UTF8
        Write-Output ('docling_failure_trace diagnostic_only=true status='+$receipt.status)
    }
}

# Pure finite log decoder. It performs no token, registry or process operations.
# Completeness is distinct from measured contrasts and confers no admission.
function Test-R17HostedAssessment {
    return ($env:MAESTRO_WINDOWS_DOCLING_QUALIFY -ceq '1' -and $env:GITHUB_ACTIONS -ceq 'true' -and $env:RUNNER_ENVIRONMENT -ceq 'github-hosted')
}

function ConvertFrom-R17RegistryAssessmentLog([string]$Body) {
    if ([Text.Encoding]::UTF8.GetByteCount($Body) -gt 4194304) { throw 'R17 assessment log exceeds its finite bound.' }
    $fixtures=@('owner_only','capability_read','capability_broad','foreign_capability','integrity_pair')
    $rights=@('query_value','enumerate_sub_keys','set_value','create_sub_key','delete','write_dac')
    $phases=@('first','second')
    $definition='R17_REGISTRY_DEFINITION version=maestro-r17-registry-assessment-v1 sha256=024fcb4fe0a6d92eaff4a933c5d25dc7c9fb7a63a68b778839c79458fd50be8e'
    $rowPattern='\AR17_REGISTRY_OBSERVATION phase=(first|second) fixture_enum=(owner_only|capability_read|capability_broad|foreign_capability|integrity_pair) right_enum=(query_value|enumerate_sub_keys|set_value|create_sub_key|delete|write_dac) win32_status=(0|[1-9][0-9]{0,9}) sentinel_match=(true|false)\z'
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    $rows=[Collections.Generic.List[object]]::new()
    $lookup=@{}; $definitionCount=0; $cleanupCount=0
    foreach ($rawLine in $Body.Split("`n")) {
        $line=$rawLine
        if ($line.EndsWith("`r")) { $line=$line.Substring(0,$line.Length-1) }
        if (-not $line.Contains('R17_REGISTRY_')) { continue }
        if ($line -ceq $definition) { $definitionCount++; continue }
        if ($line -ceq 'R17_REGISTRY_CLEANUP state=complete') { $cleanupCount++; continue }
        $match=[regex]::Match($line,$rowPattern)
        if (-not $match.Success) { throw 'R17 assessment contains an unknown, diagnostic or malformed protocol line.' }
        [uint32]$status=0
        if (-not [uint32]::TryParse($match.Groups[4].Value,[Globalization.NumberStyles]::None,[Globalization.CultureInfo]::InvariantCulture,[ref]$status)) { throw 'R17 assessment status is outside unsigned LSTATUS.' }
        $phase=$match.Groups[1].Value; $fixture=$match.Groups[2].Value; $right=$match.Groups[3].Value
        $sentinelMatch=$match.Groups[5].Value -ceq 'true'
        $key=$phase+'/'+$fixture+'/'+$right
        if (-not $seen.Add($key) -or $sentinelMatch -ne ($right -ceq 'query_value' -and $status -eq 0)) { throw 'R17 assessment has a duplicate row or invalid sentinel observation.' }
        $row=[ordered]@{phase=$phase;fixture_enum=$fixture;right_enum=$right;win32_status=$status;sentinel_match=$sentinelMatch}
        $rows.Add($row); $lookup[$key]=$row
    }
    if ($definitionCount -ne 1 -or $cleanupCount -ne 1 -or $rows.Count -ne 60) { throw 'R17 assessment requires one exact definition, terminal cleanup and 60 observations.' }
    foreach ($phase in $phases) { foreach ($fixture in $fixtures) { foreach ($right in $rights) {
        if (-not $seen.Contains($phase+'/'+$fixture+'/'+$right)) { throw 'R17 assessment matrix is incomplete.' }
    } } }
    $contrasts=[Collections.Generic.List[object]]::new()
    $discriminating=$false
    foreach ($phase in $phases) {
        $readEligible=$lookup[$phase+'/capability_read/query_value'].win32_status -eq 0
        foreach ($comparison in @('owner_only','foreign_capability')) {
            $differs=$readEligible -ne ($lookup[$phase+'/'+$comparison+'/query_value'].win32_status -eq 0)
            $contrasts.Add([ordered]@{phase=$phase;right_enum='query_value';comparison_enum=$comparison+'_vs_capability_read';eligibility_differs=$differs})
            $discriminating=$discriminating -or $differs
        }
        foreach ($right in @('set_value','create_sub_key','delete','write_dac')) {
            $broadEligible=$lookup[$phase+'/capability_broad/'+$right].win32_status -eq 0
            foreach ($comparison in @('capability_read','integrity_pair')) {
                $differs=$broadEligible -ne ($lookup[$phase+'/'+$comparison+'/'+$right].win32_status -eq 0)
                $contrasts.Add([ordered]@{phase=$phase;right_enum=$right;comparison_enum=$comparison+'_vs_capability_broad';eligibility_differs=$differs})
                $discriminating=$discriminating -or $differs
            }
        }
    }
    return [ordered]@{definition_version='maestro-r17-registry-assessment-v1';definition_sha256='024fcb4fe0a6d92eaff4a933c5d25dc7c9fb7a63a68b778839c79458fd50be8e'
        complete=$true;discriminating=$discriminating;all_denied=(@($rows | Where-Object {$_.win32_status -eq 0}).Count -eq 0)
        phase_ordinals=@([ordered]@{phase='first';ordinal=1},[ordered]@{phase='second';ordinal=2});observations=@($rows.ToArray());contrasts=@($contrasts.ToArray())}
}

function Invoke-DoclingRuntimeProof($Manifest, [string]$ProofRoot, [string]$ExpectedUserSID) {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $admin = [Security.Principal.SecurityIdentifier]::new('S-1-5-32-544')
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    Add-Type -TypeDefinition 'using System; using System.Runtime.InteropServices; public static class DoclingToken { [DllImport("advapi32.dll", SetLastError=true)] public static extern bool GetTokenInformation(IntPtr token, int kind, out uint value, uint length, out uint returned); }'
    [uint32]$elevation=0; [uint32]$returned=0
    if (-not [DoclingToken]::GetTokenInformation($identity.Token,20,[ref]$elevation,4,[ref]$returned) -or $returned -ne 4 -or $elevation -ne 0 -or
        $identity.User.Value -cne $ExpectedUserSID -or $principal.IsInRole($admin) -or @($identity.Groups | Where-Object { $_.Value -eq $admin.Value }).Count -ne 0) { throw 'Docling requires the actual intended ordinary non-admin account.' }
    foreach ($root in @($Manifest.public_root,$Manifest.source_root,(Split-Path -Parent (Split-Path -Parent $Manifest.sdk_go)),$Manifest.module_cache,$Manifest.raw_inputs)) {
        $acl=Get-Acl -LiteralPath $root
        foreach($ace in $acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier])) {
            $private = $ace.IdentityReference.Value -cin @('S-1-5-18','S-1-5-32-544')
            if (-not $private -and $ace.AccessControlType -eq 'Allow' -and ($ace.FileSystemRights -band ([Security.AccessControl.FileSystemRights]::Write -bor [Security.AccessControl.FileSystemRights]::Delete -bor [Security.AccessControl.FileSystemRights]::DeleteSubdirectoriesAndFiles -bor [Security.AccessControl.FileSystemRights]::ChangePermissions -bor [Security.AccessControl.FileSystemRights]::TakeOwnership)) -ne 0) { throw 'Docling public source, tools and inputs must be read-only to the ordinary account.' }
        }
    }
    $env:GOROOT=Split-Path -Parent (Split-Path -Parent $Manifest.sdk_go)
    $env:PATH=(Split-Path -Parent $Manifest.sdk_go)+';'+(Join-Path $env:SystemRoot 'System32')
    $env:GOMODCACHE=$Manifest.module_cache
    $env:GOCACHE=Join-Path $ProofRoot 'go-cache'
    $env:GOPATH=Join-Path $ProofRoot 'go-path'
    $env:GOTOOLCHAIN='local'; $env:GOPROXY='off'; $env:GOSUMDB='off'; $env:GOFLAGS='-mod=readonly -buildvcs=false'; $env:CGO_ENABLED='0'
    $env:MAESTRO_WINDOWS_DOCLING_QUALIFY='1'; $env:MAESTRO_WINDOWS_DOCLING_TEST_INPUTS=$Manifest.raw_inputs; $env:MAESTRO_WINDOWS_DOCLING_TEST_PROFILE=$Manifest.runtime_profile
    # The existing credentialed launcher inherits these hosted markers. They
    # prevent accidental local fixture writes; they are not host attestation.
    $hostedAssessment=Test-R17HostedAssessment
    $testImageSHA256=(Get-FileHash -LiteralPath $Manifest.test_binary -Algorithm SHA256).Hash.ToLowerInvariant()
    $pattern='^(' + ($script:DoclingRuntimeTests -join '|') + ')$'
    Push-Location -LiteralPath $Manifest.source_root
    try { $output=@(& $Manifest.test_binary '-test.run' $pattern '-test.v' '-test.timeout=2400s' 2>&1 | ForEach-Object { "$_" }); $exit=$LASTEXITCODE } finally { Pop-Location }
    $body=$output -join "`n"
    if ([Text.Encoding]::UTF8.GetByteCount($body) -gt 4194304) { throw 'Docling native evidence exceeds its 4 MiB bound.' }
    $log=Join-Path $ProofRoot 'docling-native-tests.log'
    $output | Set-Content -LiteralPath $log -Encoding UTF8
    $output | Write-Output
    if ($exit -ne 0 -or $body -match '--- (FAIL|SKIP):') {
        # This one-trial proof retains the original failure, never a native replay.
        throw 'Actual ordinary-account Docling native coverage failed or skipped.'
    }
    foreach($test in $script:DoclingRuntimeTests) { if($body -notmatch ('(?m)^--- PASS: '+[regex]::Escape($test)+' ')){throw 'A required Docling native test did not actually pass.'} }
    foreach($format in @('sample.docx','literal.txt','structured.md','quoted.csv','generic.json','generic.xml','page.html','page.htm','spine.epub','legacy.xls','sheets.xlsx','native.pdf','native.pptx','interactive.pdf','nontext.pdf','scanned.pdf','mixed-image.pdf','mixed.pdf','protected.pdf','pptx_alternate_presentation','pptx_duplicate_slide_selection','pptx_external_relationship','pptx_macro_presentation','pptx_metafile','pptx_macro_payload','pptx_DTD','pptx_slide_type_confusion','pptx_root_decoy','validated_export','installed_tamper','missing_native_runtime')) {
        if($body -notmatch ('(?m)^    --- PASS: TestWindowsRealAuthenticatedDocling/'+[regex]::Escape($format)+' ')){throw 'A required real Docling format did not actually pass.'}
    }
    $packMatches = [regex]::Matches($body, '(?m)^    native_windows_test.go:[0-9]+: maestro_native_runtime_pack_sha256=([a-f0-9]{64})\r?$')
    if ($packMatches.Count -ne 1) { throw 'Exactly one actual converter pack identity is required.' }
    $profileMatches = [regex]::Matches($body, '(?m)^    native_windows_test.go:[0-9]+: maestro_native_runtime_profile=model-free-documents-v1 lock_sha256=38b1a0257c3feec8a5b4b97856e7f2096cdb72ed2bd1734d8e61567bf4757643\r?$')
    if ($profileMatches.Count -ne 1) { throw 'Actual complete document profile/lock observation is required.' }
    $assessment=$null
    if ($hostedAssessment) {
        $assessment=ConvertFrom-R17RegistryAssessmentLog $body
        if ((Get-FileHash -LiteralPath $Manifest.test_binary -Algorithm SHA256).Hash.ToLowerInvariant() -cne $testImageSHA256) { throw 'Sealed R17 test image changed during its original proof.' }
    } elseif ($body.Contains('R17_REGISTRY_')) { throw 'Unexpected R17 assessment protocol outside the hosted qualification scope.' }
    $sidHash=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($identity.User.Value))).ToLowerInvariant()
    $proof=[ordered]@{schema_version=1; qualification_profile=$Manifest.qualification_profile; runtime_profile=$Manifest.runtime_profile; runtime_lock_sha256=$Manifest.runtime_lock_sha256; source_revision=$Manifest.source_revision; inventory_sha256=$Manifest.inventory_sha256; test_binary_sha256=(Get-FileHash -LiteralPath $Manifest.test_binary -Algorithm SHA256).Hash.ToLowerInvariant(); effective_sid_sha256=$sidHash; actual_standard_user=$true; elevated=$false; admin_member=$false; passed_tests=$script:DoclingRuntimeTests; real_formats=13; pdf_refusals=6; pptx_refusals=9; runtime_pack_sha256=$packMatches[0].Groups[1].Value; log_sha256=(Get-FileHash -LiteralPath $log -Algorithm SHA256).Hash.ToLowerInvariant(); shared_implementation_attested=$true; mcp_server_execution='not_attested'; native_client='not_run'; publisher_authority='not_granted'}
    $proof | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $ProofRoot 'docling-native-proof.json') -Encoding UTF8
    if (-not $hostedAssessment) { return }
    $registryReceipt=[ordered]@{schema_version=1;assessment=$assessment;qualification=$false;policy_decision='RGCI';qualification_profile=$Manifest.qualification_profile
        runtime_profile=$Manifest.runtime_profile;runtime_lock_sha256=$Manifest.runtime_lock_sha256;source_revision=$Manifest.source_revision;source_tree=$Manifest.source_tree
        inventory_sha256=$Manifest.inventory_sha256;test_binary_sha256=$testImageSHA256;original_log_sha256=$proof.log_sha256;runtime_pack_sha256=$packMatches[0].Groups[1].Value
        original_native_parents=$script:DoclingRuntimeTests;real_formats=13;pdf_refusals=6;pptx_refusals=9;original_lpac_access_mask=6;original_enabled_capability='registryRead'
        fixture_cleanup='complete';fixture_custody='cooperative';same_user_atomic_custody=$false;handle_eligibility_only=$true;registry_writes_executed_by_child=$false
        real_profile_authority='not_granted';export_admission='not_granted';publisher_authority='not_granted';native_client='not_run'}
    $registryReceipt | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $ProofRoot 'docling-registry-assessment.json') -Encoding UTF8
}
