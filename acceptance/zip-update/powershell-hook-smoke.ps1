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

function Assert-HookResult([bool]$Condition, [string]$Message, $Result) {
    if ($Condition) {
        Pass $Message
        return
    }
    Fail $Message
    $stdout = ([string]$Result.Stdout).Trim()
    $stderr = ([string]$Result.Stderr).Trim()
    Write-Output "      exit=$($Result.ExitCode)"
    if ($stdout) { Write-Output "      stdout=$stdout" }
    if ($stderr) { Write-Output "      stderr=$stderr" }
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
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    $psi.StandardOutputEncoding = $utf8NoBom
    $psi.StandardErrorEncoding = $utf8NoBom
    $psi.CreateNoWindow = $true
    $psi.EnvironmentVariables['PATH'] = $script:EmptyPath
    $psi.EnvironmentVariables.Remove('MAESTRO_PYTHON')
    $psi.EnvironmentVariables['CLAUDE_PROJECT_DIR'] = $ProjectDir
    $psi.EnvironmentVariables['CLAUDE_SESSION_ID'] = 'powershell-smoke-session'
    $psi.EnvironmentVariables['MAESTRO_STATE_DIR'] = (Join-Path $ProjectDir '.state')
    $process = New-Object System.Diagnostics.Process
    $process.StartInfo = $psi
    [void]$process.Start()
    $stdin = $process.StandardInput.BaseStream
    if ($InputJson) {
        $inputBytes = $utf8NoBom.GetBytes($InputJson)
        $stdin.Write($inputBytes, 0, $inputBytes.Length)
        $stdin.Flush()
    }
    $stdin.Close()
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
    'session-stop-dream.ps1',
    'session-stop-agent-check.ps1',
    'session-stop-brain-index.ps1',
    'session-stop-eod-check.ps1'
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
        Assert-True ($handlers.Count -eq 9) 'Windows settings wire exactly nine hook handlers'
        Assert-True (@($handlers | Where-Object { $_.shell -ne 'powershell' }).Count -eq 0) 'every Windows hook explicitly selects PowerShell'
        Assert-True (@($handlers | Where-Object { $_.command -match '(?i)bash|\.sh(?:\"|$)' }).Count -eq 0) 'Windows settings contain no Bash hook command'
        Assert-True (@($handlers | Where-Object { $_.command -match '\.ps1' }).Count -eq 9) 'every Windows hook invokes a ps1 implementation'
    } catch {
        Fail "Windows settings parse: $($_.Exception.Message)"
    }
} else {
    Fail 'Windows PowerShell settings file exists'
}

$scratchParent = Join-Path ([System.IO.Path]::GetTempPath()) ('maestro-powershell-smoke-' + [Guid]::NewGuid().ToString('N'))
$project = Join-Path $scratchParent ('Maestro PowerShell Space ' + [char]0x00E3)
New-Item -ItemType Directory -Path $project -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $contentRoot '.claude') -Destination $project -Recurse
Copy-Item -LiteralPath $bundlesRoot -Destination $project -Recurse
Set-Content -LiteralPath (Join-Path $project 'VERSION') -Value '0.2.0' -Encoding UTF8

$script:EmptyPath = Join-Path $scratchParent 'empty-path'
New-Item -ItemType Directory -Path $script:EmptyPath | Out-Null
if (Test-Path -LiteralPath (Join-Path $root 'runtime/manifest.json')) {
    Copy-Item -LiteralPath (Join-Path $root 'runtime') -Destination $project -Recurse
} else {
    $go = (Get-Command go -ErrorAction Stop).Source
    $goos = ((& $go env GOOS) | Out-String).Trim()
    $goarch = ((& $go env GOARCH) | Out-String).Trim()
    $exe = 'maestro-runtime'
    if ($goos -eq 'windows') { $exe += '.exe' }
    $rel = "runtime/$goos-$goarch/$exe"
    $bin = Join-Path $project $rel
    New-Item -ItemType Directory -Path (Split-Path -Parent $bin) -Force | Out-Null
    Push-Location $root
    try { & $go build -o $bin ./cmd/maestro-runtime; if ($LASTEXITCODE -ne 0) { throw 'Runtime build failed' } } finally { Pop-Location }
    @{schema_version=1;version='0.2.0';artifacts=@(@{os=$goos;arch=$goarch;path=$rel;sha256=(Get-FileHash -LiteralPath $bin -Algorithm SHA256).Hash.ToLowerInvariant()})} | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $project 'runtime/manifest.json') -Encoding UTF8
}

