package main

import (
	"encoding/json"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// PowerShell's actual shipped helper bodies are exercised without running the
// launcher, installer, Claude, login or GUI. Native Windows ACL checks remain
// separately qualified; only directory creation is substituted on other hosts.
func TestWindowsSelfServiceExactOwnerAndSelectedProfile(t *testing.T) {
	shell := "pwsh"
	var powerShellEnvironment []string
	if runtime.GOOS == "windows" {
		shell = "powershell.exe"
		// Match Maestro.cmd: Windows PowerShell must build its own module path,
		// rather than inherit the hosting PowerShell process's module selection.
		powerShellEnvironment = make([]string, 0)
		for _, entry := range os.Environ() {
			name, _, _ := strings.Cut(entry, "=")
			if !strings.EqualFold(name, "PSModulePath") {
				powerShellEnvironment = append(powerShellEnvironment, entry)
			}
		}
	}
	powerShell, err := exec.LookPath(shell)
	if err != nil {
		if runtime.GOOS == "windows" {
			t.Fatalf("shipped Windows PowerShell 5.1 is required: %v", err)
		}
		t.Skip("PowerShell required for shipped Windows producer behavior")
	}
	root := developerFixture(t)
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "windows-selfservice.ps1")
	if err := os.WriteFile(script, []byte(windowsSelfServiceBehavior), 0600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		// Match atlasTestDataRoot: repair only the new synthetic Developer,
		// whose default owner can be Administrators on elevated hosted runners.
		prepare := exec.Command(powerShell, "-NoProfile", "-NonInteractive", "-File", script, "-Fixture", root, "-PrepareFixture")
		prepare.Env = powerShellEnvironment
		if output, err := prepare.CombinedOutput(); err != nil {
			t.Fatalf("initialize synthetic Windows fixture ownership: %s\n%v", output, err)
		}
	}
	// Preserve the original private-state writer and its native custody checks.
	stateParent := filepath.Join(root, "source selection", "owner", ".maestro-install", "config")
	if err := scheduler.EnsurePrivateDirectory(stateParent); err != nil {
		t.Fatal(err)
	}
	stateBody, err := json.Marshal(installtx.State{SchemaVersion: 4, ManagedRoot: filepath.Join(root, "source selection", "product")})
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.WriteNewPrivateFile(filepath.Join(stateParent, "install-state.json"), stateBody); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(powerShell, "-NoProfile", "-NonInteractive", "-File", script, "-Source", filepath.Join(repo, "dev/pilot/templates/installed-seed/Maestro.ps1"), "-Fixture", root)
	command.Env = powerShellEnvironment
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("shipped Windows helper behavior: %s\n%v", output, err)
	}
	t.Log(string(output))
}

