package scheduler

import (
	"errors"
	"path/filepath"
	"strings"
)

// ReadReceipt is a bounded strict read of one canonical scheduler observation.
// Directory selection/bounds belong to the collector; the store validates custody,
// workspace, job, filename and the owning receipt contract without scanning history.
func (store Store) ReadReceipt(workspace, job, name string) (Receipt, error) {
	var receipt Receipt
	if err := validateStoreInput(store.Root, workspace); err != nil {
		return receipt, err
	}
	if !jobIDPattern.MatchString(job) || filepath.Base(name) != name || !strings.HasSuffix(name, ".json") {
		return receipt, errors.New("invalid receipt selector")
	}
	body, err := ReadPrivateStateFile(filepath.Join(store.Root, "workspaces", workspace, "receipts", job, name), 64<<10)
	if err != nil {
		return receipt, err
	}
	if err = decodeStrictJSON(body, &receipt); err != nil {
		return receipt, err
	}
	if receipt.SchemaVersion != 1 || receipt.WorkspaceID != workspace || receipt.JobID != job {
		return receipt, errors.New("receipt scope mismatch")
	}
	if err = validateReceipt(receipt); err != nil {
		return receipt, err
	}
	expected := receipt.AttemptedAt.UTC().Format("20060102T150405.000000000Z") + "-" + receipt.ScheduledFor.UTC().Format("20060102T150405.000000000Z") + ".json"
	if name != expected {
		return receipt, errors.New("receipt filename mismatch")
	}
	return receipt, nil
}
