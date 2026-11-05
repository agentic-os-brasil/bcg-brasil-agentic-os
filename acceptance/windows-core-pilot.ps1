[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$TestBinary,
    [Parameter(Mandatory = $true)][string]$Server,
    [Parameter(Mandatory = $true)][string]$OutputRoot
)
$ErrorActionPreference = 'Stop'
$name = 'mcp' + [Guid]::NewGuid().ToString('N').Substring(0, 10)
$password = [Guid]::NewGuid().ToString('N') + 'aA1!'
$secure = ConvertTo-SecureString $password -AsPlainText -Force
$credential = [PSCredential]::new("$env:COMPUTERNAME\$name", $secure)
$account = $null
try {
    $account = New-LocalUser -Name $name -Password $secure -AccountNeverExpires -PasswordNeverExpires -UserMayNotChangePassword
    $users = Get-LocalGroup -SID 'S-1-5-32-545'
    if (-not (Get-LocalGroupMember -Group $users | Where-Object { $_.SID -eq $account.SID })) {
        Add-LocalGroupMember -Group $users -Member $account
    }
    New-Item -ItemType Directory -Path $OutputRoot -Force | Out-Null
    $acl = [System.Security.AccessControl.DirectorySecurity]::new()
    $acl.SetAccessRuleProtection($true, $false)
    foreach ($sid in @($account.SID, [System.Security.Principal.SecurityIdentifier]::new('S-1-5-18'), [System.Security.Principal.SecurityIdentifier]::new('S-1-5-32-544'))) {
        $rule = [System.Security.AccessControl.FileSystemAccessRule]::new($sid, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow')
        $acl.AddAccessRule($rule)
    }
    Set-Acl -LiteralPath $OutputRoot -AclObject $acl
    $payload = Join-Path $OutputRoot 'payload'
    New-Item -ItemType Directory -Path $payload | Out-Null
    $nativeTest = Join-Path $payload 'windows-core.test.exe'
    $nativeServer = Join-Path $payload 'maestro-mcp.exe'
    Copy-Item -LiteralPath $TestBinary -Destination $nativeTest
    Copy-Item -LiteralPath $Server -Destination $nativeServer
    $env:MAESTRO_WINDOWS_CORE_PROOF = '1'
    $env:MAESTRO_WINDOWS_CORE_SERVER = $nativeServer
    $env:MAESTRO_WINDOWS_CORE_PROOF_ROOT = $OutputRoot
    $stdout = Join-Path $OutputRoot 'native-test.stdout.log'
    $stderr = Join-Path $OutputRoot 'native-test.stderr.log'
    $process = Start-Process -FilePath $nativeTest -ArgumentList @('-test.run', '^TestWindowsCorePilot$', '-test.v', '-test.timeout=180s') -Credential $credential -LoadUserProfile -WorkingDirectory $OutputRoot -RedirectStandardOutput $stdout -RedirectStandardError $stderr -Wait -PassThru
    Get-Content -LiteralPath $stdout
    if (Test-Path -LiteralPath $stderr) { Get-Content -LiteralPath $stderr }
    if ($process.ExitCode -ne 0) { throw "Native core test failed with exit $($process.ExitCode); retain the actual log." }
    $log = Get-Content -LiteralPath $stdout -Raw
    if ($log -notmatch 'native_windows_token elevated=false admin_member=false' -or $log -notmatch '--- PASS: TestWindowsCorePilot' -or $log -match '--- SKIP:') {
        throw 'The standard-user native proof did not run completely.'
    }
} finally {
    Remove-Item Env:MAESTRO_WINDOWS_CORE_PROOF -ErrorAction SilentlyContinue
    Remove-Item Env:MAESTRO_WINDOWS_CORE_SERVER -ErrorAction SilentlyContinue
    Remove-Item Env:MAESTRO_WINDOWS_CORE_PROOF_ROOT -ErrorAction SilentlyContinue
    if ($account) { Remove-LocalUser -SID $account.SID -ErrorAction Continue }
    $password = $null
    $secure = $null
    $credential = $null
}
