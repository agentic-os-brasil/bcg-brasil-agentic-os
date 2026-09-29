# Native lifecycle support; PS 5.1 syntax, no Bash or Python dependency.
function Test-MaestroMigrationReady([string]$Project) {
    if (-not (Test-Path -LiteralPath (Join-Path $Project 'data')) -and -not (Test-Path -LiteralPath (Join-Path $Project 'brain/.maestro/migration/state.json'))) { return $true }
    try {
        . (Join-Path $PSScriptRoot 'maestro-runtime.ps1')
        $raw = Invoke-MaestroRuntime -Root $Project -RuntimeArgs @('migration','--project',$Project)
        $result = ($raw -join "`n") | ConvertFrom-Json
        if ($result.state -notin @('committed','not_needed')) { throw 'Migration is not committed' }
        return $true
    } catch {
        [Console]::Out.WriteLine('<!-- maestro:migration-blocked -->')
        [Console]::Out.WriteLine('Migration unavailable, blocked or partial. Original data/ is preserved. Run /maestro-doctor before initialization.')
        [Console]::Error.WriteLine($_.Exception.Message)
        return $false
    }
}

function Assert-MaestroNoAlias([string]$Path) {
    $current = [IO.Path]::GetFullPath($Path)
    $projectRoot = Get-MaestroProjectDir
    while ($current) {
        if (Test-Path -LiteralPath $current) {
            $item = Get-Item -LiteralPath $current -Force
            if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Aliased state path is not allowed' }
        }
        if ($projectRoot -and $current -eq $projectRoot) { break }
        $next = Split-Path -Parent $current
        if ($next -eq $current) { break }
        $current = $next
    }
}

function Get-MaestroActiveCase([string]$Project) {
    $accounts = Join-Path $Project 'brain/accounts'
    Assert-MaestroNoAlias $accounts
    $marker = Join-Path $accounts '.active'
    Assert-MaestroNoAlias $marker
    $active = Read-MaestroTrimmed $marker
    if (-not $active) { return $null }
    $parts = @($active -split '/')
    if ($parts.Count -ne 2) { throw 'Active case must be account/case' }
    foreach ($part in $parts) {
        if (-not $part -or $part -in @('.','..') -or $part -match '[\\/:*?"<>|\x00-\x1f]' -or $part.EndsWith('.') -or $part.EndsWith(' ')) { throw 'Invalid active case' }
    }
    $path = Join-Path $accounts ($parts[0] + '/cases/' + $parts[1])
    Assert-MaestroNoAlias $path
    if (-not (Test-Path -LiteralPath $path -PathType Container)) { throw 'Active case is unavailable' }
    return [pscustomobject]@{ Id=$active; Path=$path }
}

function Read-MaestroBounded([string]$Path, [int]$MaxBytes = 4096) {
    Assert-MaestroNoAlias $Path
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return '' }
    $stream = [IO.File]::OpenRead($Path)
    try {
        $bytes = New-Object byte[] ($MaxBytes + 1)
        $count = $stream.Read($bytes, 0, $bytes.Length)
        if ($count -gt $MaxBytes) { return '[Content omitted: exceeds context budget. Read the source on demand.]' }
        return $script:Utf8NoBom.GetString($bytes, 0, $count).TrimStart([char]0xfeff)
    } finally { $stream.Dispose() }
}

