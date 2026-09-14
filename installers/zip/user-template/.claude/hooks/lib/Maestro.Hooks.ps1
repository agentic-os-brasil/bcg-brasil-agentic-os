Set-StrictMode -Version 2.0

$script:Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[Console]::OutputEncoding = $script:Utf8NoBom

function Read-MaestroStdin {
    $stdin = [Console]::OpenStandardInput()
    $buffer = New-Object byte[] 4096
    $bytes = New-Object System.IO.MemoryStream
    try {
        while (($count = $stdin.Read($buffer, 0, $buffer.Length)) -gt 0) {
            $bytes.Write($buffer, 0, $count)
        }
        $payload = $bytes.ToArray()
        $offset = 0
        if ($payload.Length -ge 3 -and $payload[0] -eq 0xEF -and $payload[1] -eq 0xBB -and $payload[2] -eq 0xBF) {
            $offset = 3
        }
        return $script:Utf8NoBom.GetString($payload, $offset, $payload.Length - $offset)
    } finally {
        $bytes.Dispose()
    }
}

function Get-MaestroProjectDir {
    if ($env:CLAUDE_PROJECT_DIR) {
        return [System.IO.Path]::GetFullPath($env:CLAUDE_PROJECT_DIR)
    }
    $candidate = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../..'))
    if (Test-Path -LiteralPath (Join-Path $candidate 'VERSION') -PathType Leaf) {
        return $candidate
    }
    $cwd = [System.IO.Path]::GetFullPath((Get-Location).Path)
    if (Test-Path -LiteralPath (Join-Path $cwd 'VERSION') -PathType Leaf) {
        return $cwd
    }
    return $null
}

function Write-MaestroUtf8([string]$Path, [string]$Content) {
    $parent = Split-Path -Parent $Path
    if ($parent -and -not (Test-Path -LiteralPath $parent -PathType Container)) {
        New-Item -ItemType Directory -Path $parent -Force | Out-Null
    }
    [System.IO.File]::WriteAllText($Path, $Content, $script:Utf8NoBom)
}

function Write-MaestroIfMissing([string]$Path, [string]$Content) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        Write-MaestroUtf8 $Path $Content
    }
}

function Append-MaestroUtf8([string]$Path, [string]$Content) {
    $parent = Split-Path -Parent $Path
    if ($parent -and -not (Test-Path -LiteralPath $parent -PathType Container)) {
        New-Item -ItemType Directory -Path $parent -Force | Out-Null
    }
    [System.IO.File]::AppendAllText($Path, $Content, $script:Utf8NoBom)
}

function Read-MaestroTrimmed([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return '' }
    try { return ([System.IO.File]::ReadAllText($Path, $script:Utf8NoBom)).Trim() } catch { return '' }
}

function Ensure-MaestroDirectory([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path -PathType Container)) {
        New-Item -ItemType Directory -Path $Path -Force | Out-Null
    }
}

function ConvertTo-MaestroFold([string]$Text) {
    if ($null -eq $Text) { return '' }
    $normalized = $Text.Normalize([Text.NormalizationForm]::FormD)
    $builder = New-Object Text.StringBuilder
    foreach ($character in $normalized.ToCharArray()) {
        $category = [Globalization.CharUnicodeInfo]::GetUnicodeCategory($character)
        if ($category -ne [Globalization.UnicodeCategory]::NonSpacingMark) {
            [void]$builder.Append($character)
        }
    }
    return $builder.ToString().Normalize([Text.NormalizationForm]::FormC).ToLowerInvariant()
}

function Limit-MaestroText([string]$Text, [int]$Maximum) {
    if ($null -eq $Text) { return '' }
    if ($Text.Length -le $Maximum) { return $Text }
    return $Text.Substring(0, $Maximum)
}

function Get-MaestroProperty($Object, [string]$Name) {
    if ($null -eq $Object) { return $null }
    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property) { return $null }
    return $property.Value
}

function Write-MaestroJson($Object) {
    [Console]::Out.WriteLine(($Object | ConvertTo-Json -Depth 12 -Compress))
}

function Get-MaestroLifetimePolicy([string]$Origin) {
    return @"
{
  "schema_version": 1,
  "policy_id": "deterministic-l3-continuity-v1",
  "description": "Promove memória permanente somente quando a consolidação semanal já carrega duas gerações de L3, preservando continuidade antes de tornar algo permanente.",
  "min_l3_generations": 2,
  "promotion": "weekly_deep_dream",
  "automatic": true,
  "versioned_updates": true,
  "direct_overwrite": false,
  "provenance_required": true,
  "initialized_by": "$Origin"
}
"@
}

