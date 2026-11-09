# Build only exact committed source and a publicly readable copied SDK.
# The existing credentialed driver owns all ordinary-account execution.
$ErrorActionPreference = 'Stop'
if (-not $env:GITHUB_ENV -or -not $env:RUNNER_TEMP) { throw 'Use this builder inside the reviewed hosted qualification job.' }
$ErrorActionPreference = 'Stop'
if ($env:GOROOT) { throw 'Native engine proof refuses caller-supplied GOROOT.' }
$installedRoot = (go env GOROOT).Trim()
if ($LASTEXITCODE -ne 0) { throw 'Pinned SDK lookup failed.' }
$publicRoot = Join-Path $env:SystemDrive ('MaestroNativeEngine-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $publicRoot -ErrorAction Stop | Out-Null
$acl = [Security.AccessControl.DirectorySecurity]::new()
$acl.SetAccessRuleProtection($true, $false)
foreach ($sid in @('S-1-5-18', 'S-1-5-32-544')) {
  $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new([Security.Principal.SecurityIdentifier]::new($sid), 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow'))
}
$acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new([Security.Principal.SecurityIdentifier]::new('S-1-5-32-545'), 'ReadAndExecute', 'ContainerInherit,ObjectInherit', 'None', 'Allow'))
Set-Acl -LiteralPath $publicRoot -AclObject $acl
$sdkRoot = Join-Path $publicRoot 'sdk'
New-Item -ItemType Directory -Path $sdkRoot | Out-Null
Get-ChildItem -LiteralPath $installedRoot -Force | Copy-Item -Destination $sdkRoot -Recurse
$sdkGo = Join-Path $sdkRoot 'bin/go.exe'
if ((Get-FileHash -LiteralPath (Join-Path $installedRoot 'bin/go.exe') -Algorithm SHA256).Hash -cne
    (Get-FileHash -LiteralPath $sdkGo -Algorithm SHA256).Hash) { throw 'Copied SDK differs from pinned executable.' }
if ((& $sdkGo env GOROOT).Trim() -cne $sdkRoot -or $LASTEXITCODE -ne 0) { throw 'Copied SDK selected an unexpected GOROOT.' }
$sourceRoot = Join-Path $publicRoot 'source'
$binaryRoot = Join-Path $publicRoot 'bin'
New-Item -ItemType Directory -Path $sourceRoot, $binaryRoot | Out-Null
$revision = (git rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $revision -cnotmatch '^[0-9a-f]{40}$') { throw 'Exact source revision is unavailable.' }
$archive = Join-Path $env:RUNNER_TEMP 'native-engine-source.tar'
git archive --format=tar --output=$archive HEAD
if ($LASTEXITCODE -ne 0) { throw 'Exact source archive failed.' }
tar -xf $archive -C $sourceRoot
if ($LASTEXITCODE -ne 0) { throw 'Exact source staging failed.' }
$env:GOCACHE = Join-Path $env:RUNNER_TEMP 'native-engine-build-cache'
$env:GOTOOLCHAIN = 'local'
$env:GOFLAGS = '-buildvcs=false'
$packages = @('execution', 'datahome', 'ownerctx', 'memory', 'scheduler', 'processidentity', 'winprivate', 'zipmigration')
$entries = @()
$moduleInputs = @('go.mod', 'go.sum', 'adapters/mcp/go.mod', 'adapters/mcp/go.sum') | ForEach-Object {
  [ordered]@{path=$_; sha256=(Get-FileHash -LiteralPath (Join-Path $sourceRoot $_) -Algorithm SHA256).Hash}
}
Push-Location -LiteralPath $sourceRoot
try {
  & $sdkGo mod vendor
  if ($LASTEXITCODE -ne 0) { throw 'Owning dependency closure staging failed.' }
  $env:GOFLAGS = '-mod=vendor -buildvcs=false'
  foreach ($package in $packages) {
    $binary = Join-Path $binaryRoot ($package + '.test.exe')
    & $sdkGo test -c ('./internal/' + $package) -o $binary
    if ($LASTEXITCODE -ne 0) { throw "Native owning test compilation failed: $package" }
    $tests = @(& $binary '-test.list' '^(Test|Example)')
    if ($LASTEXITCODE -ne 0 -or $tests.Count -eq 0 -or @($tests | Sort-Object -CaseSensitive -Unique).Count -ne $tests.Count -or
        @($tests | Where-Object { $_ -cnotmatch '^(Test|Example)[A-Za-z0-9_]*$' }).Count -ne 0) { throw 'Owning inventory must be nonempty, unique and closed.' }
    $entries += [ordered]@{name=$package; package=('./internal/' + $package); binary=$binary
      sha256=(Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant()
      working_directory=(Join-Path $sourceRoot ('internal/' + $package)); tests=$tests}
  }
} finally { Pop-Location }
$testBinary = Join-Path $binaryRoot 'mcp.test.exe'
$server = Join-Path $binaryRoot 'maestro-mcp.exe'
Push-Location -LiteralPath (Join-Path $sourceRoot 'adapters/mcp')
try {
  $env:GOFLAGS = '-buildvcs=false'
  & $sdkGo mod vendor
  if ($LASTEXITCODE -ne 0) { throw 'Public dependency closure staging failed.' }
  $env:GOFLAGS = '-mod=vendor -buildvcs=false'
  & $sdkGo test -c . -o $testBinary
  if ($LASTEXITCODE -ne 0) { throw 'Public native test compilation failed.' }
  $commandBinary = Join-Path $binaryRoot 'mcp-command.test.exe'
  & $sdkGo test -c ./cmd/maestro-mcp -o $commandBinary
  if ($LASTEXITCODE -ne 0) { throw 'Public command test compilation failed.' }
  $commandInventory = @(& $commandBinary '-test.list' '^(Test|Example)')
  if ($LASTEXITCODE -ne 0 -or $commandInventory.Count -eq 0 -or @($commandInventory | Sort-Object -CaseSensitive -Unique).Count -ne $commandInventory.Count -or
      @($commandInventory | Where-Object { $_ -cnotmatch '^(Test|Example)[A-Za-z0-9_]*$' }).Count -ne 0) { throw 'Command full inventory must be closed and unique.' }
  $setupBinary = Join-Path $binaryRoot 'setup.test.exe'
  & $sdkGo test -c ./setup -o $setupBinary
  if ($LASTEXITCODE -ne 0) { throw 'First-install setup test compilation failed.' }
  $setupInventory = @(& $setupBinary '-test.list' '^(Test|Example)')
  if ($LASTEXITCODE -ne 0 -or $setupInventory.Count -ne 2 -or
      @((Compare-Object @('TestModernFactoryFirstInstallAndRestart', 'TestDirectInnerInstallRequiresProductionAuthority') ($setupInventory | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0) { throw 'Setup full inventory must contain the two existing actual-process parents.' }
  & $sdkGo build -o $server ./cmd/maestro-mcp
  if ($LASTEXITCODE -ne 0) { throw 'Shipping MCP compilation failed.' }
  $inventory = @(& $testBinary '-test.list' '^(Test|Example)')
  if ($LASTEXITCODE -ne 0 -or 'TestWindowsCorePilot' -cnotin $inventory -or
      @($inventory | Sort-Object -Unique).Count -ne $inventory.Count -or
      @($inventory | Where-Object { $_ -cnotmatch '^(Test|Example)[A-Za-z0-9_]*$' }).Count -ne 0) { throw 'Closed test inventory is unavailable or invalid.' }
} finally { Pop-Location }
foreach ($moduleInput in $moduleInputs) {
  if ((Get-FileHash -LiteralPath (Join-Path $sourceRoot $moduleInput.path) -Algorithm SHA256).Hash -cne $moduleInput.sha256) {
    throw 'Dependency staging changed a committed module input.'
  }
}
$vendorRoot = Join-Path $sourceRoot 'vendor'
$vendorFiles = @(Get-ChildItem -LiteralPath $vendorRoot -Recurse -File | Sort-Object FullName | ForEach-Object {
  if (($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Vendored dependency cannot be a reparse file.' }
  [ordered]@{path=$_.FullName.Substring($vendorRoot.Length + 1).Replace('\','/'); sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}
})
$adapterVendorRoot = Join-Path $sourceRoot 'adapters/mcp/vendor'
$adapterVendorFiles = @(Get-ChildItem -LiteralPath $adapterVendorRoot -Recurse -File | Sort-Object FullName | ForEach-Object {
  if (($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'MCP vendored dependency cannot be a reparse file.' }
  [ordered]@{path=$_.FullName.Substring($adapterVendorRoot.Length + 1).Replace('\','/'); sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}
})
$adapterEntries = @(
  [ordered]@{name='mcp'; package='./adapters/mcp'; binary=$testBinary
    sha256=(Get-FileHash -LiteralPath $testBinary -Algorithm SHA256).Hash.ToLowerInvariant()
    working_directory=(Join-Path $sourceRoot 'adapters/mcp'); tests=$inventory},
  [ordered]@{name='mcp-command'; package='./adapters/mcp/cmd/maestro-mcp'; binary=$commandBinary
    sha256=(Get-FileHash -LiteralPath $commandBinary -Algorithm SHA256).Hash.ToLowerInvariant()
    working_directory=(Join-Path $sourceRoot 'adapters/mcp/cmd/maestro-mcp'); tests=$commandInventory},
  [ordered]@{name='setup'; package='./adapters/mcp/setup'; binary=$setupBinary
    sha256=(Get-FileHash -LiteralPath $setupBinary -Algorithm SHA256).Hash.ToLowerInvariant()
    working_directory=(Join-Path $sourceRoot 'adapters/mcp/setup'); tests=$setupInventory}
)
$manifest = [ordered]@{
  schema_version=1; qualification_profile='windows_private_engine_v1'; source_revision=$revision; source_root=$sourceRoot
  source_archive_sha256=(Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
  sdk_root=$sdkRoot; sdk_go=$sdkGo; sdk_go_sha256=(Get-FileHash -LiteralPath $sdkGo -Algorithm SHA256).Hash.ToLowerInvariant()
  vendor_files=$vendorFiles; adapter_vendor_files=$adapterVendorFiles; packages=$entries
  adapter_profile='windows_private_engine_adapters_v1'; adapter_packages=$adapterEntries
  public_test_binary_sha256=(Get-FileHash -LiteralPath $testBinary -Algorithm SHA256).Hash.ToLowerInvariant()
  public_server_sha256=(Get-FileHash -LiteralPath $server -Algorithm SHA256).Hash.ToLowerInvariant()
}
$manifest | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath (Join-Path $publicRoot 'engine-build-manifest.json') -Encoding utf8
"MAESTRO_ENGINE_PUBLIC_ROOT=$publicRoot" | Out-File -FilePath $env:GITHUB_ENV -Encoding utf8 -Append