# The real helper would let a fresh workspace initialize. Invalid binding metadata
# must stop the wrapper before that mutation, even with a valid helper checksum.
foreach ($invalidBinding in @('version-mismatch','version-missing','version-directory','version-alias','manifest-version-missing','manifest-version-number','duplicate-target','duplicate-target-other-path')) {
    $invalidRoot = Join-Path $scratchParent $invalidBinding
    New-Item -ItemType Directory -Path (Join-Path $invalidRoot 'data/owner') -Force | Out-Null
    Set-Content -LiteralPath (Join-Path $invalidRoot 'data/owner/sentinel.md') -Value 'preserved legacy owner data' -Encoding UTF8
    Copy-Item -LiteralPath (Join-Path $project 'runtime') -Destination $invalidRoot -Recurse
    Copy-Item -LiteralPath $bundlesRoot -Destination $invalidRoot -Recurse
    $versionPath = Join-Path $invalidRoot 'VERSION'
    Set-Content -LiteralPath $versionPath -Value '0.2.0' -Encoding UTF8
    $manifestPath = Join-Path $invalidRoot 'runtime/manifest.json'
    $invalidManifest = Get-Content -LiteralPath $manifestPath -Raw -Encoding UTF8 | ConvertFrom-Json
    switch ($invalidBinding) {
        'version-mismatch' { $invalidManifest.version = '0.1.12' }
        'version-missing' { Remove-Item -LiteralPath $versionPath }
        'version-directory' {
            Remove-Item -LiteralPath $versionPath
            New-Item -ItemType Directory -Path $versionPath | Out-Null
        }
        'version-alias' {
            Move-Item -LiteralPath $versionPath -Destination (Join-Path $invalidRoot 'actual-version')
            if ($env:OS -eq 'Windows_NT') {
                # Junctions exercise reparse rejection without requiring symlink privileges.
                New-Item -ItemType Junction -Path $versionPath -Target $script:EmptyPath | Out-Null
            } else {
                New-Item -ItemType SymbolicLink -Path $versionPath -Target (Join-Path $invalidRoot 'actual-version') | Out-Null
            }
        }
        'manifest-version-missing' { $invalidManifest.PSObject.Properties.Remove('version') }
        'manifest-version-number' { $invalidManifest.version = 2 }
        default {
            $duplicates = @($invalidManifest.artifacts | ForEach-Object {
                $duplicate = $_ | ConvertTo-Json | ConvertFrom-Json
                if ($invalidBinding -eq 'duplicate-target-other-path') { $duplicate.path = 'runtime/other/maestro-runtime' }
                $duplicate
            })
            $invalidManifest.artifacts = @($invalidManifest.artifacts) + $duplicates
        }
    }
    $invalidManifest | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $manifestPath -Encoding UTF8
    $rejected = Invoke-Hook 'first-run-scaffold.ps1' $invalidRoot
    Assert-HookResult (($rejected.Stdout -match 'migration-blocked') -and -not (Test-Path -LiteralPath (Join-Path $invalidRoot 'brain/.initialized'))) "$invalidBinding rejects runtime binding before initialization" $rejected
}

