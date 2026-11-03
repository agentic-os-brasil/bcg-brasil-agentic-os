package maintenance

import (
	"errors"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// VerifyEventReceipt reads the existing immutable terminal receipt through the
// maintenance owner. Event work has no calendar scheduler receipt.
func (store Store) VerifyEventReceipt(expected Receipt) error {
	if store.Root == "" || expected.Validate() != nil || expected.EventID == "" || expected.State != ReceiptSucceeded && expected.State != ReceiptReviewedNoChange {
		return errors.New("invalid terminal event evidence")
	}
	path := filepath.Join(store.Root, "workspaces", expected.WorkspaceID, "receipts", expected.JobID, expected.CommandID+"--"+expected.AttemptID+".json")
	body, err := scheduler.ReadPrivateStateFile(path, maximumReceiptBytes)
	if err != nil {
		return err
	}
	var actual Receipt
	if decodeProduct(body, &actual) != nil || actual != expected {
		return errors.New("event execution evidence changed")
	}
	return nil
}

// EventClosed asks the existing receipt owner whether this exact occurrence has
// durable terminal evidence. A current graph is effect evidence, not closure.
func (store Store) EventClosed(workspaceID, jobID, eventID string) (bool, error) {
	if ValidateEventID(eventID) != nil {
		return false, errors.New("invalid event completion lookup")
	}
	receipts, err := store.Receipts(workspaceID, jobID)
	if err != nil {
		return false, err
	}
	expected := (Command{JobID: jobID, Trigger: TriggerEvent, EventID: eventID}).OccurrenceDigest()
	closed := false
	for _, receipt := range receipts {
		if receipt.EventID != eventID {
			continue
		}
		if receipt.Trigger != TriggerEvent || receipt.OccurrenceDigest != expected {
			return false, errors.New("event completion occurrence changed")
		}
		if receipt.State == ReceiptRecoveryRequired {
			return false, errors.New("event occurrence requires explicit recovery")
		}
		if receipt.State == ReceiptSucceeded || receipt.State == ReceiptReviewedNoChange {
			if err := store.VerifyEventReceipt(receipt); err != nil {
				return false, err
			}
			closed = true
		}
	}
	return closed, nil
}
