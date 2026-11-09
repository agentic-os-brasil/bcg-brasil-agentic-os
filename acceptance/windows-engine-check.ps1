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
    @($report.public_families.required_tests).Count -ne 7 -or @($report.public_families.required_assertions).Count -ne 39 -or
    @($report.public_families.outcomes | Where-Object { $_.outcome -cne 'PASS' }).Count -ne 0 -or
    @($report.public_families.observed_skips).Count -ne 0 -or @($report.unclassified_skips).Count -ne 0 -or
    @($report.original_assertions).Count -ne 39 -or
    @($report.original_assertions | Where-Object { $_.disposition -cnotin @('executed_pass', 'classified_platform_nonapplicability') }).Count -ne 0) {
    throw 'Closed native owner/public assertion coverage is incomplete.'
}
foreach ($entry in @($report.packages) + @($report.adapter_packages) + @($report.authored_brain) + @($report.authored_adapter)) {
    $sealed = @(@($manifest.packages) + @($manifest.adapter_packages) + @($manifest.authored_package) | Where-Object { $_.name -ceq $entry.name })
    if ($sealed.Count -ne 1 -or $entry.sha256 -cne $sealed[0].sha256 -or $entry.package -cne $sealed[0].package -or
        $entry.exit_code -ne 0 -or $entry.inventory_complete -ne $true -or
        $entry.log -cnotmatch '^(engine-(execution|datahome|ownerctx|memory|scheduler|processidentity|winprivate|zipmigration)|adapter-(mcp|mcp-command|setup|authored)|authored-atlas)\.log$') {
        throw 'Owning/adapter report differs from its exact sealed executable inventory.'
    }
    $log = Get-Item -LiteralPath (Join-Path $proofRoot $entry.log) -Force
    if ($log -isnot [IO.FileInfo] -or ($log.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $log.Length -gt 2097152 -or
        (Get-FileHash -LiteralPath $log.FullName -Algorithm SHA256).Hash.ToLowerInvariant() -cne $entry.log_sha256) {
        throw 'Engine log identity changed or exceeded its bound.'
    }
    if ($entry.name -cin $expected) {
        if ($entry.canonical_contract_proof.qualified -ne $true -or
            @((Compare-Object (@($sealed[0].tests) | Sort-Object -CaseSensitive) (@($entry.expected_tests) | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0 -or
            @((Compare-Object (@($sealed[0].tests) | Sort-Object -CaseSensitive) (@($entry.outcomes.test) | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0 -or
            @($entry.outcomes.test | Sort-Object -CaseSensitive -Unique).Count -ne @($sealed[0].tests).Count -or
            @($entry.outcomes | Where-Object { $_.outcome -cnotin @('PASS', 'SKIP') }).Count -ne 0) {
            throw 'An owning capability-pair proof or full observed inventory is incomplete.'
        }
    }
}
if ($report.adapter_profile -cne 'windows_private_engine_adapters_v1' -or @($report.adapter_packages).Count -ne 3 -or
    @((Compare-Object @('mcp', 'mcp-command', 'setup') (@($report.adapter_packages.name) | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0) {
    throw 'All three exact current adapter companions must qualify before host exclusions.'
}
$driver = Join-Path $PSScriptRoot 'windows-core-pilot.ps1'
$sealedDriver = Join-Path $manifest.source_root 'acceptance/windows-core-pilot.ps1'
if ((Get-FileHash -LiteralPath $driver -Algorithm SHA256).Hash -cne (Get-FileHash -LiteralPath $sealedDriver -Algorithm SHA256).Hash) {
    throw 'Host exclusion contract differs from exact committed driver source.'
}
function Read-EngineLiteralArray($Assignment) {
    $arrays = @($Assignment.Right.FindAll({param($node) $node -is [Management.Automation.Language.ArrayExpressionAst]}, $true))
    if ($arrays.Count -ne 1 -or $arrays[0].SubExpression.Statements.Count -ne 1) { throw 'Expected one literal source array.' }
    $pipeline = $arrays[0].SubExpression.Statements[0]
    if ($pipeline -isnot [Management.Automation.Language.PipelineAst] -or $pipeline.PipelineElements.Count -ne 1 -or
        $pipeline.PipelineElements[0] -isnot [Management.Automation.Language.CommandExpressionAst]) { throw 'Source array cannot execute commands.' }
    $expression = $pipeline.PipelineElements[0].Expression
    if ($expression -isnot [Management.Automation.Language.ArrayLiteralAst] -and $expression -isnot [Management.Automation.Language.StringConstantExpressionAst]) {
        throw 'Source array must contain constant test names.'
    }
    $values = @($expression.SafeGetValue())
    if ($values.Count -eq 0 -or @($values | Where-Object { $_ -isnot [string] }).Count -ne 0) { throw 'Source array requires string constants.' }
    return $values
}
$errors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($driver, [ref]$null, [ref]$errors)
if ($errors.Count) { throw 'Host exclusion source could not be parsed.' }
$closed = @{}
foreach ($variable in @('$adapterParents', '$adapterAssertions', '$adapterCommandParents', '$authoredAtlasParents', '$authoredAtlasAssertions', '$authoredAdapterParents', '$authoredAdapterAssertions', '$adapterSetupParents', '$adapterSetupAssertions')) {
    $assignments = @($ast.FindAll({param($node) $node -is [Management.Automation.Language.AssignmentStatementAst] -and $node.Left.Extent.Text -ceq $variable}, $true))
    if ($assignments.Count -ne 1) { throw 'Expected one literal current adapter assertion source.' }
    $closed[$variable] = @(Read-EngineLiteralArray $assignments[0])
}
foreach ($entry in $report.adapter_packages) {
    $parents = switch -CaseSensitive ($entry.name) { 'mcp' { $closed['$adapterParents'] }; 'mcp-command' { $closed['$adapterCommandParents'] }; 'setup' { $closed['$adapterSetupParents'] } }
    $assertions = switch -CaseSensitive ($entry.name) { 'mcp' { $closed['$adapterAssertions'] }; 'mcp-command' { $parents }; 'setup' { $closed['$adapterSetupAssertions'] } }
    if ($entry.qualified -ne $true -or $entry.copied_binary_custody -ne $true -or $entry.proof.qualified -ne $true -or
        @((Compare-Object ($parents | Sort-Object -CaseSensitive) (@($entry.proof.required_tests) | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0 -or
        @((Compare-Object ($assertions | Sort-Object -CaseSensitive) (@($entry.proof.outcomes.test) | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0 -or
        @($entry.proof.outcomes.test | Sort-Object -CaseSensitive -Unique).Count -ne @($assertions).Count -or
        @($entry.proof.outcomes | Where-Object { $_.outcome -cne 'PASS' }).Count -ne 0) {
        throw 'Every exact selected adapter assertion must actually pass without skips or duplicates.'
    }
}
$authored = $report.authored_brain
if ($authored.name -cne 'atlasops' -or $authored.qualified -ne $true -or $authored.copied_binary_custody -ne $true -or $authored.proof.qualified -ne $true -or
    @((Compare-Object ($closed['$authoredAtlasParents'] | Sort-Object -CaseSensitive) (@($authored.proof.required_tests) | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0 -or
    @((Compare-Object ($closed['$authoredAtlasAssertions'] | Sort-Object -CaseSensitive) (@($authored.proof.outcomes.test) | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0 -or
    @($authored.proof.outcomes.test | Sort-Object -CaseSensitive -Unique).Count -ne $closed['$authoredAtlasAssertions'].Count -or
    @($authored.proof.outcomes | Where-Object { $_.outcome -cne 'PASS' }).Count -ne 0 -or @($authored.proof.observed_skips).Count -ne 0) {
    throw 'Both exact authored Atlas families and all custody/recovery assertions must actually pass.'
}
$authoredAdapter = $report.authored_adapter
if ($authoredAdapter.name -cne 'mcp' -or $authoredAdapter.qualified -ne $true -or $authoredAdapter.copied_binary_custody -ne $true -or $authoredAdapter.proof.qualified -ne $true -or
    @((Compare-Object ($closed['$authoredAdapterParents'] | Sort-Object -CaseSensitive) (@($authoredAdapter.proof.required_tests) | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0 -or
    @((Compare-Object ($closed['$authoredAdapterAssertions'] | Sort-Object -CaseSensitive) (@($authoredAdapter.proof.outcomes.test) | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0 -or
    @($authoredAdapter.proof.outcomes.test | Sort-Object -CaseSensitive -Unique).Count -ne $closed['$authoredAdapterAssertions'].Count -or
    @($authoredAdapter.proof.outcomes | Where-Object { $_.outcome -cne 'PASS' }).Count -ne 0 -or @($authoredAdapter.proof.observed_skips).Count -ne 0) {
    throw 'Every authored positive adapter assertion must pass under the ordinary account before its host exclusion.'
}
$exclusions = @($closed['$adapterParents']) + @($closed['$adapterCommandParents']) + @($closed['$authoredAdapterParents']) + @($closed['$adapterSetupParents'])
if (@($exclusions | Where-Object { $_ -cnotmatch '^Test[A-Za-z0-9_]+$' }).Count -ne 0) { throw 'Unsafe host exclusion name.' }
$pattern = '^(' + (($exclusions | ForEach-Object { [regex]::Escape($_) }) -join '|') + ')$'
if ($env:GITHUB_OUTPUT) { ('host_exclusions=' + $pattern) | Out-File -LiteralPath $env:GITHUB_OUTPUT -Encoding utf8 -Append }
Write-Output ('Exact-source ordinary engine proof passed before ' + $exclusions.Count + ' host-positive exclusions.')