function Ensure-MaestroMemoryBackfill([string]$DataDir, [string]$Origin) {
    $memory = Join-Path $DataDir 'memory'
    Ensure-MaestroDirectory $memory
    foreach ($tier in @('recent', 'weekly', 'medium-term', 'lifetime', 'policies')) {
        Ensure-MaestroDirectory (Join-Path $memory $tier)
    }
    Write-MaestroIfMissing (Join-Path $memory '.gitignore') ".dream-requested`n"
    Write-MaestroIfMissing (Join-Path $memory 'policies/lifetime.json') (Get-MaestroLifetimePolicy $Origin)
    Write-MaestroIfMissing (Join-Path $memory '.schema-version') @"
{
  "schema_version": 1,
  "layers": ["recent", "weekly", "medium-term", "lifetime", "policies"],
  "policy_source": "bundles/base/memory/policy.json",
  "initialized_by": "$Origin"
}
"@
}

function Write-MaestroSkillsRollup([string]$ProjectDir) {
    $bundles = @(
        @('bundles/base/skills', 'Maestro skills disponíveis', 'Índice compacto. A skill completa é carregada sob demanda quando o pedido do dono a aciona.'),
        @('bundles/tech-core/skills', 'Skills técnicas (tech-core)', 'Skills de engenharia — testes, revisão, pipelines de dados, entrega por spec. Carregadas sob demanda.')
    )
    foreach ($bundle in $bundles) {
        $dir = Join-Path $ProjectDir $bundle[0]
        if (-not (Test-Path -LiteralPath $dir -PathType Container)) { continue }
        $rows = @()
        foreach ($file in @(Get-ChildItem -LiteralPath $dir -Filter SKILL.md -Recurse -ErrorAction SilentlyContinue)) {
            try {
                $text = [System.IO.File]::ReadAllText($file.FullName, $script:Utf8NoBom)
                $name = [regex]::Match($text, '(?m)^name:\s*["'']?([^\r\n"'']+)').Groups[1].Value.Trim()
                $description = [regex]::Match($text, '(?m)^description:\s*["'']?([^\r\n"'']+)').Groups[1].Value.Trim()
                if ($name -and $description) {
                    $first = [regex]::Split($description, '\.\s+')[0]
                    if ($first.Length -gt 140) { $first = $first.Substring(0, 137) + '...' }
                    $rows += "- **$name** — $first"
                }
            } catch {}
        }
        if ($rows.Count -gt 0) {
            [Console]::Out.WriteLine("## $($bundle[1])`n")
            [Console]::Out.WriteLine("$($bundle[2])`n")
            foreach ($row in @($rows | Sort-Object)) { [Console]::Out.WriteLine($row) }
            [Console]::Out.WriteLine('')
        }
    }
}

function Write-MaestroActiveCaseContext([string]$ProjectDir, [string]$DataDir) {
    $cases = Join-Path $DataDir 'cases'
    $caseId = Read-MaestroTrimmed (Join-Path $cases '.active')
    if (-not $caseId) { return }
    $caseDir = Join-Path $cases $caseId
    if (-not (Test-Path -LiteralPath $caseDir -PathType Container)) { return }
    [Console]::Out.WriteLine("## Caso ativo: $caseId`n")
    $projects = Join-Path $caseDir 'brain/projects'
    $brief = @(Get-ChildItem -LiteralPath $projects -Filter '*.md' -ErrorAction SilentlyContinue | Sort-Object Name | Select-Object -First 1)
    if ($brief.Count -gt 0) {
        [Console]::Out.WriteLine("### Brief`n")
        [Console]::Out.WriteLine(((@(Get-Content -LiteralPath $brief[0].FullName -Encoding UTF8 | Select-Object -First 25)) -join "`n"))
        [Console]::Out.WriteLine('')
    }
    $decisions = Join-Path $caseDir 'brain/decisions/decision-log.md'
    if (Test-Path -LiteralPath $decisions -PathType Leaf) {
        $heads = @(Get-Content -LiteralPath $decisions -Encoding UTF8 | Where-Object { $_ -match '^## D-[0-9]+' } | Select-Object -Last 5)
        if ($heads.Count -gt 0) {
            [Console]::Out.WriteLine("### Últimas decisões`n")
            foreach ($head in $heads) { [Console]::Out.WriteLine('- ' + $head.Substring(3)) }
            [Console]::Out.WriteLine('')
        }
    }
    $tasks = Join-Path $caseDir 'brain/tasks'
    $taskFiles = @(Get-ChildItem -LiteralPath $tasks -Filter '*.md' -ErrorAction SilentlyContinue)
    if ($taskFiles.Count -gt 0) {
        [Console]::Out.WriteLine("### Tarefas abertas ($($taskFiles.Count))`n")
        foreach ($task in @($taskFiles | Sort-Object Name | Select-Object -First 10)) {
            [Console]::Out.WriteLine('- ' + $task.BaseName)
        }
        [Console]::Out.WriteLine('')
    }
}