$scaffold = Invoke-Hook 'first-run-scaffold.ps1' $project
Assert-True ($scaffold.ExitCode -eq 0) 'scaffold exits zero'
Assert-True (Test-Path -LiteralPath (Join-Path $project 'brain/.initialized')) 'scaffold creates brain/.initialized'
Assert-True (Test-Path -LiteralPath (Join-Path $project 'brain/owner/registry.json')) 'scaffold creates owner registry'
Assert-True (Test-Path -LiteralPath (Join-Path $project 'brain/memory/.schema-version')) 'scaffold creates memory schema marker'
foreach ($canonicalIndex in @('brain/craft/craft.md','brain/learnings/learnings.md')) {
    Assert-True (Test-Path -LiteralPath (Join-Path $project $canonicalIndex) -PathType Leaf) "scaffold creates canonical $canonicalIndex"
}
foreach ($oldIndex in @('brain/craft/index.md','brain/learnings/index.md')) {
    Assert-True (-not (Test-Path -LiteralPath (Join-Path $project $oldIndex))) "fresh scaffold does not create obsolete $oldIndex"
}
$sentinel = Join-Path $project 'brain/owner/observations/owner-sentinel.txt'
Set-Content -LiteralPath $sentinel -Value 'preserve-me' -Encoding UTF8
$canonicalHashes = @{}
foreach ($canonicalIndex in @('brain/craft/craft.md','brain/learnings/learnings.md')) {
    Set-Content -LiteralPath (Join-Path $project $canonicalIndex) -Value 'owner-authored canonical index' -Encoding UTF8
    $canonicalHashes[$canonicalIndex] = (Get-FileHash -LiteralPath (Join-Path $project $canonicalIndex)).Hash
}
$scaffoldAgain = Invoke-Hook 'first-run-scaffold.ps1' $project
Assert-True (($scaffoldAgain.ExitCode -eq 0) -and ((Get-Content -LiteralPath $sentinel -Raw).Trim() -eq 'preserve-me')) 'scaffold is idempotent and preserves owner data'
foreach ($canonicalIndex in $canonicalHashes.Keys) {
    Assert-True ((Get-FileHash -LiteralPath (Join-Path $project $canonicalIndex)).Hash -eq $canonicalHashes[$canonicalIndex]) "repeated scaffold preserves $canonicalIndex byte for byte"
}

$session = Invoke-Hook 'session-start-memory-inject.ps1' $project
Assert-True ($session.ExitCode -eq 0) 'SessionStart hook exits zero'
Assert-True ($session.Stdout -match 'maestro:session-context:start') 'SessionStart emits the context envelope'
Assert-True ($session.Stdout -match 'operacional') 'SessionStart emits the operator pointer'

$prompt = '{"prompt":"chama o yoda para revisar essa recomendacao"}'
$route = Invoke-Hook 'context-inject-userprompt.ps1' $project $prompt
Assert-True ($route.ExitCode -eq 0) 'UserPromptSubmit hook exits zero'
Assert-HookResult (($route.Stdout -match 'maestro:agent-route') -and ($route.Stdout -match 'yoda')) 'UserPromptSubmit routes an explicit Yoda request' $route
$ordinary = Invoke-Hook 'context-inject-userprompt.ps1' $project '{"prompt":"organize esta lista em tres bullets"}'
Assert-True ($ordinary.Stdout -notmatch 'maestro:agent-route') 'ordinary prompt does not route an agent'
foreach ($spoke in @('darwin','gamma-guardian')) {
    $routedSpoke = Invoke-Hook 'context-inject-userprompt.ps1' $project (@{prompt=$spoke} | ConvertTo-Json -Compress)
    Assert-HookResult ($routedSpoke.Stdout -match $spoke) "$spoke is routed without requiring an optional gate field" $routedSpoke
}

