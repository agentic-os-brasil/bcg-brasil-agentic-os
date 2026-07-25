package releasepack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
)

type BinaryProvenance struct {
	SchemaVersion int    `json:"schema_version"`
	Component     string `json:"component"`
	SourceSHA     string `json:"source_sha"`
	RunID         string `json:"run_id"`
	RunAttempt    string `json:"run_attempt"`
	RunnerOS      string `json:"runner_os"`
	RunnerArch    string `json:"runner_arch"`
	ImageOS       string `json:"image_os"`
	ImageVersion  string `json:"image_version"`
	GoVersion     string `json:"go_version"`
	GoCompiler    string `json:"go_compiler"`
	CGOEnabled    bool   `json:"cgo_enabled"`
	TargetOS      string `json:"target_os"`
	TargetArch    string `json:"target_arch"`
	BinaryName    string `json:"binary_name"`
	BinarySize    int64  `json:"binary_size"`
	BinarySHA256  string `json:"binary_sha256"`
}

func WriteBinaryProvenance(binary, version string, target Target) (string, error) {
	return WriteNativeProvenance(binary, version, target, NativeCLI)
}

func WriteNativeProvenance(
	binary, version string,
	target Target,
	component NativeComponent,
) (string, error) {
	if !supportedCandidateTarget(target) {
		return "", fmt.Errorf("unsupported provenance target %s/%s", target.OS, target.Arch)
	}
	expectedName := binaryName(version, target)
	if component == NativeBootstrapper {
		expectedName = bootstrapperBinaryName(version, target)
	} else if component != NativeCLI {
		return "", fmt.Errorf("unsupported provenance component %q", component)
	}
	if filepath.Base(binary) != expectedName {
		return "", fmt.Errorf("provenance binary must be named %s", expectedName)
	}
	info, binaryDigest, err := digestBoundedRegular(binary, 1<<30)
	if err != nil {
		return "", err
	}
	sourceSHA := os.Getenv("GITHUB_SHA")
	if len(sourceSHA) != 40 || strings.Trim(sourceSHA, "0123456789abcdef") != "" {
		return "", errors.New("GITHUB_SHA must be a lowercase 40-character commit hash")
	}
	fields := map[string]string{
		"GITHUB_RUN_ID":      os.Getenv("GITHUB_RUN_ID"),
		"GITHUB_RUN_ATTEMPT": os.Getenv("GITHUB_RUN_ATTEMPT"),
		"RUNNER_OS":          os.Getenv("RUNNER_OS"),
		"RUNNER_ARCH":        os.Getenv("RUNNER_ARCH"),
		"ImageOS":            os.Getenv("ImageOS"),
		"ImageVersion":       os.Getenv("ImageVersion"),
	}
	for name, value := range fields {
		if err := validateProvenanceField(name, value); err != nil {
			return "", err
		}
	}
	provenance := BinaryProvenance{
		SchemaVersion: 2,
		Component:     string(component),
		SourceSHA:     sourceSHA,
		RunID:         fields["GITHUB_RUN_ID"],
		RunAttempt:    fields["GITHUB_RUN_ATTEMPT"],
		RunnerOS:      fields["RUNNER_OS"],
		RunnerArch:    fields["RUNNER_ARCH"],
		ImageOS:       fields["ImageOS"],
		ImageVersion:  fields["ImageVersion"],
		GoVersion:     runtime.Version(),
		GoCompiler:    runtime.Compiler,
		CGOEnabled:    target.OS == "darwin",
		TargetOS:      target.OS,
		TargetArch:    target.Arch,
		BinaryName:    expectedName,
		BinarySize:    info.Size(),
		BinarySHA256:  binaryDigest,
	}
	encoded, err := json.MarshalIndent(provenance, "", "  ")
	if err != nil {
		return "", err
	}
	encoded = append(encoded, '\n')
	output := binary + ".provenance.json"
	file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := file.Write(encoded); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return output, nil
}

func digestBoundedRegular(path string, maximum int64) (os.FileInfo, string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximum {
		return nil, "", errors.New("provenance input must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, "", err
	}
	if written != info.Size() || written > maximum {
		return nil, "", errors.New("provenance input changed while hashing")
	}
	return info, hex.EncodeToString(hash.Sum(nil)), nil
}

func validateProvenanceField(name, value string) error {
	if value == "" || len(value) > 128 || !utf8.ValidString(value) {
		return fmt.Errorf("%s is unavailable or invalid", name)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%s contains a control character", name)
		}
	}
	return nil
}
