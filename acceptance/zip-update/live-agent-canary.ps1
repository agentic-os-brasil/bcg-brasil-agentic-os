param(
    [Parameter(Mandatory = $true)][string]$Zip,
    [Parameter(Mandatory = $true)][string]$Trace,
    [Parameter(Mandatory = $true)][string]$Receipt
)

$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT') { throw 'This live canary must run on native Windows PowerShell.' }
if (Test-Path -LiteralPath $Trace) { throw "Trace already exists: $Trace" }
if (Test-Path -LiteralPath $Receipt) { throw "Receipt already exists: $Receipt" }
if (-not (Get-Command claude -ErrorAction SilentlyContinue)) { throw 'Claude Code is unavailable' }
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$sourceCommit = ((& git -C $repo rev-parse HEAD 2>$null) | Out-String).Trim()
$sourceStatus = ((& git -C $repo status --porcelain 2>$null) | Out-String).Trim()
if (-not $sourceCommit) { throw 'Source commit is unavailable' }
if ($sourceStatus) { throw 'Source tree is dirty; commit and freeze the candidate before issuing a receipt' }

function Property($Object, [string]$Name) {
    if ($null -eq $Object) { return $null }
    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property) { return $null }
    return $property.Value
}

function Blocks($Event) {
    $message = Property $Event 'message'
    $content = Property $message 'content'
    if ($null -eq $content) { return @() }
    return @($content)
}

function Text-From($Value) {
    if ($null -eq $Value) { return '' }
    if ($Value -is [string]) { return $Value }
    if ($Value -is [System.Collections.IEnumerable] -and -not ($Value -is [pscustomobject])) {
        return ((@($Value) | ForEach-Object { Text-From $_ }) -join "`n")
    }
    if ($Value -is [pscustomobject]) {
        if ((Property $Value 'type') -eq 'text') { return [string](Property $Value 'text') }
        return (($Value.PSObject.Properties | ForEach-Object { Text-From $_.Value }) -join "`n")
    }
    return ''
}

function Has-ExactLine($Value, [string]$Token) {
    foreach ($line in ((Text-From $Value) -split "`r?`n")) {
        if ($line.Trim() -eq $Token) { return $true }
    }
    return $false
}

