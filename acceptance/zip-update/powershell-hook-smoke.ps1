param(
    [Parameter(Mandatory = $true)]
    [string]$MaestroRoot,

    [string]$SettingsPath
)

$ErrorActionPreference = 'Stop'
$script:Passed = 0
$script:Failed = 0

function Pass([string]$Message) {
    $script:Passed++
    Write-Output "PASS  $Message"
}

function Fail([string]$Message) {
    $script:Failed++
    Write-Output "FAIL  $Message"
}

function Assert-True([bool]$Condition, [string]$Message) {
    if ($Condition) { Pass $Message } else { Fail $Message }
}

function Invoke-Hook([string]$HookName, [string]$ProjectDir, [string]$InputJson = '') {
    $hook = Join-Path $script:ContentRoot ".claude/hooks/$HookName"
    $engine = (Get-Process -Id $PID).Path
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $engine
    $psi.Arguments = "-NoLogo -NoProfile -ExecutionPolicy Bypass -File `"$hook`""
    $psi.UseShellExecute = $false
    $psi.RedirectStandardInput = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.CreateNoWindow = $true
    $psi.EnvironmentVariables['CLAUDE_PROJECT_DIR'] = $ProjectDir
    $psi.EnvironmentVariables['CLAUDE_SESSION_ID'] = 'powershell-smoke-session'
    $psi.EnvironmentVariables['MAESTRO_STATE_DIR'] = (Join-Path $ProjectDir '.state')
    $process = New-Object System.Diagnostics.Process
    $process.StartInfo = $psi
    [void]$process.Start()
    if ($InputJson) { $process.StandardInput.Write($InputJson) }
    $process.StandardInput.Close()
    $stdout = $process.StandardOutput.ReadToEnd()
    $stderr = $process.StandardError.ReadToEnd()
    $process.WaitForExit()
    return [pscustomobject]@{
        ExitCode = $process.ExitCode
        Stdout = $stdout
        Stderr = $stderr
    }
}

$root = (Resolve-Path -LiteralPath $MaestroRoot).Path
$contentRoot = $root
$bundlesRoot = Join-Path $root 'bundles'
if (Test-Path -LiteralPath (Join-Path $root 'installers/zip/user-template') -PathType Container) {
    $contentRoot = Join-Path $root 'installers/zip/user-template'
    $bundlesRoot = Join-Path $root 'bundles'
}
$script:ContentRoot = $contentRoot
if (-not $SettingsPath) {
    $SettingsPath = Join-Path $contentRoot '.claude/settings.windows-powershell.json'
}

$hookNames = @(
    'first-run-scaffold.ps1',
    'session-start-memory-inject.ps1',
    'context-inject-userprompt.ps1',
    'block-cross-case-writes.ps1',
    'announce-agent-dispatch.ps1',
    'session-stop-dream.ps1'
)

foreach ($hookName in $hookNames) {
    Assert-True (Test-Path -LiteralPath (Join-Path $contentRoot ".claude/hooks/$hookName") -PathType Leaf) "$hookName exists"
}

if (Test-Path -LiteralPath $SettingsPath -PathType Leaf) {
    try {
        $settings = Get-Content -LiteralPath $SettingsPath -Raw -Encoding UTF8 | ConvertFrom-Json
        $handlers = @()
        foreach ($eventName in @('SessionStart', 'UserPromptSubmit', 'PreToolUse', 'Stop')) {
            foreach ($group in @($settings.hooks.$eventName)) {
                foreach ($handler in @($group.hooks)) { $handlers += $handler }
            }
        }
        Assert-True ($handlers.Count -eq 6) 'Windows settings wire exactly six hook handlers'
        Assert-True (@($handlers | Where-Object { $_.shell -ne 'powershell' }).Count -eq 0) 'every Windows hook explicitly selects PowerShell'
        Assert-True (@($handlers | Where-Object { $_.command -match '(?i)bash|\.sh(?:\"|$)' }).Count -eq 0) 'Windows settings contain no Bash hook command'
        Assert-True (@($handlers | Where-Object { $_.command -match '\.ps1' }).Count -eq 6) 'every Windows hook invokes a ps1 implementation'
    } catch {
        Fail "Windows settings parse: $($_.Exception.Message)"
    }
} else {
    Fail 'Windows PowerShell settings file exists'
}

$scratchParent = Join-Path ([System.IO.Path]::GetTempPath()) 'maestro-powershell-smoke'
if (Test-Path -LiteralPath $scratchParent) { Remove-Item -LiteralPath $scratchParent -Recurse -Force }
$project = Join-Path $scratchParent 'Maestro PowerShell Space'
New-Item -ItemType Directory -Path $project -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $contentRoot '.claude') -Destination $project -Recurse
Copy-Item -LiteralPath $bundlesRoot -Destination $project -Recurse
Set-Content -LiteralPath (Join-Path $project 'VERSION') -Value '0.1.12' -Encoding UTF8

$scaffold = Invoke-Hook 'first-run-scaffold.ps1' $project
Assert-True ($scaffold.ExitCode -eq 0) 'scaffold exits zero'
Assert-True (Test-Path -LiteralPath (Join-Path $project 'data/.initialized')) 'scaffold creates data/.initialized'
Assert-True (Test-Path -LiteralPath (Join-Path $project 'data/owner/registry.json')) 'scaffold creates owner registry'
Assert-True (Test-Path -LiteralPath (Join-Path $project 'data/memory/.schema-version')) 'scaffold creates memory schema marker'
$sentinel = Join-Path $project 'data/owner/observations/owner-sentinel.txt'
Set-Content -LiteralPath $sentinel -Value 'preserve-me' -Encoding UTF8
$scaffoldAgain = Invoke-Hook 'first-run-scaffold.ps1' $project
Assert-True (($scaffoldAgain.ExitCode -eq 0) -and ((Get-Content -LiteralPath $sentinel -Raw).Trim() -eq 'preserve-me')) 'scaffold is idempotent and preserves owner data'

$session = Invoke-Hook 'session-start-memory-inject.ps1' $project
Assert-True ($session.ExitCode -eq 0) 'SessionStart hook exits zero'
Assert-True ($session.Stdout -match 'maestro:session-context:start') 'SessionStart emits the context envelope'
Assert-True ($session.Stdout -match 'operacional') 'SessionStart emits the operator pointer'

$prompt = '{"prompt":"chama o yoda para revisar essa recomendacao"}'
$route = Invoke-Hook 'context-inject-userprompt.ps1' $project $prompt
Assert-True ($route.ExitCode -eq 0) 'UserPromptSubmit hook exits zero'
Assert-True (($route.Stdout -match 'maestro:agent-route') -and ($route.Stdout -match 'yoda')) 'UserPromptSubmit routes an explicit Yoda request'
$ordinary = Invoke-Hook 'context-inject-userprompt.ps1' $project '{"prompt":"organize esta lista em tres bullets"}'
Assert-True ($ordinary.Stdout -notmatch 'maestro:agent-route') 'ordinary prompt does not route an agent'

$cases = Join-Path $project 'data/cases'
New-Item -ItemType Directory -Path (Join-Path $cases 'case-alpha'), (Join-Path $cases 'case-beta') -Force | Out-Null
Set-Content -LiteralPath (Join-Path $cases '.active') -Value 'case-alpha' -Encoding UTF8
$samePath = Join-Path $cases 'case-alpha/notes.md'
$otherPath = Join-Path $cases 'case-beta/notes.md'
$samePayload = @{tool_name='Write'; tool_input=@{file_path=$samePath}} | ConvertTo-Json -Compress
$otherPayload = @{tool_name='Write'; tool_input=@{file_path=$otherPath}} | ConvertTo-Json -Compress
$same = Invoke-Hook 'block-cross-case-writes.ps1' $project $samePayload
$other = Invoke-Hook 'block-cross-case-writes.ps1' $project $otherPayload
Assert-True ($same.ExitCode -eq 0) 'same-case write is allowed'
Assert-True (($other.ExitCode -eq 2) -and ($other.Stderr -match 'Cross-case write blocked')) 'cross-case write is blocked with a reason'
$outsidePath = Join-Path $project 'notes-outside-cases.md'
$outsidePayload = @{tool_name='Write'; tool_input=@{file_path=$outsidePath}} | ConvertTo-Json -Compress
$outside = Invoke-Hook 'block-cross-case-writes.ps1' $project $outsidePayload
Assert-True ($outside.ExitCode -eq 0) 'write outside the cases tree is allowed'
$pendingFile = Join-Path $cases '.pending'
Set-Content -LiteralPath $pendingFile -Value 'case-beta' -Encoding UTF8
$pending = Invoke-Hook 'block-cross-case-writes.ps1' $project $otherPayload
Assert-True ($pending.ExitCode -eq 0) 'confirmed pending case write is allowed'
Remove-Item -LiteralPath $pendingFile -Force
$traversal = Join-Path $cases 'case-alpha/../case-beta/traversal.md'
$traversalPayload = @{tool_name='Write'; tool_input=@{file_path=$traversal}} | ConvertTo-Json -Compress
$traversalResult = Invoke-Hook 'block-cross-case-writes.ps1' $project $traversalPayload
Assert-True ($traversalResult.ExitCode -eq 2) 'dot-dot traversal into another case is blocked'
if ($env:OS -eq 'Windows_NT') {
    $junction = Join-Path $cases 'case-alpha/junction-to-beta'
    New-Item -ItemType Junction -Path $junction -Target (Join-Path $cases 'case-beta') -Force | Out-Null
    $junctionPayload = @{tool_name='Write'; tool_input=@{file_path=(Join-Path $junction 'escape.md')}} | ConvertTo-Json -Compress
    $junctionResult = Invoke-Hook 'block-cross-case-writes.ps1' $project $junctionPayload
    Assert-True ($junctionResult.ExitCode -eq 2) 'junction from active case into another case is blocked'
    Remove-Item -LiteralPath $junction -Force
}
Remove-Item -LiteralPath (Join-Path $cases '.active') -Force
$missingActive = Invoke-Hook 'block-cross-case-writes.ps1' $project $samePayload
Assert-True ($missingActive.ExitCode -eq 2) 'case write without an active case fails closed'

$announcePayload = @{tool_name='Agent'; tool_input=@{subagent_type='yoda'; prompt='x'}} | ConvertTo-Json -Compress
$announce = Invoke-Hook 'announce-agent-dispatch.ps1' $project $announcePayload
$announceJson = $null
try { $announceJson = $announce.Stdout | ConvertFrom-Json } catch {}
Assert-True ($announce.ExitCode -eq 0) 'agent announcement hook exits zero'
Assert-True (($null -ne $announceJson) -and ($announceJson.hookSpecificOutput.additionalContext -match 'yoda')) 'agent announcement returns additionalContext JSON'
$nonAgent = Invoke-Hook 'announce-agent-dispatch.ps1' $project (@{tool_name='Read'; tool_input=@{file_path='x'}} | ConvertTo-Json -Compress)
Assert-True (($nonAgent.ExitCode -eq 0) -and (-not $nonAgent.Stdout)) 'announcement hook ignores non-Agent tools'

$dream = Invoke-Hook 'session-stop-dream.ps1' $project
Assert-True ($dream.ExitCode -eq 0) 'Stop hook exits zero'
Assert-True (Test-Path -LiteralPath (Join-Path $project 'data/memory/.dream-requested')) 'Stop hook writes the dream marker'

Write-Output ""
Write-Output "Summary: $script:Passed pass, $script:Failed fail"
if ($script:Failed -ne 0) { exit 1 }
exit 0
