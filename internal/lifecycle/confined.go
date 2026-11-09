package lifecycle

import (
	"encoding/json"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"path/filepath"
	"time"
)

// RecordConfined preserves the same receipt schema/idempotency under the owner
// private file boundary. Only actual adapter observations call this function.
func RecordConfined(dataRoot, workspaceID string, receipt Receipt) (Receipt, error) {
	root, err := validatedReceiptRoot(dataRoot, workspaceID)
	if err != nil {
		return Receipt{}, err
	}
	if err := scheduler.ValidatePrivateDirectory(dataRoot); err != nil {
		return Receipt{}, err
	}
	if receipt.OccurredAt.IsZero() {
		receipt.OccurredAt = time.Now().UTC()
	}
	if err := validateReceipt(receipt); err != nil {
		return Receipt{}, err
	}
	err = scheduler.UpdatePrivateFile(filepath.Join(root, receipt.Event+"-"+receipt.IdempotencyKey+".json"), maximumReceiptBytes, func(body []byte) ([]byte, error) {
		if len(body) > 0 {
			var prior Receipt
			if json.Unmarshal(body, &prior) != nil || validateReceipt(prior) != nil || !sameIdempotentReceipt(prior, receipt) {
				return nil, errors.New("receipt_idempotency_conflict")
			}
			receipt = prior
			return nil, nil
		}
		return json.Marshal(receipt)
	})
	return receipt, err
}