function Invoke-MaestroFirstRunScaffold {
    $project = Get-MaestroProjectDir
    if (-not $project) {
        [Console]::Error.WriteLine('maestro first-run-scaffold: CLAUDE_PROJECT_DIR unset and no VERSION found nearby — skipping scaffold (fail-open).')
        return
    }
    $data = Join-Path $project 'data'
    $marker = Join-Path $data '.initialized'
    $log = Join-Path $data '.scaffold.log'
    $timestamp = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
    function Log([string]$Line) { try { Append-MaestroUtf8 $log "$timestamp  $Line`n" } catch {} }

    if ((Test-Path -LiteralPath $data -PathType Container) -and -not (Test-Path -LiteralPath $marker -PathType Leaf)) {
        $agents = Join-Path $data 'agents'
        if ((Test-Path -LiteralPath $agents -PathType Container) -and @(Get-ChildItem -LiteralPath $agents -Force -ErrorAction SilentlyContinue).Count -gt 0) {
            $timestampFile = [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssZ')
            Write-MaestroUtf8 (Join-Path $data ".recovered-$timestampFile") "$timestamp`n"
            Log 'RECOVERY  data/agents has content, marker missing — breadcrumb written'
        }
    }

    if (Test-Path -LiteralPath $data -PathType Container) {
        Ensure-MaestroMemoryBackfill $data 'first-run-scaffold.ps1 (backfill)'
    }

    if (Test-Path -LiteralPath $marker -PathType Leaf) {
        $running = Read-MaestroTrimmed (Join-Path $project 'VERSION')
        $installedPath = Join-Path $data '.maestro-version'
        $installed = Read-MaestroTrimmed $installedPath
        if ($running -and -not $installed) {
            Write-MaestroUtf8 $installedPath "$running`n"
        } elseif ($running -and $installed -and $running -ne $installed) {
            $upgrade = [ordered]@{
                from_version = $installed
                to_version = $running
                detected_at = $timestamp
                action = 'run /maestro-setup-update to complete the migration'
            }
            Write-MaestroUtf8 (Join-Path $data '.upgrade-pending') (($upgrade | ConvertTo-Json -Depth 4) + "`n")
        }
        Write-MaestroSkillsRollup $project
        Write-MaestroActiveCaseContext $project $data
        return
    }

    foreach ($relative in @(
        'agents','canary','cases','memory','owner','profile','workspaces',
        'owner/self','owner/operating','owner/observations','owner/interview/drafts',
        'owner/atlas/daily','owner/atlas/craft/methods','owner/atlas/craft/style',
        'owner/atlas/learnings','owner/atlas/development/cdc',
        'owner/atlas/development/project-feedback','owner/atlas/development/upward-feedback'
    )) { Ensure-MaestroDirectory (Join-Path $data $relative) }
    Ensure-MaestroMemoryBackfill $data 'first-run-scaffold.ps1'

    foreach ($facet in @('owner-identity','personal-context','professional-role','communication-style','voice','preferences','motivations','quality-bar','decision-rules','working-boundaries')) {
        Write-MaestroIfMissing (Join-Path $data "owner/self/$facet.md") "# $facet`n`n## Current`n`n_Não preenchido. Use /maestro-onboarding para configurar._`n"
    }
    Write-MaestroIfMissing (Join-Path $data 'owner/registry.json') @'
{
  "schema_version": 1,
  "trees": {
    "self": "owner/self/",
    "operating": "owner/operating/",
    "observations": "owner/observations/",
    "interview": "owner/interview/"
  },
  "initialized": false,
  "owner_type": null,
  "personal_context": {
    "state": "not_asked",
    "state_timestamp": null,
    "source_file": "owner/self/personal-context.md"
  }
}
'@
    Write-MaestroIfMissing (Join-Path $data 'owner/self/README.md') @'
# SELF — Owner Context Facets

Dez arquivos individualmente endereçáveis. Preenchidos por /maestro-onboarding.

| Facet | Arquivo |
|---|---|
| Identidade | owner-identity.md |
| Contexto pessoal | personal-context.md |
| Papel profissional | professional-role.md |
| Estilo de comunicação | communication-style.md |
| Voz | voice.md |
| Preferências | preferences.md |
| Motivações | motivations.md |
| Barra de qualidade | quality-bar.md |
| Regras de decisão | decision-rules.md |
| Limites de trabalho | working-boundaries.md |
'@
    Write-MaestroIfMissing (Join-Path $data 'owner/operating/work-state.md') @'
# Work State

_Não inicializado. Atualizado automaticamente pelo Maestro ao final de cada sessão de trabalho._

## Last session
- date: —
- active_project: —
- last_decision: —

## Open threads
_Nenhum registrado._
'@
    Write-MaestroIfMissing (Join-Path $data 'owner/observations/observations.jsonl') ''
    Write-MaestroIfMissing (Join-Path $data 'owner/atlas/craft/index.md') @'
# Craft

Métodos e calibrações de estilo que se mantêm verdadeiros entre projetos.

- `methods/` — técnicas reutilizáveis, cada uma em sua própria página.
- `style/` — como você calibra o trabalho em uma situação específica.

_Ainda vazio. As páginas são criadas por `/craft-update` e `/learnings-bridge`._
'@
    Write-MaestroIfMissing (Join-Path $data 'owner/atlas/learnings/index.md') @'
# Learnings

Aprendizados profissionais duráveis, corrigíveis e ligados às suas fontes quando aplicável.

_Ainda vazio. As páginas são criadas por `/learnings-bridge`._
'@
    Write-MaestroIfMissing (Join-Path $data 'owner/atlas/development/objectives.md') @'
# Objetivos de desenvolvimento

O que você está tentando desenvolver neste período, e o que conta como progresso.

_Não preenchido. Diga "quero definir meus objetivos" para preencher._

## Objetivos ativos

### 1. <objetivo>
- **Status:** ativo | em risco | atingido
- **Como saber que avançou:**
- **Última confirmação:** YYYY-MM-DD
- **Próxima revisão:** YYYY-MM-DD

#### Evidência — objetivo 1

<!-- append-only: uma linha datada por evidência, citando a origem -->

## Aposentados

<!-- append-only: uma linha datada por objetivo aposentado, nomeando a review que o aposentou -->
'@
    Write-MaestroIfMissing (Join-Path $data 'owner/interview/confirmations.json') @'
{
  "schema_version": 1,
  "completed_tracks": [],
  "last_updated": null
}
'@
    Write-MaestroIfMissing (Join-Path $data 'README.md') @'
# data/ — sua workspace do Maestro

Tudo dentro de `data/` é seu. Atualizações do Maestro nunca sobrescrevem este diretório.

- `agents/` — estado de cada agente
- `cases/` — casos ativos e seus brains
- `memory/` — memória de longo prazo
- `profile/` — identidade e preferências
- `workspaces/` — projetos ativos

Se quiser fazer backup, basta copiar `data/` inteiro.
'@
    Write-MaestroIfMissing (Join-Path $data 'profile/identity.json') @'
{
  "schema_version": 1,
  "display_name": "",
  "role": "",
  "context": "",
  "initialized": false
}
'@
    Write-MaestroUtf8 $marker "$timestamp`n"
    $version = Read-MaestroTrimmed (Join-Path $project 'VERSION')
    if ($version) { Write-MaestroUtf8 (Join-Path $data '.maestro-version') "$version`n" }
    Log 'DONE  marker written'
    Write-MaestroSkillsRollup $project
    Write-MaestroActiveCaseContext $project $data
}

function Write-MaestroLatestFile([string]$Label, [string]$Directory) {
    if (-not (Test-Path -LiteralPath $Directory -PathType Container)) { return }
    $file = @(Get-ChildItem -LiteralPath $Directory -Filter '*.md' -ErrorAction SilentlyContinue | Sort-Object Name | Select-Object -Last 1)
    if ($file.Count -eq 0) { return }
    [Console]::Out.WriteLine("`n## $Label")
    [Console]::Out.WriteLine("<!-- source: $($file[0].Name) -->")
    [Console]::Out.Write([System.IO.File]::ReadAllText($file[0].FullName, $script:Utf8NoBom))
}

function Write-MaestroAllFiles([string]$Label, [string]$Directory) {
    if (-not (Test-Path -LiteralPath $Directory -PathType Container)) { return }
    $files = @(Get-ChildItem -LiteralPath $Directory -Filter '*.md' -ErrorAction SilentlyContinue | Sort-Object Name)
    if ($files.Count -eq 0) { return }
    [Console]::Out.WriteLine("`n## $Label")
    foreach ($file in $files) {
        [Console]::Out.WriteLine("`n<!-- source: $($file.Name) -->")
        [Console]::Out.Write([System.IO.File]::ReadAllText($file.FullName, $script:Utf8NoBom))
    }
}

function Write-MaestroProfile([string]$Label, [string]$Path) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return }
    try {
        $raw = [System.IO.File]::ReadAllText($Path, $script:Utf8NoBom)
        $profile = $raw | ConvertFrom-Json
        $initialized = Get-MaestroProperty $profile 'initialized'
        if ($initialized -is [bool] -and -not $initialized) { return }
        [Console]::Out.WriteLine("`n## $Label")
        [Console]::Out.WriteLine('```json')
        [Console]::Out.Write($raw)
        [Console]::Out.WriteLine('```')
    } catch {}
}

