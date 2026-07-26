// Package lifecycle owns runtime-neutral lifecycle vocabulary and the small,
// metadata-only receipt outbox used to diagnose native adapter delivery.
package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	SessionStart      = "session_start"
	ContextInject     = "context_inject"
	PreActionGuard    = "pre_action_guard"
	PostActionObserve = "post_action_observe"
	StopFinalize      = "stop_finalize"
)

var validEvents = map[string]bool{
	SessionStart: true, ContextInject: true, PreActionGuard: true,
	PostActionObserve: true, StopFinalize: true,
}

// Receipt deliberately excludes prompt text, tool input/output, native session
// IDs and workspace paths. IdempotencyKey is a one-way digest of native IDs.
type Receipt struct {
	SchemaVersion  int       `json:"schema_version"`
	Runtime        string    `json:"runtime"`
	Event          string    `json:"event"`
	State          string    `json:"state"`
	OccurredAt     time.Time `json:"occurred_at"`
	IdempotencyKey string    `json:"idempotency_key"`
	ToolName       string    `json:"tool_name,omitempty"`
	Diagnostic     string    `json:"diagnostic,omitempty"`
}

type Summary struct {
	State       string   `json:"state"`
	ReceiptRoot string   `json:"receipt_root"`
	Observed    int      `json:"observed"`
	Failed      int      `json:"failed"`
	Events      []string `json:"events"`
	LatestAt    string   `json:"latest_at,omitempty"`
	Diagnostic  string   `json:"diagnostic,omitempty"`
}

// Record appends one idempotent, small receipt. It does not lock, retry or
// coordinate with a worker, so native hooks do not contend with background
// processing. A duplicate key is already the desired observed state.
func Record(dataRoot, workspaceID string, receipt Receipt) (Receipt, error) {
	if strings.TrimSpace(dataRoot) == "" || strings.TrimSpace(workspaceID) == "" {
		return Receipt{}, errors.New("receipt data root and workspace id are required")
	}
	if receipt.Runtime != "claude" && receipt.Runtime != "codex" {
		return Receipt{}, fmt.Errorf("unsupported receipt runtime %q", receipt.Runtime)
	}
	if !validEvents[receipt.Event] {
		return Receipt{}, fmt.Errorf("unsupported lifecycle event %q", receipt.Event)
	}
	if receipt.State != "observed" && receipt.State != "failed" {
		return Receipt{}, fmt.Errorf("unsupported receipt state %q", receipt.State)
	}
	if receipt.OccurredAt.IsZero() {
		receipt.OccurredAt = time.Now().UTC()
	}
	if strings.TrimSpace(receipt.IdempotencyKey) == "" {
		return Receipt{}, errors.New("receipt idempotency key is required")
	}
	if len(receipt.IdempotencyKey) > 128 || len(receipt.ToolName) > 128 || len(receipt.Diagnostic) > 512 {
		return Receipt{}, errors.New("receipt metadata exceeds its bounded contract")
	}
	root := receiptRoot(dataRoot, workspaceID)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Receipt{}, fmt.Errorf("create receipt root: %w", err)
	}
	body, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return Receipt{}, fmt.Errorf("encode receipt: %w", err)
	}
	body = append(body, '\n')
	name := receipt.Event + "-" + receipt.IdempotencyKey + ".json"
	path := filepath.Join(root, name)
	file, err := os.CreateTemp(root, ".receipt-*.tmp")
	if err != nil {
		return Receipt{}, fmt.Errorf("stage receipt: %w", err)
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return Receipt{}, fmt.Errorf("protect staged receipt: %w", err)
	}
	if _, err := file.Write(body); err != nil {
		file.Close()
		return Receipt{}, fmt.Errorf("write receipt: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return Receipt{}, fmt.Errorf("sync receipt: %w", err)
	}
	if err := file.Close(); err != nil {
		return Receipt{}, fmt.Errorf("close receipt: %w", err)
	}
	// A hard link publishes only a fully-written file and fails when another
	// duplicate event has already won. Unlike rename, it never overwrites an
	// existing receipt, so an idempotency collision cannot corrupt evidence.
	if err := os.Link(temporaryPath, path); errors.Is(err, os.ErrExist) {
		return receipt, nil
	} else if err != nil {
		return Receipt{}, fmt.Errorf("publish receipt: %w", err)
	}
	return receipt, nil
}

func Diagnose(dataRoot, workspaceID string) (Summary, error) {
	root := receiptRoot(dataRoot, workspaceID)
	summary := Summary{State: "unavailable", ReceiptRoot: root}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return summary, nil
	}
	if err != nil {
		return Summary{}, fmt.Errorf("read receipt root: %w", err)
	}
	events := map[string]bool{}
	var latest Receipt
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			return Summary{}, fmt.Errorf("read receipt %s: %w", entry.Name(), err)
		}
		var receipt Receipt
		if err := json.Unmarshal(body, &receipt); err != nil {
			return Summary{}, fmt.Errorf("parse receipt %s: %w", entry.Name(), err)
		}
		events[receipt.Event] = true
		if receipt.State == "failed" {
			summary.Failed++
		} else {
			summary.Observed++
		}
		if latest.OccurredAt.Before(receipt.OccurredAt) {
			latest = receipt
		}
	}
	for event := range events {
		summary.Events = append(summary.Events, event)
	}
	sort.Strings(summary.Events)
	if summary.Observed+summary.Failed == 0 {
		return summary, nil
	}
	if summary.Failed > 0 {
		summary.State = "warning"
	} else {
		summary.State = "observed"
	}
	summary.LatestAt = latest.OccurredAt.Format(time.RFC3339)
	summary.Diagnostic = latest.Diagnostic
	return summary, nil
}

func IdempotencyKey(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:16])
}

func receiptRoot(dataRoot, workspaceID string) string {
	return filepath.Join(dataRoot, "runtime", "receipts", workspaceID)
}
