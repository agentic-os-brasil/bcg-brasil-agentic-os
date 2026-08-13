param(
  [Parameter(Mandatory = $true)][string]$PreviousZip,
  [Parameter(Mandatory = $true)][string]$PreviousChecksum,
  [Parameter(Mandatory = $true)][string]$CandidateZip,
  [Parameter(Mandatory = $true)][string]$CandidateChecksum,
  [Parameter(Mandatory = $true)][string]$PreviousVersion,
  [Parameter(Mandatory = $true)][string]$CandidateVersion,
  [Parameter(Mandatory = $true)][string]$OutputReceipt
)

$ErrorActionPreference = 'Stop'

function Stop-Acceptance([string]$Message) {
  throw "MAESTRO-WINDOWS-ACCEPTANCE: $Message; nada foi apagado do perfil real"
}

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
  Stop-Acceptance 'este ensaio requer Windows real'
}
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  Stop-Acceptance 'execute com token de usuario normal, nunca elevado'
}

foreach ($path in @($PreviousZip, $PreviousChecksum, $CandidateZip, $CandidateChecksum)) {
  if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { Stop-Acceptance "arquivo ausente: $path" }
}
if ($PreviousVersion -notmatch '^\d+\.\d+\.\d+$' -or $CandidateVersion -notmatch '^\d+\.\d+\.\d+$' -or [version]$CandidateVersion -le [version]$PreviousVersion) { Stop-Acceptance 'versoes devem ser SemVer e candidate precisa ser mais nova' }
$receiptParent = Split-Path -Parent ([IO.Path]::GetFullPath($OutputReceipt))
if (-not (Test-Path -LiteralPath $receiptParent -PathType Container)) { Stop-Acceptance 'diretorio do receipt nao existe' }

