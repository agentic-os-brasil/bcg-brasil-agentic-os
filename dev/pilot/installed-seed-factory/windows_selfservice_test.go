package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// PowerShell's actual shipped helper bodies are exercised without running the
// launcher, installer, Claude, login or GUI. Native Windows ACL checks remain
// separately qualified; only directory creation is substituted on other hosts.
func TestWindowsSelfServiceExactOwnerAndSelectedProfile(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
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
	command := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", script, "-Source", filepath.Join(repo, "dev/pilot/templates/installed-seed/Maestro.ps1"), "-Fixture", root)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("shipped Windows helper behavior: %s\n%v", output, err)
	}
	t.Log(string(output))
}

const windowsSelfServiceBehavior = `param([string]$Source,[string]$Fixture)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$Utf8=New-Object System.Text.UTF8Encoding($false)
$tokens=$null; $errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($Source,[ref]$tokens,[ref]$errors)
if($errors.Count -ne 0){throw 'Shipped PowerShell script does not parse'}
foreach($name in @('Private-File','Resolve-CodeExecutable','Private-SelectionPath','Require-Disjoint','Prepare-ClientParent','Prepare-CodeConfiguration','Invoke-SelectedCode','Invoke-Management','Ensure-CodeLogin')){
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
  $declined=$false;try{Ensure-CodeLogin -Force}catch{$declined=$true}
  if(-not $declined -or $script:AuthCalls.Count -ne 0){throw 'Forced official login ran without consent'}
  $script:Answers.Enqueue('YES');Ensure-CodeLogin -Force
  if($script:AuthCalls.Count -ne 2 -or $script:AuthCalls[0] -cne 'auth login' -or $script:AuthCalls[1] -cne 'auth status'){throw 'Force login could not recover an expired session masked by loggedIn status'}
  $script:Config|Add-Member -NotePropertyName client_parent -NotePropertyValue $clients
  $script:Config|Add-Member -NotePropertyName workspace -NotePropertyValue $Work
  $Common=@('--workspace',$Work)
  $assignment=$ast.Find({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -ceq '$Manage'},$true)
  if($null -eq $assignment){throw 'Missing actual manager argument producer'}
  Invoke-Expression $assignment.Extent.Text
  if($Manage[$Manage.IndexOf('--client-code-root')+1] -cne (Join-Path $clients 'code') -or $Manage[$Manage.IndexOf('--client-desktop-path')+1] -cne (Join-Path $clients 'maestro-product.mcpb') -or $Manage[$Manage.IndexOf('--client-code-config')+1] -cne $profile){throw 'Original manager arguments lost exact client/profile selection'}
  function Verify-Installer([string]$Path){}
  function Ensure-CodeLogin{}
  function Test-Installer {
    if($env:CLAUDE_CONFIG_DIR -cne $profile -or $args[0] -cne 'manage' -or $args[-2] -cne '--management-intent' -or $args[-1] -cne 'update'){throw 'Manager child lost reviewed profile or update arguments'}
    $global:LASTEXITCODE=9
  }
  $Installer='Test-Installer';$env:CLAUDE_CONFIG_DIR='caller-profile'
  Invoke-Management -Extra @('--management-intent','update')
  if($script:ManagementExit -ne 9 -or $env:CLAUDE_CONFIG_DIR -cne 'caller-profile'){throw 'Manager exit or caller environment was lost'}
}finally{if($had){$env:CLAUDE_CONFIG_DIR=$prior}else{Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue}}
Write-Output 'PASS regular exe selection; exact owner clients; reserved-root and reparse preflight without child creation; explicit profile consent; existing settings preserved; selected child profile; forced official login recovery consent; original manager profile/path/update arguments; exit and caller environment restored'
`