const windowsSelfServiceBehavior = `param([string]$Source,[string]$Fixture,[switch]$PrepareFixture)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if([Environment]::OSVersion.Platform -eq 'Win32NT' -and ($PSVersionTable.PSEdition -cne 'Desktop' -or $PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion.Minor -ne 1)){throw 'Shipped frontend requires Windows PowerShell Desktop 5.1'}
if($PrepareFixture){
  if([Environment]::OSVersion.Platform -ne 'Win32NT'){throw 'Native Windows fixture preparation required'}
  $directory=Get-Item -LiteralPath $Fixture -Force
  if(-not $directory.PSIsContainer -or $directory.Name -cne 'Developer' -or ($directory.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){throw 'Expected fresh synthetic Developer directory'}
  $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User
  $acl=New-Object Security.AccessControl.DirectorySecurity
  $acl.SetSecurityDescriptorSddlForm(('O:{0}D:P(A;OICI;FA;;;{0})(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)' -f $sid.Value))
  Set-Acl -LiteralPath $Fixture -AclObject $acl
  $observed=Get-Acl -LiteralPath $Fixture
  if($observed.GetOwner([Security.Principal.SecurityIdentifier]).Value -cne $sid.Value -or -not $observed.AreAccessRulesProtected){throw 'Synthetic fixture did not retain current-SID owner and protected DACL'}
  return
}
Write-Output ('Frontend protocol shell: {0} {1}; synthetic installer stubs, not installation/client proof' -f $PSVersionTable.PSEdition,$PSVersionTable.PSVersion)
$Utf8=New-Object System.Text.UTF8Encoding($false)
$tokens=$null; $errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($Source,[ref]$tokens,[ref]$errors)
if($errors.Count -ne 0){throw 'Shipped PowerShell script does not parse'}
foreach($name in @('Write-Json','Confirm-Review','Private-File','Resolve-CodeExecutable','Private-SelectionPath','Require-Disjoint','Prepare-ClientParent','Prepare-CodeConfiguration','Invoke-SelectedCode','Invoke-Management','Ensure-CodeLogin','Test-DesktopPreparation')){
  $node=$ast.Find({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true)
  if($null -eq $node){throw "Missing shipped helper: $name"}
  Invoke-Expression $node.Extent.Text
}
function Stop-Maestro([string]$Message){throw $Message}
function Private-Directory([string]$Path){
  if(Test-Path -LiteralPath $Path){
    if(((Get-Item -LiteralPath $Path -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){throw 'Fixture link refused'}
  }else{[IO.Directory]::CreateDirectory($Path)|Out-Null}
}
function Read-Host([string]$Prompt){if($script:Answers.Count -eq 0){throw 'Unexpected prompt'};$script:Answers.Dequeue()}
$script:Answers=New-Object 'Collections.Generic.Queue[string]'
$Dev=Join-Path $Fixture 'Developer';$Owner=Join-Path $Dev 'existing owner';$Base=Join-Path $Dev 'pilot';$Role='test-role'
foreach($p in @($Dev,$Owner,$Base)){[IO.Directory]::CreateDirectory($p)|Out-Null}
$Brain=Join-Path $Owner 'brain';$Work=Join-Path $Dev 'workspace';$Product=Join-Path $Base 'product'
$originalOwner=$Owner;$originalBrain=$Brain;$originalWork=$Work;$originalBase=$Base
foreach($kind in @('brain','nested-brain','workspace','installation')){
  $Owner=Join-Path $Dev ('preflight owner '+$kind);[IO.Directory]::CreateDirectory($Owner)|Out-Null
  $container=Join-Path $Owner 'Maestro-Clients';$Brain=Join-Path $Owner 'brain';$Work=Join-Path $Dev 'workspace';$Base=$originalBase
  switch($kind){'brain'{$Brain=$container};'nested-brain'{$Brain=Join-Path $container 'brain'};'workspace'{$Work=$container};'installation'{$Base=Join-Path $container 'installation'}}
  $refused=$false;try{Prepare-ClientParent|Out-Null}catch{$refused=$true}
  if(-not $refused -or (Test-Path -LiteralPath $container)){throw "Client preflight created a child in reserved $kind"}
}
$Owner=$originalOwner;$Brain=$originalBrain;$Work=$originalWork;$Base=$originalBase
$Base=Join-Path $Owner 'pilot state'
foreach($kind in @('state','clients','base')){
  $parent=switch($kind){'state'{Join-Path $Owner '.maestro-install'};'clients'{Join-Path $Owner 'Maestro-Clients'};'base'{$Base}}
  [IO.Directory]::CreateDirectory($parent)|Out-Null;$candidate=Join-Path $parent 'profile-must-not-exist'
  $refused=$false;try{Prepare-CodeConfiguration $candidate|Out-Null}catch{$refused=$true}
  if(-not $refused -or (Test-Path -LiteralPath $candidate)){throw "Profile preflight created a child in reserved $kind"}
}
$Base=$originalBase
$linkOwner=Join-Path $Dev 'linked owner';[IO.Directory]::CreateDirectory($linkOwner)|Out-Null
$protected=Join-Path $linkOwner '.maestro-install';[IO.Directory]::CreateDirectory($protected)|Out-Null
$alias=Join-Path $linkOwner 'Maestro-Clients'
$linkType='SymbolicLink';if([Environment]::OSVersion.Platform -eq 'Win32NT'){$linkType='Junction'}
New-Item -ItemType $linkType -Path $alias -Target $protected|Out-Null
$Owner=$linkOwner
$refused=$false;try{Prepare-ClientParent|Out-Null}catch{$refused=$true}
if(-not $refused -or (Test-Path -LiteralPath (Join-Path $protected $Role))){throw 'Reparse client parent created a protected child'}
$refused=$false;try{Prepare-CodeConfiguration (Join-Path $alias 'profile')|Out-Null}catch{$refused=$true}
if(-not $refused -or (Test-Path -LiteralPath (Join-Path $protected 'profile'))){throw 'Reparse profile created a protected child'}
$Owner=$originalOwner
$native=Join-Path $Owner 'claude code.exe';[IO.File]::WriteAllText($native,'synthetic nonexecuted regular executable fixture',$Utf8)
if((Resolve-CodeExecutable $native) -cne $native){throw 'Regular absolute exe selection changed'}
$wrapper=Join-Path $Owner 'claude.cmd';[IO.File]::WriteAllText($wrapper,'synthetic nonexecuted wrapper',$Utf8)
$refused=$false;try{Resolve-CodeExecutable $wrapper|Out-Null}catch{$refused=$true}
if(-not $refused){throw 'Wrapper accepted instead of regular exe'}
$clients=Prepare-ClientParent
if($clients -cne (Join-Path (Join-Path $Owner 'Maestro-Clients') $Role) -or -not(Test-Path -LiteralPath $clients -PathType Container)){throw 'Clients must be prepared inside the exact nested owner'}
$profile=Join-Path $Owner 'Code profile'
$script:Answers.Enqueue('')
$declined=$false;try{Prepare-CodeConfiguration $profile|Out-Null}catch{$declined=$true}
if(-not $declined -or (Test-Path -LiteralPath $profile)){throw 'Decline must preserve absent profile'}
$script:Answers.Enqueue('YES');$selected=Prepare-CodeConfiguration $profile
if($selected -cne $profile){throw 'Selected profile changed'}
$settings=Join-Path $profile 'settings.json'
$settingsObject=Get-Content -LiteralPath $settings -Raw|ConvertFrom-Json
if($settingsObject.autoMemoryEnabled -ne $false -or $settingsObject.syncClaudeAiPlugins -ne $false -or $settingsObject.syncClaudeAiSkills -ne $false){throw 'New profile settings differ'}
$preserved='{"syncClaudeAiSkills":true,"custom":"preserve-existing"}'
[IO.File]::WriteAllText($settings,$preserved,$Utf8)
$null=Prepare-CodeConfiguration $profile
if([IO.File]::ReadAllText($settings) -cne $preserved){throw 'Existing settings were overwritten'}
foreach($bad in @((Join-Path $Dev 'outside owner'),$Brain,(Join-Path $Brain 'inside'),$Work,$Owner)){
  $refused=$false;try{Prepare-CodeConfiguration $bad|Out-Null}catch{$refused=$true}
  if(-not $refused -or (Test-Path -LiteralPath $bad -PathType Container) -and $bad -cne $Owner){throw 'Unsafe profile was created or accepted'}
}
$script:Config=[PSCustomObject]@{code_executable=(Get-Process -Id $PID).Path;code_configuration_directory=$profile}
$had=Test-Path Env:CLAUDE_CONFIG_DIR;$prior=$env:CLAUDE_CONFIG_DIR
try{
  $env:CLAUDE_CONFIG_DIR='caller-profile'
  $observed=Invoke-SelectedCode -Arguments @('-NoProfile','-NonInteractive','-Command','[Console]::Write($env:CLAUDE_CONFIG_DIR); exit 7')
  if($observed -cne $profile -or $script:CodeExit -ne 7 -or $env:CLAUDE_CONFIG_DIR -cne 'caller-profile'){throw 'Selected profile, native exit or caller environment was lost'}
  Remove-Item Env:CLAUDE_CONFIG_DIR
  $null=Invoke-SelectedCode -Arguments @('-NoProfile','-NonInteractive','-Command','exit 0')
  if((Test-Path Env:CLAUDE_CONFIG_DIR) -or $script:CodeExit -ne 0){throw 'Absent caller environment was not restored'}
  $script:Config.code_executable=Join-Path $Fixture 'missing-code.exe'
  $failed=$false;try{Invoke-SelectedCode -Arguments @('unused')|Out-Null}catch{$failed=$true}
  if(-not $failed -or (Test-Path Env:CLAUDE_CONFIG_DIR)){throw 'Failed child did not restore environment'}
  $Run=Join-Path $Fixture 'auth diagnostics';[IO.Directory]::CreateDirectory($Run)|Out-Null
  $script:AuthCalls=New-Object 'Collections.Generic.List[string]'
  function Invoke-SelectedCode([string[]]$Arguments){$script:AuthCalls.Add(($Arguments -join ' '));$script:CodeExit=0;if($Arguments[1] -ceq 'status'){'{"loggedIn":true}'}}
  Ensure-CodeLogin
  if($script:AuthCalls.Count -ne 1 -or $script:AuthCalls[0] -cne 'auth status'){throw 'Ordinary login status route changed'}
  $script:AuthCalls.Clear();$script:Answers.Enqueue('')
  $accepted=Ensure-CodeLogin -Force
  if($accepted -ne $false -or $script:AuthCalls.Count -ne 0){throw 'Forced official login ran without consent'}
  $script:Answers.Enqueue('YES');Ensure-CodeLogin -Force
  if($script:AuthCalls.Count -ne 2 -or $script:AuthCalls[0] -cne 'auth login' -or $script:AuthCalls[1] -cne 'auth status'){throw 'Force login could not recover an expired session masked by loggedIn status'}
  $script:Config|Add-Member -NotePropertyName client_parent -NotePropertyValue $clients
  $script:Config|Add-Member -NotePropertyName workspace -NotePropertyValue $Work
  $Common=@('--workspace',$Work)
  $ClientTarget=''
  $producer=$ast.Find({param($n) $n -is [Management.Automation.Language.IfStatementAst] -and $n.Extent.Text.Contains('$Manage = $Common')},$true)
  if($null -eq $producer){throw 'Missing actual target-aware manager argument producer'}
  Invoke-Expression $producer.Extent.Text
  if($Manage[$Manage.IndexOf('--client-code-root')+1] -cne (Join-Path $clients 'code') -or $Manage[$Manage.IndexOf('--client-desktop-path')+1] -cne (Join-Path $clients 'maestro-product.mcpb') -or $Manage[$Manage.IndexOf('--client-code-config')+1] -cne $profile){throw 'Original manager arguments lost exact client/profile selection'}
  function Verify-Installer([string]$Path){}
  function Ensure-CodeLogin{return $true}
  function Test-Installer {
    if($env:CLAUDE_CONFIG_DIR -cne $profile -or $args[0] -cne 'manage' -or $args[-2] -cne '--management-intent' -or $args[-1] -cne 'update'){throw 'Manager child lost reviewed profile or update arguments'}
    $global:LASTEXITCODE=9
  }
  $Installer='Test-Installer';$env:CLAUDE_CONFIG_DIR='caller-profile'
  $script:Answers.Enqueue('YES');Invoke-Management -Extra @('--management-intent','update')
  if($script:ManagementExit -ne 9 -or $env:CLAUDE_CONFIG_DIR -cne 'caller-profile'){throw 'Manager exit or caller environment was lost'}
  function Ensure-CodeLogin{throw 'Deferred Code/auth must not be invoked'}
  foreach($intent in @('prepare-desktop','update-desktop')){
    $script:ExpectedIntent=$intent
    function Test-DeferredInstaller{
      $values=@($args)
      if($env:CLAUDE_CONFIG_DIR -cne 'caller-profile' -or $values[0] -cne 'manage' -or $values[-2] -cne '--management-intent' -or $values[-1] -cne $script:ExpectedIntent -or @($values|Where-Object{$_ -ceq '--management-intent'}).Count -ne 1){throw 'Deferred manager lost exact scope or intent'}
      if($values -contains '--client-target' -or $values[$values.IndexOf('--client-code-config')+1] -cne $profile -or $values[$values.IndexOf('--client-desktop-path')+1] -cne (Join-Path $clients 'maestro-product.mcpb')){throw 'Deferral retargeted the saved both selection'}
      $global:LASTEXITCODE=0
    }
    $Installer='Test-DeferredInstaller';$script:Answers.Enqueue('')
    if($intent -ceq 'update-desktop'){Invoke-Management -Extra @('--management-intent','update')}else{Invoke-Management}
    if($script:ManagementExit -ne 0 -or $env:CLAUDE_CONFIG_DIR -cne 'caller-profile' -or $script:ObservationClient -cne 'desktop'){throw 'Deferral did not return truthful Desktop observation scope'}
  }
  $legacyConfig=$script:Config
  $script:Config=[PSCustomObject]@{client_target='desktop';client_parent=$clients;workspace=$Work}
  $ClientTarget='desktop';Invoke-Expression $producer.Extent.Text
  if($Manage -contains '--client-code-root' -or $Manage -contains '--client-code-config' -or $Manage -contains '--client-code-executable' -or $Manage -contains '--client-scope' -or $Manage[$Manage.IndexOf('--client-target')+1] -cne 'desktop'){throw 'Desktop manager fabricated Code selection'}
  function Ensure-CodeLogin{throw 'Desktop manager attempted Code login'}
  function Test-DesktopInstaller{
    if($env:CLAUDE_CONFIG_DIR -cne 'caller-profile' -or $args[0] -cne 'manage' -or $args -contains '--client-code-root'){throw 'Desktop manager changed profile or selected Code'}
    $global:LASTEXITCODE=7
  }
  $Installer='Test-DesktopInstaller';Invoke-Management -Extra @('--management-intent','update')
  if($script:ManagementExit -ne 7 -or $env:CLAUDE_CONFIG_DIR -cne 'caller-profile'){throw 'Desktop manager lost original exit/environment'}
  $script:Config=$legacyConfig;$ClientTarget=''
}finally{if($had){$env:CLAUDE_CONFIG_DIR=$prior}else{Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue}}

# Execute the original source-enrollment producer against a bounded synthetic owner.
$text=[IO.File]::ReadAllText($Source)
$start=$text.IndexOf('  $before = Current-Clients')
$end=$text.IndexOf('  $ClientStatus = Current-Clients',$start)
if($start -lt 0 -or $end -lt 0){throw 'Missing shipped source enrollment boundary'}
$sourceProducer=$text.Substring($start,$end-$start)
$sourceFixture=Join-Path $Fixture 'source selection'
$sourceOwner=Join-Path $sourceFixture 'owner';$Product=Join-Path $sourceFixture 'product';$Root=Join-Path $sourceFixture 'kit';$Run=Join-Path $sourceFixture 'run'
$ExpectedSource=Join-Path $sourceFixture 'hydrated team source';$stateParent=Join-Path $sourceOwner '.maestro-install/config'
foreach($p in @($sourceOwner,$Product,$Root,$Run,$ExpectedSource,$stateParent)){[IO.Directory]::CreateDirectory($p)|Out-Null}
$statePath=Join-Path $stateParent 'install-state.json'
$freshState=[IO.File]::ReadAllText($statePath)
$Config=[PSCustomObject]@{user_home=$sourceOwner;source_root=(Join-Path $Root 'source')}
$Meta=[PSCustomObject]@{identity=[PSCustomObject]@{source=[PSCustomObject]@{source_id='team';channel='canary';os='windows';arch='amd64'}}}
$CheckOnly=$false
$script:SourceReviewCount=0;$script:CheckedSource=$null
function Current-Clients { [PSCustomObject]@{state='pending'} }
function Invoke-Management { $script:ManagementExit=0 }
function Invoke-Installer([string]$Command,$Request,[string[]]$Extra=@()) {
  if($Command -ceq 'source-review'){
    $script:SourceReviewCount++
    $script:SourceRequest=$Request
    $recorded=[IO.File]::ReadAllText($statePath)|ConvertFrom-Json
    if($recorded.schema_version -ne 4 -or $recorded.managed_root -cne $Product){throw 'Original owner rejects state'}
    if($Request.ContainsKey('source_root')){
      if($Request.source.source_id -cne 'team'){throw 'Reviewed team tuple changed'}
      $selected=$Request.source_root
    }else{
      $selected=$recorded.source_control.locator
    }
    if($selected -cne $script:ExpectedSource){throw ('Source choice ignored: '+$selected)}
    $script:ReviewedRequest=$Request|ConvertTo-Json -Depth 16 -Compress
    return [PSCustomObject]@{digest='exact-review'}
  }
  if($Command -ceq 'source-check'){
    $script:SourceCheckCount++
    if($Extra -notcontains 'exact-review' -or $Extra -notcontains '--confirm' -or ($Request|ConvertTo-Json -Depth 16 -Compress) -cne $script:ReviewedRequest){throw 'Source review/digest/confirmation lost'}
    $script:CheckedSource=$script:ExpectedSource
    $recorded=[IO.File]::ReadAllText($statePath)|ConvertFrom-Json
    if($null -eq $recorded.PSObject.Properties['source_control']){
      $recorded|Add-Member -NotePropertyName source_control -NotePropertyValue ([PSCustomObject]@{locator=$script:ExpectedSource;tuple=$Meta.identity.source})
      [IO.File]::WriteAllText($statePath,($recorded|ConvertTo-Json -Depth 16),$Utf8)
    }
    if($script:SourceAdvanced){return [PSCustomObject]@{state='candidate_staged_activation_unconfirmed';plan_id='exact-plan'}}
    return [PSCustomObject]@{state='current_signed_release'}
  }
  if($Command -ceq 'update-review'){return [PSCustomObject]@{plan_id='exact-plan'}}
  if($Command -ceq 'update'){
    if($Extra -notcontains 'exact-plan' -or $Extra -notcontains '--confirm'){throw 'Existing advanced-source review lost'}
    $script:UpdateApplied=$true;return [PSCustomObject]@{state='activation_readback'}
  }
  throw ('Unexpected source owner command: '+$Command)
}
function Run-SourceAttempt([string]$Name,[string[]]$Inputs,[string]$Expected,[switch]$Advanced){
  $script:Run=Join-Path $sourceFixture $Name;[IO.Directory]::CreateDirectory($script:Run)|Out-Null
  $script:ExpectedSource=$Expected;$script:SourceAdvanced=$Advanced.IsPresent
  $script:SourceReviewCount=0;$script:SourceCheckCount=0;$script:CheckedSource=$null;$script:UpdateApplied=$false
  $script:Answers=New-Object 'Collections.Generic.Queue[string]'
  foreach($answer in $Inputs){$script:Answers.Enqueue($answer)}
  Invoke-Expression $sourceProducer
  if($script:CheckedSource -cne $Expected -or $script:SourceReviewCount -ne 1 -or $script:SourceCheckCount -ne 1 -or $script:Answers.Count -ne 0){throw 'Source was not exactly reviewed/applied once'}
}
$shared=$ExpectedSource
Run-SourceAttempt 'first' @($shared,'YES') $shared
$boundBefore=[IO.File]::ReadAllText($statePath)
Run-SourceAttempt 'rerun' @('YES') $shared
if($script:SourceRequest.ContainsKey('source_root') -or $script:SourceRequest.ContainsKey('source') -or [IO.File]::ReadAllText($statePath) -cne $boundBefore){throw 'Rerun resubmitted stale source cache or rewrote custody'}
foreach($scenario in @('local-test','decline','empty-choice','advanced-source','unknown-schema','wrong-schema-type','malformed','wrong-root','invalid-source')){
  $body=$freshState
  switch($scenario){
    'unknown-schema'{$body='{"schema_version":99}'}
    'wrong-schema-type'{$body=@{schema_version='4';managed_root=$Product}|ConvertTo-Json}
    'malformed'{$body='{'}
    'wrong-root'{$body='{"schema_version":4,"managed_root":"another-root"}'}
    'invalid-source'{$body=@{schema_version=4;managed_root=$Product;source_control='invalid'}|ConvertTo-Json}
  }
  [IO.File]::WriteAllText($statePath,$body,$Utf8)
  $failed=$false
  try{
    switch($scenario){
      'local-test'{Run-SourceAttempt $scenario @('TESTE-LOCAL','YES') (Join-Path $Root 'source')}
      'decline'{Run-SourceAttempt $scenario @($shared,'') $shared}
      'empty-choice'{Run-SourceAttempt $scenario @('') $shared}
      'advanced-source'{Run-SourceAttempt $scenario @($shared,'YES','YES') $shared -Advanced}
      default{Run-SourceAttempt $scenario @() $shared}
    }
  }catch{$failed=$true}
  $success=$scenario -in @('local-test','advanced-source')
  if($failed -eq $success){throw "Wrong source scenario outcome: $scenario"}
  if($scenario -ceq 'advanced-source' -and -not $script:UpdateApplied){throw 'Advanced first source lost original reviewed activation'}
  if(-not $success){
    if($script:SourceCheckCount -ne 0 -or [IO.File]::ReadAllText($statePath) -cne $body){throw "Refusal changed source custody: $scenario"}
    if($scenario -notin @('decline','empty-choice') -and $script:SourceReviewCount -ne 1){throw "Unknown state bypassed original owner diagnostic: $scenario"}
  }
}
Write-Output 'PASS regular exe selection; exact owner clients; reserved-root and reparse preflight without child creation; explicit profile consent; existing settings preserved; selected child profile; forced official login recovery consent; original manager profile/path/update arguments; exit and caller environment restored'
`

