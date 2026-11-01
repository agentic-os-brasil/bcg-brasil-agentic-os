# Shared trace classification; this function cannot qualify a runtime or issue a release receipt.
function Get-MaestroAgentTraceEvaluation([object[]]$Events, [int]$ClaudeExitCode) {
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

    $sessionStartHookIds = @()
    $routeHook = $false
    $stopHookIndexes = @()
    $projectInit = $false
    $agentCalls = @()
    $allAgentCalls = @()
    for ($index = 0; $index -lt $events.Count; $index++) {
        $event = $events[$index]
        if ((Property $event 'type') -eq 'system' -and (Property $event 'subtype') -eq 'hook_response') {
            $hookText = ([string](Property $event 'output')) + "`n" + ([string](Property $event 'stdout'))
            if ((Property $event 'hook_event') -eq 'SessionStart' -and (Property $event 'outcome') -ne 'error' -and [int](Property $event 'exit_code') -eq 0) {
                $hookId = [string](Property $event 'hook_id')
                if ($hookId -and $sessionStartHookIds -notcontains $hookId) { $sessionStartHookIds += $hookId }
            }
            if ((Property $event 'hook_event') -eq 'UserPromptSubmit' -and (Property $event 'outcome') -ne 'error' -and [int](Property $event 'exit_code') -eq 0 -and $hookText.Contains('<!-- maestro:agent-route -->') -and $hookText.Contains('`yoda`')) { $routeHook = $true }
            if ((Property $event 'hook_event') -eq 'Stop' -and (Property $event 'outcome') -ne 'error' -and [int](Property $event 'exit_code') -eq 0) { $stopHookIndexes += $index }
        }
        if ((Property $event 'type') -eq 'system' -and (Property $event 'subtype') -eq 'init') {
            $cwd = [string](Property $event 'cwd')
            $agents = @(Property $event 'agents')
            $commands = @(Property $event 'slash_commands')
            $normalizedCwd = $cwd.Replace('\', '/')
            $leaf = [System.IO.Path]::GetFileName($normalizedCwd.TrimEnd('/'))
            $projectInit = ($leaf -eq 'Maestro' -and $agents -contains 'yoda' -and $commands -contains 'maestro-setup-update')
        }
        if ((Property $event 'type') -eq 'assistant') {
            foreach ($block in Blocks $event) {
                $inputObject = Property $block 'input'
                $subagentType = [string](Property $inputObject 'subagent_type')
                if ((Property $block 'type') -eq 'tool_use' -and (Property $block 'name') -eq 'Agent') {
                    $allAgentCalls += [pscustomobject]@{ Index=$index; Id=[string](Property $block 'id'); SubagentType=$subagentType }
                }
                if ((Property $block 'type') -eq 'tool_use' -and (Property $block 'name') -eq 'Agent' -and $subagentType -eq 'yoda') {
                    $agentCalls += [pscustomobject]@{ Index=$index; Id=[string](Property $block 'id') }
                }
            }
        }
    }

    $agentIds = [ordered]@{ yoda = $null }
    $callIndex = -1
    if ($agentCalls.Count -eq 1) {
        $agentIds['yoda'] = $agentCalls[0].Id
        $callIndex = $agentCalls[0].Index
    }
    $agentId = $agentIds['yoda']
    $resultIndex = -1
    $agentReturn = $false
    if ($agentId) {
        for ($index = 0; $index -lt $events.Count; $index++) {
            $event = $events[$index]
            if ((Property $event 'type') -eq 'assistant' -and (Property $event 'parent_tool_use_id') -eq $agentId -and (Has-ExactLine (Blocks $event) 'QUALIFICACAO_YODA_OK')) {
                $agentReturn = $true
                $resultIndex = [Math]::Max($resultIndex, $index)
            }
            if ((Property $event 'type') -eq 'user') {
                foreach ($block in Blocks $event) {
                    if ((Property $block 'type') -eq 'tool_result' -and (Property $block 'tool_use_id') -eq $agentId) {
                        $resultIndex = [Math]::Max($resultIndex, $index)
                        if (Has-ExactLine (Property $block 'content') 'QUALIFICACAO_YODA_OK') { $agentReturn = $true }
                    }
                }
            }
        }
    }
    $pretoolHook = $false
    if ($callIndex -ge 0) {
        $upper = $(if ($resultIndex -ge 0) { $resultIndex } else { $events.Count - 1 })
        for ($index = $callIndex + 1; $index -le $upper; $index++) {
            $event = $events[$index]
            if ((Property $event 'type') -eq 'system' -and (Property $event 'subtype') -eq 'hook_response' -and (Property $event 'hook_event') -eq 'PreToolUse' -and (Property $event 'outcome') -ne 'error' -and [int](Property $event 'exit_code') -eq 0) {
                $hookText = ([string](Property $event 'output')) + "`n" + ([string](Property $event 'stdout'))
                if ($hookText -match 'yoda') { $pretoolHook = $true }
            }
        }
    }

    $hubReturn = $false
    $hubReturnIndex = -1
    if ($resultIndex -ge 0) {
        for ($index = $resultIndex + 1; $index -lt $events.Count; $index++) {
            $event = $events[$index]
            if ((Property $event 'type') -eq 'assistant' -and -not (Property $event 'parent_tool_use_id') -and (Has-ExactLine (Blocks $event) 'QUALIFICACAO_MAESTRO_OK')) { $hubReturn = $true; $hubReturnIndex = $index }
        }
    }

    $checks = [ordered]@{
        claude_exit_zero = ($ClaudeExitCode -eq 0)
        both_session_start_hooks_succeeded = ($sessionStartHookIds.Count -ge 2)
        project_init_loaded_yoda_and_update_contract = $projectInit
        user_prompt_route_hook_returned_yoda = $routeHook
        exactly_one_agent_tool_call_total = ($allAgentCalls.Count -eq 1)
        exactly_one_agent_tool_yoda_observed = ($agentCalls.Count -eq 1)
        yoda_pretool_hook_correlated = $pretoolHook
        yoda_return_correlated_to_tool_use = $agentReturn
        hub_return_after_yoda_result = $hubReturn
        stop_hook_succeeded_after_hub = ($hubReturnIndex -ge 0 -and @($stopHookIndexes | Where-Object { $_ -gt $hubReturnIndex }).Count -gt 0)
    }
    $allPassed = -not (@($checks.Values | Where-Object { -not $_ }).Count)
    return [pscustomobject]@{ Passed=[bool]$allPassed; Checks=$checks; AgentToolUseIds=$agentIds }
}
