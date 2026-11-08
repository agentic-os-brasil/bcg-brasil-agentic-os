// This new harness links to an unmodified historical library; it is not an old release.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

const historicalRevision = "f93e9ce21edcd19246e7e5985d97489297dae18b"
const positiveMarker = "MAESTRO_HISTORICAL_LIBRARY_POSITIVE"

var linkedLibraryRevision = "unbound"

type config struct {
	Policy    memory.Policy            `json:"policy"`
	Budgets   map[string]int           `json:"budgets"`
	Freshness map[string]time.Duration `json:"freshness"`
	Clock     time.Time                `json:"clock"`
}

type apiCall struct {
	API           string `json:"api"`
	Outcome       string `json:"outcome"`
	Error         string `json:"error,omitempty"`
	ReturnedEmpty bool   `json:"returned_empty"`
}

type result struct {
	SchemaVersion      int       `json:"schema_version"`
	ArtifactKind       string    `json:"artifact_kind"`
	LibraryRevision    string    `json:"library_revision"`
	Platform           string    `json:"platform"`
	Operation          string    `json:"operation"`
	PositiveControl    bool      `json:"positive_control"`
	OwnedSupported     bool      `json:"owned_supported"`
	LegacyContextFound bool      `json:"legacy_context_found"`
	Calls              []apiCall `json:"calls"`
}

func classify(api string, err error, returnedEmpty bool) apiCall {
	call := apiCall{API: api, Outcome: "success", ReturnedEmpty: returnedEmpty}
	if err == nil {
		return call
	}
	call.Error = err.Error()
	call.Outcome = "unexpected_error"
	if errors.Is(err, memory.ErrConfinementUnavailable) {
		call.Outcome = "confinement_unavailable"
	} else if call.Error == "invalid memory visibility checkpoint requirement" {
		call.Outcome = "visibility_requirement_refused"
	}
	return call
}

