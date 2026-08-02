package darwin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
)

// Store persists only Darwin's bounded, metadata-only receipts. It never
// stores the health packet, prompts, paths outside the maintenance scope or
// client content.
type Store struct {
	Root string
}

func (receipt Receipt) Validate() error {
	if receipt.SchemaVersion != SchemaVersion || receipt.AgentID != AgentID || receipt.DisplayName != DisplayName || receipt.Emoji != Emoji || !idPattern.MatchString(receipt.WindowID) || !validMode(receipt.Mode) || receipt.RecordedAt.IsZero() {
		return errors.New("invalid Darwin receipt header")
	}
	switch receipt.Outcome {
	case OutcomeSucceeded, OutcomeFailed, OutcomeBlocked, OutcomeNoAction, OutcomePartial:
	default:
		return errors.New("invalid Darwin receipt outcome")
	}
	if len(receipt.Actions) > maxActions {
		return errors.New("Darwin receipt contains too many actions")
	}
	for _, action := range receipt.Actions {
		if !idPattern.MatchString(action.ProposalID) || !validAction(action.Action) || !validAction(action.Rollback) || !validOutcome(action.Outcome) || !validToolOperation(action.Tool, action.Operation) || !validResource(action.Resource) {
			return errors.New("Darwin receipt contains invalid action metadata")
		}
	}
	return nil
}

func (store Store) Append(receipt Receipt) error {
	if strings.TrimSpace(store.Root) == "" {
		return errors.New("Darwin receipt root is required")
	}
	if err := receipt.Validate(); err != nil {
		return err
	}
	root := filepath.Join(store.Root, "receipts")
	if err := ensurePrivateDirectory(root); err != nil {
		return err
	}
	prefix := receipt.RecordedAt.UTC().Format("20060102T150405.000000000Z") + "-" + receipt.WindowID
	for suffix := 0; suffix < 1000; suffix++ {
		name := prefix
		if suffix > 0 {
			name += fmt.Sprintf("-%d", suffix)
		}
		if err := writeNewJSON(filepath.Join(root, name+".json"), receipt); !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	return errors.New("Darwin receipt collision limit exceeded")
}

func (store Store) Receipts() ([]Receipt, error) {
	if strings.TrimSpace(store.Root) == "" {
		return nil, errors.New("Darwin receipt root is required")
	}
	root := filepath.Join(store.Root, "receipts")
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("Darwin receipt root must be a private local directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	receipts := make([]Receipt, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, fmt.Errorf("invalid Darwin receipt entry %q", entry.Name())
		}
		var receipt Receipt
		if err := readStrictJSON(filepath.Join(root, entry.Name()), &receipt); err != nil {
			return nil, err
		}
		if err := receipt.Validate(); err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	sort.Slice(receipts, func(left, right int) bool {
		return receipts[left].RecordedAt.Before(receipts[right].RecordedAt)
	})
	return receipts, nil
}

func validAction(action Action) bool {
	switch action {
	case ActionRecordCapabilityGap, ActionRefreshDerivedState, ActionReconcileScheduler, ActionRunContractValidation, ActionReviewStateDocuments:
		return true
	default:
		return false
	}
}

func validOutcome(outcome Outcome) bool {
	switch outcome {
	case OutcomeSucceeded, OutcomeFailed, OutcomeBlocked, OutcomeNoAction:
		return true
	default:
		return false
	}
}

func validResource(resource string) bool {
	parsed, err := url.Parse(resource)
	if err != nil || parsed.Scheme != "bcgos" || parsed.Host != "health" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil || parsed.Opaque != "" || parsed.RawPath != "" {
		return false
	}
	return strings.HasPrefix(parsed.Path, "/maestro-system/") && !strings.Contains(parsed.Path, "..") && pathpkg.Clean(parsed.Path) == parsed.Path && idPattern.MatchString(pathpkg.Base(parsed.Path))
}

func validToolOperation(tool, operation string) bool {
	if tool == "filesystem" {
		return operation == "read" || operation == "write" || operation == "edit"
	}
	if tool == "probe" {
		return operation == "execute"
	}
	return tool == "validation" && operation == "run"
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("Darwin receipt root must be a private local directory")
	}
	if info.Mode().Perm() != 0o700 {
		return os.Chmod(path, 0o700)
	}
	return nil
}

func readStrictJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("Darwin receipt contains multiple JSON values")
		}
		return err
	}
	return nil
}

func writeNewJSON(path string, value any) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
