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

// MaximumContinuityReceiptEntries bounds the continuous-use diagnostic view.
// Detailed receipt history remains in this store and is never read wholesale at
// Session Start.
const MaximumContinuityReceiptEntries = 64
const maximumReceiptBytes = 16 << 10

func (store Store) AppendReceipt(receipt Receipt) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	if receipt.State == ReceiptAccepted || receipt.State == ReceiptBusy {
		return errors.New("nonterminal maintenance receipt cannot be persisted")
	}
	if store.Root == "" {
		return errors.New("maintenance receipt root is required")
	}
	root, err := ensurePrivateTree(store.Root, "workspaces", receipt.WorkspaceID, "receipts", receipt.JobID)
	if err != nil {
		return err
	}
	if existing, readErr := store.Receipts(receipt.WorkspaceID, receipt.JobID); readErr != nil {
		return readErr
	} else {
		if receipt.RecoveryPhase != "" {
			if err := validateRecoveryChain(receipt, existing); err != nil {
				return err
			}
		}
		for _, prior := range existing {
			if prior.OccurrenceDigest != receipt.OccurrenceDigest {
				continue
			}
			if receipt.RecoveryPhase != "" && prior.RecoveryPhase != "" {
				if prior.RecoveryPhase == receipt.RecoveryPhase {
					if prior.RecoveryIntentDigest == receipt.RecoveryIntentDigest && prior.FenceTokenDigest == receipt.FenceTokenDigest {
						return nil
					}
					return errors.New("maintenance recovery audit conflicts with an existing phase")
				}
				continue
			}
			if prior.State == ReceiptSucceeded || prior.State == ReceiptReviewedNoChange || prior.State == ReceiptProposalEmitted || prior.State == ReceiptRecoveryRequired {
				// Recovery intent/outcome records are a separate immutable audit
				// chain. They must remain appendable after the operation's success
				// receipt; otherwise a release failure cannot be recorded.
				if receipt.RecoveryPhase != "" {
					continue
				}
				if receipt.State == ReceiptRecoveryRequired && prior.State != ReceiptRecoveryRequired {
					continue
				}
				if prior.State != receipt.State || prior.ProposalDigest != receipt.ProposalDigest || prior.ProposalArtifactID != receipt.ProposalArtifactID {
					return errors.New("maintenance receipt occurrence has conflicting terminal evidence")
				}
				return nil
			}
		}
	}
	path := filepath.Join(root, receipt.CommandID+"--"+receipt.AttemptID+".json")
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

func validateRecoveryChain(receipt Receipt, existing []Receipt) error {
	if receipt.RecoveryPhase == "intent" {
		for _, prior := range existing {
			if prior.OccurrenceDigest != receipt.OccurrenceDigest || prior.RecoveryPhase == "" {
				continue
			}
			if prior.RecoveryPhase != "intent" {
				return errors.New("maintenance recovery intent follows a terminal recovery outcome")
			}
			if sameRecoveryBinding(prior, receipt) {
				return nil
			}
			return errors.New("maintenance recovery intent conflicts with the existing occurrence fence")
		}
		return nil
	}

	matchedIntent := false
	for _, prior := range existing {
		if prior.OccurrenceDigest != receipt.OccurrenceDigest || prior.RecoveryPhase == "" {
			continue
		}
		if prior.RecoveryPhase == "intent" {
			if !sameRecoveryBinding(prior, receipt) {
				return errors.New("maintenance recovery outcome does not match its persisted intent")
			}
			matchedIntent = true
			continue
		}
		if prior.RecoveryPhase == receipt.RecoveryPhase && sameRecoveryBinding(prior, receipt) {
			return nil
		}
		return errors.New("maintenance recovery occurrence already has a different terminal outcome")
	}
	if !matchedIntent {
		return errors.New("maintenance recovery outcome requires a persisted intent")
	}
	return nil
}

func sameRecoveryBinding(left, right Receipt) bool {
	return left.WorkspaceID == right.WorkspaceID && left.JobID == right.JobID && left.OccurrenceDigest == right.OccurrenceDigest && left.Trigger == right.Trigger && left.ProposalOnly == right.ProposalOnly && left.RecoveryIntentDigest == right.RecoveryIntentDigest && left.FenceTokenDigest == right.FenceTokenDigest
}

func (store Store) Receipts(workspaceID, jobID string) ([]Receipt, error) {
	if store.Root == "" || !commandIDPattern.MatchString(workspaceID) || !commandIDPattern.MatchString(jobID) {
		return nil, errors.New("invalid maintenance receipt lookup")
	}
	root, err := ensurePrivateTree(store.Root, "workspaces", workspaceID, "receipts", jobID)
	if err != nil {
		return nil, err
	}
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
		if receipt.JobID != jobID || receipt.WorkspaceID != workspaceID || receipt.CommandID+"--"+receipt.AttemptID+".json" != entry.Name() {
			return nil, errors.New("maintenance receipt identity mismatch")
		}
		if err := receipt.Validate(); err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}

// BoundedValidatedReceiptCount reads a small, strict receipt projection without
// creating directories. It is the only maintenance receipt view used by
// continuous-use status.
func (store Store) BoundedValidatedReceiptCount(workspaceID, jobID string) (int, error) {
	if store.Root == "" || !commandIDPattern.MatchString(workspaceID) || !commandIDPattern.MatchString(jobID) {
		return 0, errors.New("invalid maintenance bounded receipt lookup")
	}
	directoryPath := filepath.Join(store.Root, "workspaces", workspaceID, "receipts", jobID)
	before, err := os.Lstat(directoryPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return 0, errors.New("maintenance receipt directory is not private")
	}
	directory, err := os.Open(directoryPath)
	if err != nil {
		return 0, err
	}
	defer directory.Close()
	opened, err := directory.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return 0, errors.New("maintenance receipt directory changed during secure open")
	}
	entries, err := directory.ReadDir(MaximumContinuityReceiptEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	if len(entries) > MaximumContinuityReceiptEntries {
		return 0, errors.New("maintenance receipt history exceeds continuity scan bound")
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
			return 0, fmt.Errorf("invalid maintenance continuity receipt entry %q", entry.Name())
		}
		receipt, err := readReceipt(filepath.Join(directoryPath, entry.Name()))
		if err != nil || receipt.JobID != jobID || receipt.WorkspaceID != workspaceID || receipt.CommandID+"--"+receipt.AttemptID+".json" != entry.Name() {
			if err != nil {
				return 0, err
			}
			return 0, errors.New("maintenance continuity receipt identity mismatch")
		}
		count++
	}
	return count, nil
}

func writeReceipt(path string, receipt Receipt) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = file.Close()
			_ = os.Remove(path)
		}
	}()
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(receipt); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	committed = true
	return nil
}

func readReceipt(path string) (Receipt, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return Receipt{}, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() || before.Size() > maximumReceiptBytes {
		return Receipt{}, errors.New("maintenance receipt must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return Receipt{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return Receipt{}, errors.New("maintenance receipt changed during secure open")
	}
	body, err := io.ReadAll(io.LimitReader(file, maximumReceiptBytes+1))
	if err != nil {
		return Receipt{}, err
	}
	if len(body) > maximumReceiptBytes {
		return Receipt{}, errors.New("maintenance receipt exceeds bounded read limit")
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