func readConfig(input io.Reader) (config, error) {
	var cfg config
	body, err := io.ReadAll(io.LimitReader(input, (1<<20)+1))
	if err != nil {
		return cfg, err
	}
	if len(body) > 1<<20 {
		return cfg, errors.New("witness configuration exceeds 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return cfg, errors.New("exactly one witness configuration object is required")
	}
	if err := cfg.Policy.Validate(); err != nil {
		return cfg, err
	}
	if cfg.Clock.IsZero() || len(cfg.Budgets) != 4 {
		return cfg, errors.New("fixture clock and exact source budgets are required")
	}
	for _, layer := range []string{"L1", "L2", "L3", "lifetime"} {
		if cfg.Budgets[layer] <= 0 {
			return cfg, errors.New("source layer budgets must be positive")
		}
	}
	for layer, freshness := range cfg.Freshness {
		if _, ok := cfg.Budgets[layer]; !ok || freshness < 0 {
			return cfg, errors.New("invalid source freshness")
		}
	}
	return cfg, nil
}

func engine(root string, cfg config) *memory.Engine {
	return &memory.Engine{Root: root, Policy: cfg.Policy, Budgets: cfg.Budgets,
		Freshness: cfg.Freshness, Now: func() time.Time { return cfg.Clock }}
}

func legacyCapture(cfg config) memory.Capture {
	return memory.Capture{WorkspaceID: "case-a", RecordedAt: cfg.Clock,
		Kind: "note", Text: positiveMarker, Sanitized: true}
}

func positive(root string, cfg config, receipt *result) error {
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		return errors.New("positive control requires a separate existing empty root")
	}
	legacy := engine(filepath.Join(root, "legacy", "brain", "memory"), cfg)
	if err := memory.Bootstrap(legacy.Root, "case-a"); err != nil {
		return fmt.Errorf("legacy Bootstrap: %w", err)
	}
	receipt.Calls = append(receipt.Calls, classify("memory.Bootstrap(legacy)", nil, true))
	path, err := legacy.Capture(legacyCapture(cfg))
	if err != nil || path == "" {
		return fmt.Errorf("legacy Capture positive: path_present=%t error=%v", path != "", err)
	}
	receipt.Calls = append(receipt.Calls, classify("Engine.Capture(legacy_journal)", nil, false))
	attestor := memory.CaptureAttestor{Root: legacy.Root}
	attestedInput := legacyCapture(cfg)
	attestedInput.Kind = "skill_route"
	attestedInput.ProducerID = "codex.context-injection"
	attestedInput.SanitizerID = memory.SkillRouteSanitizerID
	digest := sha256.Sum256([]byte("historical-synthetic-route\x00" + positiveMarker))
	attestedInput.SourceDigest = hex.EncodeToString(digest[:])
	attested, err := attestor.Seal(attestedInput)
	if err != nil {
		return fmt.Errorf("legacy attestation positive: %w", err)
	}
	receipt.Calls = append(receipt.Calls, classify("CaptureAttestor.Seal", nil, false))
	if path, err = legacy.Capture(attested); err != nil || path == "" {
		return fmt.Errorf("legacy attested Capture positive: path_present=%t error=%v", path != "", err)
	}
	receipt.Calls = append(receipt.Calls, classify("Engine.Capture(attested_journal)", nil, false))
	legacy.Synthesizer = memory.DeterministicL1Synthesizer{MaxRunes: 4096, MaxEntries: 64,
		MaxInputBytes: 1 << 20, MaxInputEntries: 256, Attestor: attestor}
	legacy.SynthesizerID = memory.DeterministicL1SynthesizerID
	dream, err := legacy.DreamDailyAttested(context.Background(), "case-a", cfg.Clock)
	if err != nil || dream.Skipped || len(dream.ActivatedLayers) != 1 {
		return fmt.Errorf("legacy DreamDailyAttested positive: activated=%d error=%v", len(dream.ActivatedLayers), err)
	}
	receipt.Calls = append(receipt.Calls, classify("Engine.DreamDailyAttested", nil, false))
	status, err := legacy.Status("case-a")
	if err != nil || status.State != "ready" || status.CaptureFiles < 2 {
		return fmt.Errorf("legacy Status positive: state=%s captures=%d error=%v", status.State, status.CaptureFiles, err)
	}
	receipt.Calls = append(receipt.Calls, classify("Engine.Status", nil, false))
	bundle, err := legacy.AssembleContext("case-a")
	if err != nil {
		return fmt.Errorf("legacy AssembleContext positive: %w", err)
	}
	for _, section := range bundle.Sections {
		receipt.LegacyContextFound = receipt.LegacyContextFound || strings.Contains(section.Content, positiveMarker)
	}
	if !receipt.LegacyContextFound {
		return errors.New("legacy AssembleContext did not return the genuine source marker")
	}
	receipt.Calls = append(receipt.Calls, classify("Engine.AssembleContext", nil, false))
	owned := engine(filepath.Join(root, "owned", "brain", "memory"), cfg)
	if err := memory.Bootstrap(owned.Root, "case-a"); err != nil {
		return fmt.Errorf("owned Bootstrap positive: %w", err)
	}
	receipt.Calls = append(receipt.Calls, classify("memory.Bootstrap(owned)", nil, true))
	prepareErr := owned.PrepareOwnedCaptureAuthority("case-a")
	receipt.Calls = append(receipt.Calls, classify("Engine.PrepareOwnedCaptureAuthority", prepareErr, true))
	input, sealErr := owned.SealOwnedCapture("case-a", "", 0, positiveMarker, cfg.Clock)
	receipt.Calls = append(receipt.Calls, classify("Engine.SealOwnedCapture", sealErr, sealErr != nil))
	capture, captureErr := owned.CaptureOwned("case-a", input)
	receipt.Calls = append(receipt.Calls, classify("Engine.CaptureOwned", captureErr, reflect.DeepEqual(capture, memory.CaptureReceipt{})))
	id := capture.ID
	if id == "" {
		id = strings.Repeat("a", 32)
	}
	record, readErr := owned.ReadCapture("case-a", id)
	receipt.Calls = append(receipt.Calls, classify("Engine.ReadCapture", readErr, reflect.DeepEqual(record, memory.CaptureRecord{})))
	removed, deleteErr := owned.DeleteCapture("case-a", id, 1)
	receipt.Calls = append(receipt.Calls, classify("Engine.DeleteCapture", deleteErr, reflect.DeepEqual(removed, memory.MutationReceipt{})))
	if errors.Is(prepareErr, memory.ErrConfinementUnavailable) && runtime.GOOS == "windows" {
		for _, err := range []error{sealErr, captureErr, readErr, deleteErr} {
			if !errors.Is(err, memory.ErrConfinementUnavailable) {
				return errors.New("historical Windows owned preflight outcomes disagree")
			}
		}
	} else {
		if prepareErr != nil || sealErr != nil || captureErr != nil || readErr != nil || deleteErr != nil ||
			!capture.Eligible || capture.Revision != 1 || record.Text != positiveMarker || !removed.Complete || removed.Removed != 1 {
			return errors.New("historical owned positive did not genuinely capture, read and delete")
		}
		receipt.OwnedSupported = true
	}
	receipt.PositiveControl = true
	return nil
}