$cases = Join-Path $project 'brain/accounts'
New-Item -ItemType Directory -Path (Join-Path $cases 'alpha/cases/same-case'), (Join-Path $cases 'beta/cases/same-case') -Force | Out-Null
Set-Content -LiteralPath (Join-Path $cases '.active') -Value 'alpha/same-case' -Encoding UTF8
$samePath = Join-Path $cases 'alpha/cases/same-case/notes.md'
$otherPath = Join-Path $cases 'beta/cases/same-case/notes.md'
$samePayload = @{tool_name='Write'; tool_input=@{file_path=$samePath}} | ConvertTo-Json -Compress
$otherPayload = @{tool_name='Write'; tool_input=@{file_path=$otherPath}} | ConvertTo-Json -Compress
$same = Invoke-Hook 'block-cross-case-writes.ps1' $project $samePayload
$other = Invoke-Hook 'block-cross-case-writes.ps1' $project $otherPayload
Assert-HookResult ($same.ExitCode -eq 0) 'same-case write is allowed' $same
Assert-True (($other.ExitCode -eq 2) -and ($other.Stderr -match 'Cross-case write blocked')) 'cross-case write is blocked with a reason'
$outsidePath = Join-Path $project 'notes-outside-cases.md'
$outsidePayload = @{tool_name='Write'; tool_input=@{file_path=$outsidePath}} | ConvertTo-Json -Compress
$outside = Invoke-Hook 'block-cross-case-writes.ps1' $project $outsidePayload
Assert-HookResult ($outside.ExitCode -eq 0) 'write outside the cases tree is allowed' $outside
$pendingFile = Join-Path $cases '.pending'
Set-Content -LiteralPath $pendingFile -Value 'beta/same-case' -Encoding UTF8
$pending = Invoke-Hook 'block-cross-case-writes.ps1' $project $otherPayload
Assert-HookResult ($pending.ExitCode -eq 0) 'confirmed pending case write is allowed' $pending
Remove-Item -LiteralPath $pendingFile -Force
$unicodeCase = 'alpha/caso-s' + [char]0x00E3 + 'o'
New-Item -ItemType Directory -Path (Join-Path $cases $unicodeCase.Replace('/','/cases/')) -Force | Out-Null
Set-Content -LiteralPath (Join-Path $cases '.active') -Value $unicodeCase -Encoding UTF8
$unicodePayload = @{tool_name='Write'; tool_input=@{file_path=(Join-Path $cases ($unicodeCase.Replace('/','/cases/') + '/notes.md'))}} | ConvertTo-Json -Compress
$unicodeWrite = Invoke-Hook 'block-cross-case-writes.ps1' $project $unicodePayload
Assert-HookResult ($unicodeWrite.ExitCode -eq 0) 'UTF-8 case path is decoded and allowed' $unicodeWrite
Set-Content -LiteralPath (Join-Path $cases '.active') -Value 'alpha/same-case' -Encoding UTF8
$traversal = Join-Path $cases 'alpha/cases/same-case/../../../beta/cases/same-case/traversal.md'
$traversalPayload = @{tool_name='Write'; tool_input=@{file_path=$traversal}} | ConvertTo-Json -Compress
$traversalResult = Invoke-Hook 'block-cross-case-writes.ps1' $project $traversalPayload
Assert-True ($traversalResult.ExitCode -eq 2) 'dot-dot traversal into another case is blocked'
if ($env:OS -eq 'Windows_NT') {
    $junction = Join-Path $cases 'alpha/cases/same-case/junction-to-beta'
    New-Item -ItemType Junction -Path $junction -Target (Join-Path $cases 'beta/cases/same-case') -Force | Out-Null
    $junctionPayload = @{tool_name='Write'; tool_input=@{file_path=(Join-Path $junction 'escape.md')}} | ConvertTo-Json -Compress
    $junctionResult = Invoke-Hook 'block-cross-case-writes.ps1' $project $junctionPayload
    Assert-True ($junctionResult.ExitCode -eq 2) 'junction from active case into another case is blocked'
    Remove-Item -LiteralPath $junction -Force
}
$nestedPayload = @{tool_name='Write';cwd=(Join-Path $cases 'alpha/cases/same-case');tool_input=@{file_path='nested/notes.md'}} | ConvertTo-Json -Compress
$nested = Invoke-Hook 'block-cross-case-writes.ps1' $project $nestedPayload
Assert-HookResult ($nested.ExitCode -eq 0) 'relative path is resolved against the actual nested hook cwd' $nested
$escapeTarget = Join-Path $project 'outside-state'
New-Item -ItemType Directory -Path $escapeTarget | Out-Null
$alias = Join-Path $cases 'alpha/cases/same-case/outside-alias'
$aliasType = 'SymbolicLink'
if ($env:OS -eq 'Windows_NT') { $aliasType = 'Junction' }
New-Item -ItemType $aliasType -Path $alias -Target $escapeTarget | Out-Null
$escape = Invoke-Hook 'block-cross-case-writes.ps1' $project (@{tool_name='Write';tool_input=@{file_path=(Join-Path $alias 'note.md')}} | ConvertTo-Json -Compress)
Assert-HookResult ($escape.ExitCode -eq 2) 'case path escaping through directory alias is denied' $escape
Remove-Item -LiteralPath $alias -Force
Remove-Item -LiteralPath (Join-Path $cases '.active') -Force
$missingActive = Invoke-Hook 'block-cross-case-writes.ps1' $project $samePayload
Assert-True ($missingActive.ExitCode -eq 2) 'case write without an active case fails closed'

