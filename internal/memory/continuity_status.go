package memory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	MaximumContinuityCaptureFiles   = 7
	MaximumContinuityCaptureEntries = 64
	MaximumContinuityCommitEntries  = 32
	maximumContinuityCaptureBytes   = 64 << 10
	maximumContinuityManifestBytes  = 16 << 10
)

var continuityCaptureName = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.jsonl$`)

// ContinuityStatus is the bounded, authenticated memory input for Session
// Start. It intentionally does not enumerate the legacy capture journal.
type ContinuityStatus struct {
	State                string
	AttestedCaptureFiles int
}

func (engine *Engine) ContinuityStatus(workspaceID string) (ContinuityStatus, error) {
	if strings.TrimSpace(engine.Root) == "" {
		return ContinuityStatus{}, errors.New("memory root is required")
	}
	if err := validateWorkspaceID(workspaceID); err != nil {
		return ContinuityStatus{}, err
	}
	count, err := engine.validAttestedCaptureCount(workspaceID)
	if err != nil {
		return ContinuityStatus{}, err
	}
	status := ContinuityStatus{State: "empty", AttestedCaptureFiles: count}
	err = engine.validateContinuityManifestWatermark(workspaceID)
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if errors.Is(err, ErrNoValidCommit) {
		status.State = "unavailable"
		return status, nil
	}
	if err != nil {
		return ContinuityStatus{}, err
	}
	status.State = "available"
	return status, nil
}

func (engine *Engine) validateContinuityManifestWatermark(workspaceID string) error {
	directoryPath := filepath.Join(engine.workspaceRoot(workspaceID), "commits")
	before, err := os.Lstat(directoryPath)
	if err != nil {
		return err
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return errors.New("memory commit directory is not private")
	}
	directory, err := os.Open(directoryPath)
	if err != nil {
		return err
	}
	defer directory.Close()
	opened, err := directory.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return errors.New("memory commit directory changed during secure open")
	}
	entries, err := directory.ReadDir(MaximumContinuityCommitEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) > MaximumContinuityCommitEntries {
		return errors.New("memory commit history exceeds continuity scan bound")
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.HasPrefix(entry.Name(), ".") || !strings.HasSuffix(entry.Name(), ".json") {
			return fmt.Errorf("invalid memory continuity commit entry %q", entry.Name())
		}
		names = append(names, entry.Name())
	}
	if len(names) == 0 {
		return os.ErrNotExist
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return readBoundedContinuityManifest(filepath.Join(directoryPath, names[0]), workspaceID)
}

func readBoundedContinuityManifest(path, workspaceID string) error {
	before, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() || before.Size() > maximumContinuityManifestBytes {
		return errors.New("memory continuity manifest is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return errors.New("memory continuity manifest changed during secure open")
	}
	body, err := io.ReadAll(io.LimitReader(file, maximumContinuityManifestBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maximumContinuityManifestBytes {
		return errors.New("memory continuity manifest exceeds bounded read limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest CommitManifest
	if err := decoder.Decode(&manifest); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	if manifest.SchemaVersion != 1 || manifest.WorkspaceID != workspaceID || manifest.TransactionID == "" || manifest.CommittedAt.IsZero() || len(manifest.Artifacts) == 0 || len(manifest.Artifacts) > 128 {
		return errors.New("memory continuity manifest is invalid")
	}
	for key, relative := range manifest.Artifacts {
		clean := filepath.Clean(filepath.FromSlash(relative))
		if key == "" || clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return errors.New("memory continuity manifest artifact escapes workspace")
		}
	}
	return nil
}

func (engine *Engine) validAttestedCaptureCount(workspaceID string) (int, error) {
	directoryPath := engine.currentPath(workspaceID, "l1", "attested-captures")
	before, err := os.Lstat(directoryPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return 0, errors.New("attested capture directory is not private")
	}
	directory, err := os.Open(directoryPath)
	if err != nil {
		return 0, err
	}
	defer directory.Close()
	opened, err := directory.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return 0, errors.New("attested capture directory changed during secure open")
	}
	entries, err := directory.ReadDir(MaximumContinuityCaptureFiles + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	if len(entries) > MaximumContinuityCaptureFiles {
		return 0, errors.New("attested capture history exceeds continuity scan bound")
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	attestor := CaptureAttestor{Root: engine.Root}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !continuityCaptureName.MatchString(entry.Name()) {
			return 0, fmt.Errorf("invalid attested capture entry %q", entry.Name())
		}
		captures, err := readValidatedCaptureFile(filepath.Join(directoryPath, entry.Name()), workspaceID, attestor)
		if err != nil {
			return 0, err
		}
		count += captures
		if count > MaximumContinuityCaptureEntries {
			return 0, errors.New("attested capture entries exceed continuity scan bound")
		}
	}
	return count, nil
}

func readValidatedCaptureFile(path, workspaceID string, attestor CaptureAttestor) (int, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() || before.Size() > maximumContinuityCaptureBytes {
		return 0, errors.New("attested capture is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return 0, errors.New("attested capture changed during secure open")
	}
	body, err := io.ReadAll(io.LimitReader(file, maximumContinuityCaptureBytes+1))
	if err != nil {
		return 0, err
	}
	if len(body) > maximumContinuityCaptureBytes {
		return 0, errors.New("attested capture exceeds bounded read limit")
	}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 4096), 16<<10)
	count := 0
	for scanner.Scan() {
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		var capture Capture
		if err := decoder.Decode(&capture); err != nil {
			return 0, err
		}
		if err := ensureJSONEOF(decoder); err != nil {
			return 0, err
		}
		if capture.WorkspaceID != workspaceID {
			return 0, errors.New("attested capture workspace does not match continuity status")
		}
		if err := attestor.Verify(capture); err != nil {
			return 0, err
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, errors.New("attested capture file is empty")
	}
	return count, nil
}