function Invoke-MaestroSessionStartMemoryInject {
    $project = Get-MaestroProjectDir
    if (-not $project -or -not (Test-MaestroMigrationReady $project)) { return }
    $brain = Join-Path $project 'brain'
    if (-not (Test-Path -LiteralPath $brain)) { return }
    Assert-MaestroNoAlias $brain
    $lines = New-Object 'System.Collections.Generic.List[string]'
    $lines.Add('<!-- maestro:session-context:start -->')
    $lines.Add('# Maestro - Contexto da sessao')
    $lines.Add('Metodo operacional: load bundles/base/skills/maestro-operator/SKILL.md before control-plane work.')
    $lines.Add('Memory: brain/memory/. Skills: bundles/base/skills/ and bundles/tech-core/skills/. Read specific sources on demand.')
    $identity = Read-MaestroBounded (Join-Path $brain 'owner/identity.json') 2048
    if ($identity) { $lines.Add($identity) }
    foreach ($tier in @('lifetime','medium-term','weekly','recent')) {
        $dir = Join-Path $brain ('memory/' + $tier)
        Assert-MaestroNoAlias $dir
        $files = @(Get-ChildItem -LiteralPath $dir -Filter '*.md' -ErrorAction SilentlyContinue | Sort-Object Name | Select-Object -Last 1)
        if ($files.Count) { $lines.Add('Memory ' + $tier + ': ' + (Read-MaestroBounded $files[0].FullName 1024)) }
    }
    try {
        $active = Get-MaestroActiveCase $project
        if ($active) {
            $lines.Add('Active case: ' + $active.Id)
            $lines.Add('Case source: brain/accounts/' + $active.Id.Replace('/','/cases/'))
            $lines.Add((Read-MaestroBounded (Join-Path $active.Path 'brief.md') 1024))
        }
    } catch { $lines.Add('Active case unavailable: verify brain/accounts/.active and filesystem aliases before reading case context.') }
    foreach ($entry in @(
        @('memory/.dream-requested','Dream pending: load dream-memory.'),
        @('.upgrade-pending','Upgrade pending: load maestro-setup-update.'),
        @('owner/.eod-requested','Day closing pending: ask the owner before writing a closing entry.'),
        @('.maestro/.index-failed','Optional brain indexing unavailable. Use the brain-index recipe or consented managed Python setup. No fresh indexing health claim.'),
        @('.maestro/.health-requested','Brain health needs review: inspect brain/.maestro/diagnostics.json.')
    )) {
        $path = Join-Path $brain $entry[0]
        Assert-MaestroNoAlias $path
        if (Test-Path -LiteralPath $path) { $lines.Add($entry[1]) }
    }
    $policy = Read-MaestroBounded (Join-Path $project 'bundles/base/agents/activation-policy.json') 65536 | ConvertFrom-Json
    foreach ($agent in @($policy.agents)) {
        $auto = Get-MaestroProperty $agent 'auto'
        if (-not $auto -or $agent.status -ne 'active' -or -not $agent.dispatchable) { continue }
        $marker = Join-Path $project ([string]$auto.marker)
        if (-not (Test-MaestroPathInside $marker (Join-Path $brain '.maestro'))) { throw 'Invalid agent marker path' }
        Assert-MaestroNoAlias $marker
        if (Test-Path -LiteralPath $marker) { $lines.Add('Agent review requested: ' + $agent.id + '. This is a pending request, not an executed agent call. Load the policy packet and announce only an actual dispatch.') }
    }
    if (Test-Path -LiteralPath (Join-Path $project 'data')) {
        . (Join-Path $PSScriptRoot 'maestro-runtime.ps1')
        foreach ($ns in @('agents','workspaces','canary')) {
            try {
                $resolved = Invoke-MaestroRuntime -Root $project -RuntimeArgs @('migration','--project',$project,'--resolve-legacy',$ns)
                $lines.Add('Validated retained legacy ' + $ns + ': ' + ($resolved -join ' '))
            } catch { $lines.Add('Retained legacy ' + $ns + ': unavailable; verify migration status before reading.') }
        }
    }
    $lines.Add('<!-- maestro:session-context:end -->')
    $text = $lines -join "`n"
    if ($script:Utf8NoBom.GetByteCount($text) -gt 8192) { $text = '<!-- maestro:session-context:start -->Context omitted: total budget exceeded. Load maestro-operator and brain sources on demand.<!-- maestro:session-context:end -->' }
    [Console]::Out.WriteLine($text)
}

