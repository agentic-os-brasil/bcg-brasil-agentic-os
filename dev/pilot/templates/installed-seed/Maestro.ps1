param([switch]$CheckOnly)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$Utf8 = New-Object System.Text.UTF8Encoding($false)
$OutputEncoding = $Utf8
[Console]::OutputEncoding = $Utf8
function Stop-Maestro([string]$Message) {
  $reportVariable = Get-Variable -Name Report -Scope Script -ErrorAction SilentlyContinue
  if ($null -ne $reportVariable -and (Test-Path -LiteralPath $reportVariable.Value -PathType Leaf)) { [IO.File]::AppendAllText($reportVariable.Value, "`nEtapa pendente: $Message`n", $Utf8) }
  throw "$Message`nOs arquivos existentes foram preservados. Compartilhe apenas RELATORIO.md com o time."
}
function Hash([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Write-Json([string]$Path, $Value) {
  if (Test-Path -LiteralPath $Path) { Stop-Maestro 'Destino JSON ja existe; preserve-o.' }
  [IO.File]::WriteAllText($Path, ($Value | ConvertTo-Json -Depth 16), $Utf8)
}
function Private-Directory([string]$Path) {
  $Path = [IO.Path]::GetFullPath($Path)
  if (-not $Path.StartsWith($Dev + '\', [StringComparison]::OrdinalIgnoreCase)) { Stop-Maestro 'Os dados privados devem ficar em Developer.' }
  $walk = $Path
  while ($walk -ne $Dev) {
    if (Test-Path -LiteralPath $walk) {
      $entry = Get-Item -LiteralPath $walk -Force
      if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { Stop-Maestro 'Link ou pasta de nuvem no estado privado. Preserve o local.' }
    }
    $walk = [IO.Path]::GetDirectoryName($walk)
  }
  $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User
  $system = New-Object Security.Principal.SecurityIdentifier('S-1-5-18')
  $admins = New-Object Security.Principal.SecurityIdentifier('S-1-5-32-544')
  if (-not (Test-Path -LiteralPath $Path)) {
    # Create with the private ACL in the create operation; keep SYSTEM/Admins.
    $acl = New-Object Security.AccessControl.DirectorySecurity
    $acl.SetAccessRuleProtection($true, $false)
    $acl.SetOwner($sid)
    foreach ($who in @($sid, $system, $admins)) {
      $rule = New-Object Security.AccessControl.FileSystemAccessRule($who, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow')
      $acl.AddAccessRule($rule)
    }
    [IO.Directory]::CreateDirectory($Path, $acl) | Out-Null
  }
  $current = Get-Acl -LiteralPath $Path
  if ($current.GetOwner([Security.Principal.SecurityIdentifier]).Value -ne $sid.Value) { Stop-Maestro 'Pasta privada pertence a outro usuario.' }
  foreach ($rule in $current.Access) {
    $who = $rule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
    if ($rule.AccessControlType -eq 'Allow' -and $who -notin @($sid.Value,$system.Value,$admins.Value)) { Stop-Maestro 'A pasta privada permite outros usuarios. Escolha uma pasta privada nova; nenhuma ACL existente foi alterada.' }
  }
}
function Private-File([string]$Path) {
  $f = Get-Item -LiteralPath $Path -Force
  if ($f.PSIsContainer -or ($f.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $f.LinkType -eq 'HardLink') { Stop-Maestro 'Arquivo privado com link ou tipo diferente. Preserve a preparacao.' }
  return $f
}
function Confirm-Review($Review) {
  $Review | ConvertTo-Json -Depth 16 | Write-Host
  if ((Read-Host 'Confira os efeitos. Digite YES para aplicar; Enter deixa pendente') -cne 'YES') { Stop-Maestro 'Operacao deixada pendente por sua escolha.' }
}
function Invoke-Installer([string]$Command, $Request, [string[]]$Extra = @()) {
  Verify-Installer $script:Installer
  if ($null -ne $Request) { $lines = ($Request | ConvertTo-Json -Depth 16 -Compress) | & $script:Installer $Command @Extra }
  else { $lines = & $script:Installer $Command @Extra }
  if ($LASTEXITCODE -ne 0) { Stop-Maestro "Etapa $Command pendente. Preserve o resultado e confira o login/permissoes pelo fluxo normal." }
  return ([string]::Join("`n", @($lines)) | ConvertFrom-Json)
}
function Verify-Installer([string]$Path) {
  $null = Private-File $Path
  if ((Hash $Path) -cne $Meta.identity.installer_sha256) { Stop-Maestro 'O instalador mudou. Nada sera executado.' }
}
function Resolve-CodeExecutable([string]$Selected) {
  if (-not [IO.Path]::IsPathRooted($Selected) -or $Selected.Contains("`n") -or $Selected.Contains("`r")) { Stop-Maestro 'Selecione o executavel absoluto do Claude Code.' }
  $Selected = [IO.Path]::GetFullPath($Selected)
  if ([IO.Path]::GetExtension($Selected) -ine '.exe' -or -not (Test-Path -LiteralPath $Selected -PathType Leaf)) { Stop-Maestro 'Selecione o arquivo .exe regular do Claude Code pelo fluxo normal.' }
  $walk = $Selected
  while ($walk) {
    if (((Get-Item -LiteralPath $walk -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { Stop-Maestro 'O executavel selecionado atravessa um link. Selecione o .exe local regular.' }
    $walk = [IO.Path]::GetDirectoryName($walk)
  }
  $null = Private-File $Selected
  return $Selected
}
function Private-SelectionPath([string]$Selected) {
  if (-not [IO.Path]::IsPathRooted($Selected) -or $Selected.Contains("`n") -or $Selected.Contains("`r")) { Stop-Maestro 'Selecao privada com caminho ambiguo. Preserve o vinculo.' }
  $normalized = [IO.Path]::GetFullPath($Selected)
  $separator = [IO.Path]::DirectorySeparatorChar
  if (-not $normalized.Equals($Selected,[StringComparison]::OrdinalIgnoreCase) -or -not $normalized.StartsWith($Dev.TrimEnd($separator) + $separator,[StringComparison]::OrdinalIgnoreCase) -or $normalized.EndsWith([string]$separator)) { Stop-Maestro 'Selecao privada deve ser um caminho local sem ambiguidade em Developer.' }
  $walk = $normalized
  while ($walk -ine $Dev) {
    if (Test-Path -LiteralPath $walk) {
      if (((Get-Item -LiteralPath $walk -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { Stop-Maestro 'Selecao privada atravessa um link. Preserve o local e confira o vinculo.' }
    }
    $walk = [IO.Path]::GetDirectoryName($walk)
  }
  return $normalized
}
function Require-Disjoint([string]$Candidate,[string[]]$Reserved) {
  $Candidate = Private-SelectionPath $Candidate
  $separator = [IO.Path]::DirectorySeparatorChar
  foreach ($path in $Reserved) {
    $path = Private-SelectionPath $path
    if ($Candidate.Equals($path,[StringComparison]::OrdinalIgnoreCase) -or $Candidate.StartsWith($path + $separator,[StringComparison]::OrdinalIgnoreCase) -or $path.StartsWith($Candidate + $separator,[StringComparison]::OrdinalIgnoreCase)) { Stop-Maestro 'Escolha pastas separadas para perfil, clientes, brain, workspace e instalacao.' }
  }
}
function Prepare-ClientParent {
  $parent = Join-Path $Owner 'Maestro-Clients'
  Require-Disjoint $parent @($Brain,$Work,$Base,(Join-Path $Owner '.maestro-install'))
  Private-Directory $parent
  $selected = Join-Path $parent $Role; Private-Directory $selected
  return $selected
}
function Prepare-CodeConfiguration([string]$Profile) {
  if (-not [IO.Path]::IsPathRooted($Profile) -or $Profile.Contains("`n") -or $Profile.Contains("`r")) { Stop-Maestro 'O perfil Code deve ficar dentro do owner selecionado.' }
  $Profile = [IO.Path]::GetFullPath($Profile)
  $separator = [IO.Path]::DirectorySeparatorChar
  if (-not $Profile.StartsWith($Owner.TrimEnd($separator) + $separator, [StringComparison]::OrdinalIgnoreCase) -or -not (Test-Path -LiteralPath ([IO.Path]::GetDirectoryName($Profile)) -PathType Container)) { Stop-Maestro 'Escolha um perfil privado dentro do owner, com a pasta pai existente.' }
  Require-Disjoint $Profile @($Brain,$Work,$Base,(Join-Path $Owner '.maestro-install'),(Join-Path $Owner 'Maestro-Clients'))
  if (-not (Test-Path -LiteralPath $Profile)) {
    Write-Host "Criar perfil Code privado em $Profile, sem copiar login nem alterar seu perfil normal."
    Write-Host 'Somente neste perfil novo: memoria automatica e sincronizacao de skills e plugins da conta serao desativadas. Hooks e plugins do Maestro passam pela qualificacao normal.'
    if ((Read-Host 'Digite YES para criar; Enter deixa pendente') -cne 'YES') { Stop-Maestro 'Criacao do perfil deixada pendente.' }
    Private-Directory $Profile
    $file = [IO.File]::Open((Join-Path $Profile 'settings.json'),[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    try {
      $body = $Utf8.GetBytes('{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false,"syncClaudeAiSkills":false}')
      $file.Write($body,0,$body.Length)
      $file.Flush()
    } finally { $file.Dispose() }
  } else { Private-Directory $Profile }
  return $Profile
}
function Invoke-SelectedCode([string[]]$Arguments) {
  $hadProfile = Test-Path Env:CLAUDE_CONFIG_DIR
  $previousProfile = $env:CLAUDE_CONFIG_DIR
  $script:CodeExit = $null
  try {
    $env:CLAUDE_CONFIG_DIR = $Config.code_configuration_directory
    & $Config.code_executable @Arguments
    $script:CodeExit = $LASTEXITCODE
  } finally {
    if ($hadProfile) { $env:CLAUDE_CONFIG_DIR = $previousProfile }
    else { Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue }
  }
}
function Ensure-CodeLogin([switch]$Force) {
  if ($null -ne $Config.PSObject.Properties['client_target'] -and $Config.client_target -ceq 'desktop') { Stop-Maestro 'Este alvo Desktop nao configura login Code.' }
  $statusPath = Join-Path $Run 'code-auth-private.json'
  $logPath = Join-Path $Run 'code-auth-private.log'
  $loggedIn = $false
  if (-not $Force) { try {
    $lines = Invoke-SelectedCode -Arguments @('auth','status') 2> $logPath
    [IO.File]::WriteAllText($statusPath,[string]::Join("`n",@($lines)),$Utf8)
    if ($script:CodeExit -eq 0) { $loggedIn = (Get-Content -LiteralPath $statusPath -Raw | ConvertFrom-Json).loggedIn -eq $true }
  } catch { $loggedIn = $false } }
  if ($loggedIn) { return }
  if ((Read-Host 'O perfil Code do piloto precisa de login oficial. Nenhuma credencial sera copiada. Digite YES para abrir o login; Enter deixa pendente') -cne 'YES') { Stop-Maestro 'Login do perfil de teste pendente. Reabra este programa quando quiser continuar.' }
  Invoke-SelectedCode -Arguments @('auth','login') | Out-Host
  if ($script:CodeExit -ne 0) { Stop-Maestro 'Login nao concluido; use o fluxo oficial deste perfil e retome.' }
  $lines = Invoke-SelectedCode -Arguments @('auth','status') 2> $logPath
  [IO.File]::WriteAllText($statusPath,[string]::Join("`n",@($lines)),$Utf8)
  if ($script:CodeExit -ne 0 -or (Get-Content -LiteralPath $statusPath -Raw | ConvertFrom-Json).loggedIn -ne $true) { Stop-Maestro 'Login ainda pendente.' }
}
function Invoke-Management([string[]]$Extra = @()) {
  Verify-Installer $Installer
  if ($null -ne $Config.PSObject.Properties['client_target'] -and $Config.client_target -ceq 'desktop') {
    & $Installer manage @Manage @Extra
    $script:ManagementExit = $LASTEXITCODE
    return
  }
  Ensure-CodeLogin
  $hadProfile = Test-Path Env:CLAUDE_CONFIG_DIR
  $previousProfile = $env:CLAUDE_CONFIG_DIR
  try {
    $env:CLAUDE_CONFIG_DIR = $Config.code_configuration_directory
    & $Installer manage @Manage @Extra
    $script:ManagementExit = $LASTEXITCODE
  } finally {
    if ($hadProfile) { $env:CLAUDE_CONFIG_DIR = $previousProfile }
    else { Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue }
  }
}
try {
  $Root = $PSScriptRoot
  $metadata = Join-Path $Root 'DISTRIBUTION.json'
  if (-not (Test-Path -LiteralPath $metadata) -or (Hash $metadata) -cne '@@DISTRIBUTION_SHA256@@') { Stop-Maestro 'Kit incompleto ou alterado. Marque Sempre manter neste dispositivo, aguarde sincronizar e tente de novo.' }
  $Meta = Get-Content -LiteralPath $metadata -Raw | ConvertFrom-Json
  if ($Meta.schema_version -ne 1 -or $Meta.profile -cne 'engineering-installed-seed-v1' -or $Meta.evidence_class -cne 'engineering_only' -or $Meta.identity.target_os -cne 'windows') { Stop-Maestro 'Este kit e de outro perfil ou sistema.' }
  $hostArch = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
  if ($hostArch -eq 'x64') { $hostArch = 'amd64' }
  if ($hostArch -cne $Meta.identity.target_arch) { Stop-Maestro 'Este kit e para outra arquitetura de Windows.' }
  $Role = $Meta.identity.admission_sha256
  $Dev = [IO.Path]::GetFullPath((Join-Path $env:USERPROFILE 'Developer'))
  if (-not (Test-Path -LiteralPath $Dev)) { [IO.Directory]::CreateDirectory($Dev) | Out-Null }
  if (-not (Test-Path -LiteralPath $Dev -PathType Container) -or ((Get-Item -LiteralPath $Dev).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { Stop-Maestro 'Crie uma pasta local Developer no seu perfil antes de continuar.' }
  $pilot = Join-Path $Dev 'Maestro-Pilot'; Private-Directory $pilot
  $Base = Join-Path $pilot $Role; Private-Directory $Base
  $Stage = Join-Path $Base 'stage'; Private-Directory $Stage
  foreach ($f in $Meta.files) {
    if ($f.path -cnotmatch '^(bootstrap|initial-release|test-documents)/[A-Za-z0-9_./-]+$' -or $f.path.Contains('..') -or $f.sha256 -cnotmatch '^[0-9a-f]{64}$' -or $f.size -lt 0) { Stop-Maestro 'Inventario invalido.' }
    $source = Join-Path $Root $f.path
    # Public OneDrive placeholders hydrate through the OS. Hash the named bytes.
    if (-not (Test-Path -LiteralPath $source -PathType Leaf) -or (Get-Item -LiteralPath $source).Length -ne $f.size -or (Hash $source) -cne $f.sha256) { Stop-Maestro "Sincronizacao incompleta em $($f.path). Mantenha a pasta offline e tente novamente." }
    $destination = Join-Path $Stage $f.path
    $parent = [IO.Path]::GetDirectoryName($destination)
    $parents = New-Object 'Collections.Generic.List[string]'
    while ($parent -ne $Stage) { $parents.Add($parent); $parent = [IO.Path]::GetDirectoryName($parent) }
    for ($i = $parents.Count - 1; $i -ge 0; $i--) { Private-Directory $parents[$i] }
    if (-not (Test-Path -LiteralPath $destination)) {
      $inputFile = [IO.File]::OpenRead($source)
      try {
        $outputFile = [IO.File]::Open($destination, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
        try { $inputFile.CopyTo($outputFile) } finally { $outputFile.Dispose() }
      } finally { $inputFile.Dispose() }
    }
    $copied = Private-File $destination
    if ($copied.Length -ne $f.size -or (Hash $destination) -cne $f.sha256) { Stop-Maestro "Copia privada divergente em $($f.path). Preserve-a e envie o diagnostico." }
  }
  $Installer = Join-Path $Stage $Meta.installer
  Verify-Installer $Installer
  $diagnostics = Join-Path $Base 'diagnostics'; Private-Directory $diagnostics
  $Run = Join-Path $diagnostics ('teste-' + (Get-Date -Format yyyyMMdd-HHmmss) + '-' + [Guid]::NewGuid().ToString('N')); Private-Directory $Run
  $Report = Join-Path $Run 'RELATORIO.md'
  [IO.File]::WriteAllText($Report, "# Maestro MCP - preparacao pendente`n`nPiloto experimental (engineering_only). A preparacao/qualificacao ainda nao foi concluida. JSONs e logs sao privados.`n", $Utf8)
  Write-Host "Relatorio desta tentativa: $Report"
  $seed = Invoke-Installer 'seed-status' $null
  if ($seed.schema_version -ne 3 -or $seed.experimental_seed.profile -cne 'engineering-installed-seed-v1' -or $seed.experimental_seed.admission_sha256 -cne $Role) { Stop-Maestro 'Admissao experimental nao aceita.' }
  $configPath = Join-Path $Base 'participant.json'
  if (-not (Test-Path -LiteralPath $configPath)) {
    if ($CheckOnly) { Stop-Maestro 'Ainda nao preparado. Abra Maestro.cmd uma vez para revisar e escolher seu workspace.' }
    Write-Host "`nMaestro MCP - piloto experimental. O brain existente sera preservado."
    $ClientTarget = Read-Host 'Clientes: both para Code e Desktop, ou desktop sem configurar Code [both]'
    if (-not $ClientTarget -or $ClientTarget -ceq 'both') { $ClientTarget = '' }
    elseif ($ClientTarget -cne 'desktop') { Stop-Maestro 'Escolha both ou desktop; nenhuma selecao foi salva.' }
    $Code = ''; $CodeConfig = ''
    $Owner = Read-Host "Owner existente em Developer [$Dev; Enter se nunca usou Maestro]"
    if (-not $Owner) { $Owner = $Dev }
    $Owner = [IO.Path]::GetFullPath($Owner)
    if (-not (Test-Path -LiteralPath $Owner -PathType Container) -or ($Owner -ne $Dev -and -not $Owner.StartsWith($Dev + '\',[StringComparison]::OrdinalIgnoreCase))) { Stop-Maestro 'Selecione o owner existente em Developer. Nao crie outro para contornar uma instalacao.' }
    $Work = Read-Host "Workspace em Developer [$(Join-Path $Dev 'Maestro-Workspace')]"
    if (-not $Work) { $Work = Join-Path $Dev 'Maestro-Workspace' }; $Work = [IO.Path]::GetFullPath($Work)
    if (-not (Test-Path -LiteralPath ([IO.Path]::GetDirectoryName($Work)) -PathType Container) -or -not $Work.StartsWith($Dev + '\',[StringComparison]::OrdinalIgnoreCase)) { Stop-Maestro 'Escolha um workspace em Developer com a pasta pai existente.' }
    if ($ClientTarget -cne 'desktop') {
    $defaultCode = Get-Command claude.exe -CommandType Application -ErrorAction SilentlyContinue
    $proposed = ''; if ($null -ne $defaultCode) { $proposed = $defaultCode.Source }
    $Code = Read-Host "Executavel absoluto do Claude Code [$proposed]"
    if (-not $Code) { $Code = $proposed }
    $Code = Resolve-CodeExecutable $Code
    }
    $Brain = $env:MAESTRO_HOME
    $locator = Join-Path $Owner '.maestro-home.json'
    if (-not $Brain -and (Test-Path -LiteralPath $locator)) { $Brain = (Get-Content -LiteralPath $locator -Raw | ConvertFrom-Json).home }
    if (-not $Brain -and (Test-Path -LiteralPath (Join-Path $Owner '.maestro'))) { $Brain = Join-Path $Owner '.maestro' }
    if (-not $Brain) {
      $Brain = Read-Host "Novo home do Maestro em Developer [$(Join-Path $Dev 'Maestro')]"
      if (-not $Brain) { $Brain = Join-Path $Dev 'Maestro' }
    }
    $Brain = [IO.Path]::GetFullPath($Brain)
    if (-not $Brain.StartsWith($Dev + '\',[StringComparison]::OrdinalIgnoreCase)) { Stop-Maestro 'O brain selecionado fica fora de Developer. Preserve-o e peca orientacao; este piloto nao migra dados.' }
    $Clients = Prepare-ClientParent
    if ($ClientTarget -cne 'desktop') {
    $CodeConfig = Read-Host "Perfil Code privado [$(Join-Path $Owner 'Maestro-Code-Profile')]"
    if (-not $CodeConfig) { $CodeConfig = Join-Path $Owner 'Maestro-Code-Profile' }
    $CodeConfig = Prepare-CodeConfiguration $CodeConfig
    }
    if ($ClientTarget -ceq 'desktop') {
      Write-Json $configPath @{admission_sha256=$Role;user_home=$Owner;workspace=$Work;carrier_id=[Guid]::NewGuid().ToString('N');brain_home=$Brain;client_parent=$Clients;client_target='desktop'}
    } else {
    Write-Json $configPath @{admission_sha256=$Role;user_home=$Owner;workspace=$Work;code_executable=$Code;carrier_id=[Guid]::NewGuid().ToString('N');brain_home=$Brain;client_parent=$Clients;code_configuration_directory=$CodeConfig}
    }
  }
  $null = Private-File $configPath
  $Config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
  if ($Config.admission_sha256 -cne $Role) { Stop-Maestro 'Configuracao local divergente.' }
  $ClientTarget = ''; if ($null -ne $Config.PSObject.Properties['client_target']) { $ClientTarget = $Config.client_target }
  if ($ClientTarget -isnot [string] -or $ClientTarget -cnotin @('','desktop')) { Stop-Maestro 'Alvo local desconhecido; preserve participant.json.' }
  if ($null -eq $Config.PSObject.Properties['client_parent'] -or -not $Config.client_parent) { Stop-Maestro 'Selecao privada dos clientes ausente; preserve a configuracao.' }
  $privateSelections = @($Config.client_parent)
  if ($ClientTarget -ceq 'desktop') {
    foreach ($field in @('code_executable','code_configuration_directory')) {
      if ($null -ne $Config.PSObject.Properties[$field] -and $Config.$field) { Stop-Maestro 'A selecao Desktop contem configuracao Code; preserve a configuracao.' }
    }
  } else {
    if ($null -eq $Config.PSObject.Properties['code_configuration_directory'] -or -not $Config.code_configuration_directory) { Stop-Maestro 'Esta preparacao e anterior ao perfil isolado. Preserve participant.json e o brain; envie o relatorio.' }
    $privateSelections += $Config.code_configuration_directory
  }
  foreach ($selected in $privateSelections) {
    if (-not $selected.StartsWith($Config.user_home.TrimEnd('\') + '\',[StringComparison]::OrdinalIgnoreCase) -or -not (Test-Path -LiteralPath $selected -PathType Container)) { Stop-Maestro 'Selecao privada ausente ou fora do owner. Preserve a configuracao, sem recriar perfil ou clientes.' }
    Private-Directory $selected
  }
  if ($env:MAESTRO_HOME -and $env:MAESTRO_HOME -cne $Config.brain_home) { Stop-Maestro 'MAESTRO_HOME mudou. Preserve o brain e confira a configuracao.' }
  $env:MAESTRO_HOME = $Config.brain_home
  $Product = Join-Path $Base 'product'
  $Common = @('--product-root',$Product,'--user-home',$Config.user_home,'--workspace',$Config.workspace,'--carrier-id',$Config.carrier_id)
  $installed = Join-Path $Product 'maestro-install.exe'
  if (-not (Test-Path -LiteralPath $installed)) {
    if ($CheckOnly) { Stop-Maestro 'Instalacao pendente. Abra Maestro.cmd para revisar.' }
    if ($ClientTarget -ceq 'desktop') {
      $request = @{release_directory=(Join-Path $Stage 'initial-release');managed_root=$Product;user_home=$Config.user_home;workspace=$Config.workspace;target='desktop'}
    } else {
    $request = @{release_directory=(Join-Path $Stage 'initial-release');managed_root=$Product;user_home=$Config.user_home;workspace=$Config.workspace;code_packet=(Join-Path $Base 'initial-code');desktop_packet=(Join-Path $Base 'initial-desktop.mcpb')}
    }
    $review = Invoke-Installer 'review' $request
    Write-Json (Join-Path $Run 'setup-review-private.json') $review
    Confirm-Review $review
    $result = Invoke-Installer 'apply' $request @('--review-digest',$review.review_digest,'--confirm')
    Write-Json (Join-Path $Run 'setup-result-private.json') $result
  }
  $Installer = $installed; Verify-Installer $Installer
  if ($ClientTarget -ceq 'desktop') {
    $Manage = $Common + @('--client-target','desktop','--client-desktop-path',(Join-Path $Config.client_parent 'maestro-product.mcpb'))
  } else {
  $Manage = $Common + @('--client-code-root',(Join-Path $Config.client_parent 'code'),'--client-desktop-path',(Join-Path $Config.client_parent 'maestro-product.mcpb'),'--client-code-executable',$Config.code_executable,'--client-code-config',$Config.code_configuration_directory,'--client-scope','user')
  }
  function Current-Clients {
    Verify-Installer $Installer
    $StatusArgs = $Common
    if ($ClientTarget -ceq 'desktop') { $StatusArgs += @('--client-target','desktop') }
    $lines = & $Installer clients-status @StatusArgs
    if ($LASTEXITCODE -ne 0) { return [PSCustomObject]@{state='unavailable'} }
    return ([string]::Join("`n", @($lines)) | ConvertFrom-Json)
  }
  $before = Current-Clients
  if ($before.state -ceq 'current_materialization') {
    $recordedTarget = ''; if ($null -ne $before.record.materialization.PSObject.Properties['target']) { $recordedTarget = $before.record.materialization.target }
    if ($recordedTarget -cne $ClientTarget) { Stop-Maestro 'O alvo atual difere da selecao salva; preserve o carrier e revise pelo gerenciador.' }
  }
  if (-not $CheckOnly -and $before.state -cne 'current_materialization') {
    $request = @{managed_root=$Product;user_home=$Config.user_home}
    # This hint controls only first-choice UI; the installed owner validates state and binds the source.
    $firstSource = $false
    try {
      $sourceState = Join-Path $Config.user_home '.maestro-install/config/install-state.json'
      $null = Private-File $sourceState
      $recorded = [IO.File]::ReadAllText($sourceState) | ConvertFrom-Json
      if (($recorded.schema_version -is [int] -or $recorded.schema_version -is [long]) -and $recorded.schema_version -eq 4 -and $recorded.managed_root -is [string] -and [string]::Equals($recorded.managed_root,$Product,[StringComparison]::OrdinalIgnoreCase)) {
        $sourceProperty = $recorded.PSObject.Properties['source_control']
        $firstSource = $null -eq $sourceProperty -or $null -eq $sourceProperty.Value
      }
    } catch { $firstSource = $false }
    if ($firstSource) {
      Write-Host 'Selecione a pasta SOURCE sincronizada do time e disponivel offline, separada do ZIP local.'
      Write-Host 'Digite TESTE-LOCAL somente para ensaio com a amostra deste kit; ela nao acompanha a origem do time.'
      $shared = Read-Host 'Pasta SOURCE (Enter deixa pendente)'
      if (-not $shared) { Stop-Maestro 'Escolha da origem deixada pendente; nenhuma origem foi inscrita.' }
      if ($shared -ceq 'TESTE-LOCAL') { $shared = Join-Path $Root 'source'; Write-Host "Ensaio local, sem acompanhar outra pasta compartilhada: $shared" }
      $request['source_root'] = $shared
      $request['source'] = $Meta.identity.source
    }
    $review = Invoke-Installer 'source-review' $request
    Write-Json (Join-Path $Run 'source-review-private.json') $review
    Confirm-Review $review
    $observed = Invoke-Installer 'source-check' $request @('--review-digest',$review.digest,'--confirm')
    if ($observed.state -eq 'candidate_staged_activation_unconfirmed') {
      $update = Invoke-Installer 'update-review' $request @('--plan-id',$observed.plan_id)
      Confirm-Review $update
      $result = Invoke-Installer 'update' $request @('--plan-id',$observed.plan_id,'--confirm')
      Write-Json (Join-Path $Run 'update-result-private.json') $result
    }
    Write-Host 'O gerenciador prepara/qualifica os clientes completos. Se faltar login/prova nativa/permissao, permanece pendente. Nao use resources-only para contornar.'
    Invoke-Management
    if ($script:ManagementExit -ne 0) { Write-Host 'Registro/qualificacao pendente. O relatorio preserva esse estado.' }
  }
  $ClientStatus = Current-Clients
  $Server = Join-Path $Product 'bin\maestro-mcp.exe'; $null = Private-File $Server
  if ((Hash $Server) -cnotin $Meta.server_hashes) { Stop-Maestro 'Backend fora das duas versoes admitidas. Nada sera executado.' }
  $result = & $Server installed-pilot-check @Common --installer-version $Meta.identity.installer_version --experimental-seed-sha256 $Role --experimental-installed-root $Product --check-only
  $checkExit = $LASTEXITCODE
  $Checked = ([string]::Join("`n", @($result)) | ConvertFrom-Json)
  if ($Checked.schema_version -ne 2 -or $Checked.evidence_class -cne 'engineering_only') { Stop-Maestro 'Verificador nao produziu um relatorio reconhecido.' }
  Write-Json (Join-Path $Run 'report-private.json') $Checked
  $prompts = @{markdown='RELATORIO.md';code_prompt='CODE.txt';code_readback_prompt='CODE-RELEITURA.txt';desktop_prompt='DESKTOP.txt';code_ready_prompt='CODE-CONEXAO.txt';desktop_ready_prompt='DESKTOP-CONEXAO.txt'}
  foreach ($name in $prompts.Keys) { [IO.File]::WriteAllText((Join-Path $Run $prompts[$name]), [string]$Checked.$name, $Utf8) }
  Write-Host "Relatorio para compartilhar: $Report`nJSONs e logs sao privados; nao envie a pasta inteira."
  if ($CheckOnly) { exit $checkExit }
  function Assert-TestPrompt([string]$PromptFile) {
    if ($Checked.status -ceq 'blocked') { Stop-Maestro 'O diagnostico bloqueou a jornada. Preserve o relatorio; nenhum cliente sera aberto.' }
    if (-not (Test-Path -LiteralPath (Join-Path $Run $PromptFile) -PathType Leaf) -or [string]::IsNullOrWhiteSpace((Get-Content -LiteralPath (Join-Path $Run $PromptFile) -Raw))) { Stop-Maestro 'Prompt indisponivel. Reabra o diagnostico antes de continuar.' }
  }
  function Prepare-ConversionDocument($ClientStatus,[string]$PromptFile) {
    if ($PromptFile -cnotin @('CODE.txt','DESKTOP.txt')) { Stop-Maestro 'Documento disponivel somente nas opcoes completas 1 e 2.' }
    Assert-TestPrompt $PromptFile
    $conversion = $Checked.conversion
    if ($conversion.state -ceq 'pending_runtime') { Write-Host 'Conversao pendente: o runtime atual nao esta pronto. O restante do ensaio pode continuar; nenhum documento sera criado.'; return }
    if ($conversion.state -cne 'ready_for_participant_test') { Stop-Maestro 'Identidade do conversor divergente ou indisponivel. Jornada interrompida.' }
    if ($conversion.expected_manifest_sha256 -cnotmatch '^[0-9a-f]{64}$' -or $conversion.expected_pack_sha256 -cnotmatch '^[0-9a-f]{64}$' -or $ClientStatus.state -cne 'current_materialization' -or $ClientStatus.record.binding.workspace -cne $Config.workspace -or $ClientStatus.record.materialization.manifest_sha256 -cne $conversion.expected_manifest_sha256) { Stop-Maestro 'Workspace ou versao atual mudou desde o diagnostico. Reabra o teste.' }
    $publicSource = Join-Path $Stage 'test-documents\maestro-pilot.docx'
    $null = Private-File $publicSource
    if ($conversion.source -cne 'maestro-pilot.docx' -or $conversion.source_sha256 -cne '6bd282a12170035e8af5352f2de5a21f4fe4bc5c7919283482a26703c206bf81' -or (Hash $publicSource) -cne $conversion.source_sha256) { Stop-Maestro 'Documento sintetico divergente. Nenhum documento foi preparado.' }
    $null = Private-File $Server
    if ((Hash $Server) -cnotin $Meta.server_hashes) { Stop-Maestro 'O backend mudou desde o diagnostico. Reabra o teste.' }
    $lines = & $Server installed-pilot-prepare-document @Common --installer-version $Meta.identity.installer_version --experimental-seed-sha256 $Role --experimental-installed-root $Product
    if ($LASTEXITCODE -ne 0) { Stop-Maestro 'Documento nao preparado. Preserve qualquer arquivo existente e o diagnostico; nada sera sobrescrito.' }
    $prepared = [string]::Join("`n",@($lines)) | ConvertFrom-Json
    Write-Json (Join-Path $Run ('prepare-document-' + [Guid]::NewGuid().ToString('N') + '.json')) $prepared
    if ($prepared.schema_version -ne 1 -or $prepared.state -cnotin @('fixture_prepared','fixture_already_present') -or $prepared.source -cne 'maestro-pilot.docx' -or $prepared.source_sha256 -cne '6bd282a12170035e8af5352f2de5a21f4fe4bc5c7919283482a26703c206bf81') { Stop-Maestro 'Preparador nao devolveu o documento esperado. Jornada interrompida.' }
    Write-Host 'Documento sintetico preparado ou ja presente sem sobrescrita. Conversao e execucao nativa ainda nao foram testadas; siga o prompt no cliente.'
  }
  function Open-Code([string]$PromptFile) {
    if ($ClientTarget -ceq 'desktop') { Stop-Maestro 'Este alvo Desktop nao seleciona Code.' }
    Assert-TestPrompt $PromptFile
    $ClientStatus = Current-Clients
    if ($ClientStatus.state -cne 'current_materialization' -or $ClientStatus.record.code.state -cne 'registered' -or -not $ClientStatus.record.code.enabled) { Stop-Maestro 'Code ainda nao esta registrado/habilitado. Conclua pelo gerenciador.' }
    Verify-Installer $Installer
    $null = & $Installer client-plugin-path @Common
    if ($LASTEXITCODE -ne 0) { Stop-Maestro 'Plugin atual nao admitido.' }
    $selected = $ClientStatus.record.code.executable
    if (-not [IO.Path]::IsPathRooted($selected) -or (Hash $selected) -cne $ClientStatus.record.code.executable_sha256) { Stop-Maestro 'O Code selecionado mudou. Revise o registro pelo gerenciador.' }
    if ($null -eq $ClientStatus.record.code.PSObject.Properties['configuration_directory'] -or $ClientStatus.record.code.configuration_directory -cne $Config.code_configuration_directory -or $selected -cne $Config.code_executable) { Stop-Maestro 'O perfil Code registrado divergiu. Preserve o registro e confira pelo gerenciador.' }
    if ($PromptFile -ceq 'CODE.txt') { Prepare-ConversionDocument $ClientStatus $PromptFile }
    Push-Location -LiteralPath $Config.workspace
    try {
      Invoke-SelectedCode -Arguments @((Get-Content -LiteralPath (Join-Path $Run $PromptFile) -Raw)) | Out-Host
      if ($script:CodeExit -ne 0) { Stop-Maestro "O Code encerrou com codigo $script:CodeExit. Preserve o relatorio e retome pelo fluxo normal." }
    } finally { Pop-Location }
  }
  function Test-CurrentDesktopInstall($ClientStatus) {
    $material = $ClientStatus.record.materialization
    $desktop = $ClientStatus.record.desktop
    $package = $desktop.PSObject.Properties['package_sha256']
    $contract = $desktop.PSObject.Properties['native_contract_sha256']
    return ($desktop.install_confirmation -ceq 'user_confirmed_install' -and $desktop.evidence_class -ceq 'operator_attested' -and $material.desktop_sha256 -cmatch '^[0-9a-f]{64}$' -and $material.desktop_native_contract_sha256 -cmatch '^[0-9a-f]{64}$' -and $null -ne $package -and $null -ne $contract -and $package.Value -ceq $material.desktop_sha256 -and $contract.Value -ceq $material.desktop_native_contract_sha256)
  }
  function Open-Desktop([string]$PromptFile) {
    Assert-TestPrompt $PromptFile
    $ClientStatus = Current-Clients
    if ($ClientStatus.state -cne 'current_materialization') { Stop-Maestro 'Pacotes ainda nao estao atuais.' }
    if ($PromptFile -ceq 'DESKTOP.txt') { Prepare-ConversionDocument $ClientStatus $PromptFile }
    Set-Clipboard -Value (Get-Content -LiteralPath (Join-Path $Run $PromptFile) -Raw)
    if (Test-CurrentDesktopInstall $ClientStatus) {
      Write-Host 'A instalacao deste pacote/contrato ja foi confirmada por voce. O instalador da extensao nao sera reaberto; conexao e funcionamento continuam pendentes de teste.'
    } else {
      Write-Host 'A instalacao deste pacote/contrato precisa de confirmacao atual. Instale/ative somente esta extensao e confirme o pacote pelo gerenciador (opcao 5).'
      Start-Process -FilePath $ClientStatus.record.materialization.desktop_path
    }
    Write-Host ('Biblioteca de skills ' + $ClientStatus.record.materialization.desktop_methods.version + ': ' + $ClientStatus.record.materialization.desktop_methods.archive)
    Write-Host 'A importacao no Desktop nao foi observada pelo menu. Se a biblioteca ainda nao estiver disponivel, importe esse arquivo pelo fluxo normal.'
    Write-Host 'Prompt copiado. Abra sua conversa no Desktop, use somente o Maestro deste ensaio, cole uma vez e execute. Registre o resultado na opcao 7.'
  }
  do {
    if ($ClientTarget -ceq 'desktop') {
      Write-Host "`n2 - Ensaio completo Desktop`n4 - Conexao Desktop`n5 - Gerenciar Desktop e metodos`n7 - Resultados e relatorio`n8 - Atualizar da pasta compartilhada`nEnter - Sair"
    } else {
    Write-Host "`n1 - Ensaio completo Code`n2 - Retomar Desktop (um prompt)`n3 - Conexao Code`n4 - Conexao Desktop`n5 - Gerenciar/qualificar hooks e skills`n6 - Persistencia Code`n7 - Resultados e relatorio`n8 - Atualizar da pasta compartilhada`n9 - Renovar login do perfil Code do piloto`nEnter - Sair"
    }
    $choice = Read-Host 'Escolha'
    switch -CaseSensitive ($choice) {
      '1' { Open-Code 'CODE.txt' }
      '2' { Open-Desktop 'DESKTOP.txt' }
      '3' { Open-Code 'CODE-CONEXAO.txt' }
      '4' { Open-Desktop 'DESKTOP-CONEXAO.txt' }
      '5' { Invoke-Management; if ($script:ManagementExit -ne 0) { Write-Host 'Gerenciamento pendente. Preserve o relatorio.' }; Write-Host 'Reabra Maestro.cmd para conferir a nova geracao.'; $choice = '' }
      '6' { Open-Code 'CODE-RELEITURA.txt' }
      '7' {
        [IO.File]::AppendAllText($Report, "`n## Observacoes do participante`n`nAutodeclaracoes; nao substituem qualificacao nativa.`n", $Utf8)
        foreach ($label in @('Code: identidade/skills','Desktop: conexao/skills','Onboarding e mesmo workspace','Memoria: salvar e reler','Hooks e delegacao','Conversao de documento')) {
          if ($ClientTarget -ceq 'desktop' -and $label -cin @('Code: identidade/skills','Hooks e delegacao')) {
            [IO.File]::AppendAllText($Report, "`n- ${label}: not_applicable (alvo Desktop)`n", $Utf8)
            continue
          }
          $answer = Read-Host "$label (passou/falhou/nao-testei)"
          if ($answer -cnotin @('passou','falhou','nao-testei')) { $answer = 'nao-testei' }
          [IO.File]::AppendAllText($Report, "`n- ${label}: $answer (participante)`n", $Utf8)
        }
        Start-Process -FilePath notepad.exe -ArgumentList @('"' + $Report + '"')
      }
      '8' { Invoke-Management -Extra @('--management-intent','update'); if ($script:ManagementExit -ne 0) { Write-Host 'Atualizacao pendente. Preserve o relatorio.' }; Write-Host 'Reabra Maestro.cmd para conferir a nova geracao.'; $choice = '' }
      '9' { Ensure-CodeLogin -Force }
    }
  } while ($choice -ne '')
} catch {
  [Console]::Error.WriteLine("Maestro: " + $_.Exception.Message)
  exit 1
}
