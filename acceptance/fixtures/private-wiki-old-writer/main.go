// Command private-wiki-old-writer is an acceptance fixture, never a product tool.
// The provisioner compiles these exact bytes against immutable pre-wiki memory.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

var sourceRevision, sourceArchiveSHA256, fixtureSHA256 string

type config struct {
	Policy    memory.Policy            `json:"policy"`
	Budgets   map[string]int           `json:"budgets"`
	Freshness map[string]time.Duration `json:"freshness"`
	At        time.Time                `json:"at"`
	Text      string                   `json:"text"`
}
type result struct {
	Refused             bool                    `json:"refused"`
	Code                string                  `json:"code"`
	SourceRevision      string                  `json:"source_revision"`
	SourceArchiveSHA256 string                  `json:"source_archive_sha256"`
	FixtureSHA256       string                  `json:"fixture_sha256"`
	ExecutableSHA256    string                  `json:"executable_sha256"`
	ConfigSHA256        string                  `json:"config_sha256"`
	Capture             *memory.CaptureReceipt  `json:"capture,omitempty"`
	Record              *memory.CaptureRecord   `json:"record,omitempty"`
	Mutation            *memory.MutationReceipt `json:"mutation,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 5 {
		return errors.New("expected operation, memory root, capture id and config")
	}
	operation, root, id := os.Args[1], os.Args[2], os.Args[3]
	body, err := os.ReadFile(os.Args[4])
	if err != nil {
		return err
	}
	var cfg config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return err
	}
	if cfg.At.IsZero() || cfg.Text == "" {
		return errors.New("fixture requires exact time and synthetic text")
	}
	image, err := os.Executable()
	if err != nil {
		return err
	}
	program, err := os.ReadFile(image)
	if err != nil {
		return err
	}
	imageHash, configHash := sha256.Sum256(program), sha256.Sum256(body)
	out := result{Code: "executed", SourceRevision: sourceRevision, SourceArchiveSHA256: sourceArchiveSHA256, FixtureSHA256: fixtureSHA256, ExecutableSHA256: hex.EncodeToString(imageHash[:]), ConfigSHA256: hex.EncodeToString(configHash[:])}
	engine := memory.Engine{Root: root, Policy: cfg.Policy, Budgets: cfg.Budgets, Freshness: cfg.Freshness, Now: func() time.Time { return cfg.At }}
	var operationErr error
	switch operation {
	case "seed", "capture":
		if operation == "seed" {
			operationErr = engine.PrepareOwnedCaptureAuthority("case-a")
		}
		if operationErr == nil {
			var input memory.OwnedCaptureInput
			input, operationErr = engine.SealOwnedCapture("case-a", "", 0, cfg.Text, cfg.At)
			if operationErr == nil {
				var receipt memory.CaptureReceipt
				receipt, operationErr = engine.CaptureOwned("case-a", input)
				out.Capture = &receipt
			}
		}
	case "read":
		var record memory.CaptureRecord
		record, operationErr = engine.ReadCapture("case-a", id)
		out.Record = &record
	case "delete":
		var receipt memory.MutationReceipt
		receipt, operationErr = engine.DeleteCapture("case-a", id, 1)
		out.Mutation = &receipt
	default:
		return errors.New("unsupported fixture operation")
	}
	if operationErr != nil {
		// Classify only the real historical schema-1 consumer's exact schema refusal.
		// A marker alone or an unrelated API error cannot manufacture this result.
		marker, readErr := os.ReadFile(filepath.Join(root, "workspaces", "case-a", ".visibility-checkpoint-required"))
		var requirement struct {
			SchemaVersion int    `json:"schema_version"`
			WorkspaceID   string `json:"workspace_id"`
		}
		if readErr != nil || json.Unmarshal(marker, &requirement) != nil || requirement.SchemaVersion != 2 || requirement.WorkspaceID != "case-a" || operationErr.Error() != "invalid memory visibility checkpoint requirement" {
			return operationErr
		}
		out.Refused, out.Code = true, "protected_cohort"
		out.Capture, out.Record, out.Mutation = nil, nil, nil
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}
