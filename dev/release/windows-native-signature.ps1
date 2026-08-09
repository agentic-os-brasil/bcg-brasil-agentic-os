Set-StrictMode -Version Latest

function Read-MaestroUInt16LE([IO.FileStream]$Stream, [long]$Offset) {
    if ($Offset -lt 0 -or $Offset -gt $Stream.Length - 2) {
        throw "PE field lies outside the bootstrapper."
    }
    $buffer = [byte[]]::new(2)
    $Stream.Position = $Offset
    if ($Stream.Read($buffer, 0, $buffer.Length) -ne $buffer.Length) {
        throw "Could not read the complete PE field."
    }
    return [BitConverter]::ToUInt16($buffer, 0)
}

function Read-MaestroUInt32LE([IO.FileStream]$Stream, [long]$Offset) {
    if ($Offset -lt 0 -or $Offset -gt $Stream.Length - 4) {
        throw "PE field lies outside the bootstrapper."
    }
    $buffer = [byte[]]::new(4)
    $Stream.Position = $Offset
    if ($Stream.Read($buffer, 0, $buffer.Length) -ne $buffer.Length) {
        throw "Could not read the complete PE field."
    }
    return [BitConverter]::ToUInt32($buffer, 0)
}

function Get-MaestroPECertificateTableStatus([string]$Path) {
    $stream = [IO.File]::Open($Path, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    try {
        if ($stream.Length -lt 64) {
            throw "Bootstrapper is too small to be a well-formed PE file."
        }
        $dos = [byte[]]::new(2)
        $stream.Position = 0
        if ($stream.Read($dos, 0, $dos.Length) -ne 2 -or $dos[0] -ne 0x4d -or $dos[1] -ne 0x5a) {
            throw "Bootstrapper is missing the PE DOS signature."
        }
        $peOffset = [long](Read-MaestroUInt32LE $stream 0x3c)
        if ($peOffset -lt 64 -or $peOffset -gt $stream.Length - 24) {
            throw "Bootstrapper has an invalid PE header offset."
        }
        $pe = [byte[]]::new(4)
        $stream.Position = $peOffset
        if ($stream.Read($pe, 0, $pe.Length) -ne 4 -or
            $pe[0] -ne 0x50 -or $pe[1] -ne 0x45 -or $pe[2] -ne 0 -or $pe[3] -ne 0) {
            throw "Bootstrapper is missing the PE signature."
        }
        $optionalHeaderSize = [long](Read-MaestroUInt16LE $stream ($peOffset + 20))
        $optionalHeaderOffset = $peOffset + 24
        if ($optionalHeaderSize -le 0 -or $optionalHeaderOffset + $optionalHeaderSize -gt $stream.Length) {
            throw "Bootstrapper has an invalid PE optional header."
        }
        $magic = Read-MaestroUInt16LE $stream $optionalHeaderOffset
        $dataDirectoryOffset = switch ($magic) {
            0x10b { 96 }
            0x20b { 112 }
            default { throw "Bootstrapper uses an unsupported PE optional-header format." }
        }
        $certificateEntryOffset = $dataDirectoryOffset + (4 * 8)
        if ($optionalHeaderSize -lt $certificateEntryOffset + 8) {
            throw "Bootstrapper PE optional header does not contain the certificate-table entry."
        }
        $numberOfDataDirectories = Read-MaestroUInt32LE $stream ($optionalHeaderOffset + $dataDirectoryOffset - 4)
        if ($numberOfDataDirectories -lt 5) {
            throw "Bootstrapper PE optional header does not declare the certificate-table data directory."
        }
        $certificateOffset = [uint64](Read-MaestroUInt32LE $stream ($optionalHeaderOffset + $certificateEntryOffset))
        $certificateSize = [uint64](Read-MaestroUInt32LE $stream ($optionalHeaderOffset + $certificateEntryOffset + 4))
        if ($certificateOffset -eq 0 -and $certificateSize -eq 0) {
            return "NotSigned"
        }
        if ($certificateOffset -eq 0 -or $certificateSize -eq 0) {
            throw "Bootstrapper PE certificate-table entry is malformed."
        }
        if ($certificateOffset + $certificateSize -gt [uint64]$stream.Length) {
            throw "Bootstrapper PE certificate table lies outside the file."
        }
        return "CertificatePresent"
    }
    finally {
        $stream.Dispose()
    }
}

function Get-MaestroAuthenticodeStatus([string]$Path) {
    $authenticodeCommand = Get-Command "Get-AuthenticodeSignature" -CommandType Cmdlet -ErrorAction SilentlyContinue
    if ($null -ne $authenticodeCommand) {
        return (Get-AuthenticodeSignature -LiteralPath $Path).Status.ToString()
    }
    return Get-MaestroPECertificateTableStatus $Path
}