$announcePayload = @{tool_name='Agent'; tool_input=@{subagent_type='yoda'; prompt='x'}} | ConvertTo-Json -Compress
$announce = Invoke-Hook 'announce-agent-dispatch.ps1' $project $announcePayload
$announceJson = $null
try { $announceJson = $announce.Stdout | ConvertFrom-Json } catch {}
Assert-True ($announce.ExitCode -eq 0) 'agent announcement hook exits zero'
Assert-HookResult (($null -ne $announceJson) -and ($announceJson.hookSpecificOutput.additionalContext -match 'yoda')) 'agent announcement returns additionalContext JSON' $announce
$nonAgent = Invoke-Hook 'announce-agent-dispatch.ps1' $project (@{tool_name='Read'; tool_input=@{file_path='x'}} | ConvertTo-Json -Compress)
Assert-True (($nonAgent.ExitCode -eq 0) -and (-not $nonAgent.Stdout)) 'announcement hook ignores non-Agent tools'

$dream = Invoke-Hook 'session-stop-dream.ps1' $project
Assert-True ($dream.ExitCode -eq 0) 'Stop hook exits zero'
Assert-True (-not (Test-Path -LiteralPath (Join-Path $project 'brain/memory/.dream-requested'))) 'No-op Stop does not request dreaming'
$checkpointQueue = Join-Path $project 'brain/.maestro/daily-pending'
New-Item -ItemType Directory -Path $checkpointQueue -Force | Out-Null
$captureTime = [DateTimeOffset]::Now
$checkpoint = @{schema_version=1;id='smoke-checkpoint';session_id='smoke-session';workspace=[IO.Path]::GetFullPath($project);scope='owner';captured_at=$captureTime.ToString('yyyy-MM-ddTHH:mm:sszzz');local_date=$captureTime.ToString('yyyy-MM-dd');summary='Synthetic PowerShell checkpoint';decisions=@();next_actions=@();provenance='agent-authored'}
[IO.File]::WriteAllText((Join-Path $checkpointQueue 'smoke-checkpoint.json'),($checkpoint | ConvertTo-Json -Compress),(New-Object Text.UTF8Encoding($false)))
$dreamSaved = Invoke-Hook 'session-stop-dream.ps1' $project
Assert-HookResult (Test-Path -LiteralPath (Join-Path $project 'brain/memory/.dream-requested')) 'Persisted checkpoint creates dream fingerprint' $dreamSaved