function Invoke-MaestroSessionStartMemoryInject {
    $project = Get-MaestroProjectDir
    if (-not $project) { return }
    $data = Join-Path $project 'data'
    if (-not (Test-Path -LiteralPath $data -PathType Container)) { return }
    $memory = Join-Path $data 'memory'
    $profile = Join-Path $data 'profile'
    [Console]::Out.WriteLine('<!-- maestro:session-context:start -->')
    [Console]::Out.WriteLine('# Maestro — Contexto da sessão')
    [Console]::Out.WriteLine('_Injetado automaticamente pelo hook de início de sessão._')
    [Console]::Out.WriteLine("`n## Método operacional")
    [Console]::Out.WriteLine('<!-- maestro:pointer: maestro-operator · reason: deterministic_operational_method -->')
    [Console]::Out.WriteLine("Skill: $project/bundles/base/skills/maestro-operator/SKILL.md")
    [Console]::Out.WriteLine('Instrução: carregar este skill antes de escolher, interpretar ou recuperar qualquer operação de controle do Maestro.')
    $tech = Join-Path $project 'bundles/tech-core/skills'
    if (Test-Path -LiteralPath $tech -PathType Container) {
        [Console]::Out.WriteLine("`n## Skills técnicas (tech-core)")
        [Console]::Out.WriteLine('<!-- maestro:pointer: tech-core · reason: engineering_skills_bundle -->')
        [Console]::Out.WriteLine("Bundle path: $project/bundles/tech-core")
        if (Test-Path -LiteralPath (Join-Path $tech 'INDEX.md')) { [Console]::Out.WriteLine("Índice: $tech/INDEX.md") }
        if (Test-Path -LiteralPath (Join-Path $tech 'catalog.json')) { [Console]::Out.WriteLine("Catálogo: $tech/catalog.json") }
    }
    $dream = Join-Path $memory '.dream-requested'
    if (Test-Path -LiteralPath $dream -PathType Leaf) {
        [Console]::Out.WriteLine("`n## ⚠️ Dreaming pendente — executar antes de qualquer outra tarefa")
        [Console]::Out.WriteLine("<!-- maestro:dream-trigger: marker=$dream -->")
        [Console]::Out.WriteLine('**Ação obrigatória:** leia `bundles/base/skills/dream-memory/SKILL.md` e execute o ciclo diário como primeira ação desta sessão.')
    }
    $upgrade = Join-Path $data '.upgrade-pending'
    if (Test-Path -LiteralPath $upgrade -PathType Leaf) {
        [Console]::Out.WriteLine("`n## ⚠️ Upgrade Maestro pendente — verificar antes de qualquer outra tarefa")
        [Console]::Out.WriteLine("<!-- maestro:upgrade-trigger: marker=$upgrade -->")
        [Console]::Out.WriteLine('```json')
        [Console]::Out.Write([System.IO.File]::ReadAllText($upgrade, $script:Utf8NoBom))
        [Console]::Out.WriteLine('```')
        [Console]::Out.WriteLine('**Ação obrigatória:** invoque `/maestro-setup-update` antes de seguir.')
    }
    Write-MaestroProfile 'Identidade do usuário' (Join-Path $profile 'identity.json')
    Write-MaestroProfile 'Preferências e estilo' (Join-Path $profile 'style.json')
    Write-MaestroAllFiles 'SELF do usuário' (Join-Path $data 'owner/self')
    Write-MaestroAllFiles 'Memória de longo prazo (L3)' (Join-Path $memory 'lifetime')
    Write-MaestroLatestFile 'Resumo semanal (L2)' (Join-Path $memory 'weekly')
    Write-MaestroLatestFile 'Último log diário consolidado (L1)' (Join-Path $memory 'recent')
    [Console]::Out.WriteLine("`n<!-- maestro:session-context:end -->")
}