function Invoke-MaestroSessionStopAgentCheck {
    $project = Get-MaestroProjectDir
    if (-not $project -or -not (Test-Path -LiteralPath (Join-Path $project 'brain/.initialized'))) { return }
    $machine = Join-Path $project 'brain/.maestro'
    Assert-MaestroNoAlias $machine
    $policy = Read-MaestroBounded (Join-Path $project 'bundles/base/agents/activation-policy.json') 65536 | ConvertFrom-Json
    $runsPath = Join-Path $machine 'agent-runs.json'
    $runs = @{}
    $priorRaw = Read-MaestroBounded $runsPath 65536
    if ($priorRaw) { $prior = $priorRaw | ConvertFrom-Json; foreach ($p in $prior.PSObject.Properties) { $runs[$p.Name] = $p.Value } }
    $now = [DateTimeOffset]::UtcNow
    $changed = $false
    foreach ($agent in @($policy.agents)) {
        $auto = Get-MaestroProperty $agent 'auto'
        if (-not $auto -or $agent.status -ne 'active' -or -not $agent.dispatchable) { continue }
        $marker = Join-Path $project ([string]$auto.marker)
        if (-not (Test-MaestroPathInside $marker $machine)) { throw 'Invalid agent marker path' }
        Assert-MaestroNoAlias $marker
        if (Test-Path -LiteralPath $marker) { continue }
        $previous = Get-MaestroProperty $runs[$agent.id] 'last_requested'
        $stamp = [DateTimeOffset]::MinValue
        if ($previous -and -not [DateTimeOffset]::TryParse([string]$previous,[ref]$stamp)) { throw 'Invalid agent request timestamp' }
        $age = ($now - $stamp).TotalDays
        $cooldown = [double]$auto.cooldown_days
        if ($cooldown -le 0) { throw 'Invalid agent cooldown' }
        $due = -not $previous -or $age -ge $cooldown
        if ($auto.kind -eq 'on_change' -and $previous -and $due) {
            $due = $false
            foreach ($pattern in @($auto.watch)) {
                foreach ($file in @(Get-ChildItem -Path (Join-Path $project $pattern) -File -ErrorAction SilentlyContinue)) {
                    if ($file.LastWriteTimeUtc -gt $stamp.UtcDateTime) { $due = $true }
                }
            }
        } elseif ($auto.kind -eq 'periodic' -and -not $due -and $age -ge 1) {
            $diagRaw = Read-MaestroBounded (Join-Path $machine 'diagnostics.json') 65536
            if ($diagRaw) {
                $diag = $diagRaw | ConvertFrom-Json
                foreach ($key in @('broken_links','contract_gaps','dead_paths')) { $v = Get-MaestroProperty $diag $key; if ($v -and @($v).Count) { $due = $true } }
            }
        }
        if ($auto.kind -notin @('periodic','on_change') -or -not $due) { continue }
        $when = $now.ToString('yyyy-MM-ddTHH:mm:ssZ')
        $payload = @{agent=$agent.id; subagent_type=$agent.subagent_type; requested_at=$when; reason='Policy cadence due'; packet=$agent.packet; policy='bundles/base/agents/activation-policy.json'; evidence='requested_not_executed'}
        Write-MaestroUtf8 $marker (($payload | ConvertTo-Json -Depth 8) + "`n")
        $runs[$agent.id] = @{last_requested=$when; last_reason='Policy cadence due'}
        $changed = $true
    }
    if ($changed) { $runs['schema_version'] = 1; Write-MaestroUtf8 $runsPath (($runs | ConvertTo-Json -Depth 8) + "`n") }
}

