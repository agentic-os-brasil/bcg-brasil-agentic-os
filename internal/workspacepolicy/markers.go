package workspacepolicy

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// CaseMarkers is the existing selector's exact bounded snapshot, not a new registry.
type CaseMarkers struct {
	Active  string `json:"active,omitempty"`
	Pending string `json:"pending,omitempty"`
}

func SnapshotCaseMarkers(workspace string) (CaseMarkers, error) {
	files, e := scheduler.HoldDirectory(filepath.Join(workspace, "brain"), nil)
	if errors.Is(e, os.ErrNotExist) {
		return CaseMarkers{}, nil
	}
	if e != nil {
		return CaseMarkers{}, e
	}
	defer files.Close()
	return SnapshotCaseMarkersAt(files)
}
func SnapshotCaseMarkersAt(files *scheduler.HeldDirectory) (CaseMarkers, error) {
	var out CaseMarkers
	for _, entry := range []struct {
		name   string
		target *string
	}{{".active", &out.Active}, {".pending", &out.Pending}} {
		body, err := files.ReadFile(filepath.Join("accounts", entry.name), 1024, false)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return out, err
		}

		pair := MarkerPair(string(body))
		if !validPair(pair) {
			return out, errors.New("case_marker_invalid")
		}
		*entry.target = string(body)
	}
	return out, nil
}
func validPair(pair string) bool {
	parts := strings.Split(pair, "/")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 96 {
			return false
		}
		for _, c := range part {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return false
			}
		}
	}
	return true
}

// PrepareCaseSelection publishes only the reviewed pending pair. A concurrent
// marker transition is fenced by its exact preimage under the canonical lock.
func PrepareCaseSelection(workspace, pair string, expected CaseMarkers) error {
	files, e := scheduler.HoldDirectory(filepath.Join(workspace, "brain"), nil)
	if e != nil {
		return e
	}
	defer files.Close()
	return PrepareCaseSelectionAt(files, pair, expected)
}
func PrepareCaseSelectionAt(files *scheduler.HeldDirectory, pair string, expected CaseMarkers) error {
	if !validPair(pair) {
		return errors.New("case_pair_invalid")
	}
	if expected.Active != "" && MarkerPair(expected.Active) != pair || expected.Pending != "" && MarkerPair(expected.Pending) != pair {
		return errors.New("case_selection_conflict")
	}
	dir := "accounts"
	if err := files.EnsureDirectory(dir); err != nil {
		return err
	}
	return files.UpdatePrivateFile(filepath.Join(dir, ".pending"), 1024, func(body []byte) ([]byte, error) {
		current, err := SnapshotCaseMarkersAt(files)
		if err != nil {
			return nil, err
		}
		if current != expected || !bytes.Equal(body, []byte(expected.Pending)) {
			return nil, errors.New("case_selection_changed")
		}
		if len(body) > 0 {
			return nil, nil
		}
		return []byte(pair + "\n"), nil
	})
}

// CommitCaseSelection makes durable active publication the commit point.
func CommitCaseSelection(workspace, pair string, expected CaseMarkers) error {
	files, e := scheduler.HoldDirectory(filepath.Join(workspace, "brain"), nil)
	if e != nil {
		return e
	}
	defer files.Close()
	return CommitCaseSelectionAt(files, pair, expected)
}
func CommitCaseSelectionAt(files *scheduler.HeldDirectory, pair string, expected CaseMarkers) error {
	if !validPair(pair) || MarkerPair(expected.Pending) != pair {
		return errors.New("case_pending_required")
	}
	return files.UpdatePrivateFile("accounts/.active", 1024, func(body []byte) ([]byte, error) {
		current, err := SnapshotCaseMarkersAt(files)
		if err != nil {
			return nil, err
		}
		if current != expected || !bytes.Equal(body, []byte(expected.Active)) {
			return nil, errors.New("case_selection_changed")
		}
		if current.Active != "" && strings.TrimSpace(current.Active) != pair {
			return nil, errors.New("case_selection_conflict")
		}
		if len(body) > 0 {
			return nil, nil
		}
		return []byte(pair + "\n"), nil
	})
}

// CleanCaseSelection removes only the exact pending preimage after confirming
// the committed active pair. Never restore an old active selector on failure.
func CleanCaseSelection(workspace, pair string, expected CaseMarkers) error {
	files, e := scheduler.HoldDirectory(filepath.Join(workspace, "brain"), nil)
	if e != nil {
		return e
	}
	defer files.Close()
	return CleanCaseSelectionAt(files, pair, expected)
}
func CleanCaseSelectionAt(files *scheduler.HeldDirectory, pair string, expected CaseMarkers) error {
	if MarkerPair(expected.Active) != pair || MarkerPair(expected.Pending) != pair {
		return errors.New("case_cleanup_changed")
	}
	return files.RemoveExact("accounts/.pending", 1024, []byte(expected.Pending), func(publish func() error) error {
		current, err := SnapshotCaseMarkersAt(files)
		if err != nil {
			return err
		}
		if current != expected {
			return errors.New("case_cleanup_changed")
		}
		return publish()
	})
}

// MarkerPair normalizes existing selection vocabulary without changing the raw
// bytes retained separately for exact compare-and-publish/conditional cleanup.
func MarkerPair(raw string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "\uFEFF")))
}
