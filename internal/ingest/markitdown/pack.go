package markitdown

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const packSchemaVersion = 1

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Pack struct {
	State   string
	Reason  string
	Command []string
}

type manifest struct {
	SchemaVersion int    `json:"schema_version"`
	Adapter       string `json:"adapter"`
	Version       string `json:"version"`
	PythonPath    string `json:"python_path"`
	ScriptPath    string `json:"script_path"`
	PythonSHA256  string `json:"python_sha256"`
	ScriptSHA256  string `json:"script_sha256"`
	Provenance    string `json:"provenance"`
}

type ManifestVerifier func([]byte) error

func ResolvePack(dataRoot string, verify ManifestVerifier) (Pack, error) {
	if strings.TrimSpace(dataRoot) == "" {
		return Pack{State: "unavailable", Reason: "local data root is not configured"}, nil
	}
	if verify == nil {
		return Pack{State: "unavailable", Reason: "approved ingestion pack verifier is unavailable"}, nil
	}
	packRoot := filepath.Join(dataRoot, "ingestion", "markitdown")
	manifestPath := filepath.Join(packRoot, "pack.json")
	file, err := os.Open(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return Pack{State: "unavailable", Reason: "verified MarkItDown runtime pack is not installed"}, nil
	}
	if err != nil {
		return Pack{State: "unavailable", Reason: "MarkItDown runtime pack cannot be inspected"}, nil
	}
	defer file.Close()

	body, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil || len(body) > 1<<20 {
		return Pack{State: "unavailable", Reason: "MarkItDown runtime pack manifest cannot be inspected"}, nil
	}
	if err := verify(body); err != nil {
		return Pack{State: "unavailable", Reason: "MarkItDown runtime pack manifest verification failed"}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var value manifest
	if err := decoder.Decode(&value); err != nil {
		return Pack{State: "unavailable", Reason: "MarkItDown runtime pack manifest is invalid"}, nil
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Pack{State: "unavailable", Reason: "MarkItDown runtime pack manifest has trailing data"}, nil
	}
	if err := validateManifest(value); err != nil {
		return Pack{State: "unavailable", Reason: "MarkItDown runtime pack manifest is invalid"}, nil
	}
	pythonPath, err := safePackPath(packRoot, value.PythonPath)
	if err != nil {
		return Pack{State: "unavailable", Reason: "MarkItDown runtime pack executable path is unsafe"}, nil
	}
	scriptPath, err := safePackPath(packRoot, value.ScriptPath)
	if err != nil {
		return Pack{State: "unavailable", Reason: "MarkItDown runtime pack adapter path is unsafe"}, nil
	}
	if err := regularFile(pythonPath); err != nil {
		return Pack{State: "unavailable", Reason: "MarkItDown runtime pack executable is unavailable"}, nil
	}
	if err := regularFile(scriptPath); err != nil {
		return Pack{State: "unavailable", Reason: "MarkItDown runtime pack adapter is unavailable"}, nil
	}
	if err := verifyDigest(pythonPath, value.PythonSHA256); err != nil {
		return Pack{State: "unavailable", Reason: "MarkItDown runtime pack executable verification failed"}, nil
	}
	if err := verifyDigest(scriptPath, value.ScriptSHA256); err != nil {
		return Pack{State: "unavailable", Reason: "MarkItDown runtime pack adapter verification failed"}, nil
	}
	return Pack{State: "ready", Command: []string{pythonPath, scriptPath, "--request-stdin"}}, nil
}

func validateManifest(value manifest) error {
	if value.SchemaVersion != packSchemaVersion {
		return fmt.Errorf("unsupported pack schema version %d", value.SchemaVersion)
	}
	if value.Adapter != "markitdown" || !versionPattern.MatchString(value.Version) {
		return errors.New("invalid MarkItDown pack identity")
	}
	if value.PythonPath == "" || value.ScriptPath == "" || value.Provenance != "bcgos-managed-installer" {
		return errors.New("MarkItDown pack paths are required")
	}
	if !sha256Pattern.MatchString(value.PythonSHA256) || !sha256Pattern.MatchString(value.ScriptSHA256) {
		return errors.New("MarkItDown pack digests are required")
	}
	return nil
}

func safePackPath(root, relative string) (string, error) {
	if filepath.IsAbs(relative) || strings.Contains(relative, `\`) {
		return "", errors.New("pack path must be relative")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative)))
	if clean != relative || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("pack path escapes runtime root")
	}
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}

func regularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("path is not a regular file")
	}
	return nil
}

func verifyDigest(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != expected {
		return errors.New("runtime pack digest mismatch")
	}
	return nil
}