function Confirm-ArchiveDigest([string]$Archive, [string]$Sidecar) {
  $line = ([IO.File]::ReadAllText($Sidecar)).Trim()
  if ($line -notmatch '^([a-f0-9]{64})  ([^/\\]+)$') { Stop-Acceptance "checksum malformado: $Sidecar" }
  if ($Matches[2] -ne (Split-Path $Archive -Leaf)) { Stop-Acceptance "checksum aponta para outro artefato: $Sidecar" }
  $actual = (Get-FileHash -LiteralPath $Archive -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($actual -ne $Matches[1]) { Stop-Acceptance "checksum divergente: $Archive" }
  return $actual
}

function Find-One([string]$Root, [string]$Name) {
  $matches = @(Get-ChildItem -LiteralPath $Root -File -Recurse -Filter $Name)
  if ($matches.Count -ne 1) { Stop-Acceptance "esperava exatamente um $Name em $Root" }
  return $matches[0].FullName
}

function Invoke-ChildScript([string]$Script, [string[]]$Arguments, [string]$InputBody = '') {
  $allArguments = @('-NoLogo', '-NoProfile', '-NonInteractive', '-File', $Script) + $Arguments
  $output = $InputBody | & powershell.exe @allArguments 2>&1
  if ($LASTEXITCODE -ne 0) { Stop-Acceptance "script falhou ($LASTEXITCODE): $($output -join ' ')" }
  return ($output -join "`n")
}

function Invoke-ChildScriptExpectFailure([string]$Script, [string[]]$Arguments) {
  $allArguments = @('-NoLogo', '-NoProfile', '-NonInteractive', '-File', $Script) + $Arguments
  $previousPreference = $ErrorActionPreference
  try {
    $ErrorActionPreference = 'Continue'
    $output = & powershell.exe @allArguments 2>&1
    $exitCode = $LASTEXITCODE
  } finally {
    $ErrorActionPreference = $previousPreference
  }
  if ($exitCode -eq 0) { Stop-Acceptance 'script aceitou CLAUDE.md com encoding proibido' }
  return ($output -join "`n")
}

function Confirm-EncodingRejectedWithoutWorkspaceMutation([string]$Installer, [string]$Workspace, [byte[]]$Bytes, [string]$Label) {
  New-Item -ItemType Directory -Path $Workspace | Out-Null
  $claude = Join-Path $Workspace 'CLAUDE.md'
  [IO.File]::WriteAllBytes($claude, $Bytes)
  $before = [Convert]::ToBase64String([IO.File]::ReadAllBytes($claude))
  $output = Invoke-ChildScriptExpectFailure $Installer @('-Action','install','-Workspace',$Workspace)
  if (-not $output.Contains('MAESTRO-SCRIPT-ENCODING')) { Stop-Acceptance "$Label nao falhou pelo guard estrito de encoding" }
  $after = [Convert]::ToBase64String([IO.File]::ReadAllBytes($claude))
  if ($after -ne $before) { Stop-Acceptance "$Label alterou os bytes originais de CLAUDE.md" }
  $unexpected = @(Get-ChildItem -LiteralPath $Workspace -Force | Where-Object { $_.Name -ne 'CLAUDE.md' })
  if ($unexpected.Count -ne 0) { Stop-Acceptance "$Label alterou o workspace antes de rejeitar CLAUDE.md" }
}

function Invoke-StartLauncher([string]$Launcher) {
  $startInfo = New-Object System.Diagnostics.ProcessStartInfo
  $startInfo.FileName = $env:ComSpec
  $startInfo.Arguments = '/d /s /c ""' + $Launcher + '""'
  $startInfo.UseShellExecute = $false
  $startInfo.CreateNoWindow = $true
  $startInfo.RedirectStandardInput = $true
  $startInfo.RedirectStandardOutput = $true
  $startInfo.RedirectStandardError = $true
  $process = New-Object System.Diagnostics.Process
  $process.StartInfo = $startInfo
  try {
    if (-not $process.Start()) { Stop-Acceptance 'Start Maestro.cmd nao iniciou' }
    $process.StandardInput.WriteLine('S')
    $process.StandardInput.WriteLine('')
    $process.StandardInput.Close()
    if (-not $process.WaitForExit(60000)) {
      $process.Kill()
      Stop-Acceptance 'Start Maestro.cmd nao terminou em 60 segundos'
    }
    $stdout = $process.StandardOutput.ReadToEnd()
    $stderr = $process.StandardError.ReadToEnd()
    $exitCode = $process.ExitCode
  } finally {
    $process.Dispose()
  }
  $output = ($stdout + "`n" + $stderr).Trim()
  if ($exitCode -ne 0) { Stop-Acceptance "Start Maestro.cmd falhou ($exitCode): $output" }
  if (-not $output.Contains('Maestro preparado')) { Stop-Acceptance "Start Maestro.cmd nao confirmou a preparacao: $output" }
  return $output
}

function Confirm-BoundedHookInputBeforeEOF([string]$Hook, [string]$Workspace) {
  New-Item -ItemType Directory -Path (Join-Path $Workspace '.maestro-script') | Out-Null
  $prefix = [Text.Encoding]::ASCII.GetBytes('{"session_id":"windows-oversized-without-eof"}')
  [byte[]]$payload = [Array]::CreateInstance([byte], 65537)
  for ($index = 0; $index -lt $payload.Length; $index++) { $payload[$index] = 0x20 }
  [Array]::Copy($prefix, 0, $payload, 0, $prefix.Length)

  $startInfo = New-Object System.Diagnostics.ProcessStartInfo
  $startInfo.FileName = 'powershell.exe'
  $startInfo.Arguments = '-NoLogo -NoProfile -NonInteractive -File "' + $Hook + '" -Event context-injection -Workspace "' + $Workspace + '"'
  $startInfo.UseShellExecute = $false
  $startInfo.CreateNoWindow = $true
  $startInfo.RedirectStandardInput = $true
  $startInfo.RedirectStandardOutput = $true
  $startInfo.RedirectStandardError = $true
  $process = New-Object System.Diagnostics.Process
  $process.StartInfo = $startInfo
  try {
    if (-not $process.Start()) { Stop-Acceptance 'hook bounded-stdin nao iniciou' }
    $process.StandardInput.BaseStream.Write($payload, 0, $payload.Length)
    $process.StandardInput.BaseStream.Flush()
    if (-not $process.WaitForExit(15000)) {
      $process.StandardInput.Close()
      $process.Kill()
      Stop-Acceptance 'hook leu alem de 65537 bytes ou aguardou EOF'
    }
    $stdout = $process.StandardOutput.ReadToEnd()
    $stderr = $process.StandardError.ReadToEnd()
    if ($process.ExitCode -ne 0) { Stop-Acceptance "hook bounded-stdin falhou ($($process.ExitCode)): $stdout $stderr" }
  } finally {
    $process.StandardInput.Close()
    $process.Dispose()
  }
  $json = Convert-HookJson $stdout 'bounded-stdin-before-eof'
  if ($json.hookSpecificOutput.hookEventName -ne 'UserPromptSubmit' -or -not ([string]$json.hookSpecificOutput.additionalContext).Contains('Native route authority remains unavailable')) {
    Stop-Acceptance 'hook nao rejeitou overflow com fallback bounded'
  }
  $entries = @(Get-ChildItem -LiteralPath (Join-Path $Workspace '.maestro-script') -Force)
  if ($entries.Count -ne 0) { Stop-Acceptance 'hook criou estado ao rejeitar overflow bounded' }
}

function Convert-HookJson([string]$Body, [string]$Event) {
  try { return $Body | ConvertFrom-Json } catch { Stop-Acceptance "hook $Event nao retornou JSON valido" }
}

$previousDigest = Confirm-ArchiveDigest $PreviousZip $PreviousChecksum
$candidateDigest = Confirm-ArchiveDigest $CandidateZip $CandidateChecksum
$policyBefore = @(Get-ExecutionPolicy -List | ForEach-Object { [ordered]@{scope=$_.Scope.ToString();policy=$_.ExecutionPolicy.ToString()} })
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ("Maestro Windows artifact acceptance with spaces " + [guid]::NewGuid().ToString('N'))
$previousRoot = Join-Path $testRoot 'previous package'
$candidateRoot = Join-Path $testRoot 'candidate package'
$runtimeRoot = Join-Path $testRoot 'local app data\Maestro script runtime'
$workspaceHome = Join-Path $testRoot 'owner home\Maestro'
$env:MAESTRO_SCRIPT_HOME = $runtimeRoot
$env:MAESTRO_WORKSPACE_HOME = $workspaceHome

try {
  New-Item -ItemType Directory -Path $previousRoot, $candidateRoot | Out-Null
  Expand-Archive -LiteralPath $PreviousZip -DestinationPath $previousRoot
  Expand-Archive -LiteralPath $CandidateZip -DestinationPath $candidateRoot
  $previousInstaller = Find-One $previousRoot 'Install-Maestro.ps1'
  $candidateInstaller = Find-One $candidateRoot 'Install-Maestro.ps1'
  $candidateLauncher = Find-One $candidateRoot 'Start Maestro.cmd'

  $env:MAESTRO_SCRIPT_HOME = Join-Path $testRoot 'encoding guard\runtime'
  $bomWorkspace = Join-Path $testRoot 'encoding guard\utf8 bom workspace'
  Confirm-EncodingRejectedWithoutWorkspaceMutation $candidateInstaller $bomWorkspace ([byte[]](0xEF,0xBB,0xBF,0x23,0x20,0x4D,0x61,0x65,0x73,0x74,0x72,0x6F,0x0A)) 'UTF-8 BOM'
  $invalidWorkspace = Join-Path $testRoot 'encoding guard\invalid utf8 workspace'
  Confirm-EncodingRejectedWithoutWorkspaceMutation $candidateInstaller $invalidWorkspace ([byte[]](0x23,0x20,0x4D,0x61,0x65,0x73,0x74,0x72,0x6F,0x0A,0xC3,0x28)) 'invalid UTF-8'
  $env:MAESTRO_SCRIPT_HOME = $runtimeRoot

  $env:MAESTRO_SCRIPT_HOME = Join-Path $testRoot 'candidate launcher runtime'
  $env:MAESTRO_WORKSPACE_HOME = Join-Path $testRoot 'candidate launcher owner home'
  [void](Invoke-StartLauncher $candidateLauncher)
  $candidateLauncherActive = ([IO.File]::ReadAllText((Join-Path $env:MAESTRO_SCRIPT_HOME 'state\active-version'))).Trim()
  if ($candidateLauncherActive -ne $CandidateVersion) { Stop-Acceptance 'Start Maestro.cmd candidato nao instalou sua propria versao' }
  $env:MAESTRO_SCRIPT_HOME = $runtimeRoot
  $env:MAESTRO_WORKSPACE_HOME = $workspaceHome
  [void](Invoke-ChildScript $previousInstaller @('-Action','install'))
  $workspace = Join-Path $workspaceHome 'maestro-os'
  $ownerState = Join-Path $workspace '.maestro-script'
  $sentinel = Join-Path $ownerState 'acceptance-owner-sentinel.txt'
  [IO.File]::WriteAllText($sentinel, "owner-local sentinel`n")

  [void](Invoke-ChildScript $candidateInstaller @('-Action','install'))
  $active = ([IO.File]::ReadAllText((Join-Path $runtimeRoot 'state\active-version'))).Trim()
  $previous = ([IO.File]::ReadAllText((Join-Path $runtimeRoot 'state\previous-version'))).Trim()
  if ($active -ne $CandidateVersion -or $previous -ne $PreviousVersion) { Stop-Acceptance "ponteiros apos update divergiram: active=$active previous=$previous" }
  $installedCandidate = Join-Path $runtimeRoot "releases\$active\Install-Maestro.ps1"
  $doctorUpdate = Invoke-ChildScript $installedCandidate @('-Action','doctor')
  if (-not $doctorUpdate.Contains('configured and intact on disk')) { Stop-Acceptance 'doctor do update nao confirmou a projecao' }
  if (([IO.File]::ReadAllText($sentinel)) -ne "owner-local sentinel`n") { Stop-Acceptance 'update alterou sentinel owner-local' }

  $hook = Join-Path $workspace '.maestro-script\hooks\Maestro-Hook.ps1'
  Confirm-BoundedHookInputBeforeEOF $hook (Join-Path $testRoot 'bounded stdin workspace')
  $privateProfile = "WINDOWS ACCEPTANCE PRIVATE PROFILE BODY`n"
  $profilePath = Join-Path $ownerState 'local-profile.md'
  $profileStatePath = Join-Path $ownerState 'session-profile.json'
  [IO.File]::WriteAllText($profilePath, $privateProfile)
  $profileDigest = (Get-FileHash -LiteralPath $profilePath -Algorithm SHA256).Hash.ToLowerInvariant()
  [IO.File]::WriteAllText($profileStatePath, (@{schema_version=1;interaction_profile='power';revision=4;local_profile='.maestro-script/local-profile.md';profile_sha256=$profileDigest;session_use_confirmed=$true} | ConvertTo-Json -Compress))

  $sessionBody = Invoke-ChildScript $hook @('-Event','session-start','-Workspace',$workspace)
  $sessionJson = Convert-HookJson $sessionBody 'session-start'
  if ($sessionJson.hookSpecificOutput.hookEventName -ne 'SessionStart' -or -not ([string]$sessionJson.hookSpecificOutput.additionalContext).Contains('interaction profile: power') -or $sessionBody.Contains('WINDOWS ACCEPTANCE PRIVATE PROFILE BODY') -or $sessionBody.Contains($profileDigest)) { Stop-Acceptance 'SessionStart nao comprovou perfil bounded sem vazamento' }

  $contextJson = Convert-HookJson (Invoke-ChildScript $hook @('-Event','context-injection','-Workspace',$workspace)) 'context-injection'
  if ($contextJson.hookSpecificOutput.hookEventName -ne 'UserPromptSubmit') { Stop-Acceptance 'UserPromptSubmit nao foi observado estruturalmente' }

  $guardJson = Convert-HookJson (Invoke-ChildScript $hook @('-Event','pre-action-guard','-Workspace',$workspace) '{"tool_name":"Write","tool_input":{"file_path":".claude/settings.local.json"}}') 'pre-action-guard'
  if ($guardJson.hookSpecificOutput.hookEventName -ne 'PreToolUse' -or $guardJson.hookSpecificOutput.permissionDecision -ne 'deny') { Stop-Acceptance 'PreToolUse nao negou o path gerenciado estruturalmente' }

  [void](Invoke-ChildScript $hook @('-Event','post-action-receipt','-Workspace',$workspace) '{}')
  $stopJson = Convert-HookJson (Invoke-ChildScript $hook @('-Event','stop-finalization','-Workspace',$workspace) '{}') 'stop-finalization'
  if ($stopJson.continue -isnot [bool] -or -not [bool]$stopJson.continue) { Stop-Acceptance 'Stop nao retornou continue=true estruturalmente' }
  $subagentJson = Convert-HookJson (Invoke-ChildScript $hook @('-Event','subagent-start','-Workspace',$workspace) '{}') 'subagent-start'
  if ($subagentJson.hookSpecificOutput.hookEventName -ne 'SubagentStart') { Stop-Acceptance 'SubagentStart nao foi observado estruturalmente' }
  [void](Invoke-ChildScript $hook @('-Event','subagent-stop','-Workspace',$workspace) '{}')

  $routeSession = 'WINDOWS ACCEPTANCE SESSION SECRET'
  $routeBegin = Convert-HookJson (Invoke-ChildScript $hook @('-Event','context-injection','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '","prompt":"CLIENT PROMPT MUST NOT PERSIST"}')) 'route-begin'
  if (-not ([string]$routeBegin.hookSpecificOutput.additionalContext).Contains('agent-route-lite')) { Stop-Acceptance 'agent-route-lite nao iniciou' }
  [void](Invoke-ChildScript $hook @('-Event','subagent-start','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '","agent_id":"account-secret-1","agent_type":"client-account-agent"}'))
  $routeActive = Convert-HookJson (Invoke-ChildScript $hook @('-Event','stop-finalization','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '"}')) 'route-active-stop'
  if ($routeActive.decision -ne 'block') { Stop-Acceptance 'agent-route-lite nao bloqueou especialista ativo' }
  $routeStringEscape = Convert-HookJson (Invoke-ChildScript $hook @('-Event','stop-finalization','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '","stop_hook_active":"true"}')) 'route-string-escape'
  if ($routeStringEscape.decision -ne 'block') { Stop-Acceptance 'agent-route-lite aceitou stop_hook_active com tipo incorreto' }
  $routeState = @(Get-ChildItem -LiteralPath (Join-Path $ownerState 'agent-route-lite') -File -Filter '*.json')
  if ($routeState.Count -ne 1) { Stop-Acceptance 'agent-route-lite nao produziu estado unico antes do teste de lock' }
  $routeLock = $routeState[0].FullName + '.lock'
  $lockStream = [IO.File]::Open($routeLock, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
  try {
    $routeBusy = Convert-HookJson (Invoke-ChildScript $hook @('-Event','stop-finalization','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '"}')) 'route-busy-stop'
    if ($routeBusy.decision -ne 'block' -or -not (Test-Path -LiteralPath $routeLock -PathType Leaf)) { Stop-Acceptance 'agent-route-lite nao falhou fechado sob contencao' }
  } finally { $lockStream.Dispose(); Remove-Item -LiteralPath $routeLock -Force -ErrorAction SilentlyContinue }
  [void](Invoke-ChildScript $hook @('-Event','subagent-stop','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '","agent_id":"account-secret-1","agent_type":"client-account-agent"}'))
  [void](Invoke-ChildScript $hook @('-Event','subagent-start','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '","agent_id":"case-secret-1","agent_type":"case-agent"}'))
  [void](Invoke-ChildScript $hook @('-Event','subagent-stop','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '","agent_id":"case-secret-1","agent_type":"case-agent"}'))
  [void](Invoke-ChildScript $hook @('-Event','subagent-start','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '","agent_id":"account-secret-2","agent_type":"client-account-agent"}'))
  [void](Invoke-ChildScript $hook @('-Event','subagent-stop','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '","agent_id":"account-secret-2","agent_type":"client-account-agent"}'))
  $routeComplete = Convert-HookJson (Invoke-ChildScript $hook @('-Event','stop-finalization','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '"}')) 'route-complete-stop'
  if ($routeComplete.continue -isnot [bool] -or -not [bool]$routeComplete.continue) { Stop-Acceptance 'agent-route-lite nao concluiu a rota estrategica' }
  [void](Invoke-ChildScript $hook @('-Event','context-injection','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '","prompt":"direct Walter review"}'))
  [void](Invoke-ChildScript $hook @('-Event','subagent-start','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '","agent_id":"walter-direct-secret","agent_type":"walter"}'))
  [void](Invoke-ChildScript $hook @('-Event','subagent-stop','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '","agent_id":"walter-direct-secret","agent_type":"walter"}'))
  $walterComplete = Convert-HookJson (Invoke-ChildScript $hook @('-Event','stop-finalization','-Workspace',$workspace) ('{"session_id":"' + $routeSession + '"}')) 'route-direct-walter-stop'
  if ($walterComplete.continue -isnot [bool] -or -not [bool]$walterComplete.continue) { Stop-Acceptance 'agent-route-lite nao concluiu Walter direto' }
  $routeFiles = @(Get-ChildItem -LiteralPath (Join-Path $ownerState 'agent-route-lite') -File -Filter '*.json')
  if ($routeFiles.Count -ne 1 -or $routeFiles[0].Length -gt 2048) { Stop-Acceptance 'agent-route-lite nao produziu estado bounded unico' }
  $routeBody = [IO.File]::ReadAllText($routeFiles[0].FullName)
  foreach ($secret in @($routeSession,'account-secret','case-secret','CLIENT PROMPT')) { if ($routeBody.Contains($secret)) { Stop-Acceptance 'agent-route-lite persistiu conteudo ou identidade bruta' } }

  $eventLog = Join-Path $workspace '.maestro-script\hook-events.jsonl'
  if (-not (Test-Path -LiteralPath $eventLog -PathType Leaf)) { Stop-Acceptance 'hook-events.jsonl ausente' }
  $persistedEvents = @{}
  foreach ($line in Get-Content -LiteralPath $eventLog) {
    try { $entry = $line | ConvertFrom-Json } catch { Stop-Acceptance 'hook-events.jsonl contem JSON invalido' }
    if ($entry.schema_version -eq 1 -and $entry.profile -eq 'script-only' -and $entry.state -eq 'observed') { $persistedEvents[[string]$entry.event] = $true }
  }
  foreach ($persisted in @('PostToolUse','Stop','SubagentStart','SubagentStop')) {
    if (-not $persistedEvents.ContainsKey($persisted)) { Stop-Acceptance "evento persistido ausente: $persisted" }
  }

  [void](Invoke-ChildScript $installedCandidate @('-Action','rollback'))
  $rolledBack = ([IO.File]::ReadAllText((Join-Path $runtimeRoot 'state\active-version'))).Trim()
  $rollbackPrevious = ([IO.File]::ReadAllText((Join-Path $runtimeRoot 'state\previous-version'))).Trim()
  if ($rolledBack -ne $PreviousVersion -or $rollbackPrevious -ne $CandidateVersion) { Stop-Acceptance "ponteiros apos rollback divergiram: active=$rolledBack previous=$rollbackPrevious" }
  $installedPrevious = Join-Path $runtimeRoot "releases\$rolledBack\Install-Maestro.ps1"
  $doctorRollback = Invoke-ChildScript $installedPrevious @('-Action','doctor')
  if (-not $doctorRollback.Contains('configured and intact on disk')) { Stop-Acceptance 'doctor do rollback nao confirmou a projecao' }
  if (([IO.File]::ReadAllText($sentinel)) -ne "owner-local sentinel`n") { Stop-Acceptance 'rollback alterou sentinel owner-local' }
  if (([IO.File]::ReadAllText($profilePath)) -ne $privateProfile -or -not (Test-Path -LiteralPath $profileStatePath -PathType Leaf)) { Stop-Acceptance 'rollback alterou perfil owner-local' }

  $policyAfter = @(Get-ExecutionPolicy -List | ForEach-Object { [ordered]@{scope=$_.Scope.ToString();policy=$_.ExecutionPolicy.ToString()} })
  if (($policyBefore | ConvertTo-Json -Compress) -ne ($policyAfter | ConvertTo-Json -Compress)) { Stop-Acceptance 'politica PowerShell mudou durante o ensaio' }
  $receipt = [ordered]@{
    schema_version = 1
    status = 'passed'
    os_version = [Environment]::OSVersion.VersionString
    architecture = $env:PROCESSOR_ARCHITECTURE
    elevated = $false
    previous_zip_sha256 = $previousDigest
    candidate_zip_sha256 = $candidateDigest
    updated_active_version = $active
    updated_previous_version = $previous
    rolled_back_active_version = $rolledBack
    hooks_stdout_observed = @('session-start','context-injection','pre-action-guard','stop-finalization','subagent-start')
    hooks_persisted_observed = @('PostToolUse','Stop','SubagentStart','SubagentStop')
    utf8_bom_rejected_without_workspace_mutation = $true
    invalid_utf8_rejected_without_workspace_mutation = $true
    nontechnical_launcher_install = $true
    agent_route_lite_strategic_completion = $true
    agent_route_lite_direct_walter = $true
    agent_route_lite_stop_fail_closed = $true
    agent_route_lite_input_bounded_before_eof = $true
    reviewed_session_profile_preserved = $true
    execution_policy_before = $policyBefore
    execution_policy_after = $policyAfter
    owner_sentinel_preserved = $true
  }
  [IO.File]::WriteAllText($OutputReceipt, ($receipt | ConvertTo-Json -Depth 5) + "`n")
  Write-Output "Maestro Windows artifact acceptance passed: $OutputReceipt"
} catch {
  Write-Error $_
  Write-Error "Evidencia preservada para diagnostico em: $testRoot"
  exit 1
}