$malformed = Invoke-Hook 'block-cross-case-writes.ps1' $project '{"tool_name":"Write","tool_input":'
Assert-True ($malformed.ExitCode -eq 2) 'malformed write payload fails closed'
$missingPath = Invoke-Hook 'block-cross-case-writes.ps1' $project '{"tool_name":"Write","tool_input":{}}'
Assert-True ($missingPath.ExitCode -eq 2) 'write without path fails closed'
Set-Content -LiteralPath (Join-Path $cases '.active') -Value 'alpha/same-case' -Encoding UTF8
$agentCheck = Invoke-Hook 'session-stop-agent-check.ps1' $project
$darwinMarker = Join-Path $project 'brain/.maestro/.darwin-requested'
Assert-HookResult (($agentCheck.ExitCode -eq 0) -and (Test-Path -LiteralPath $darwinMarker)) 'periodic policy creates Darwin request without claiming dispatch' $agentCheck
$agentMarkerBefore = Get-Content -LiteralPath $darwinMarker -Raw
$agentAgain = Invoke-Hook 'session-stop-agent-check.ps1' $project
Assert-True ((Get-Content -LiteralPath $darwinMarker -Raw) -eq $agentMarkerBefore) 'pending periodic marker is not refreshed at every Stop'
$index = Invoke-Hook 'session-stop-brain-index.ps1' $project
$indexState = Get-Content -LiteralPath (Join-Path $project 'brain/.maestro/index-capability.json') -Raw | ConvertFrom-Json
Assert-True (($index.ExitCode -eq 0) -and ($indexState.status -eq 'unavailable')) 'optional indexer honestly unavailable without selected Python; handler succeeds'
$eod = Invoke-Hook 'session-stop-eod-check.ps1' $project
Assert-HookResult (($eod.ExitCode -eq 0) -and (Test-Path -LiteralPath (Join-Path $project 'brain/.maestro/day-brief.json'))) 'native EOD derives day brief without Python' $eod
$sessionPending = Invoke-Hook 'session-start-memory-inject.ps1' $project
Assert-HookResult (($sessionPending.Stdout -match 'Agent review requested: darwin') -and ($sessionPending.Stdout -match 'indexing unavailable')) 'next session surfaces pending review and unavailable optional index' $sessionPending
Assert-True ([Text.Encoding]::UTF8.GetByteCount($sessionPending.Stdout) -le 8192) 'session context has a pre-emission UTF8 byte budget'
$today = [DateTime]::Today.ToString('yyyy-MM-dd')
Set-Content -LiteralPath (Join-Path $project "brain/daily/$today.md") -Value '### 18:00 - fechamento' -Encoding UTF8
$closed = Invoke-Hook 'session-stop-eod-check.ps1' $project
Assert-HookResult (-not (Test-Path -LiteralPath (Join-Path $project 'brain/owner/.eod-requested'))) 'closed daily page clears only EOD marker' $closed
Set-Content -LiteralPath (Join-Path $project "brain/memory/recent/$today.md") -Value 'consolidated' -Encoding UTF8
$dreamAgain = Invoke-Hook 'session-stop-dream.ps1' $project
Assert-True (Test-Path -LiteralPath (Join-Path $project 'brain/memory/.dream-requested')) 'Date file alone never clears a pending dream request'
. (Join-Path $script:ContentRoot '.claude/hooks/lib/maestro-runtime.ps1')
$dreamDigest = (Get-Content -LiteralPath (Join-Path $project 'brain/memory/.dream-requested') -Raw).Trim()
$ackPayload = @{scope='owner';digest=$dreamDigest} | ConvertTo-Json -Compress
$ack = Invoke-MaestroRuntime -Root $project -RuntimeArgs @('daily-dream-ack','--root',$project) -Payload $ackPayload
Assert-True ((($ack -join "`n") | ConvertFrom-Json).state -eq 'acknowledged') 'Matching successful synthesis acknowledgement accepted'
$dreamNoop = Invoke-Hook 'session-stop-dream.ps1' $project
Assert-True (-not (Test-Path -LiteralPath (Join-Path $project 'brain/memory/.dream-requested'))) 'Acknowledged work remains clear after no-op Stop'

