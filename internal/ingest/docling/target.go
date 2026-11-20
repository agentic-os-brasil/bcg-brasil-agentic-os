package docling

import (
	_ "embed"
	"errors"
)

const WindowsPythonArchive = "cpython-3.12.16+20261026-x86_64-pc-windows-msvc-install_only.tar.gz"
const WindowsPythonArchiveSHA256 = "590dfcbe3a34c07301e6332e13e2188b98655cd072b08afa45a7998dc4c22bc5"
const WindowsLockSHA256 = "728aab0b59dd5011070dfc4e9159e3ed6180b1668539f84275f1b88e81f5be58"

//go:embed runtime/windows-amd64/inputs.json
var windowsInputs []byte

//go:embed runtime/windows-amd64/pylock.toml
var windowsLock []byte

//go:embed runtime/windows-amd64/bridge.py
var windowsBridge []byte

type targetInputs struct {
	os, arch, pythonArchive, pythonSHA, lockSHA string
	inputs, lock, bridge, notices               []byte
	profile                                     string
	count                                       int
}

// SupportsTarget describes source construction support, never native qualification.
func SupportsTarget(goos, goarch string) bool {
	_, err := inputsForTarget(goos, goarch)
	return err == nil
}

func inputsForTarget(goos, goarch string) (targetInputs, error) {
	switch {
	case goos == "darwin" && goarch == "arm64":
		return targetInputs{goos, goarch, PythonArchive, PythonArchiveSHA256, LockSHA256, lockedInputs, rawLock, bridge, parserLicenses, LegacyProfile, 51}, nil
	case goos == "windows" && goarch == "amd64":
		return targetInputs{goos, goarch, WindowsPythonArchive, WindowsPythonArchiveSHA256, WindowsLockSHA256, windowsInputs, windowsLock, windowsBridge, parserLicenses, LegacyProfile, 52}, nil
	default:
		return targetInputs{}, errors.New("platform_unavailable")
	}
}