func protected(operation, root, id string, revision uint64, cfg config, receipt *result) error {
	e := engine(root, cfg)
	switch operation {
	case "legacy_capture":
		path, err := e.Capture(legacyCapture(cfg))
		receipt.Calls = append(receipt.Calls, classify("Engine.Capture", err, path == ""))
	case "legacy_status":
		status, err := e.Status("case-a")
		receipt.Calls = append(receipt.Calls, classify("Engine.Status", err, reflect.DeepEqual(status, memory.StatusReport{})))
	case "legacy_context":
		bundle, err := e.AssembleContext("case-a")
		receipt.Calls = append(receipt.Calls, classify("Engine.AssembleContext", err, reflect.DeepEqual(bundle, memory.ContextBundle{})))
	case "owned_read":
		record, err := e.ReadCapture("case-a", id)
		receipt.Calls = append(receipt.Calls, classify("Engine.ReadCapture", err, reflect.DeepEqual(record, memory.CaptureRecord{})))
	case "owned_capture":
		input, sealErr := e.SealOwnedCapture("case-a", "", 0, positiveMarker, cfg.Clock)
		receipt.Calls = append(receipt.Calls, classify("Engine.SealOwnedCapture", sealErr, sealErr != nil))
		if sealErr != nil && !errors.Is(sealErr, memory.ErrConfinementUnavailable) {
			return fmt.Errorf("old input preparation failed before capture could execute: %w", sealErr)
		}
		capture, err := e.CaptureOwned("case-a", input)
		receipt.Calls = append(receipt.Calls, classify("Engine.CaptureOwned", err, reflect.DeepEqual(capture, memory.CaptureReceipt{})))
	case "owned_delete":
		removed, err := e.DeleteCapture("case-a", id, revision)
		receipt.Calls = append(receipt.Calls, classify("Engine.DeleteCapture", err, reflect.DeepEqual(removed, memory.MutationReceipt{})))
	default:
		return errors.New("unknown historical operation")
	}
	return nil
}

func run(args []string, output io.Writer) error {
	if linkedLibraryRevision != historicalRevision {
		return errors.New("harness is not bound to the pinned historical library build")
	}
	if !(len(args) == 3 && args[0] == "positive") && !(len(args) == 5 && args[0] != "positive") {
		return errors.New("usage: old-cohort positive ROOT CONFIG or OP ROOT CONFIG CAPTURE_ID REVISION")
	}
	if !filepath.IsAbs(args[1]) || !filepath.IsAbs(args[2]) {
		return errors.New("absolute root and configuration paths are required")
	}
	file, err := os.Open(args[2])
	if err != nil {
		return err
	}
	defer file.Close()
	cfg, err := readConfig(file)
	if err != nil {
		return err
	}
	receipt := result{SchemaVersion: 2, ArtifactKind: "new_harness_unmodified_historical_library",
		LibraryRevision: linkedLibraryRevision, Platform: runtime.GOOS, Operation: args[0]}
	if args[0] == "positive" {
		err = positive(args[1], cfg, &receipt)
	} else {
		revision, parseErr := strconv.ParseUint(args[4], 10, 64)
		if parseErr != nil || revision == 0 || len(args[3]) != 32 || strings.Trim(args[3], "0123456789abcdef") != "" {
			return errors.New("actual capture identity and positive revision are required")
		}
		err = protected(args[0], args[1], args[3], revision, cfg, &receipt)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(receipt)
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
