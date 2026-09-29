function Invoke-MaestroRuntime {
  param([Parameter(Mandatory=$true)][string]$Root, [string[]]$RuntimeArgs, [string]$Payload)
  $ErrorActionPreference = 'Stop'
  $osName = 'windows'
  $extension = '.exe'
  $arch = $env:PROCESSOR_ARCHITEW6432
  if (-not $arch) { $arch = $env:PROCESSOR_ARCHITECTURE }
  if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    # PS7 contract checks use the actual host artifact; never emulate Windows.
    if ($PSVersionTable.PSVersion.Major -lt 7) { throw 'Unsupported non-Windows PowerShell' }
    if ([Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([Runtime.InteropServices.OSPlatform]::OSX)) { $osName = 'darwin' }
    elseif ([Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([Runtime.InteropServices.OSPlatform]::Linux)) { $osName = 'linux' }
    else { throw 'Unsupported operating system' }
    $arch = [Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture.ToString()
    $extension = ''
  }
  switch ($arch.ToUpperInvariant()) { 'ARM64' { $arch = 'arm64' }; 'AMD64' { $arch = 'amd64' }; 'X64' { $arch = 'amd64' }; default { throw 'Unsupported architecture' } }
  $rel = "runtime/$osName-$arch/maestro-runtime$extension"
  $binary = Join-Path $Root $rel
  foreach ($candidate in @((Join-Path $Root 'runtime'), (Join-Path $Root "runtime/$osName-$arch"), (Join-Path $Root 'runtime/manifest.json'), $binary)) {
    $entry = Get-Item -LiteralPath $candidate -Force
    if ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Aliased runtime artifact' }
  }
  $manifest = Get-Content -LiteralPath (Join-Path $Root 'runtime/manifest.json') -Raw -Encoding UTF8 | ConvertFrom-Json
  if ($manifest.schema_version -ne 1) { throw 'Invalid manifest' }
  $entries = @($manifest.artifacts | Where-Object { $_.path -ceq $rel -and $_.os -ceq $osName -and $_.arch -ceq $arch })
  if ($entries.Count -ne 1 -or $entries[0].sha256 -notmatch '^[a-f0-9]{64}$') { throw 'Invalid artifact' }
  $item = Get-Item -LiteralPath $binary
  if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Aliased executable' }
  $hash = (Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($hash -cne $entries[0].sha256) { throw 'Integrity mismatch' }
  $OutputEncoding = New-Object System.Text.UTF8Encoding($false)
  [Console]::InputEncoding = $OutputEncoding
  $Payload | & $binary @RuntimeArgs
  if ($LASTEXITCODE -ne 0) { throw 'Maestro helper failed' }
}
