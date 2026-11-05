#Requires -Version 5.1
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$Kit,
    [Parameter(Mandatory=$true)][string]$Report
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$Utf8 = New-Object Text.UTF8Encoding($false)
if ($env:OS -ne 'Windows_NT' -or $PSVersionTable.PSEdition -ne 'Desktop') { throw 'Native stock Windows PowerShell5.1 is required.' }
if (Test-Path Env:MAESTRO_HOME) { throw 'Proof requires no inherited HOME selection.' }
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$admin = New-Object Security.Principal.SecurityIdentifier('S-1-5-32-544')
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if ($principal.IsInRole($admin) -or @($identity.Groups | Where-Object { $_.Value -eq $admin.Value }).Count -ne 0) { throw 'The proof must run as an actual ordinary account outside Administrators.' }
$Kit = [IO.Path]::GetFullPath($Kit)
$setup = Join-Path $Kit 'Preparar Maestro.ps1'
$shell = Join-Path $env:SystemRoot 'System32/WindowsPowerShell/v1.0/powershell.exe'
$developer = Join-Path ([Environment]::GetFolderPath([Environment+SpecialFolder]::UserProfile)) 'Developer'
if (-not (Test-Path -LiteralPath $developer)) { [void][IO.Directory]::CreateDirectory($developer) }
$proof = Join-Path $developer ('MaestroKitProof-' + [Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($proof)
$root = Join-Path $proof 'participant with spaces'
$commands = New-Object 'Collections.Generic.List[object]'

function Assert([bool]$Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
function Hash([string]$Path) { return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Snapshot([string]$Path) {
    $rows = New-Object 'Collections.Generic.List[string]'
    $entries = @((Get-Item -LiteralPath $Path -Force)) + @(Get-ChildItem -LiteralPath $Path -Recurse -Force)
    foreach ($entry in $entries) {
        Assert (([int]$entry.Attributes -band 0x00441400) -eq 0) 'Snapshot encountered a reparse/offline entry.'
        $acl = if ($entry.PSIsContainer) { [IO.Directory]::GetAccessControl($entry.FullName, [Security.AccessControl.AccessControlSections]'Owner,Access') } else { [IO.File]::GetAccessControl($entry.FullName, [Security.AccessControl.AccessControlSections]'Owner,Access') }
        $digest = if ($entry.PSIsContainer) { 'directory' } else { Hash $entry.FullName }
        $relative = $entry.FullName.Substring($Path.Length)
        $rows.Add($relative + '|' + [int]$entry.Attributes + '|' + $digest + '|' + $acl.GetSecurityDescriptorSddlForm([Security.AccessControl.AccessControlSections]'Owner,Access'))
    }
    $result = $rows.ToArray()
    [Array]::Sort($result, [StringComparer]::Ordinal)
    return $result -join "`n"
}
function Run-Setup([string]$Destination, [bool]$Success, [string]$Script = $setup) {
    $preference = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = @(& $shell -NoProfile -File $Script -Root $Destination 2>&1)
        $code = $LASTEXITCODE
    } finally { $ErrorActionPreference = $preference }
    $commands.Add([ordered]@{ root=$Destination; exit=$code; output=($output -join "`n") })
    Assert (($code -eq 0) -eq $Success) ('Unexpected public preparer result: ' + ($output -join "`n"))
}
function Native-Json([string]$Executable, [string[]]$Arguments) {
    $output = @(& $Executable @Arguments)
    Assert ($LASTEXITCODE -eq 0) 'Actual shipping CLI command failed.'
    return ($output -join "`n") | ConvertFrom-Json
}

Run-Setup $root $true
$binary = Join-Path $root 'product/maestro-mcp.exe'
$binding = @('--product-root', (Join-Path $root 'product'), '--user-home', (Join-Path $root 'user'), '--workspace', (Join-Path $root 'workspace'))
$inspection = Native-Json $binary (@('inspect') + $binding)
Assert ($inspection.writable -eq $true) 'Fresh canonical HOME is not writable.'
$initialized = Get-Content -LiteralPath (Join-Path $root 'diagnostico-init.json') -Raw -Encoding UTF8 | ConvertFrom-Json
Assert ($null -ne $initialized.workspace_id -and $initialized.workspace_id -ne '') 'Actual workspace identity is missing.'
$plugin = Join-Path $root 'exports/code-marketplace/plugins/maestro-product'
$packet = Get-Content -LiteralPath (Join-Path $plugin 'maestro-packet.json') -Raw -Encoding UTF8 | ConvertFrom-Json
Assert ($packet.schema_version -eq 1 -and $packet.mode -eq 'mcp_only') 'Wrong participant packet profile.'
Assert ($packet.target.os -eq 'windows' -and $packet.target.arch -eq 'amd64' -and $packet.native.state -eq 'unavailable') 'Packet inferred unsupported target/native qualification.'
$config = Get-Content -LiteralPath (Join-Path $plugin '.mcp.json') -Raw -Encoding UTF8 | ConvertFrom-Json
Assert (@($config.mcpServers.PSObject.Properties).Count -eq 1) 'Unexpected additional MCP server.'
Assert ($config.mcpServers.maestro.command -ceq '${CLAUDE_PLUGIN_ROOT}/scripts/maestro-mcp.exe') 'Plugin uses an unexpected entrypoint.'
$expectedArgs = @('serve') + $binding
Assert (($config.mcpServers.maestro.args | ConvertTo-Json -Compress) -ceq ($expectedArgs | ConvertTo-Json -Compress)) 'Plugin does not bind the exact participant paths.'
Assert ((Hash $binary) -ceq (Hash (Join-Path $Kit 'bin/maestro-mcp-windows-amd64.exe'))) 'Preparation changed the shipping binary.'
Assert ((Hash $binary) -ceq (Hash (Join-Path $plugin 'scripts/maestro-mcp.exe'))) 'Plugin changed the actual ordinary server.'
foreach ($file in $packet.files) {
    Assert ((Hash (Join-Path $plugin $file.path)) -ceq $file.sha256) ('Packet member hash differs: ' + $file.path)
}
$methods = [IO.File]::ReadAllLines([IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../adapters/mcp/packaging/testdata/method-ids.txt')))
Assert ($methods.Count -eq 92) 'Independent method catalog changed.'
foreach ($method in $methods) { Assert (Test-Path -LiteralPath (Join-Path $plugin ('skills/' + $method + '/SKILL.md'))) ('Method missing: ' + $method) }
Add-Type -AssemblyName System.IO.Compression.FileSystem
$extension = [IO.Compression.ZipFile]::OpenRead((Join-Path $root 'exports/maestro-pilot.mcpb'))
try {
    $manifestEntry = $extension.GetEntry('manifest.json')
    $reader = New-Object IO.StreamReader($manifestEntry.Open(), $Utf8)
    try { $manifest = $reader.ReadToEnd() | ConvertFrom-Json } finally { $reader.Dispose() }
    Assert ($manifest.version -eq '0.0.0' -and $manifest.compatibility.platforms.Count -eq 1 -and $manifest.compatibility.platforms[0] -eq 'win32') 'Wrong Desktop packet label/platform.'
    Assert ($manifest.server.mcp_config.command -ceq '${__dirname}/server/maestro-mcp.exe') 'Wrong Desktop entrypoint.'
    Assert (($manifest.server.mcp_config.args | ConvertTo-Json -Compress) -ceq ($expectedArgs | ConvertTo-Json -Compress)) 'Desktop does not bind the same participant paths.'
    $serverEntry = $extension.GetEntry('server/maestro-mcp.exe')
    $stream = $serverEntry.Open()
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $extensionHash = ([BitConverter]::ToString($sha.ComputeHash($stream))).Replace('-', '').ToLowerInvariant() } finally { $sha.Dispose(); $stream.Dispose() }
    Assert ($extensionHash -ceq (Hash $binary)) 'Desktop packet changed the shipping server.'
} finally { $extension.Dispose() }
$before = Snapshot $root
Run-Setup $root $true
Assert ((Snapshot $root) -ceq $before) 'Repeat preparation changed whole private/workspace/product bytes or ACLs.'

$foreign = Join-Path $proof 'foreign'
[void][IO.Directory]::CreateDirectory($foreign)
[IO.File]::WriteAllText((Join-Path $foreign 'keep.txt'), 'foreign synthetic data', $Utf8)
$foreignBefore = Snapshot $foreign
Run-Setup $foreign $false
Assert ((Snapshot $foreign) -ceq $foreignBefore) 'Foreign root was changed.'
$partial = Join-Path $proof 'partial'
[void][IO.Directory]::CreateDirectory($partial)
[IO.File]::WriteAllText((Join-Path $partial '.maestro-pilot-owned'), [IO.File]::ReadAllText((Join-Path $Kit 'KIT-ID.txt')), $Utf8)
$before = Snapshot $partial
Run-Setup $partial $false
Assert ((Snapshot $partial) -ceq $before) 'Partial root was changed.'

$unsafe = Join-Path $proof 'unsafe-acl'
[void][IO.Directory]::CreateDirectory($unsafe)
[IO.File]::WriteAllText((Join-Path $unsafe 'keep.txt'), 'unsafe ACL canary', $Utf8)
$acl = New-Object Security.AccessControl.DirectorySecurity
$acl.SetOwner($identity.User)
$acl.SetAccessRuleProtection($true, $false)
$acl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($identity.User, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow')))
$everyone = New-Object Security.Principal.SecurityIdentifier('S-1-1-0')
$acl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($everyone, 'ReadAndExecute', 'ContainerInherit,ObjectInherit', 'None', 'Allow')))
[IO.Directory]::SetAccessControl($unsafe, $acl)
$before = Snapshot $unsafe
Run-Setup $unsafe $false
Assert ((Snapshot $unsafe) -ceq $before) 'Unsafe existing ACL or data was repaired.'
Run-Setup '\\unavailable.invalid\share\must-not-contact' $false

$env:MAESTRO_HOME = $foreign
$redirected = Join-Path $proof 'redirected'
try { Run-Setup $redirected $false } finally { Remove-Item Env:MAESTRO_HOME }
Assert (-not (Test-Path -LiteralPath $redirected)) 'Inherited HOME created an alternate owner.'
Assert ((Snapshot $foreign) -ceq $foreignBefore) 'Inherited HOME changed foreign data.'

$alias = Join-Path $proof 'alias'
[void](New-Item -ItemType Junction -Path $alias -Target $foreign)
$foreignBefore = Snapshot $foreign
Run-Setup (Join-Path $alias 'must-not-exist') $false
Assert ((Snapshot $foreign) -ceq $foreignBefore) 'A reparse path changed its target.'

$tampered = Join-Path $proof 'tampered-kit'
[void][IO.Directory]::CreateDirectory($tampered)
foreach ($entry in Get-ChildItem -LiteralPath $Kit -Force) { Copy-Item -LiteralPath $entry.FullName -Destination $tampered -Recurse -Force }
[IO.File]::AppendAllText((Join-Path $tampered 'LEIA-ME.md'), "`nUnexpected bytes.", $Utf8)
$tamperedRoot = Join-Path $proof 'tampered-target'
Run-Setup $tamperedRoot $false (Join-Path $tampered 'Preparar Maestro.ps1')
Assert (-not (Test-Path -LiteralPath $tamperedRoot)) 'Tampered kit created participant state.'

$managed = Join-Path $plugin '.mcp.json'
[IO.File]::AppendAllText($managed, "`n", $Utf8)
$before = Snapshot $root
Run-Setup $root $false
Assert ((Snapshot $root) -ceq $before) 'Changed managed content was repaired or private state was altered.'

$zip = [IO.Compression.ZipFile]::OpenRead($Kit + '.zip')
try {
    $payloads = @($zip.Entries | Where-Object { $_.Name -ne '' })
    Assert ($payloads.Count -eq 9) 'Generic archive payload inventory changed.'
    foreach ($entry in $payloads) {
        Assert ($entry.FullName.StartsWith([IO.Path]::GetFileName($Kit) + '/', [StringComparison]::Ordinal)) 'Generic archive uses an unexpected root.'
        Assert (-not $entry.FullName.StartsWith('/') -and $entry.FullName -notmatch '(^|/)\.\.(/|$)' -and $entry.FullName -notmatch '(^|/)(user|workspace|exports)(/|$)') 'Generic archive has private state or unsafe paths.'
        $relative = $entry.FullName.Substring($entry.FullName.IndexOf('/') + 1)
        $stream = $entry.Open(); $sha = [Security.Cryptography.SHA256]::Create()
        try { $digest = ([BitConverter]::ToString($sha.ComputeHash($stream))).Replace('-', '').ToLowerInvariant() } finally { $sha.Dispose(); $stream.Dispose() }
        Assert ($digest -ceq (Hash (Join-Path $Kit $relative))) 'ZIP differs from the actual prepared generic payload.'
    }
} finally { $zip.Dispose() }
$result = [ordered]@{
    passed=$true; mode='ordinary_unversioned_developer_rehearsal'; actual_standard_user=$true
    shipping_server_sha256=(Hash $binary); workspace_id=$initialized.workspace_id
    fresh_setup=$true; local_plugin_mcpb_exact_binding=$true; complete_repeat_snapshot_unchanged=$true
    foreign_partial_unsafeACL_HOME_UNC_reparse_tamper_refused=$true; changed_managed_content_preserved=$true
    generic_zip_exact_payloads=9; native_Code_Desktop='not_run'; host_confirmation='not_exercised_by_preparation'
    private_proof_root=$proof; commands=$commands.ToArray()
}
if (Test-Path -LiteralPath $Report) { throw 'Report destination already exists; previous evidence is preserved.' }
[IO.File]::WriteAllText($Report, ($result | ConvertTo-Json -Depth 15) + "`n", $Utf8)
Write-Output ($result | ConvertTo-Json -Depth 15)
