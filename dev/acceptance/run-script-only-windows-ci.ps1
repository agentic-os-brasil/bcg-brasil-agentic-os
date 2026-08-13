param(
  [switch]$Child,
  [string]$ConfigPath,
  [string]$RepositoryRoot,
  [string]$TestBinary,
  [string]$PreviousZip,
  [string]$PreviousChecksum,
  [string]$CandidateZip,
  [string]$CandidateChecksum,
  [string]$OutputReceipt
)

$ErrorActionPreference = 'Stop'

function Stop-WindowsCI([string]$Message) {
  throw "MAESTRO-WINDOWS-CI: $Message; nenhum dado do owner foi removido"
}

function Test-AdministratorToken {
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = New-Object Security.Principal.WindowsPrincipal($identity)
  return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Invoke-ChildAcceptance([pscustomobject]$Config) {
  if (Test-AdministratorToken) {
    Stop-WindowsCI 'o processo de acceptance recebeu token administrativo'
  }
  $scratch = [string]$Config.scratch
  New-Item -ItemType Directory -Path $scratch -Force | Out-Null
  $env:TEMP = $scratch
  $env:TMP = $scratch
  $env:USERPROFILE = [string]$Config.user_profile
  Set-Location (Join-Path ([string]$Config.repository_root) 'internal\dev\releasepack')

  & ([string]$Config.test_binary) `
    '-test.run=^TestWindowsScriptPortableInstallsUpdatesRollsBackAndRunsSevenHooksAsUnelevatedUser$' `
    '-test.v' '-test.count=1' '-test.timeout=15m'
  if ($LASTEXITCODE -ne 0) {
    Stop-WindowsCI "teste condicionado falhou com exit code $LASTEXITCODE"
  }

  $acceptance = Join-Path ([string]$Config.repository_root) 'dev\acceptance\script-only-windows-artifact.ps1'
  & powershell.exe -NoLogo -NoProfile -NonInteractive -File $acceptance `
    -PreviousZip ([string]$Config.previous_zip) `
    -PreviousChecksum ([string]$Config.previous_checksum) `
    -CandidateZip ([string]$Config.candidate_zip) `
    -CandidateChecksum ([string]$Config.candidate_checksum) `
    -PreviousVersion '0.1.17' `
    -CandidateVersion '0.1.18' `
    -OutputReceipt ([string]$Config.output_receipt)
  if ($LASTEXITCODE -ne 0) {
    Stop-WindowsCI "acceptance do ZIP falhou com exit code $LASTEXITCODE"
  }
}

if ($Child) {
  if (-not (Test-Path -LiteralPath $ConfigPath -PathType Leaf)) {
    Stop-WindowsCI 'configuração child ausente'
  }
  $childConfig = [IO.File]::ReadAllText($ConfigPath) | ConvertFrom-Json
  Invoke-ChildAcceptance $childConfig
  exit 0
}

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
  Stop-WindowsCI 'este orquestrador requer Windows real'
}
if (-not (Test-AdministratorToken)) {
  Stop-WindowsCI 'o runner hospedado precisa criar um usuário padrão descartável antes do ensaio'
}
foreach ($path in @($RepositoryRoot, $TestBinary, $PreviousZip, $PreviousChecksum, $CandidateZip, $CandidateChecksum)) {
  if (-not (Test-Path -LiteralPath $path)) {
    Stop-WindowsCI "entrada ausente: $path"
  }
}

$runRoot = Join-Path $env:RUNNER_TEMP ("maestro-windows-standard-user-" + [guid]::NewGuid().ToString('N'))
$childScratch = Join-Path $runRoot 'scratch with spaces'
$childProfile = Join-Path $runRoot 'standard user profile'
$inputRoot = Join-Path $runRoot 'inputs'
$childReceipt = Join-Path $runRoot 'windows-preview18-receipt.json'
$receiptParent = Split-Path -Parent ([IO.Path]::GetFullPath($OutputReceipt))
New-Item -ItemType Directory -Path $runRoot, $childScratch, $childProfile, $inputRoot, $receiptParent -Force | Out-Null
$childTestBinary = Join-Path $inputRoot 'releasepack-tests.exe'
$childPreviousZip = Join-Path $inputRoot (Split-Path $PreviousZip -Leaf)
$childPreviousChecksum = Join-Path $inputRoot (Split-Path $PreviousChecksum -Leaf)
$childCandidateZip = Join-Path $inputRoot (Split-Path $CandidateZip -Leaf)
$childCandidateChecksum = Join-Path $inputRoot (Split-Path $CandidateChecksum -Leaf)
Copy-Item -LiteralPath $TestBinary -Destination $childTestBinary
Copy-Item -LiteralPath $PreviousZip -Destination $childPreviousZip
Copy-Item -LiteralPath $PreviousChecksum -Destination $childPreviousChecksum
Copy-Item -LiteralPath $CandidateZip -Destination $childCandidateZip
Copy-Item -LiteralPath $CandidateChecksum -Destination $childCandidateChecksum

$suffix = [guid]::NewGuid().ToString('N').Substring(0, 6)
$userName = "MaestroCI$suffix"
$plainPassword = "M!a$([guid]::NewGuid().ToString('N'))z9"
$securePassword = ConvertTo-SecureString $plainPassword -AsPlainText -Force
$credential = New-Object Management.Automation.PSCredential("$env:COMPUTERNAME\$userName", $securePassword)
$createdUser = $false

try {
  New-LocalUser -Name $userName -Password $securePassword -PasswordNeverExpires -UserMayNotChangePassword | Out-Null
  $createdUser = $true
  $principalName = "$env:COMPUTERNAME\$userName"
  & icacls.exe $RepositoryRoot /grant "${principalName}:(OI)(CI)RX" /T /C | Out-Null
  if ($LASTEXITCODE -ne 0) { Stop-WindowsCI 'não foi possível conceder leitura do checkout ao usuário padrão' }
  & icacls.exe $runRoot /grant "${principalName}:(OI)(CI)M" /T /C | Out-Null
  if ($LASTEXITCODE -ne 0) { Stop-WindowsCI 'não foi possível preparar a área descartável do usuário padrão' }

  $config = [ordered]@{
    repository_root = [IO.Path]::GetFullPath($RepositoryRoot)
    test_binary = $childTestBinary
    previous_zip = $childPreviousZip
    previous_checksum = $childPreviousChecksum
    candidate_zip = $childCandidateZip
    candidate_checksum = $childCandidateChecksum
    output_receipt = $childReceipt
    scratch = $childScratch
    user_profile = $childProfile
  }
  $configPath = Join-Path $runRoot 'child-config.json'
  [IO.File]::WriteAllText($configPath, ($config | ConvertTo-Json -Compress) + "`n")

  $self = [IO.Path]::GetFullPath($MyInvocation.MyCommand.Path)
  $childCommand = "& '$($self.Replace("'", "''"))' -Child -ConfigPath '$($configPath.Replace("'", "''"))'"
  $encodedCommand = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($childCommand))
  $stdout = Join-Path $runRoot 'child.stdout.log'
  $stderr = Join-Path $runRoot 'child.stderr.log'
  $process = Start-Process -FilePath powershell.exe -Credential $credential -LoadUserProfile `
    -ArgumentList "-NoLogo -NoProfile -NonInteractive -EncodedCommand $encodedCommand" `
    -RedirectStandardOutput $stdout -RedirectStandardError $stderr -Wait -PassThru
  $stdoutBody = if (Test-Path -LiteralPath $stdout) { [IO.File]::ReadAllText($stdout) } else { '' }
  $stderrBody = if (Test-Path -LiteralPath $stderr) { [IO.File]::ReadAllText($stderr) } else { '' }
  Write-Output $stdoutBody
  if ($stderrBody) { Write-Output "MAESTRO-WINDOWS-CI child stderr:`n$stderrBody" }
  if ($process.ExitCode -ne 0) {
    Stop-WindowsCI "processo padrão falhou com exit code $($process.ExitCode)"
  }
  if (-not (Test-Path -LiteralPath $childReceipt -PathType Leaf)) {
    Stop-WindowsCI 'processo padrão não produziu receipt'
  }
  Copy-Item -LiteralPath $childReceipt -Destination $OutputReceipt -Force
} finally {
  $plainPassword = $null
  if ($createdUser) {
    Remove-LocalUser -Name $userName -ErrorAction SilentlyContinue
  }
}