function Invoke-MaestroSessionStopBrainIndex {
    $project = Get-MaestroProjectDir
    if (-not $project -or -not (Test-Path -LiteralPath (Join-Path $project 'brain/.initialized'))) { return }
    $machine = Join-Path $project 'brain/.maestro'
    Assert-MaestroNoAlias $machine
    # Optional compiler: never install or discover ambient Python at Stop.
    $python = $env:MAESTRO_PYTHON
    $state = @{schema_version=1; capability='brain-index'; status='unavailable'; reason='No explicitly selected Python runtime'; observed_at=[DateTime]::UtcNow.ToString('o')}
    if ($python -and [IO.Path]::IsPathRooted($python) -and (Test-Path -LiteralPath $python -PathType Leaf)) {
        Assert-MaestroNoAlias $python
        & $python -c 'import sys; sys.exit(0 if sys.version_info >= (3, 8) else 1)' 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) {
            Push-Location $project
            try { & $python (Join-Path $project 'bundles/base/tools/brain-index.py') 2>$null | Out-Null; $success = $LASTEXITCODE -eq 0 } finally { Pop-Location }
            if ($success) { $state.status='available'; $state.reason='Compiler completed through explicitly selected runtime' }
            else { $state.reason='Compiler failed; existing diagnostics are stale' }
        }
    }
    Write-MaestroUtf8 (Join-Path $machine 'index-capability.json') (($state | ConvertTo-Json) + "`n")
    $failed = Join-Path $machine '.index-failed'
    if ($state.status -eq 'available') { if (Test-Path -LiteralPath $failed) { Remove-Item -LiteralPath $failed -Force } }
    else { Write-MaestroUtf8 $failed ($state.reason + "`n") }
}

function Invoke-MaestroSessionStopEodCheck {
    $project = Get-MaestroProjectDir
    if (-not $project -or -not (Test-Path -LiteralPath (Join-Path $project 'brain/.initialized'))) { return }
    $brain = Join-Path $project 'brain'
    $daily = Join-Path $brain 'daily'
    Assert-MaestroNoAlias $daily
    if (-not (Test-Path -LiteralPath $daily -PathType Container)) { return }
    $today = [DateTime]::Today
    $dates = New-Object 'System.Collections.Generic.List[string]'
    for ($i=0; $i -lt 30; $i++) {
        $date = $today.AddDays(-$i).ToString('yyyy-MM-dd')
        $page = Join-Path $daily ($date + '.md')
        $text = Read-MaestroBounded $page 65536
        if ($text -match '(?m)^### .*fechamento') { break }
        if ($text -or $i -eq 0) { $dates.Insert(0,$date) }
    }
    $pages = @()
    foreach ($file in @(Get-ChildItem -LiteralPath $daily -Filter '*.md' | Where-Object { $_.BaseName -match '^\d{4}-\d{2}-\d{2}$' -and $_.BaseName -lt $today.ToString('yyyy-MM-dd') } | Sort-Object Name -Descending | Select-Object -First 2)) {
        $text = Read-MaestroBounded $file.FullName 65536
        $row = @{date=$file.BaseName; path=('brain/daily/'+$file.Name); briefing_last=$null; fechamento_last=$null}
        foreach ($label in @('briefing','fechamento')) {
            $matches = [regex]::Matches($text, '(?ms)^### \d{2}:\d{2} [^\r\n]*' + $label + '\r?\n(.*?)(?=^### |\z)')
            if ($matches.Count) { $row[$label+'_last'] = Limit-MaestroText $matches[$matches.Count-1].Groups[1].Value.Trim() 600 }
        }
        $pages += $row
    }
    $objectives = @([regex]::Matches((Read-MaestroBounded (Join-Path $brain 'development/objectives.md') 65536),'(?m)^### \d+\. (.+)$') | ForEach-Object { $_.Groups[1].Value.Trim() } | Where-Object { $_ -ne '<objetivo>' })
    $brief = @{generated_at=[DateTime]::UtcNow.ToString('o');today=$today.ToString('yyyy-MM-dd');eod_open_dates=@($dates.ToArray());last_daily_pages=$pages;objectives_active=$objectives}
    Write-MaestroUtf8 (Join-Path $brain '.maestro/day-brief.json') (($brief | ConvertTo-Json -Depth 6) + "`n")
    $marker = Join-Path $brain 'owner/.eod-requested'
    if ($dates.Count) { Write-MaestroUtf8 $marker (($dates -join ',') + "`n") }
    elseif (Test-Path -LiteralPath $marker) { Assert-MaestroNoAlias $marker; Remove-Item -LiteralPath $marker -Force }
}
