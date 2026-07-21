[CmdletBinding()]
param(
    [string]$GitName = "",
    [string]$GitEmail = "",
    [switch]$NonInteractive
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$script:CompletedSteps = @()

function Stop-Onboarding {
    param(
        [string]$Reason,
        [string]$NextAction
    )

    Write-Host "[FALHOU] $Reason" -ForegroundColor Red
    if ($script:CompletedSteps.Count -gt 0) {
        Write-Host "Concluido antes da falha: $($script:CompletedSteps -join '; ')."
    }
    else {
        Write-Host "Nenhuma etapa de configuracao foi concluida."
    }
    Write-Host "O onboarding nao terminou. Nenhum arquivo de trabalho ou historico Git foi apagado."
    Write-Host "Proxima acao segura: $NextAction" -ForegroundColor Yellow
    exit 1
}

function Invoke-Checked {
    param(
        [string]$Label,
        [scriptblock]$Command
    )

    & $Command
    if ($LASTEXITCODE -ne 0) {
        Stop-Onboarding "$Label falhou." "mostre esta mensagem ao Claude e peca uma unica correcao"
    }
}

$scriptDirectory = Split-Path -Parent $MyInvocation.MyCommand.Path
$repositoryRoot = (Resolve-Path (Join-Path $scriptDirectory "..\..")).Path
Push-Location $repositoryRoot

try {
    Write-Host "BCG Brasil Agentic OS - onboarding Windows" -ForegroundColor Cyan

    if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
        Stop-Onboarding "Git nao esta disponivel no PATH." "peca ao Claude para verificar Git.Git no winget antes de instalar"
    }
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Stop-Onboarding "Go nao esta disponivel no PATH." "peca ao Claude para verificar GoLang.Go no winget antes de instalar"
    }
    if (-not (Get-Command claude -ErrorAction SilentlyContinue)) {
        if ($NonInteractive) {
            Write-Host "[AVISO] Claude Code ausente; permitido somente no smoke test nao interativo." -ForegroundColor Yellow
        }
        else {
            Stop-Onboarding "Claude Code nao esta disponivel no PATH." "instale ou atualize o Claude Code pelo canal corporativo antes de continuar"
        }
    }
    else {
        $claudeVersionText = (& claude --version 2>$null | Out-String).Trim()
        if ($claudeVersionText -notmatch '(\d+\.\d+\.\d+)') {
            Stop-Onboarding "nao foi possivel identificar a versao do Claude Code: '$claudeVersionText'." "execute claude doctor e mostre o resultado ao Daniel"
        }
        $claudeVersion = [version]$Matches[1]
        $minimumClaudeVersion = [version]"2.1.177"
        if ($claudeVersion -lt $minimumClaudeVersion) {
            Stop-Onboarding "Claude Code $claudeVersion e anterior ao minimo $minimumClaudeVersion." "execute claude update pelo canal corporativo e rode o onboarding novamente"
        }
        Write-Host "[OK] Claude Code $claudeVersion compativel com os hooks de skills"
        $script:CompletedSteps += "Claude Code compativel"
    }
    if (-not (Test-Path (Join-Path $repositoryRoot ".git"))) {
        Stop-Onboarding "este diretorio nao e um clone Git do projeto." "volte ao prompt de clone e confirme o diretorio escolhido"
    }

    $expectedOrigins = @(
        "https://github.com/agentic-os-brasil/bcg-brasil-agentic-os",
        "https://github.com/agentic-os-brasil/bcg-brasil-agentic-os.git"
    )
    $origin = (& git remote get-url origin 2>$null | Out-String).Trim()
    if ($expectedOrigins -notcontains $origin) {
        Stop-Onboarding "origin nao aponta para o repositorio privado esperado no github.com." "mostre o valor de origin ao Daniel antes de continuar"
    }

    $branch = (& git branch --show-current | Out-String).Trim()
    if ($branch -ne "main") {
        Stop-Onboarding "o clone nao esta na branch main." "use `$recover-work no Claude antes de trocar de branch"
    }

    $initialPending = (& git status --short | Out-String).Trim()
    if (-not [string]::IsNullOrWhiteSpace($initialPending)) {
        Stop-Onboarding "o clone ja possui arquivos locais pendentes." "use `$recover-work no Claude; nao sobrescreva nem descarte os arquivos"
    }
    $script:CompletedSteps += "clone, origin e branch validados"

    $localName = (& git config --local user.name 2>$null | Out-String).Trim()
    $localEmail = (& git config --local user.email 2>$null | Out-String).Trim()
    $effectiveName = (& git config user.name 2>$null | Out-String).Trim()
    $effectiveEmail = (& git config user.email 2>$null | Out-String).Trim()

    if ([string]::IsNullOrWhiteSpace($localName)) {
        if ([string]::IsNullOrWhiteSpace($GitName) -and -not $NonInteractive) {
            $GitName = Read-Host "Nome local para commits (configuracao efetiva atual: '$effectiveName')"
        }
        if (-not [string]::IsNullOrWhiteSpace($GitName)) {
            Invoke-Checked "configuracao do nome Git" { git config --local user.name $GitName }
            $localName = $GitName
        }
    }
    elseif (-not [string]::IsNullOrWhiteSpace($GitName) -and $localName -ne $GitName) {
        Write-Host "[INFO] substituindo nome Git local confirmado: '$localName' -> '$GitName'"
        Invoke-Checked "atualizacao do nome Git" { git config --local user.name $GitName }
        $localName = $GitName
    }

    if ([string]::IsNullOrWhiteSpace($localEmail)) {
        if ([string]::IsNullOrWhiteSpace($GitEmail) -and -not $NonInteractive) {
            $GitEmail = Read-Host "Email local para commits (configuracao efetiva atual: '$effectiveEmail')"
        }
        if (-not [string]::IsNullOrWhiteSpace($GitEmail)) {
            Invoke-Checked "configuracao do email Git" { git config --local user.email $GitEmail }
            $localEmail = $GitEmail
        }
    }
    elseif (-not [string]::IsNullOrWhiteSpace($GitEmail) -and $localEmail -ne $GitEmail) {
        Write-Host "[INFO] substituindo email Git local confirmado: '$localEmail' -> '$GitEmail'"
        Invoke-Checked "atualizacao do email Git" { git config --local user.email $GitEmail }
        $localEmail = $GitEmail
    }

    if ([string]::IsNullOrWhiteSpace($localName) -or [string]::IsNullOrWhiteSpace($localEmail)) {
        if ($NonInteractive) {
            Write-Host "[AVISO] identidade Git ausente; permitido somente no smoke test nao interativo." -ForegroundColor Yellow
        }
        else {
            Stop-Onboarding "nome ou email Git continua ausente." "informe ao Claude seu nome e email corporativo exatos"
        }
    }
    else {
        Write-Host "[OK] identidade Git local configurada para este clone"
        $script:CompletedSteps += "identidade Git local configurada"
    }

    Invoke-Checked "instalacao dos hooks locais" { go run ./dev/harness setup }
    $script:CompletedSteps += "hooks locais instalados"
    Invoke-Checked "validacao inicial" { go run ./dev/harness validate }
    $script:CompletedSteps += "validacao inicial aprovada"

    $pending = (& git status --short | Out-String).Trim()
    if ([string]::IsNullOrWhiteSpace($pending)) {
        Write-Host "[OK] clone limpo e pronto para contribuir" -ForegroundColor Green
    }
    else {
        Write-Host "[AVISO] existem arquivos locais pendentes; nada foi alterado automaticamente." -ForegroundColor Yellow
        Write-Host "Proxima acao segura: use `$recover-work no Claude"
        exit 1
    }

    Write-Host "Proxima acao: feche esta sessao, abra um terminal nesta pasta, inicie uma nova sessao do Claude e escreva:" -ForegroundColor Cyan
    Write-Host 'Use $start-contributing e me guie passo a passo.'
}
finally {
    Pop-Location
}
