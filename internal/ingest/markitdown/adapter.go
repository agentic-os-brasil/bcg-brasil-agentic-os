// Package markitdown invokes the managed local MarkItDown adapter without
// widening the core ingestion policy.
package markitdown

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
)

var workspaceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type Adapter struct {
	Command      []string
	ArtifactRoot string
	WorkspaceID  string
	Route        string
	Policy       ingest.Policy
	Timeout      time.Duration
}

type request struct {
	SourcePath     string `json:"source_path"`
	OutputPath     string `json:"output_path"`
	MaxOutputBytes int64  `json:"max_output_bytes"`
	AllowNetwork   bool   `json:"allow_network"`
	EnablePlugins  bool   `json:"enable_plugins"`
}

type response struct {
	Status      ingest.Status   `json:"status"`
	Fidelity    ingest.Fidelity `json:"fidelity"`
	Format      string          `json:"format"`
	OutputBytes int64           `json:"output_bytes"`
	Warnings    []string        `json:"warnings"`
}

func (a Adapter) Convert(ctx context.Context, source ingest.Request) (ingest.Result, error) {
	if source.Policy.MaxInputBytes == 0 {
		source.Policy = a.Policy
	}
	if source.Policy.MaxInputBytes == 0 {
		source.Policy = ingest.DefaultPolicy()
	}
	result := ingest.Result{
		SchemaVersion: ingest.SchemaVersion,
		Adapter:       "markitdown",
		Route:         a.Route,
		Status:        ingest.StatusUnavailable,
		Fidelity:      ingest.FidelityUnknown,
		WorkspaceID:   a.WorkspaceID,
	}
	if result.Route == "" {
		result.Route = "markitdown_local"
	}

	if !workspaceIDPattern.MatchString(a.WorkspaceID) {
		result.Status = ingest.StatusBlocked
		return result, errors.New("markitdown workspace identity is invalid")
	}
	if strings.TrimSpace(a.ArtifactRoot) == "" {
		result.Status = ingest.StatusBlocked
		return result, errors.New("markitdown artifact root is required")
	}
	if a.Timeout <= 0 {
		a.Timeout = 30 * time.Second
	}

	_, err := source.Validate()
	if err != nil {
		result.Status = ingest.StatusBlocked
		result.Warnings = []string{err.Error()}
		return result, err
	}
	result.SourceName = filepath.Base(source.SourcePath)
	result.Format = strings.TrimPrefix(strings.ToLower(filepath.Ext(source.SourcePath)), ".")
	result.SourceSHA256, err = ingest.Fingerprint(source.SourcePath)
	if err != nil {
		result.Warnings = []string{"source fingerprint unavailable"}
		return result, err
	}
	if len(a.Command) == 0 || strings.TrimSpace(a.Command[0]) == "" {
		result.Warnings = []string{"managed MarkItDown runtime command is not configured"}
		return result, nil
	}

	if err := ensureArtifactRoot(a.ArtifactRoot); err != nil {
		result.Status = ingest.StatusBlocked
		return result, err
	}
	artifactDir := filepath.Join(a.ArtifactRoot, a.WorkspaceID)
	if err := ensureArtifactRoot(artifactDir); err != nil {
		return result, fmt.Errorf("create ingestion artifact directory: %w", err)
	}
	outputName := result.SourceSHA256 + ".md"
	outputPath := filepath.Join(artifactDir, outputName)
	result.ArtifactRef = filepath.ToSlash(filepath.Join(a.WorkspaceID, outputName))

	body, err := json.Marshal(request{
		SourcePath:     source.SourcePath,
		OutputPath:     outputPath,
		MaxOutputBytes: source.Policy.MaxOutputBytes,
		AllowNetwork:   false,
		EnablePlugins:  false,
	})
	if err != nil {
		return result, err
	}

	deadlineCtx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()
	command := exec.CommandContext(deadlineCtx, a.Command[0], a.Command[1:]...)
	command.Stdin = bytes.NewReader(body)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		if errors.Is(deadlineCtx.Err(), context.DeadlineExceeded) {
			result.Warnings = []string{"MarkItDown conversion exceeded the time limit"}
		} else {
			result.Warnings = []string{"MarkItDown runtime was unavailable or conversion failed"}
		}
		return result, nil
	}

	decoder := json.NewDecoder(&stdout)
	decoder.DisallowUnknownFields()
	var converted response
	if err := decoder.Decode(&converted); err != nil {
		result.Warnings = []string{"MarkItDown runtime returned an invalid result"}
		return result, nil
	}
	if converted.Fidelity == "" {
		converted.Fidelity = ingest.FidelityTextual
	}
	result.Fidelity = converted.Fidelity
	result.Warnings = append(result.Warnings, converted.Warnings...)
	if converted.Format != "" && converted.Format != result.Format {
		result.Warnings = append(result.Warnings, "MarkItDown runtime reported an unexpected format")
		result.Status = ingest.StatusDegraded
		return result, nil
	}

	outputInfo, err := os.Lstat(outputPath)
	if err != nil || outputInfo.Mode()&os.ModeSymlink != 0 || !outputInfo.Mode().IsRegular() {
		result.Status = ingest.StatusDegraded
		result.Warnings = append(result.Warnings, "MarkItDown produced no safe local artifact")
		return result, nil
	}
	if outputInfo.Size() > source.Policy.MaxOutputBytes {
		result.Status = ingest.StatusDegraded
		result.Warnings = append(result.Warnings, "MarkItDown output exceeded the configured limit")
		return result, nil
	}
	result.OutputBytes = outputInfo.Size()
	result.Status = converted.Status
	if result.Status == "" {
		result.Status = ingest.StatusPartial
	}
	if !validStatus(result.Status) {
		result.Status = ingest.StatusDegraded
		result.Warnings = append(result.Warnings, "MarkItDown returned an unknown quality status")
	}
	return result, nil
}

func ensureArtifactRoot(root string) error {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(root, 0o700)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("markitdown artifact root must be a real directory")
	}
	return nil
}

func validStatus(value ingest.Status) bool {
	switch value {
	case ingest.StatusUsable, ingest.StatusPartial, ingest.StatusDegraded, ingest.StatusUnavailable, ingest.StatusBlocked:
		return true
	default:
		return false
	}
}
