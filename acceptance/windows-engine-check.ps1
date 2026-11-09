# Verify exact-source ordinary-account proof before excluding those same positive
# tests from the elevated host. This reads evidence and changes no account/state.
$ErrorActionPreference = 'Stop'
$publicRoot = $env:MAESTRO_ENGINE_PUBLIC_ROOT
$proofRoot = Join-Path $env:RUNNER_TEMP 'native-engine-proof'
function Read-BoundedEngineJSON([string]$Path) {
    $file = Get-Item -LiteralPath $Path -Force
    if ($file -isnot [IO.FileInfo] -or ($file.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $file.Length -gt 2097152) {
        throw 'Engine evidence must be a bounded ordinary file.'
    }
    return Get-Content -LiteralPath $Path -Raw -Encoding UTF8 | ConvertFrom-Json
}
$manifest = Read-BoundedEngineJSON (Join-Path $publicRoot 'engine-build-manifest.json')
$report = Read-BoundedEngineJSON (Join-Path $proofRoot 'engine-contract-results.json')
$core = Read-BoundedEngineJSON (Join-Path $proofRoot 'native-core-proof.json')
$revision = (git rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $revision -cnotmatch '^[0-9a-f]{40}$' -or $manifest.source_revision -cne $revision -or
    $report.source_revision -cne $revision -or $report.qualification_profile -cne 'windows_private_engine_v1' -or
    $report.actual_standard_user -ne $true -or $report.private_current_user_temp -ne $true -or
    $report.public_source_and_sdk_read_only -ne $true -or $report.qualification_complete -ne $true -or
    $report.sdk_go_sha256 -cne $manifest.sdk_go_sha256 -or $report.public_families.qualified -ne $true -or
    $core.server_sha256 -cne $manifest.public_server_sha256 -or $core.elevated -ne $false -or $core.admin_member -ne $false) {
    throw 'Exact-source ordinary-account engine and shipping proof did not qualify.'
}
$expected = @('execution', 'datahome', 'ownerctx', 'memory', 'scheduler', 'processidentity', 'winprivate', 'zipmigration')
if (@($report.packages).Count -ne 8 -or
    @((Compare-Object ($expected | Sort-Object -CaseSensitive) (@($report.packages.name) | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0 -or
    @($report.public_families.required_tests).Count -ne 4 -or @($report.public_families.required_assertions).Count -ne 21 -or
    @($report.public_families.outcomes | Where-Object { $_.outcome -cne 'PASS' }).Count -ne 0 -or
    @($report.public_families.observed_skips).Count -ne 0 -or @($report.unclassified_skips).Count -ne 0 -or
    @($report.original_assertions | Where-Object { $_.disposition -cnotin @('executed_pass', 'classified_platform_nonapplicability') }).Count -ne 0) {
    throw 'Closed native owner/public assertion coverage is incomplete.'
}
foreach ($entry in @($report.packages) + @($report.adapter_packages)) {
    $sealed = @(@($manifest.packages) + @($manifest.adapter_packages) | Where-Object { $_.name -ceq $entry.name })
    if ($sealed.Count -ne 1 -or $entry.sha256 -cne $sealed[0].sha256 -or $entry.package -cne $sealed[0].package -or
        $entry.exit_code -ne 0 -or $entry.inventory_complete -ne $true -or
        $entry.log -cnotmatch '^(engine-(execution|datahome|ownerctx|memory|scheduler|processidentity|winprivate|zipmigration)|adapter-(mcp|mcp-command))\.log$') {
        throw 'Owning/adapter report differs from its exact sealed executable inventory.'
    }
    $log = Get-Item -LiteralPath (Join-Path $proofRoot $entry.log) -Force
    if ($log -isnot [IO.FileInfo] -or ($log.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $log.Length -gt 2097152 -or
        (Get-FileHash -LiteralPath $log.FullName -Algorithm SHA256).Hash.ToLowerInvariant() -cne $entry.log_sha256) {
        throw 'Engine log identity changed or exceeded its bound.'
    }
    if ($entry.name -cin $expected -and $entry.canonical_contract_proof.qualified -ne $true) { throw 'An owning capability-pair proof is incomplete.' }
}
if ($report.adapter_profile -cne 'windows_private_engine_adapters_v1' -or @($report.adapter_packages).Count -ne 2 -or
    @((Compare-Object @('mcp', 'mcp-command') (@($report.adapter_packages.name) | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0) {
    throw 'Both exact current adapter companions must qualify before host exclusions.'
}
$driver = Join-Path $PSScriptRoot 'windows-core-pilot.ps1'
$sealedDriver = Join-Path $manifest.source_root 'acceptance/windows-core-pilot.ps1'
if ((Get-FileHash -LiteralPath $driver -Algorithm SHA256).Hash -cne (Get-FileHash -LiteralPath $sealedDriver -Algorithm SHA256).Hash) {
    throw 'Host exclusion contract differs from exact committed driver source.'
}
$errors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($driver, [ref]$null, [ref]$errors)
if ($errors.Count) { throw 'Host exclusion source could not be parsed.' }
$closed = @{}
foreach ($variable in @('$adapterParents', '$adapterAssertions', '$adapterCommandParents')) {
    $assignments = @($ast.FindAll({param($node) $node -is [Management.Automation.Language.AssignmentStatementAst] -and $node.Left.Extent.Text -ceq $variable}, $true))
    if ($assignments.Count -ne 1) { throw 'Expected one literal current adapter assertion source.' }
    $closed[$variable] = @($assignments[0].Right.SafeGetValue())
}
foreach ($entry in $report.adapter_packages) {
    $parents = if ($entry.name -ceq 'mcp') { $closed['$adapterParents'] } else { $closed['$adapterCommandParents'] }
    $assertions = if ($entry.name -ceq 'mcp') { $closed['$adapterAssertions'] } else { $parents }
    if ($entry.qualified -ne $true -or $entry.copied_binary_custody -ne $true -or $entry.proof.qualified -ne $true -or
        @((Compare-Object ($parents | Sort-Object -CaseSensitive) (@($entry.proof.required_tests) | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0 -or
        @((Compare-Object ($assertions | Sort-Object -CaseSensitive) (@($entry.proof.outcomes.test) | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0 -or
        @($entry.proof.outcomes.test | Sort-Object -CaseSensitive -Unique).Count -ne @($assertions).Count -or
        @($entry.proof.outcomes | Where-Object { $_.outcome -cne 'PASS' }).Count -ne 0) {
        throw 'Every exact selected adapter assertion must actually pass without skips or duplicates.'
    }
}
$exclusions = @($closed['$adapterParents']) + @($closed['$adapterCommandParents'])
if (@($exclusions | Where-Object { $_ -cnotmatch '^Test[A-Za-z0-9_]+$' }).Count -ne 0) { throw 'Unsafe host exclusion name.' }
$pattern = '^(' + (($exclusions | ForEach-Object { [regex]::Escape($_) }) -join '|') + ')$'
if ($env:GITHUB_OUTPUT) { ('host_exclusions=' + $pattern) | Out-File -LiteralPath $env:GITHUB_OUTPUT -Encoding utf8 -Append }
Write-Output ('Exact-source ordinary engine proof passed before ' + $exclusions.Count + ' host-positive exclusions.')
