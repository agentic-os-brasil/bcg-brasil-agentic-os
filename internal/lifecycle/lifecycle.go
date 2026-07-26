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
	"regexp"
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

var (
	validEvents = map[string]bool{
		SessionStart: true, ContextInject: true, PreActionGuard: true,
		PostActionObserve: true, StopFinalize: true,
	}
	workspaceIDPattern  = regexp.MustCompile(`^[a-f0-9]{32}$`)
	idempotencyPattern  = regexp.MustCompile(`^[a-f0-9]{32}$`)
	metadataNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]{0,127}$`)
	validDiagnostics    = map[string]bool{"": true}
)

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
	Events      []string `json:"events"`
	LatestAt    string   `json:"latest_at,omitempty"`
	Diagnostic  string   `json:"diagnostic,omitempty"`
}

// Record appends one idempotent, small receipt. It does not lock, retry or
// coordinate with a worker, so native hooks do not contend with background
// processing. A duplicate key is already the desired observed state.
func Record(dataRoot, workspaceID string, receipt Receipt) (Receipt, error) {
	root, err := validatedReceiptRoot(dataRoot, workspaceID)
	if err != nil {
		return Receipt{}, err
	}
	if err := validateReceipt(receipt); err != nil {
		return Receipt{}, err
	}
	if receipt.OccurredAt.IsZero() {
		receipt.OccurredAt = time.Now().UTC()
	}
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
	// Publish only a complete file, without overwriting a prior receipt.
	if err := os.Link(temporaryPath, path); errors.Is(err, os.ErrExist) {
		existing, readErr := readReceipt(path)
		if readErr != nil {
			return Receipt{}, fmt.Errorf("validate duplicate receipt: %w", readErr)
		}
		if !sameIdempotentReceipt(existing, receipt) {
			return Receipt{}, errors.New("receipt idempotency collision")
		}
		return existing, nil
	} else if err != nil {
		return Receipt{}, fmt.Errorf("publish receipt: %w", err)
	}
	return receipt, nil
}

func Diagnose(dataRoot, workspaceID string) (Summary, error) {
	root, err := validatedReceiptRoot(dataRoot, workspaceID)
	if err != nil {
		return Summary{}, err
	}
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
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		receipt, err := readReceipt(filepath.Join(root, entry.Name()))
		if err != nil {
			return Summary{}, fmt.Errorf("read receipt %s: %w", entry.Name(), err)
		}
		expectedName := receipt.Event + "-" + receipt.IdempotencyKey + ".json"
		if entry.Name() != expectedName {
			return Summary{}, fmt.Errorf("receipt filename does not match bounded metadata")
		}
		events[receipt.Event] = true
		summary.Observed++
		if latest.OccurredAt.Before(receipt.OccurredAt) {
			latest = receipt
		}
	}
	for event := range events {
		summary.Events = append(summary.Events, event)
	}
	sort.Strings(summary.Events)
	if summary.Observed == 0 {
		return summary, nil
	}
	summary.State = "observed"
	summary.LatestAt = latest.OccurredAt.Format(time.RFC3339)
	summary.Diagnostic = latest.Diagnostic
	return summary, nil
}

func IdempotencyKey(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:16])
}

func validatedReceiptRoot(dataRoot, workspaceID string) (string, error) {
	if strings.TrimSpace(dataRoot) == "" {
		return "", errors.New("receipt data root is required")
	}
	if !workspaceIDPattern.MatchString(workspaceID) {
		return "", errors.New("invalid opaque workspace ID")
	}
	absolute, err := filepath.Abs(dataRoot)
	if err != nil {
		return "", fmt.Errorf("resolve receipt data root: %w", err)
	}
	return filepath.Join(filepath.Clean(absolute), "runtime", "receipts", workspaceID), nil
}

func validateReceipt(receipt Receipt) error {
	if receipt.SchemaVersion != 1 {
		return errors.New("unsupported receipt schema version")
	}
	if receipt.Runtime != "claude" && receipt.Runtime != "codex" {
		return fmt.Errorf("unsupported receipt runtime %q", receipt.Runtime)
	}
	if !validEvents[receipt.Event] {
		return fmt.Errorf("unsupported lifecycle event %q", receipt.Event)
	}
	if receipt.State != "observed" {
		return fmt.Errorf("unsupported receipt state %q", receipt.State)
	}
	if !idempotencyPattern.MatchString(receipt.IdempotencyKey) {
		return errors.New("invalid receipt idempotency key")
	}
	if receipt.ToolName != "" && !metadataNamePattern.MatchString(receipt.ToolName) {
		return errors.New("invalid receipt tool name")
	}
	if !validDiagnostics[receipt.Diagnostic] {
		return errors.New("unsupported receipt diagnostic")
	}
	return nil
}

func readReceipt(path string) (Receipt, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Receipt{}, err
	}
	var receipt Receipt
	if err := json.Unmarshal(body, &receipt); err != nil {
		return Receipt{}, err
	}
	if err := validateReceipt(receipt); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func sameIdempotentReceipt(left, right Receipt) bool {
	return left.SchemaVersion == right.SchemaVersion &&
		left.Runtime == right.Runtime &&
		left.Event == right.Event &&
		left.State == right.State &&
		left.IdempotencyKey == right.IdempotencyKey &&
		left.ToolName == right.ToolName &&
		left.Diagnostic == right.Diagnostic
}
