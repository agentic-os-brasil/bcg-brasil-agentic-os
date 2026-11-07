#Requires -Version 5.1
[CmdletBinding()]
param([string]$Root)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$Utf8 = New-Object System.Text.UTF8Encoding($false)
$UnsafeAttributes = 0x00000400 -bor 0x00001000 -bor 0x00040000 -bor 0x00400000

function Assert-PlainPath([string]$Path) {
    if ($Path -notmatch '^[A-Za-z]:\\' -or $Path -match '[\x00-\x1f]' -or
        -not [string]::Equals([IO.Path]::GetFullPath($Path), $Path, [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Use um caminho local absoluto, sem atalhos ou segmentos . e ...'
    }
    $drive = New-Object IO.DriveInfo([IO.Path]::GetPathRoot($Path))
    if ($drive.DriveType -ne [IO.DriveType]::Fixed) { throw 'Este ensaio requer um disco local fixo; unidades remotas ou removiveis permanecem indisponiveis.' }
    $current = [IO.Path]::GetPathRoot($Path)
    foreach ($part in $Path.Substring($current.Length).Split([char]'\')) {
        if (-not $part) { throw 'Caminho incompleto.' }
        $current = Join-Path $current $part
        try { $attributes = [IO.File]::GetAttributes($current) }
        catch [IO.FileNotFoundException] { break }
        catch [IO.DirectoryNotFoundException] { break }
        if (([int]$attributes -band $UnsafeAttributes) -ne 0) { throw 'O caminho contem link, reparse ou entrada offline; preserve-o e escolha uma pasta local comum.' }
        if (($attributes -band [IO.FileAttributes]::Directory) -eq 0 -and $current -ne $Path) { throw 'Um ancestral do destino nao e um diretorio.' }
    }
}

function Assert-OwnedDirectory([string]$Path) {
    Assert-PlainPath $Path
    $entry = Get-Item -LiteralPath $Path -Force
    if (-not $entry.PSIsContainer) { throw 'O destino existente nao e um diretorio.' }
    $acl = [IO.Directory]::GetAccessControl($Path, [Security.AccessControl.AccessControlSections]'Owner,Access')
    if ($acl.GetOwner([Security.Principal.SecurityIdentifier]).Value -ne $script:UserSid.Value) { throw 'O diretorio pertence a outro usuario; nenhuma permissao sera alterada.' }
    $ownerAllowed = $false
    foreach ($rule in $acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])) {
        if (($rule.PropagationFlags -band [Security.AccessControl.PropagationFlags]::InheritOnly) -ne 0) { continue }
        if ($rule.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow -or [int]$rule.FileSystemRights -eq 0) { continue }
        if ($script:AllowedSids -notcontains $rule.IdentityReference.Value) { throw 'O diretorio concede acesso a outro principal; os dados existentes foram preservados.' }
        if ($rule.IdentityReference.Value -eq $script:UserSid.Value) { $ownerAllowed = $true }
    }
    if (-not $ownerAllowed) { throw 'O diretorio nao tem uma concessao privada verificavel para este usuario.' }
}

function New-PrivateDirectory([string]$Path) {
    if (Test-Path -LiteralPath $Path) { throw 'A pasta de preparacao ja existe; nada foi substituido.' }
    $acl = New-Object Security.AccessControl.DirectorySecurity
    $acl.SetOwner($script:UserSid)
    $acl.SetAccessRuleProtection($true, $false)
    foreach ($sid in $script:AllowedSids) {
        $principal = New-Object Security.Principal.SecurityIdentifier($sid)
        $rule = New-Object Security.AccessControl.FileSystemAccessRule($principal, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow')
        $acl.AddAccessRule($rule)
    }
    [void][IO.Directory]::CreateDirectory($Path, $acl)
    Assert-OwnedDirectory $Path
}

function Write-NewText([string]$Path, [string]$Text) {
    $stream = New-Object IO.FileStream($Path, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
    try {
        $bytes = $script:Utf8.GetBytes($Text)
        $stream.Write($bytes, 0, $bytes.Length)
        $stream.Flush($true)
    } finally { $stream.Dispose() }
}

function Get-SafeFiles([string]$Directory) {
    foreach ($entry in (Get-Item -LiteralPath $Directory -Force).EnumerateFileSystemInfos()) {
        if (([int]$entry.Attributes -band $script:UnsafeAttributes) -ne 0) { throw 'O inventario contem link, reparse ou entrada offline.' }
        if (($entry.Attributes -band [IO.FileAttributes]::Directory) -ne 0) { Get-SafeFiles $entry.FullName }
        else { $entry }
    }
}

function Get-ManagedInventory([string]$Directory) {
    $lines = @(
        foreach ($section in @('product', 'exports')) {
            $path = Join-Path $Directory $section
            Assert-OwnedDirectory $path
            foreach ($file in @(Get-SafeFiles $path)) {
                $relative = $file.FullName.Substring($Directory.Length + 1).Replace('\', '/')
                '{0}  {1}  {2}' -f (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant(), $file.Length, $relative
            }
        }
    )
    [Array]::Sort($lines, [StringComparer]::Ordinal)
    return ($lines -join "`n") + "`n"
}

function Invoke-Maestro([string[]]$Arguments) {
    $result = @(& $script:Binary @Arguments)
    if ($LASTEXITCODE -ne 0) { throw 'O comando local do Maestro foi recusado. Preserve a preparacao incompleta e a mensagem acima.' }
    return $result -join "`n"
}

try {
    if ($env:OS -ne 'Windows_NT' -or $PSVersionTable.PSEdition -ne 'Desktop') { throw 'Abra Preparar Maestro.cmd no Windows com o Windows PowerShell 5.1 instalado no sistema.' }
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Feche esta janela elevada e abra o preparador como usuario comum. Nenhum estado do ensaio foi criado.' }
    if (-not [Environment]::Is64BitProcess) { throw 'Este pacote requer o Windows PowerShell de 64 bits.' }
    $processors = @(Get-CimInstance -ClassName Win32_Processor -Property Architecture)
    if ($processors.Count -lt 1 -or @($processors | Where-Object { $_.Architecture -ne 9 }).Count -ne 0) { throw 'Este pacote atende Windows x64; ARM64 permanece sem qualificacao neste ensaio.' }
    if (Test-Path Env:MAESTRO_HOME) { throw 'A sessao define MAESTRO_HOME; o ensaio nao remove nem redireciona esse estado. Use uma sessao sem essa selecao explicita.' }
    [Console]::OutputEncoding = $Utf8
    $UserSid = $identity.User
    $AllowedSids = @($UserSid.Value, 'S-1-5-18', 'S-1-5-32-544')
    $kit = [IO.Path]::GetFullPath($PSScriptRoot)
    Assert-PlainPath $kit
    $required = @('Preparar Maestro.cmd', 'Preparar Maestro.ps1', 'LEIA-ME.md', 'ROTEIRO.md', 'LICENSE.md', 'ARTIFACT.txt', 'KIT-ID.txt', 'bin/maestro-mcp-windows-amd64.exe')
    $expected = New-Object 'Collections.Generic.Dictionary[string,string]' ([StringComparer]::Ordinal)
    foreach ($line in [IO.File]::ReadAllLines((Join-Path $kit 'SHA256SUMS'), $Utf8)) {
        if ($line -notmatch '^(?<hash>[0-9a-f]{64})  \./(?<path>.+)$' -or $required -cnotcontains $Matches.path -or $expected.ContainsKey($Matches.path)) { throw 'O inventario do pacote e invalido.' }
        $expected.Add($Matches.path, $Matches.hash)
    }
    if ($expected.Count -ne $required.Count) { throw 'O pacote esta incompleto; extraia o ZIP inteiro.' }
    $actualFiles = @(Get-SafeFiles $kit)
    if ($actualFiles.Count -ne ($required.Count + 1)) { throw 'O pacote contem arquivos inesperados; preserve-o e solicite uma copia integra.' }
    foreach ($file in $actualFiles) {
        $relative = $file.FullName.Substring($kit.Length + 1).Replace('\', '/')
        if ($relative -ceq 'SHA256SUMS') { continue }
        if (-not $expected.ContainsKey($relative) -or (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant() -cne $expected[$relative]) { throw 'A verificacao do pacote falhou; nenhum executavel sera iniciado.' }
    }
    $kitId = [IO.File]::ReadAllText((Join-Path $kit 'KIT-ID.txt'), $Utf8).Trim()
    if ($kitId -notmatch '^maestro-windows-rehearsal-20261105-[0-9a-f]{64}$') { throw 'Identidade local do pacote invalida.' }
    $profile = [Environment]::GetFolderPath([Environment+SpecialFolder]::UserProfile)
    if (-not $profile) { throw 'O perfil local do usuario nao esta disponivel.' }
    $developer = Join-Path $profile 'Developer'
    Assert-PlainPath $developer
    if (-not $Root) { $Root = Join-Path $developer 'MaestroPilot' }
    $Root = $Root.TrimEnd([char]'\')
    Assert-PlainPath $Root
    if (-not $Root.StartsWith($developer + '\', [StringComparison]::OrdinalIgnoreCase)) { throw 'Escolha uma pasta dentro de seu perfil local Developer.' }
    if (-not (Test-Path -LiteralPath $developer)) { New-PrivateDirectory $developer }
    Assert-OwnedDirectory $developer
    if (Test-Path -LiteralPath $Root) {
        Assert-OwnedDirectory $Root
        foreach ($name in @('.maestro-pilot-owned', '.maestro-pilot-complete', 'kit-files.sha256')) {
            $entry = Get-Item -LiteralPath (Join-Path $Root $name) -Force
            if ($entry.PSIsContainer -or ([int]$entry.Attributes -band $UnsafeAttributes) -ne 0) { throw 'A preparacao existente e parcial ou contem um marcador inesperado.' }
        }
        if ([IO.File]::ReadAllText((Join-Path $Root '.maestro-pilot-owned'), $Utf8).Trim() -cne $kitId -or
            [IO.File]::ReadAllText((Join-Path $Root '.maestro-pilot-complete'), $Utf8).Trim() -cne $kitId) { throw 'A pasta nao pertence a este pacote; os arquivos existentes foram preservados.' }
        if ((Get-ManagedInventory $Root) -cne [IO.File]::ReadAllText((Join-Path $Root 'kit-files.sha256'), $Utf8)) { throw 'Os arquivos gerenciados mudaram; preserve a pasta para diagnostico.' }
        $Binary = Join-Path $Root 'product/maestro-mcp.exe'
        Write-Output (Invoke-Maestro (@('inspect', '--product-root', (Join-Path $Root 'product'), '--user-home', (Join-Path $Root 'user'), '--workspace', (Join-Path $Root 'workspace'))))
        Write-Output ('Este piloto ja esta preparado. Instrucoes: ' + (Join-Path $Root 'COMECE-AQUI.txt'))
        exit 0
    }
    $parent = [IO.Path]::GetDirectoryName($Root)
    if (-not (Test-Path -LiteralPath $parent)) { throw 'A pasta pai precisa existir; escolha um destino direto dentro de Developer.' }
    Assert-OwnedDirectory $parent
    $stage = Join-Path $parent ('.maestro-pilot-new-' + [Guid]::NewGuid().ToString('N'))
    New-PrivateDirectory $stage
    Write-NewText (Join-Path $stage '.maestro-pilot-owned') ($kitId + "`n")
    [IO.Directory]::Move($stage, $Root)
    foreach ($name in @('product', 'user', 'exports')) { New-PrivateDirectory (Join-Path $Root $name) }
    $Binary = Join-Path $Root 'product/maestro-mcp.exe'
    [IO.File]::Copy((Join-Path $kit 'bin/maestro-mcp-windows-amd64.exe'), $Binary, $false)
    $binding = @('--product-root', (Join-Path $Root 'product'), '--user-home', (Join-Path $Root 'user'), '--workspace', (Join-Path $Root 'workspace'))
    $initialized = Invoke-Maestro (@('init') + $binding)
    $inspection = (Invoke-Maestro (@('inspect') + $binding)) | ConvertFrom-Json
    if (-not $inspection.writable) { throw 'A preparacao nao obteve HOME gravavel; preserve o estado para diagnostico.' }
    Write-NewText (Join-Path $Root 'diagnostico-init.json') ($initialized + "`n")
    Write-NewText (Join-Path $Root 'workspace/EXEMPLO.md') "# Exemplo do piloto`n`nEste arquivo e sintetico; teste a continuidade entre Code e Desktop.`n"
    $marketplace = Join-Path $Root 'exports/code-marketplace'
    foreach ($name in @('exports/code-marketplace', 'exports/code-marketplace/plugins', 'exports/code-marketplace/.claude-plugin')) { New-PrivateDirectory (Join-Path $Root $name) }
    Write-Output (Invoke-Maestro (@('plugin', '--lifecycle-context') + $binding + @('--server', $Binary, '--destination', (Join-Path $marketplace 'plugins/maestro-product'), '--version', '0.0.0')))
    Write-NewText (Join-Path $marketplace '.claude-plugin/marketplace.json') '{"name":"maestro-pilot-20261105","owner":{"name":"Maestro"},"plugins":[{"name":"maestro-product","source":"./plugins/maestro-product"}]}'
    Write-Output (Invoke-Maestro (@('mcpb') + $binding + @('--server', $Binary, '--destination', (Join-Path $Root 'exports/maestro-pilot.mcpb'), '--version', '0.0.0')))
    $workspaceQuoted = (Join-Path $Root 'workspace').Replace("'", "''")
    $pluginQuoted = (Join-Path $marketplace 'plugins/maestro-product').Replace("'", "''")
    $settings = Join-Path $Root 'exports/code-pilot-settings.json'
    Write-NewText $settings '{"autoMemoryEnabled":false}'
    $settingsQuoted = $settings.Replace("'", "''")
    $codeLauncher = "`$ErrorActionPreference = 'Stop'`r`nSet-Location -LiteralPath '$workspaceQuoted'`r`n& claude --plugin-dir '$pluginQuoted' --setting-sources project,local --settings '$settingsQuoted'`r`nif (`$LASTEXITCODE -ne 0) { exit `$LASTEXITCODE }`r`n"
    Write-NewText (Join-Path $Root 'exports/Abrir Code.ps1') (([string][char]0xfeff) + $codeLauncher)
    Write-NewText (Join-Path $Root 'exports/Abrir Code.cmd') "@echo off`r`nsetlocal DisableDelayedExpansion`r`n`"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe`" -NoLogo -NoProfile -File `"%~dp0Abrir Code.ps1`"`r`nset `"MAESTRO_CODE_EXIT=%errorlevel%`"`r`nif not `"%MAESTRO_CODE_EXIT%`"==`"0`" pause`r`nexit /b %MAESTRO_CODE_EXIT%`r`n"
    Write-NewText (Join-Path $Root 'kit-files.sha256') (Get-ManagedInventory $Root)
    $instructions = @"
MAESTRO - ENSAIO DE DESENVOLVIMENTO

Servidor ordinary unversioned: versao unavailable. O rotulo 0.0.0 identifica apenas os pacotes.
Nao ha assinatura de release ou atualizacao automatica.

Abra este arquivo para iniciar o Code com o plugin deste ensaio:
  $(Join-Path $Root 'exports/Abrir Code.cmd')
O Code precisa estar instalado e autenticado pelo fluxo normal. O launcher carrega o plugin apenas nessa janela, sem cadastro no perfil global.

O plugin inclui hooks de contexto SessionStart, UserPromptSubmit e Stop. Eles exigem que o Claude Code suporte hooks command com shell powershell; nao dependem de Git Bash ou WSL. Configuracao gerada nao comprova execucao no cliente. Guardas, agentes, captura automatica e manutencao recorrente permanecem indisponiveis.

No Desktop, importe pelo fluxo normal:
  $(Join-Path $Root 'exports/maestro-pilot.mcpb')

Compare o workspace_id de maestro_context nos dois clientes com a criacao real:
$initialized

Siga ROTEIRO.md. Estado privado: $(Join-Path $Root 'user')
Se a politica do computador ou do cliente recusar, pare e registre a mensagem.
O preparador nao altera autenticacao, perfis de cliente ou politicas de seguranca.
"@
    Write-NewText (Join-Path $Root 'COMECE-AQUI.txt') ($instructions + "`n")
    Write-NewText (Join-Path $Root '.maestro-pilot-complete') ($kitId + "`n")
    Write-Output $instructions
} catch {
    [Console]::Error.WriteLine('Maestro: ' + $_.Exception.Message + ' Preserve os arquivos existentes; nenhum reparo ou exclusao automatica foi executado.')
    exit 1
}
