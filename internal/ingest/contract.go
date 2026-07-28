// Package ingest contains the provider-neutral contract for local document
// ingestion. Providers produce bounded artifacts; this package owns request
// validation and the metadata-safe result shape.
package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const SchemaVersion = 1

type Status string

const (
	StatusUsable      Status = "usable"
	StatusPartial     Status = "partial"
	StatusDegraded    Status = "degraded"
	StatusUnavailable Status = "unavailable"
	StatusBlocked     Status = "blocked"
)

type Fidelity string

const (
	FidelityStructured Fidelity = "structured"
	FidelityTextual    Fidelity = "textual"
	FidelityUnknown    Fidelity = "unknown"
)

// Policy is intentionally explicit. A provider must not widen these limits
// because it happens to support more formats or remote inputs.
type Policy struct {
	MaxInputBytes  int64
	MaxOutputBytes int64
	AllowedExts    map[string]bool
	AllowNetwork   bool
	AllowPlugins   bool
}

func DefaultPolicy() Policy {
	return Policy{
		MaxInputBytes:  50 << 20,
		MaxOutputBytes: 100 << 20,
		AllowedExts: map[string]bool{
			".csv": true, ".docx": true, ".epub": true, ".html": true,
			".htm": true, ".json": true, ".md": true, ".txt": true,
			".xls": true, ".xlsx": true, ".xml": true,
		},
		AllowNetwork: false,
		AllowPlugins: false,
	}
}

type Request struct {
	SourcePath    string
	WorkspacePath string
	Policy        Policy
}

type Result struct {
	SchemaVersion int      `json:"schema_version"`
	Adapter       string   `json:"adapter"`
	Route         string   `json:"route"`
	Status        Status   `json:"status"`
	Fidelity      Fidelity `json:"fidelity"`
	SourceName    string   `json:"source_name"`
	SourceSHA256  string   `json:"source_sha256"`
	Format        string   `json:"format"`
	WorkspaceID   string   `json:"workspace_id"`
	OutputBytes   int64    `json:"output_bytes"`
	Warnings      []string `json:"warnings,omitempty"`
	ArtifactRef   string   `json:"artifact_ref,omitempty"`
}

func (r Request) Validate() (os.FileInfo, error) {
	if strings.TrimSpace(r.SourcePath) == "" {
		return nil, errors.New("ingestion source path is required")
	}
	if strings.TrimSpace(r.WorkspacePath) == "" {
		return nil, errors.New("ingestion workspace path is required")
	}
	policy := r.Policy
	if policy.MaxInputBytes <= 0 {
		return nil, errors.New("ingestion input limit must be positive")
	}
	if policy.MaxOutputBytes <= 0 {
		return nil, errors.New("ingestion output limit must be positive")
	}
	if policy.AllowNetwork {
		return nil, errors.New("network ingestion is unavailable in the local contract")
	}
	if policy.AllowPlugins {
		return nil, errors.New("plugin ingestion is unavailable in the local contract")
	}

	workspaceInfo, err := os.Stat(r.WorkspacePath)
	if err != nil {
		return nil, fmt.Errorf("inspect ingestion workspace: %w", err)
	}
	if !workspaceInfo.IsDir() {
		return nil, errors.New("ingestion workspace path is not a directory")
	}

	info, err := os.Lstat(r.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("inspect ingestion source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("ingestion source symlinks are not allowed")
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("ingestion source must be a regular file")
	}
	if info.Size() > policy.MaxInputBytes {
		return nil, fmt.Errorf("ingestion source exceeds %d byte limit", policy.MaxInputBytes)
	}
	ext := strings.ToLower(filepath.Ext(r.SourcePath))
	if !policy.AllowedExts[ext] {
		return nil, fmt.Errorf("ingestion format %q is not allowlisted", ext)
	}
	return info, nil
}

func Fingerprint(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
