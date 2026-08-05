// Package lifecycle owns runtime-neutral lifecycle vocabulary and the small,
// metadata-only receipt outbox used to diagnose adapter-command delivery.
package lifecycle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	// AdapterCommand means the bounded Maestro adapter command produced this
	// receipt. It deliberately does not claim that a qualifying native runtime
	// session invoked that command.
	AdapterCommand = "adapter_command"
	// MaximumDiagnosticReceiptEntries bounds a read-only diagnostic projection;
	// excess historical receipts fail closed rather than being enumerated.
	MaximumDiagnosticReceiptEntries = 64
	maximumReceiptBytes             = 8 << 10
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
	validProvenance     = map[string]bool{AdapterCommand: true}
)

// Receipt deliberately excludes prompt text, tool input/output, native session
// IDs and workspace paths. IdempotencyKey is a one-way digest of native IDs.
// Provenance describes only the local producer and never serves as proof of
// native runtime invocation.
type Receipt struct {
	SchemaVersion  int       `json:"schema_version"`
	Runtime        string    `json:"runtime"`
	Event          string    `json:"event"`
	State          string    `json:"state"`
	Provenance     string    `json:"provenance"`
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
	Provenance  []string `json:"provenance"`
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
	if receipt.OccurredAt.IsZero() {
		receipt.OccurredAt = time.Now().UTC()
	}
	if err := validateReceipt(receipt); err != nil {
		return Receipt{}, err
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
	return diagnose(dataRoot, workspaceID, "")
}

// DiagnoseRuntime reports bounded adapter-command evidence for one runtime.
// It does not promote a receipt to native observation.
func DiagnoseRuntime(dataRoot, workspaceID, runtime string) (Summary, error) {
	if runtime != "claude" && runtime != "codex" {
		return Summary{}, fmt.Errorf("unsupported lifecycle runtime %q", runtime)
	}
	return diagnose(dataRoot, workspaceID, runtime)
}

func diagnose(dataRoot, workspaceID, runtime string) (Summary, error) {
	root, err := validatedReceiptRoot(dataRoot, workspaceID)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{State: "unavailable", ReceiptRoot: root}
	before, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return summary, nil
	}
	if err != nil {
		return Summary{}, fmt.Errorf("read receipt root: %w", err)
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return Summary{}, errors.New("lifecycle receipt root is not private")
	}
	directory, err := os.Open(root)
	if err != nil {
		return Summary{}, fmt.Errorf("open receipt root: %w", err)
	}
	defer directory.Close()
	opened, err := directory.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return Summary{}, errors.New("lifecycle receipt root changed during secure open")
	}
	entries, err := directory.ReadDir(MaximumDiagnosticReceiptEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return Summary{}, fmt.Errorf("read bounded receipt root: %w", err)
	}
	if len(entries) > MaximumDiagnosticReceiptEntries {
		return Summary{}, errors.New("lifecycle receipt history exceeds diagnostic scan bound")
	}
	events := map[string]bool{}
	provenance := map[string]bool{}
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
		if runtime != "" && receipt.Runtime != runtime {
			continue
		}
		events[receipt.Event] = true
		provenance[receipt.Provenance] = true
		summary.Observed++
		if latest.OccurredAt.Before(receipt.OccurredAt) {
			latest = receipt
		}
	}
	for event := range events {
		summary.Events = append(summary.Events, event)
	}
	for source := range provenance {
		summary.Provenance = append(summary.Provenance, source)
	}
	sort.Strings(summary.Events)
	sort.Strings(summary.Provenance)
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
	if receipt.State != "observed" || receipt.OccurredAt.IsZero() {
		return fmt.Errorf("unsupported receipt state %q", receipt.State)
	}
	if !validProvenance[receipt.Provenance] {
		return fmt.Errorf("unsupported lifecycle receipt provenance %q", receipt.Provenance)
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
	before, err := os.Lstat(path)
	if err != nil {
		return Receipt{}, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() || before.Size() > maximumReceiptBytes {
		return Receipt{}, errors.New("lifecycle receipt is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return Receipt{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return Receipt{}, errors.New("lifecycle receipt changed during secure open")
	}
	body, err := io.ReadAll(io.LimitReader(file, maximumReceiptBytes+1))
	if err != nil {
		return Receipt{}, err
	}
	if len(body) > maximumReceiptBytes {
		return Receipt{}, errors.New("lifecycle receipt exceeds bounded read limit")
	}
	var receipt Receipt
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return Receipt{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Receipt{}, errors.New("lifecycle receipt contains multiple JSON values")
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
		left.Provenance == right.Provenance &&
		left.IdempotencyKey == right.IdempotencyKey &&
		left.ToolName == right.ToolName &&
		left.Diagnostic == right.Diagnostic
}