# Reconstructed 0.1.11/0.1.12 fixtures use the real shared binary and preserve originals.
foreach ($baseline in @('0.1.11','0.1.12')) {
    $upgrade = Join-Path $scratchParent ('upgrade-' + $baseline)
    New-Item -ItemType Directory -Path $upgrade | Out-Null
    Copy-Item -LiteralPath (Join-Path $project 'runtime') -Destination $upgrade -Recurse
    Copy-Item -LiteralPath $bundlesRoot -Destination $upgrade -Recurse
    Set-Content -LiteralPath (Join-Path $upgrade 'VERSION') -Value '0.2.0' -Encoding UTF8
    foreach ($dir in @('data/profile','data/cases/example/brain/canon','data/workspaces/example','data/agents/custom')) {
        New-Item -ItemType Directory -Path (Join-Path $upgrade $dir) -Force | Out-Null
    }
    Set-Content -LiteralPath (Join-Path $upgrade 'data/.maestro-version') -Value $baseline -Encoding UTF8
    Set-Content -LiteralPath (Join-Path $upgrade 'data/.initialized') -Value 'legacy' -Encoding UTF8
    Set-Content -LiteralPath (Join-Path $upgrade 'data/profile/identity.json') -Value '{"display_name":"Owner","initialized":true}' -Encoding UTF8
    Set-Content -LiteralPath (Join-Path $upgrade 'data/cases/.active') -Value 'example' -Encoding UTF8
    Set-Content -LiteralPath (Join-Path $upgrade 'data/cases/example/brain/canon/note.md') -Value 'original' -Encoding UTF8
    Set-Content -LiteralPath (Join-Path $upgrade 'data/workspaces/example/local.md') -Value 'workspace-original' -Encoding UTF8
    Set-Content -LiteralPath (Join-Path $upgrade 'data/agents/custom/agent.md') -Value 'agent-original' -Encoding UTF8
    $before = (Get-FileHash -LiteralPath (Join-Path $upgrade 'data/profile/identity.json')).Hash
    $migrate = Invoke-Hook 'first-run-scaffold.ps1' $upgrade
    Assert-HookResult ((Test-Path -LiteralPath (Join-Path $upgrade 'brain/.initialized')) -and ($migrate.Stdout -notmatch 'migration-blocked')) "$baseline upgrade commits before initialization" $migrate
    Assert-True ((Get-Content -LiteralPath (Join-Path $upgrade 'brain/accounts/.active') -Raw).Trim() -eq '_sem-conta/example') "$baseline active marker becomes account/case"
    Assert-True ((Get-FileHash -LiteralPath (Join-Path $upgrade 'data/profile/identity.json')).Hash -eq $before) "$baseline original identity preserved byte for byte"
    $legacy = Invoke-Hook 'session-start-memory-inject.ps1' $upgrade
    Assert-HookResult (($legacy.Stdout -match 'Validated retained legacy workspaces') -and ($legacy.Stdout -match 'Validated retained legacy agents')) "$baseline retained workspaces and agents are resolved for session consumption" $legacy
    $again = Invoke-Hook 'first-run-scaffold.ps1' $upgrade
    Assert-HookResult ($again.Stdout -notmatch 'migration-blocked') "$baseline repeated startup remains usable" $again
}
$blocked = Join-Path $scratchParent 'blocked-upgrade'
New-Item -ItemType Directory -Path (Join-Path $blocked 'data/owner'),(Join-Path $blocked 'brain/owner') -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $project 'runtime') -Destination $blocked -Recurse
Set-Content -LiteralPath (Join-Path $blocked 'VERSION') -Value '0.2.0' -Encoding UTF8
Set-Content -LiteralPath (Join-Path $blocked 'data/owner/a.md') -Value 'old' -Encoding UTF8
Set-Content -LiteralPath (Join-Path $blocked 'brain/owner/a.md') -Value 'new' -Encoding UTF8
$blockedRun = Invoke-Hook 'first-run-scaffold.ps1' $blocked
Assert-HookResult (($blockedRun.Stdout -match 'migration-blocked') -and -not (Test-Path -LiteralPath (Join-Path $blocked 'brain/.initialized'))) 'ambiguous dual tree cannot write initialized' $blockedRun
Assert-True (-not (Test-Path -LiteralPath (Join-Path $blocked 'brain/memory'))) 'blocked migration does not perform scaffold backfills'
Assert-True ((Get-Content -LiteralPath (Join-Path $blocked 'brain/owner/a.md') -Raw).Trim() -eq 'new') 'blocked migration preserves current authored content'
Set-Content -LiteralPath (Join-Path $blocked 'brain/.initialized') -Value 'stale-marker' -Encoding UTF8
$staleRun = Invoke-Hook 'first-run-scaffold.ps1' $blocked
Assert-HookResult (($staleRun.Stdout -match 'migration-blocked') -and -not (Test-Path -LiteralPath (Join-Path $blocked 'brain/memory'))) 'stale initialized marker cannot bypass migration gate' $staleRun
$missingRuntime = Join-Path $scratchParent 'no-runtime'
New-Item -ItemType Directory -Path (Join-Path $missingRuntime 'data/owner') -Force | Out-Null
Set-Content -LiteralPath (Join-Path $missingRuntime 'data/owner/a.md') -Value 'preserved' -Encoding UTF8
$unavailable = Invoke-Hook 'first-run-scaffold.ps1' $missingRuntime
Assert-HookResult (($unavailable.Stdout -match 'migration-blocked') -and -not (Test-Path -LiteralPath (Join-Path $missingRuntime 'brain/.initialized'))) 'unavailable migration runtime cannot initialize' $unavailable

Write-Output ""
Write-Output "Summary: $script:Passed pass, $script:Failed fail"
if ($script:Failed -ne 0) { exit 1 }
exit 0
