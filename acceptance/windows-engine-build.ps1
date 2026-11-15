# Build only exact committed source and a publicly readable copied SDK.
# The existing credentialed driver owns all ordinary-account execution.
param([string]$KitArchive, [string]$KitIdentity)
$ErrorActionPreference = 'Stop'
if (-not $env:GITHUB_ENV -or -not $env:RUNNER_TEMP) { throw 'Use this builder inside the reviewed hosted qualification job.' }
$ErrorActionPreference = 'Stop'
if ($env:GOROOT) { throw 'Native engine proof refuses caller-supplied GOROOT.' }
$artifactMode = -not [string]::IsNullOrWhiteSpace($KitArchive) -or -not [string]::IsNullOrWhiteSpace($KitIdentity)
$artifactInput = $null
if ($artifactMode) {
  if (-not $KitArchive -or -not $KitIdentity -or -not (Test-Path -LiteralPath $KitArchive -PathType Leaf) -or -not (Test-Path -LiteralPath $KitIdentity -PathType Leaf)) { throw 'Exact pilot archive and identity must both exist.' }
  $artifactInput = Get-Content -LiteralPath $KitIdentity -Raw -Encoding UTF8 | ConvertFrom-Json
  $revisionInput = (git rev-parse HEAD).Trim()
  $treeInput = (git rev-parse 'HEAD^{tree}').Trim()
  if ($LASTEXITCODE -ne 0 -or $artifactInput.schema_version -ne 1 -or $artifactInput.source_revision -cne $revisionInput -or $artifactInput.source_tree -cne $treeInput -or
      $artifactInput.producer_run_id -cne $env:GITHUB_RUN_ID -or $artifactInput.generic_zip_sha256 -cnotmatch '^[a-f0-9]{64}$' -or
      (Get-FileHash -LiteralPath $KitArchive -Algorithm SHA256).Hash.ToLowerInvariant() -cne $artifactInput.generic_zip_sha256) { throw 'Exact pilot producer, source or archive identity differs.' }
  if ((Get-Content -LiteralPath ($KitArchive + '.sha256') -Raw -Encoding UTF8).Trim() -cne ($artifactInput.generic_zip_sha256 + '  ' + [IO.Path]::GetFileName($KitArchive))) { throw 'Exact pilot checksum sidecar differs.' }
}

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
if ($artifactMode) {
  $artifactRoot = Join-Path $publicRoot 'pilot-artifact'
  New-Item -ItemType Directory -Path $artifactRoot | Out-Null
  $kitName = [IO.Path]::GetFileNameWithoutExtension($KitArchive)
  Add-Type -AssemblyName System.IO.Compression.FileSystem
  $zip = [IO.Compression.ZipFile]::OpenRead($KitArchive)
  try {
    $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    [long]$total = 0
    foreach ($entry in $zip.Entries) {
      $total += $entry.Length
      if ($entry.Length -gt 134217728 -or $total -gt 268435456 -or $seen.Count -ge 512) { throw 'Exact pilot archive exceeds its bounded input contract.' }
      $name = $entry.FullName
      if ($name -cnotmatch ('^' + [regex]::Escape($kitName) + '/') -or $name -match '(^|/)\.\.?(/|$)|[\\:\x00-\x1f]' -or -not $seen.Add($name) -or
          (($entry.ExternalAttributes -shr 16) -band 0xF000) -eq 0xA000) { throw 'Exact pilot archive has an unsafe or duplicate entry.' }
    }
  } finally { $zip.Dispose() }
  $nativeArchive = Join-Path $artifactRoot ([IO.Path]::GetFileName($KitArchive))
  Copy-Item -LiteralPath $KitArchive -Destination $nativeArchive
  Copy-Item -LiteralPath ($KitArchive + '.sha256') -Destination ($nativeArchive + '.sha256')
  Expand-Archive -LiteralPath $nativeArchive -DestinationPath $artifactRoot
  $kitRoot = Join-Path $artifactRoot $kitName
  $kitServer = Join-Path $kitRoot 'bin/maestro-mcp-windows-amd64.exe'
  if ((Get-FileHash -LiteralPath $nativeArchive -Algorithm SHA256).Hash.ToLowerInvariant() -cne $artifactInput.generic_zip_sha256 -or
      (Get-FileHash -LiteralPath $kitServer -Algorithm SHA256).Hash.ToLowerInvariant() -cne $artifactInput.shipping_server_sha256 -or
      (Get-FileHash -LiteralPath (Join-Path $kitRoot 'SHA256SUMS') -Algorithm SHA256).Hash.ToLowerInvariant() -cne $artifactInput.kit_id) { throw 'Exact pilot extracted ZIP, KIT-ID or PE differs.' }
  $nativeKitIdentity = Join-Path $publicRoot 'pilot-artifact-input.json'
  Copy-Item -LiteralPath $KitIdentity -Destination $nativeKitIdentity
}

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
$atlasBinary = Join-Path $binaryRoot 'atlasops.test.exe'
Push-Location -LiteralPath $sourceRoot
try {
  & $sdkGo test -c ./internal/atlasops -o $atlasBinary
  if ($LASTEXITCODE -ne 0) { throw 'Authored Atlas companion compilation failed.' }
  $atlasInventory = @(& $atlasBinary '-test.list' '^(Test|Example)')
  if ($LASTEXITCODE -ne 0 -or $atlasInventory.Count -eq 0 -or
      @($atlasInventory | Sort-Object -CaseSensitive -Unique).Count -ne $atlasInventory.Count -or
      @($atlasInventory | Where-Object { $_ -cnotmatch '^(Test|Example)[A-Za-z0-9_]*$' }).Count -ne 0) { throw 'Authored Atlas full inventory must be closed and unique.' }
} finally { Pop-Location }
$authoredPackage = [ordered]@{name='atlasops'; package='./internal/atlasops'; binary=$atlasBinary
  sha256=(Get-FileHash -LiteralPath $atlasBinary -Algorithm SHA256).Hash.ToLowerInvariant()
  working_directory=(Join-Path $sourceRoot 'internal/atlasops'); tests=$atlasInventory}
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
  # Complete compiled inventory is distinct from the four selected native setup parents.
  $expectedSetupInventory = @(
    'TestModernFactoryFirstInstallAndRestart',
    'TestDirectInnerInstallRequiresProductionAuthority',
    'TestModernSyncedSourceLifecycle',
    'TestSyncedPublicationInterleavingAndReadOnlyReviews',
    'TestCodeRegistrationPreservesDisabledAndRefusesForeignSource',
    'TestCodeRegistrationWithUnrelatedCachedPlugin',
    'TestCodeRegistrationDoesNotTrustExitAcknowledgement',
    'TestCodeReadbackRequiresExplicitEnabledState',
    'TestCurrentClientShippingNativeRefusal',
    'TestModernClientRegistrationLifecycle'
  )
  if ($LASTEXITCODE -ne 0 -or $setupInventory.Count -ne 10 -or
      @((Compare-Object $expectedSetupInventory ($setupInventory | Sort-Object -CaseSensitive) -CaseSensitive)).Count -ne 0) { throw 'Setup full inventory must contain the ten existing first-install, source and client parents.' }
  $packagingBinary = Join-Path $binaryRoot 'packaging.test.exe'
  & $sdkGo test -c ./packaging -o $packagingBinary
  if ($LASTEXITCODE -ne 0) { throw 'Native owned-process packaging test compilation failed.' }
  $packagingInventory = @(& $packagingBinary '-test.list' '^(Test|Example)')
  if ($LASTEXITCODE -ne 0 -or 'TestNativeWindowsOwnedProcessTree' -cnotin $packagingInventory -or
      @($packagingInventory | Sort-Object -CaseSensitive -Unique).Count -ne $packagingInventory.Count -or
      @($packagingInventory | Where-Object { $_ -cnotmatch '^(Test|Example)[A-Za-z0-9_]*$' }).Count -ne 0) {
    throw 'Packaging full inventory must be closed, unique and contain the native owned-process family.'
  }
  if ($artifactMode) {
    Copy-Item -LiteralPath $kitServer -Destination $server
    if ((Get-FileHash -LiteralPath $server -Algorithm SHA256).Hash.ToLowerInvariant() -cne $artifactInput.shipping_server_sha256) { throw 'Exact pilot PE changed during sealing; rebuilding is unavailable.' }
  } else {
    & $sdkGo build -o $server ./cmd/maestro-mcp
    if ($LASTEXITCODE -ne 0) { throw 'Shipping MCP compilation failed.' }
  }
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
    working_directory=(Join-Path $sourceRoot 'adapters/mcp/setup'); tests=$setupInventory},
  [ordered]@{name='packaging'; package='./adapters/mcp/packaging'; binary=$packagingBinary
    sha256=(Get-FileHash -LiteralPath $packagingBinary -Algorithm SHA256).Hash.ToLowerInvariant()
    working_directory=(Join-Path $sourceRoot 'adapters/mcp/packaging'); tests=$packagingInventory}
)
$manifest = [ordered]@{
  schema_version=1; qualification_profile='windows_private_engine_v1'; source_revision=$revision; source_root=$sourceRoot
  source_archive_sha256=(Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
  sdk_root=$sdkRoot; sdk_go=$sdkGo; sdk_go_sha256=(Get-FileHash -LiteralPath $sdkGo -Algorithm SHA256).Hash.ToLowerInvariant()
  vendor_files=$vendorFiles; adapter_vendor_files=$adapterVendorFiles; packages=$entries
  adapter_profile='windows_private_engine_adapters_v1'; adapter_packages=$adapterEntries
  authored_package=$authoredPackage
  public_test_binary_sha256=(Get-FileHash -LiteralPath $testBinary -Algorithm SHA256).Hash.ToLowerInvariant()
  public_server_sha256=(Get-FileHash -LiteralPath $server -Algorithm SHA256).Hash.ToLowerInvariant()
}
$manifest | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath (Join-Path $publicRoot 'engine-build-manifest.json') -Encoding utf8
"MAESTRO_ENGINE_PUBLIC_ROOT=$publicRoot" | Out-File -FilePath $env:GITHUB_ENV -Encoding utf8 -Append

if ($artifactMode) {
  "MAESTRO_ENGINE_PILOT_KIT=$kitRoot" | Out-File -FilePath $env:GITHUB_ENV -Encoding utf8 -Append
  "MAESTRO_ENGINE_PILOT_IDENTITY=$nativeKitIdentity" | Out-File -FilePath $env:GITHUB_ENV -Encoding utf8 -Append
}