function Get-MaestroAgentRoute([string]$Project, [string]$Prompt) {
    $policyPath = Join-Path $Project 'bundles/base/agents/activation-policy.json'
    if (-not (Test-Path -LiteralPath $policyPath -PathType Leaf)) { return '' }
    try { $policy = Get-Content -LiteralPath $policyPath -Raw -Encoding UTF8 | ConvertFrom-Json } catch { return '' }
    $foldedPrompt = ConvertTo-MaestroFold $Prompt
    $hits = @()
    foreach ($agent in @($policy.agents)) {
        if ($agent.layer -ne 'spoke' -or $agent.status -ne 'active' -or -not $agent.dispatchable) { continue }
        $score = 0
        $agentId = ConvertTo-MaestroFold ([string]$agent.id)
        if ($foldedPrompt.Trim([char[]]@(' ', '$', '/', '@')) -eq $agentId) { $score = 999 }
        if ($score -eq 0) {
            foreach ($trigger in @($agent.triggers)) {
                $foldedTrigger = ConvertTo-MaestroFold ([string]$trigger)
                if ($foldedTrigger -and $foldedPrompt.Contains($foldedTrigger)) { $score = [Math]::Max($score, 100) }
                $words = @([regex]::Matches($foldedTrigger, '[a-z0-9-]{4,}') | ForEach-Object { $_.Value } | Select-Object -Unique)
                $matched = @($words | Where-Object { $foldedPrompt -match ('(?<![a-z0-9])' + [regex]::Escape($_) + '(?![a-z0-9])') }).Count
                if ($matched -ge 2) { $score = [Math]::Max($score, $matched * 10) }
            }
        }
        if ($score -gt 0) { $hits += [pscustomobject]@{ Agent=$agent; Score=$score } }
    }
    $hits = @($hits | Sort-Object @{Expression='Score';Descending=$true}, @{Expression={$_.Agent.id};Descending=$false} | Select-Object -First 2)
    if ($hits.Count -eq 0) { return '' }
    $lines = New-Object System.Collections.Generic.List[string]
    [void]$lines.Add('<!-- maestro:agent-route -->')
    [void]$lines.Add('Agente(s) que este pedido provavelmente exige:')
    foreach ($hit in $hits) {
        $agent = $hit.Agent
        [void]$lines.Add("- **$($agent.emoji) ``$($agent.id)``** — $($agent.when)")
        if ($agent.gate) { [void]$lines.Add("  - Portão: passar por ``$($agent.gate)`` antes.") }
        if (@($agent.packet).Count -gt 0) { [void]$lines.Add('  - Pacote fechado (vai inteiro no prompt): ' + (@($agent.packet) -join '; ') + '.') }
        [void]$lines.Add("  - Despachar com a ferramenta Agent, ``subagent_type: `"$($agent.subagent_type)`"``.")
        if ($policy.announce.required) {
            $format = [string]$policy.announce.format
            $line = $format.Replace('{emoji}', [string]$agent.emoji).Replace('{display}', [string]$agent.id)
            [void]$lines.Add("  - **Anuncie no chat ANTES de despachar**, assim: $line")
        }
    }
    [void]$lines.Add('Regra: o spoke não busca contexto e o veredito dele é insumo, não saída.')
    return ($lines -join "`n") + "`n"
}

function Invoke-MaestroContextInject {
    $project = Get-MaestroProjectDir
    if (-not $project) { return }
    $inputRaw = Read-MaestroStdin
    $prompt = ''
    try { $prompt = [string](Get-MaestroProperty ($inputRaw | ConvertFrom-Json) 'prompt') } catch {}
    if ($prompt) {
        $route = Get-MaestroAgentRoute $project $prompt
        if ($route) { [Console]::Out.Write((Limit-MaestroText $route 1600)) }
    }
    $data = Join-Path $project 'data'
    $state = $env:MAESTRO_STATE_DIR
    if (-not $state) {
        if ($env:USERPROFILE) { $state = Join-Path $env:USERPROFILE '.claude/state' }
        else { $state = Join-Path ([System.IO.Path]::GetTempPath()) 'maestro-claude-state' }
    }
    try { Ensure-MaestroDirectory $state } catch { $state = [System.IO.Path]::GetTempPath() }
    $sessionId = $env:CLAUDE_SESSION_ID
    if (-not $sessionId) { $sessionId = "$PID-$([DateTime]::UtcNow.ToString('yyyyMMdd'))" }
    $safeSession = [regex]::Replace($sessionId, '[^A-Za-z0-9._-]', '_')
    $marker = Join-Path $state "context-inject-$safeSession.marker"
    if (Test-Path -LiteralPath $marker -PathType Leaf) {
        $stub = "<!-- maestro:context-inject:stub -->`nMemory: $project/data/memory/ · Load specific tiers on demand.`n"
        [Console]::Out.Write((Limit-MaestroText $stub 160))
        return
    }
    try { Write-MaestroUtf8 $marker '' } catch {}
    $lines = New-Object System.Collections.Generic.List[string]
    [void]$lines.Add('<!-- maestro:context-inject:first -->')
    [void]$lines.Add('# Context pointers')
    $identityPath = Join-Path $data 'profile/identity.json'
    if (Test-Path -LiteralPath $identityPath -PathType Leaf) {
        try {
            $identity = Get-Content -LiteralPath $identityPath -Raw -Encoding UTF8 | ConvertFrom-Json
            $initialized = Get-MaestroProperty $identity 'initialized'
            if (-not ($initialized -is [bool] -and -not $initialized)) {
                $parts = @()
                foreach ($key in @('name','display_name','role','track')) {
                    $value = Get-MaestroProperty $identity $key
                    if ($value) { $parts += "$key=$value" }
                }
                if ($parts.Count -gt 0) { [void]$lines.Add('Identity: ' + ($parts -join ' · ')) }
            }
        } catch {}
    }
    $memory = Join-Path $data 'memory'
    [void]$lines.Add("Memory: $memory/")
    if (Test-Path -LiteralPath (Join-Path $memory 'MEMORY.md')) { [void]$lines.Add("Memory index: $memory/MEMORY.md") }
    if (Test-Path -LiteralPath (Join-Path $memory 'decisions/decision-log.md')) { [void]$lines.Add("Decision log: $memory/decisions/decision-log.md") }
    if (Test-Path -LiteralPath $identityPath) { [void]$lines.Add("Profile: $identityPath") }
    [void]$lines.Add('Reminder: full memory tiers are loaded at SessionStart. Read specific files on demand — do not request a re-dump each turn.')
    [Console]::Out.Write((Limit-MaestroText (($lines -join "`n") + "`n") 1600))
}

function Resolve-MaestroPathAliases([string]$Path) {
    $full = [System.IO.Path]::GetFullPath($Path)
    $root = [System.IO.Path]::GetPathRoot($full)
    $relative = $full.Substring($root.Length)
    $segments = @($relative -split '[\\/]+' | Where-Object { $_ })
    $current = $root
    for ($index = 0; $index -lt $segments.Count; $index++) {
        $candidate = Join-Path $current $segments[$index]
        if (-not (Test-Path -LiteralPath $candidate)) {
            for ($rest = $index; $rest -lt $segments.Count; $rest++) { $current = Join-Path $current $segments[$rest] }
            return [System.IO.Path]::GetFullPath($current)
        }
        $item = Get-Item -LiteralPath $candidate -Force -ErrorAction Stop
        if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            $targetProperty = $item.PSObject.Properties['Target']
            if ($null -eq $targetProperty -or $null -eq $targetProperty.Value) { throw "reparse target unavailable: $candidate" }
            $target = @($targetProperty.Value)[0]
            if (-not [System.IO.Path]::IsPathRooted([string]$target)) {
                $candidateParent = Split-Path -Parent $candidate
                if (-not $candidateParent) { $candidateParent = $root }
                $target = Join-Path $candidateParent ([string]$target)
            }
            $current = [System.IO.Path]::GetFullPath([string]$target)
        } else {
            $current = $candidate
        }
    }
    return [System.IO.Path]::GetFullPath($current)
}

function Test-MaestroPathInside([string]$Child, [string]$Parent) {
    $childFull = [System.IO.Path]::GetFullPath($Child).TrimEnd([char[]]@('\','/'))
    $parentFull = [System.IO.Path]::GetFullPath($Parent).TrimEnd([char[]]@('\','/'))
    if ($childFull.Equals($parentFull, [StringComparison]::OrdinalIgnoreCase)) { return $true }
    $prefix = $parentFull + [System.IO.Path]::DirectorySeparatorChar
    return $childFull.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)
}

function Stop-MaestroCrossCase([string]$Reason) {
    [Console]::Error.WriteLine("Cross-case write blocked. $Reason")
    exit 2
}

function Invoke-MaestroCrossCaseGuard {
    $project = Get-MaestroProjectDir
    $raw = Read-MaestroStdin
    if (-not $raw) { return }
    try { $payload = $raw | ConvertFrom-Json } catch {
        if ($raw -match 'file_path|notebook_path') { Stop-MaestroCrossCase 'The guard could not parse this file-writing call, so client isolation cannot be verified.' }
        return
    }
    $tool = [string](Get-MaestroProperty $payload 'tool_name')
    if (@('Edit','MultiEdit','Write','NotebookEdit') -notcontains $tool) { return }
    $inputObject = Get-MaestroProperty $payload 'tool_input'
    $key = 'file_path'
    if ($tool -eq 'NotebookEdit') { $key = 'notebook_path' }
    $target = [string](Get-MaestroProperty $inputObject $key)
    if (-not $target) { Stop-MaestroCrossCase 'A file-writing call arrived without a readable target path.' }
    if (-not $project) { Stop-MaestroCrossCase 'The project root is unavailable, so client isolation cannot be verified.' }
    if (-not [System.IO.Path]::IsPathRooted($target)) { $target = Join-Path $project $target }
    $cases = Join-Path $project 'data/cases'
    try {
        $lexicalTarget = [System.IO.Path]::GetFullPath($target)
        $lexicalCases = [System.IO.Path]::GetFullPath($cases)
        $resolvedTarget = Resolve-MaestroPathAliases $lexicalTarget
        $resolvedCases = Resolve-MaestroPathAliases $lexicalCases
    } catch {
        Stop-MaestroCrossCase 'The guard could not resolve filesystem aliases for this target, so client isolation cannot be verified.'
    }
    $lexicalInside = Test-MaestroPathInside $lexicalTarget $lexicalCases
    $resolvedInside = Test-MaestroPathInside $resolvedTarget $resolvedCases
    if ($lexicalInside -and -not $resolvedInside) { Stop-MaestroCrossCase 'The requested path is written inside data/cases but resolves outside that tree through a filesystem alias.' }
    if (-not $resolvedInside) { return }
    $relative = $resolvedTarget.Substring($resolvedCases.TrimEnd([char[]]@('\','/')).Length).TrimStart([char[]]@('\','/'))
    $targetCase = @($relative -split '[\\/]+')[0]
    if (-not $targetCase) { return }
    if ($targetCase.StartsWith('.')) { return }
    $activeFile = Join-Path $cases '.active'
    $active = (Read-MaestroTrimmed $activeFile).ToLowerInvariant()
    $targetCase = $targetCase.ToLowerInvariant()
    if (-not $active) { Stop-MaestroCrossCase "No active case is recorded, and this write targets case '$targetCase'." }
    if ($targetCase -eq $active) { return }
    $pending = (Read-MaestroTrimmed (Join-Path $cases '.pending')).ToLowerInvariant()
    if ($pending -and $targetCase -eq $pending) { return }
    Stop-MaestroCrossCase "Active case: $active. This write targets case: $targetCase. Ask the owner to confirm a case switch first."
}

function Invoke-MaestroAgentAnnouncement {
    $project = Get-MaestroProjectDir
    if (-not $project) { return }
    $raw = Read-MaestroStdin
    if (-not $raw -or $raw -notmatch 'subagent_type') { return }
    try { $payload = $raw | ConvertFrom-Json } catch { return }
    $tool = [string](Get-MaestroProperty $payload 'tool_name')
    if (@('Agent','Task') -notcontains $tool) { return }
    $agentType = [string](Get-MaestroProperty (Get-MaestroProperty $payload 'tool_input') 'subagent_type')
    if (-not $agentType) { return }
    $policyPath = Join-Path $project 'bundles/base/agents/activation-policy.json'
    try { $policy = Get-Content -LiteralPath $policyPath -Raw -Encoding UTF8 | ConvertFrom-Json } catch { return }
    if (-not $policy.announce.required) { return }
    $agent = @($policy.agents | Where-Object { $_.subagent_type -eq $agentType -or $_.id -eq $agentType } | Select-Object -First 1)
    $rule = [string]$policy.announce.rule
    if ($agent.Count -eq 0) {
        $context = "[maestro] Você despachou o subagente ``$agentType``, que não está na política de agentes. $rule Diga na resposta qual subagente foi ativado e por quê."
    } elseif ($agent[0].status -ne 'active' -or -not $agent[0].dispatchable) {
        $context = "[maestro] ATENÇÃO: ``$($agent[0].id)`` está declarado como não despachável. Reveja a rota antes de usar o resultado."
    } else {
        $format = [string]$policy.announce.format
        $line = $format.Replace('{emoji}', [string]$agent[0].emoji).Replace('{display}', [string]$agent[0].id)
        $context = "[maestro] Despacho de $($agent[0].emoji) ``$($agent[0].id)`` em curso. $rule Anuncie assim, se ainda não anunciou: $line"
        if ($policy.announce.on_return) { $context += ' Na volta: ' + [string]$policy.announce.on_return }
    }
    Write-MaestroJson ([ordered]@{hookSpecificOutput=[ordered]@{hookEventName='PreToolUse';additionalContext=$context}})
}

function Invoke-MaestroSessionStopDream {
    $project = Get-MaestroProjectDir
    if (-not $project) { return }
    $memory = Join-Path $project 'data/memory'
    if (-not (Test-Path -LiteralPath $memory -PathType Container)) { return }
    $marker = Join-Path $memory '.dream-requested'
    $today = [DateTime]::Now.ToString('yyyy-MM-dd')
    if (Test-Path -LiteralPath (Join-Path $memory "recent/$today.md") -PathType Leaf) {
        Remove-Item -LiteralPath $marker -Force -ErrorAction SilentlyContinue
        return
    }
    Write-MaestroUtf8 $marker ([DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ') + "`n")
}
