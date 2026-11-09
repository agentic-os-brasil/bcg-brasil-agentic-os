package memory

import (
	"errors"
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
	barrier, err := engine.readVisibility(workspaceID)
	if err != nil {
		return err
	}
	if barrier.Generation > 0 {
		manifest, _, err := engine.latestManifest(workspaceID)
		if err != nil {
			return err
		}
		if len(manifest.Artifacts) == 0 {
			return os.ErrNotExist
		}
		return nil
	}
	directoryPath := filepath.Join(engine.workspaceRoot(workspaceID), "commits")
	entries, err := memoryReadDir(directoryPath)
	if err != nil {
		return err
	}
	if len(entries) > MaximumContinuityCommitEntries {
		return errors.New("memory commit history exceeds continuity scan bound")
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.HasPrefix(entry.Name(), ".") || !strings.HasSuffix(entry.Name(), ".json") {
			return errors.New("invalid memory continuity commit entry")
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
	body, err := memoryReadFile(path, maximumContinuityManifestBytes)
	if err != nil {
		return err
	}
	var manifest CommitManifest
	if err := decodeMemoryJSON(body, &manifest); err != nil {
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
	barrier, err := engine.readVisibility(workspaceID)
	if err != nil {
		return 0, err
	}
	paths, err := engine.journalPaths(workspaceID, "attested-captures")
	if err != nil {
		return 0, err
	}
	if len(paths) > MaximumContinuityCaptureFiles {
		return 0, errors.New("attested capture history exceeds continuity scan bound")
	}
	count := 0
	for _, path := range paths {
		body, err := memoryReadFile(path, maximumContinuityCaptureBytes)
		if err != nil {
			return 0, err
		}
		captures, err := engine.decodeJournal(workspaceID, body, true)
		if err != nil {
			return 0, err
		}
		for _, c := range captures {
			if barrier.Denied[journalCaptureID(c)] < 1 {
				count++
			}
		}
	}
	owned, err := engine.ownedCaptures(workspaceID)
	if err != nil {
		return 0, err
	}
	for _, c := range owned {
		if c.Revision > barrier.Denied[c.ID] {
			count++
		}
	}
	if count > MaximumContinuityCaptureEntries {
		return 0, errors.New("attested capture entries exceed continuity scan bound")
	}
	after, err := engine.readVisibility(workspaceID)
	if err != nil {
		return 0, err
	}
	if after.Revision != barrier.Revision {
		return 0, ErrCaptureConflict
	}
	return count, nil
}
