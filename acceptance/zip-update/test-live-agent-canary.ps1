param([Parameter(Mandatory = $true)][string]$Zip)

$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$zipPath = (Resolve-Path -LiteralPath $Zip).Path
$scratch = Join-Path ([System.IO.Path]::GetTempPath()) ("maestro-live-test-" + [Guid]::NewGuid().ToString('N'))
$bin = Join-Path $scratch 'bin'
New-Item -ItemType Directory -Path $bin -Force | Out-Null

try {
    if ($env:OS -eq 'Windows_NT') {
        $fake = Join-Path $bin 'claude.cmd'
        $body = "@echo off`r`nif `"%1`"==`"--version`" (echo claude-fake 1.0& exit /b 0)`r`ntype `"%FAKE_CLAUDE_TRACE%`"`r`n"
        [System.IO.File]::WriteAllText($fake, $body, [System.Text.Encoding]::ASCII)
    } else {
        $fake = Join-Path $bin 'claude'
        $body = "#!/bin/sh`nif [ `"`$1`" = `"--version`" ]; then echo 'claude-fake 1.0'; exit 0; fi`ncat `"`$FAKE_CLAUDE_TRACE`"`n"
        [System.IO.File]::WriteAllText($fake, $body, [System.Text.Encoding]::ASCII)
        & chmod +x $fake
    }
    $originalPath = $env:PATH
    $originalOs = $env:OS
    $env:PATH = $bin + [System.IO.Path]::PathSeparator + $env:PATH
    $env:OS = 'Windows_NT'

    function Run-Canary([string[]]$FixtureLines, [string]$Name) {
        $fixture = Join-Path $scratch "$Name-fixture.jsonl"
        [System.IO.File]::WriteAllLines($fixture, $FixtureLines, (New-Object System.Text.UTF8Encoding($false)))
        $trace = Join-Path $scratch "$Name-trace.jsonl"
        $receipt = Join-Path $scratch "$Name-receipt.json"
        $env:FAKE_CLAUDE_TRACE = $fixture
        $engine = (Get-Process -Id $PID).Path
        $psi = New-Object System.Diagnostics.ProcessStartInfo
        $psi.FileName = $engine
        $psi.Arguments = "-NoLogo -NoProfile -File `"$repo/acceptance/zip-update/live-agent-canary.ps1`" -Zip `"$zipPath`" -Trace `"$trace`" -Receipt `"$receipt`""
        $psi.UseShellExecute = $false
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true
        $process = New-Object System.Diagnostics.Process
        $process.StartInfo = $psi
        [void]$process.Start()
        $stdout = $process.StandardOutput.ReadToEnd()
        $stderr = $process.StandardError.ReadToEnd()
        $process.WaitForExit()
        return [pscustomobject]@{ ExitCode=$process.ExitCode; Stdout=$stdout; Stderr=$stderr; Receipt=$receipt }
    }

    $promptOnly = @(
        '{"type":"user","message":{"content":[{"type":"text","text":"yoda CANARIO_YODA_OK CANARIO_HUB_OK"}]}}',
        '{"type":"assistant","message":{"content":[{"type":"text","text":"CANARIO_YODA_OK\nCANARIO_HUB_OK"}]}}'
    )
    $falsePositive = Run-Canary $promptOnly 'false-positive'
    if ($falsePositive.ExitCode -eq 0) { throw 'Prompt tokens alone produced a false PASS' }
    Write-Host 'PASS  prompt tokens alone cannot pass the PowerShell live evaluator'

    $correlated = @(
        '{"type":"system","subtype":"hook_response","hook_event":"SessionStart","exit_code":0,"outcome":"success"}',
        '{"type":"system","subtype":"hook_response","hook_event":"UserPromptSubmit","exit_code":0,"output":"<!-- maestro:agent-route --> - `yoda`"}',
        '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_yoda","name":"Agent","input":{"subagent_type":"yoda"}}]}}',
        '{"type":"system","subtype":"hook_response","hook_event":"PreToolUse","exit_code":0,"output":"dispatch yoda"}',
        '{"type":"assistant","parent_tool_use_id":"toolu_yoda","message":{"content":[{"type":"text","text":"CANARIO_YODA_OK"}]}}',
        '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_yoda","content":"CANARIO_YODA_OK"}]}}',
        '{"type":"assistant","message":{"content":[{"type":"text","text":"CANARIO_HUB_OK"}]}}'
    )
    $pass = Run-Canary $correlated 'correlated'
    if ($pass.ExitCode -ne 0) { throw "Correlated trace failed: $($pass.Stderr) $($pass.Stdout)" }
    $receiptObject = Get-Content -LiteralPath $pass.Receipt -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($receiptObject.verdict -ne 'PASS' -or $receiptObject.agent_tool_use_ids.yoda -ne 'toolu_yoda') { throw 'Correlated trace receipt is invalid' }
    Write-Host 'PASS  correlated Yoda trace passes the PowerShell live evaluator'

    $routeError = @($correlated)
    $routeError[1] = '{"type":"system","subtype":"hook_response","hook_event":"UserPromptSubmit","exit_code":2,"outcome":"error","output":"<!-- maestro:agent-route --> - `yoda`"}'
    if ((Run-Canary $routeError 'route-hook-error').ExitCode -eq 0) { throw 'A failed UserPromptSubmit hook produced PASS' }
    Write-Host 'PASS  failed UserPromptSubmit hook cannot pass'

    $pretoolError = @($correlated)
    $pretoolError[3] = '{"type":"system","subtype":"hook_response","hook_event":"PreToolUse","exit_code":2,"outcome":"error","output":"dispatch yoda"}'
    if ((Run-Canary $pretoolError 'pretool-hook-error').ExitCode -eq 0) { throw 'A failed Agent PreToolUse hook produced PASS' }
    Write-Host 'PASS  failed Agent PreToolUse hook cannot pass'

    $mismatchedResult = @($correlated)
    $mismatchedResult[4] = '{"type":"assistant","parent_tool_use_id":"toolu_wrong","message":{"content":[{"type":"text","text":"CANARIO_YODA_OK"}]}}'
    $mismatchedResult[5] = '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_wrong","content":"CANARIO_YODA_OK"}]}}'
    if ((Run-Canary $mismatchedResult 'mismatched-result').ExitCode -eq 0) { throw 'An uncorrelated Yoda result produced PASS' }
    Write-Host 'PASS  Yoda result must correlate to the Agent tool-use id'

    $extraAgent = @($correlated[0..($correlated.Count - 2)]) + @(
        '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_extra","name":"Agent","input":{"subagent_type":"gamma-guardian"}}]}}'
    ) + @($correlated[$correlated.Count - 1])
    if ((Run-Canary $extraAgent 'extra-agent').ExitCode -eq 0) { throw 'An extra Agent call produced PASS' }
    Write-Host 'PASS  extra Agent calls cannot pass the one-call contract'
} finally {
    if ($null -ne $originalPath) { $env:PATH = $originalPath }
    if ($null -ne $originalOs) { $env:OS = $originalOs }
    if (Test-Path -LiteralPath $scratch) { Remove-Item -LiteralPath $scratch -Recurse -Force }
}