// Execute the actual first-selection block under Windows path semantics, with
// Code discovery/profile/login set to fail. This is not a native client test.
func TestWindowsSelfServiceDesktopSelectionAvoidsCode(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows participant selection paths")
	}
	root := developerFixture(t)
	powerShell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "desktop-target.ps1")
	if err := os.WriteFile(script, []byte(windowsDesktopSelectionBehavior), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(powerShell, "-NoProfile", "-NonInteractive", "-File", script, "-Source", filepath.Join(repo, "dev/pilot/templates/installed-seed/Maestro.ps1"), "-Fixture", root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shipped Desktop selection invoked Code: %s %v", output, err)
	}
}

const windowsDesktopSelectionBehavior = `param([string]$Source,[string]$Fixture)
$ErrorActionPreference='Stop';Set-StrictMode -Version Latest
$Utf8=New-Object System.Text.UTF8Encoding($false)
$tokens=$null;$errors=$null;$ast=[Management.Automation.Language.Parser]::ParseFile($Source,[ref]$tokens,[ref]$errors)
if($errors.Count -ne 0){throw 'Frontend does not parse'}
$writer=$ast.Find({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Write-Json'},$true)
Invoke-Expression $writer.Extent.Text
$selection=$ast.Find({param($n) $n -is [Management.Automation.Language.IfStatementAst] -and $n.Extent.Text.StartsWith('if (-not (Test-Path -LiteralPath $configPath))')},$true)
if($null -eq $selection){throw 'Missing original first-selection block'}
function Stop-Maestro([string]$Message){throw $Message}
function Get-Command {throw 'CODE DISCOVERY MUST NOT RUN'}
function Resolve-CodeExecutable {throw 'CODE EXECUTABLE MUST NOT RUN'}
function Prepare-CodeConfiguration {throw 'CODE PROFILE MUST NOT RUN'}
function Ensure-CodeLogin {throw 'CODE LOGIN MUST NOT RUN'}
function Prepare-ClientParent {$p=Join-Path $Owner 'Maestro-Clients';[IO.Directory]::CreateDirectory($p)|Out-Null;return $p}
function Read-Host([string]$Prompt){if($script:Answers.Count -eq 0){throw ('Unexpected prompt '+$Prompt)};$script:Answers.Dequeue()}
$Dev=Join-Path $Fixture 'Developer';$Owner=Join-Path $Dev 'existing owner';$Base=Join-Path $Dev 'pilot';$Role='desktop-fixture';$CheckOnly=$false
foreach($p in @($Dev,$Owner,$Base)){[IO.Directory]::CreateDirectory($p)|Out-Null}
$configPath=Join-Path $Base 'participant.json';$env:MAESTRO_HOME=Join-Path $Owner 'brain'
$script:Answers=New-Object 'Collections.Generic.Queue[string]'
foreach($answer in @('desktop',$Owner,(Join-Path $Dev 'workspace'))){$script:Answers.Enqueue($answer)}
Invoke-Expression $selection.Extent.Text
$config=Get-Content -LiteralPath $configPath -Raw|ConvertFrom-Json
if($config.client_target -cne 'desktop' -or $null -ne $config.PSObject.Properties['code_executable'] -or $null -ne $config.PSObject.Properties['code_configuration_directory']){throw 'Desktop selection fabricated Code state'}
$before=[IO.File]::ReadAllText($configPath)
Invoke-Expression $selection.Extent.Text
if([IO.File]::ReadAllText($configPath) -cne $before -or $script:Answers.Count -ne 0){throw 'Resume changed selection or consumed more prompts'}
$setup=$ast.Find({param($n) $n -is [Management.Automation.Language.IfStatementAst] -and $n.Extent.Text.StartsWith('if (-not (Test-Path -LiteralPath $installed))')},$true)
if($null -eq $setup){throw 'Missing original bootstrap setup block'}
$Product=Join-Path $Base 'product';$installed=Join-Path $Product 'maestro-install.exe';$Stage=Join-Path $Base 'stage';$Run=Join-Path $Base 'run'
[IO.Directory]::CreateDirectory($Run)|Out-Null
$script:BootstrapCommands=New-Object 'Collections.Generic.List[string]'
function Confirm-Review($Review){}
function Invoke-Installer([string]$Command,$Request,[string[]]$Extra=@()){
  if($Request.target -cne 'desktop' -or $Request.ContainsKey('code_packet') -or $Request.ContainsKey('desktop_packet')){throw 'Desktop bootstrap created portable client placeholders'}
  $script:BootstrapCommands.Add($Command)
  if($Command -ceq 'review'){return [PSCustomObject]@{review_digest='reviewed-desktop'}}
  if($Command -cne 'apply' -or ($Extra -join ',') -cne '--review-digest,reviewed-desktop,--confirm'){throw 'Original bootstrap review was not preserved'}
  return [PSCustomObject]@{state='ready'}
}
Invoke-Expression $setup.Extent.Text
if(($script:BootstrapCommands -join ',') -cne 'review,apply'){throw 'Original bootstrap route was not executed'}
`