$zipPath = (Resolve-Path -LiteralPath $Zip).Path
$tracePath = [System.IO.Path]::GetFullPath($Trace)
$receiptPath = [System.IO.Path]::GetFullPath($Receipt)
$scratch = Join-Path ([System.IO.Path]::GetTempPath()) ("maestro-live-agent-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $scratch -Force | Out-Null
try {
    Expand-Archive -LiteralPath $zipPath -DestinationPath $scratch -Force
    $prompt = 'Valida este canario com duas chamadas reais e sequenciais da ferramenta Agent. Primeiro chama Darwin e pede apenas CANARIO_DARWIN_OK. Aguarda o retorno real. Depois chama Yoda e pede apenas CANARIO_YODA_OK. Nao simule, substitua ou paralelize as chamadas. Responde CANARIO_HUB_OK somente depois de receber os dois retornos, nessa ordem.'
    Push-Location (Join-Path $scratch 'Maestro')
    try {
        & claude -p `
            --setting-sources project `
            --output-format stream-json `
            --verbose `
            --include-hook-events `
            --forward-subagent-text `
            --no-session-persistence `
            --model opus `
            --effort xhigh `
            --max-budget-usd 1.00 `
            --permission-mode dontAsk `
            --allowedTools=Agent `
            -- $prompt | Set-Content -LiteralPath $tracePath -Encoding UTF8
        $claudeRc = $LASTEXITCODE
    } finally {
        Pop-Location
    }

    $events = @()
    foreach ($line in Get-Content -LiteralPath $tracePath -Encoding UTF8) {
        try { $events += ($line | ConvertFrom-Json) } catch {}
    }

    $sessionStart = $false
    $routeHook = $false
    $agentCalls = @{ darwin = @(); yoda = @() }
    $allAgentCalls = @()
    for ($index = 0; $index -lt $events.Count; $index++) {
        $event = $events[$index]
        if ((Property $event 'type') -eq 'system' -and (Property $event 'subtype') -eq 'hook_response') {
            $hookText = ([string](Property $event 'output')) + "`n" + ([string](Property $event 'stdout'))
            if ((Property $event 'hook_event') -eq 'SessionStart' -and (Property $event 'outcome') -ne 'error' -and [int](Property $event 'exit_code') -eq 0) { $sessionStart = $true }
            if ((Property $event 'hook_event') -eq 'UserPromptSubmit' -and (Property $event 'outcome') -ne 'error' -and [int](Property $event 'exit_code') -eq 0 -and $hookText.Contains('<!-- maestro:agent-route -->') -and $hookText.Contains('`darwin`') -and $hookText.Contains('`yoda`')) { $routeHook = $true }
        }
        if ((Property $event 'type') -eq 'assistant') {
            foreach ($block in Blocks $event) {
                $inputObject = Property $block 'input'
                $subagentType = [string](Property $inputObject 'subagent_type')
                if ((Property $block 'type') -eq 'tool_use' -and (Property $block 'name') -eq 'Agent') {
                    $allAgentCalls += [pscustomobject]@{ Index=$index; Id=[string](Property $block 'id'); SubagentType=$subagentType }
                }
                if ((Property $block 'type') -eq 'tool_use' -and (Property $block 'name') -eq 'Agent' -and $agentCalls.ContainsKey($subagentType)) {
                    $agentCalls[$subagentType] += [pscustomobject]@{ Index=$index; Id=[string](Property $block 'id') }
                }
            }
        }
    }

    $agentIds = [ordered]@{ darwin = $null; yoda = $null }
    $callIndexes = @{ darwin = -1; yoda = -1 }
    $resultIndexes = @{ darwin = -1; yoda = -1 }
    $agentReturns = @{ darwin = $false; yoda = $false }
    $pretoolHooks = @{ darwin = $false; yoda = $false }
    $tokens = @{ darwin = 'CANARIO_DARWIN_OK'; yoda = 'CANARIO_YODA_OK' }
    foreach ($agent in @('darwin', 'yoda')) {
        if ($agentCalls[$agent].Count -eq 1) {
            $agentIds[$agent] = $agentCalls[$agent][0].Id
            $callIndexes[$agent] = $agentCalls[$agent][0].Index
        }
        $agentId = $agentIds[$agent]
        if ($agentId) {
            for ($index = 0; $index -lt $events.Count; $index++) {
                $event = $events[$index]
                if ((Property $event 'type') -eq 'assistant' -and (Property $event 'parent_tool_use_id') -eq $agentId -and (Has-ExactLine (Blocks $event) $tokens[$agent])) {
                    $agentReturns[$agent] = $true
                    $resultIndexes[$agent] = [Math]::Max($resultIndexes[$agent], $index)
                }
                if ((Property $event 'type') -eq 'user') {
                    foreach ($block in Blocks $event) {
                        if ((Property $block 'type') -eq 'tool_result' -and (Property $block 'tool_use_id') -eq $agentId) {
                            $resultIndexes[$agent] = [Math]::Max($resultIndexes[$agent], $index)
                            if (Has-ExactLine (Property $block 'content') $tokens[$agent]) { $agentReturns[$agent] = $true }
                        }
                    }
                }
            }
        }
        if ($callIndexes[$agent] -ge 0) {
            $upper = $(if ($resultIndexes[$agent] -ge 0) { $resultIndexes[$agent] } else { $events.Count - 1 })
            for ($index = $callIndexes[$agent] + 1; $index -le $upper; $index++) {
                $event = $events[$index]
                if ((Property $event 'type') -eq 'system' -and (Property $event 'subtype') -eq 'hook_response' -and (Property $event 'hook_event') -eq 'PreToolUse' -and (Property $event 'outcome') -ne 'error' -and [int](Property $event 'exit_code') -eq 0) {
                    $hookText = ([string](Property $event 'output')) + "`n" + ([string](Property $event 'stdout'))
                    if ($hookText -match [regex]::Escape($agent)) { $pretoolHooks[$agent] = $true }
                }
            }
        }
    }

    $darwinBeforeYoda = ($resultIndexes['darwin'] -ge 0 -and $callIndexes['yoda'] -gt $resultIndexes['darwin'])
    $hubReturn = $false
    if ($resultIndexes['yoda'] -ge 0) {
        for ($index = $resultIndexes['yoda'] + 1; $index -lt $events.Count; $index++) {
            $event = $events[$index]
            if ((Property $event 'type') -eq 'assistant' -and -not (Property $event 'parent_tool_use_id') -and (Has-ExactLine (Blocks $event) 'CANARIO_HUB_OK')) { $hubReturn = $true }
        }
    }

    $checks = [ordered]@{
        claude_exit_zero = ($claudeRc -eq 0)
        session_start_hook_succeeded = $sessionStart
        user_prompt_route_hook_returned_darwin_and_yoda = $routeHook
        exactly_two_agent_tool_calls_total = ($allAgentCalls.Count -eq 2)
        exactly_one_agent_tool_darwin_observed = ($agentCalls['darwin'].Count -eq 1)
        darwin_pretool_hook_correlated = $pretoolHooks['darwin']
        darwin_return_correlated_to_tool_use = $agentReturns['darwin']
        yoda_called_after_darwin_result = $darwinBeforeYoda
        exactly_one_agent_tool_yoda_observed = ($agentCalls['yoda'].Count -eq 1)
        yoda_pretool_hook_correlated = $pretoolHooks['yoda']
        yoda_return_correlated_to_tool_use = $agentReturns['yoda']
        hub_return_after_yoda_result = $hubReturn
    }
    $allPassed = -not (@($checks.Values | Where-Object { -not $_ }).Count)
    $receiptObject = [ordered]@{
        schema_version = 1
        evidence_kind = 'maestro_claude_agent_live'
        platform = 'Windows'
        architecture = $env:PROCESSOR_ARCHITECTURE
        powershell_version = $PSVersionTable.PSVersion.ToString()
        release_sha256 = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
        source_commit = $sourceCommit
        claude_code = ((& claude --version 2>$null | Out-String).Trim())
        agent_tool_use_ids = $agentIds
        checks = $checks
        verdict = $(if ($allPassed) { 'PASS' } else { 'FAIL' })
        limits = @('synthetic prompt only', 'not release publication or signing evidence')
    }
    $receiptObject | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $receiptPath -Encoding UTF8
    Write-Host $receiptObject.verdict
    if (-not $allPassed) { exit 1 }
} finally {
    if (Test-Path -LiteralPath $scratch) { Remove-Item -LiteralPath $scratch -Recurse -Force }
}
