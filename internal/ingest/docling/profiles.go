package docling

import (
	_ "embed"
	"errors"
)

const LegacyProfile = "policy-formats-v1"
const DocumentsProfile = "model-free-documents-v1"

//go:embed runtime/documents/darwin-arm64/inputs.json
var documentDarwinInputs []byte

//go:embed runtime/documents/darwin-arm64/pylock.toml
var documentDarwinLock []byte

//go:embed runtime/documents/darwin-arm64/bridge.py
var documentDarwinBridge []byte

//go:embed runtime/documents/darwin-arm64/parser-licenses.json
var documentDarwinNotices []byte

//go:embed runtime/documents/windows-amd64/inputs.json
var documentWindowsInputs []byte

//go:embed runtime/documents/windows-amd64/pylock.toml
var documentWindowsLock []byte

//go:embed runtime/documents/windows-amd64/bridge.py
var documentWindowsBridge []byte

//go:embed runtime/documents/windows-amd64/parser-licenses.json
var documentWindowsNotices []byte

// ProfileDescriptor is a copied, finite description of public authenticated inputs.
// It grants neither acquisition nor execution authority. Archive identity remains
// the independently signed release manifest's responsibility.
type ProfileDescriptor struct {
	Profile              string  `json:"profile"`
	OS                   string  `json:"os"`
	Arch                 string  `json:"arch"`
	Python               Input   `json:"python"`
	Inputs               []Input `json:"inputs"`
	LockSHA256           string  `json:"lock_sha256"`
	BridgeSHA256         string  `json:"bridge_sha256"`
	NoticesSHA256        string  `json:"notices_sha256"`
	MaximumArchiveBytes  int64   `json:"maximum_archive_bytes"`
	MaximumExpandedBytes int64   `json:"maximum_expanded_bytes"`
	MaximumFiles         int     `json:"maximum_files"`
}

func inputsForProfile(goos, goarch, profile string) (targetInputs, error) {
	t, err := inputsForTarget(goos, goarch)
	if err != nil {
		return t, err
	}
	switch profile {
	case LegacyProfile:
		return t, nil
	case DocumentsProfile:
		t.profile = profile
		if goos == "darwin" {
			t.inputs, t.lock, t.bridge, t.notices = documentDarwinInputs, documentDarwinLock, documentDarwinBridge, documentDarwinNotices
			t.count = 55
		} else {
			t.inputs, t.lock, t.bridge, t.notices = documentWindowsInputs, documentWindowsLock, documentWindowsBridge, documentWindowsNotices
			t.count = 56
		}
		t.lockSHA = digest(t.lock)
		return t, nil
	default:
		return targetInputs{}, errors.New("profile_unavailable")
	}
}

func inputsForLock(goos, goarch, lock string) (targetInputs, error) {
	for _, profile := range []string{LegacyProfile, DocumentsProfile} {
		t, err := inputsForProfile(goos, goarch, profile)
		if err == nil && t.lockSHA == lock {
			return t, nil
		}
	}
	return targetInputs{}, errors.New("profile_unavailable")
}

// DescribeProfile rejects unknown platform/profile combinations before effects.
func DescribeProfile(goos, goarch, profile string) (ProfileDescriptor, error) {
	t, err := inputsForProfile(goos, goarch, profile)
	if err != nil {
		return ProfileDescriptor{}, err
	}
	inputs := []Input{}
	if err := decode(t.inputs, &inputs); err != nil {
		return ProfileDescriptor{}, err
	}
	pythonSize := int64(25172086)
	if goos == "windows" {
		pythonSize = 47113364
	}
	python := Input{Name: "cpython", Version: PythonVersion, Filename: t.pythonArchive, SHA256: t.pythonSHA, Size: pythonSize, URL: "https://github.com/astral-sh/python-build-standalone/releases/download/20261026/" + t.pythonArchive}
	return ProfileDescriptor{Profile: profile, OS: goos, Arch: goarch, Python: python, Inputs: inputs, LockSHA256: t.lockSHA, BridgeSHA256: digest(t.bridge), NoticesSHA256: digest(t.notices), MaximumArchiveBytes: maximumArchiveBytes, MaximumExpandedBytes: maximumExpandedBytes, MaximumFiles: maximumFiles}, nil
}
