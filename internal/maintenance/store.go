package maintenance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Store persists only command receipts. It never receives packets, prompts,
// tool payloads or proposal bodies.
type Store struct {
	Root string
}

func (store Store) AppendReceipt(receipt Receipt) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	if store.Root == "" {
		return errors.New("maintenance receipt root is required")
	}
	root := filepath.Join(store.Root, "workspaces", receipt.WorkspaceID, "receipts", receipt.JobID)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	if err := ensurePrivateDirectory(root); err != nil {
		return err
	}
	path := filepath.Join(root, receipt.CommandID+".json")
	if err := writeReceipt(path, receipt); errors.Is(err, os.ErrExist) {
		existing, readErr := readReceipt(path)
		if readErr != nil {
			return readErr
		}
		if existing != receipt {
			return errors.New("maintenance receipt idempotency collision")
		}
		return nil
	} else {
		return err
	}
}

func (store Store) Receipts(workspaceID, jobID string) ([]Receipt, error) {
	if store.Root == "" || !commandIDPattern.MatchString(workspaceID) || !commandIDPattern.MatchString(jobID) {
		return nil, errors.New("invalid maintenance receipt lookup")
	}
	root := filepath.Join(store.Root, "workspaces", workspaceID, "receipts", jobID)
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	receipts := make([]Receipt, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
			return nil, fmt.Errorf("invalid maintenance receipt entry %q", entry.Name())
		}
		body, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		var receipt Receipt
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&receipt); err != nil {
			return nil, err
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			if err == nil {
				return nil, errors.New("maintenance receipt contains multiple JSON values")
			}
			return nil, err
		}
		if receipt.JobID != jobID || receipt.WorkspaceID != workspaceID || receipt.CommandID+".json" != entry.Name() {
			return nil, errors.New("maintenance receipt identity mismatch")
		}
		if err := receipt.Validate(); err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}

func writeReceipt(path string, receipt Receipt) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(receipt); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func readReceipt(path string) (Receipt, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Receipt{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var receipt Receipt
	if err := decoder.Decode(&receipt); err != nil {
		return Receipt{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Receipt{}, errors.New("maintenance receipt contains multiple JSON values")
		}
		return Receipt{}, err
	}
	if err := receipt.Validate(); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func ensurePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("maintenance receipt root must be a private local directory")
	}
	if info.Mode().Perm() != 0o700 {
		return os.Chmod(path, 0o700)
	}
	return nil
}
