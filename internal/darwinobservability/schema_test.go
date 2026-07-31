package darwinobservability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestSchemasCompileAndRejectContent(t *testing.T) {
	root := filepath.Join("..", "..", "schemas")
	evidence := compileObservabilitySchema(t, filepath.Join(root, "darwin-evidence.schema.json"))
	scorecard := compileObservabilitySchema(t, filepath.Join(root, "darwin-scorecard.schema.json"))
	scope := strings.Repeat("f", 64)
	windowID := OpaqueWindowID("week-1")
	valid := decodeObservabilityJSON(t, `{"schema_version":1,"kind":"proposal","evidence_id":"ev-1","window_id":"`+windowID+`","scope_sha256":"`+scope+`","evidence_authority":"caller_asserted_shadow","recorded_at":"2026-07-30T12:00:00Z","proposal":{"proposal_sha256":"`+strings.Repeat("a", 64)+`","proposal_kind":"policy_calibration","status":"draft","author_role":"darwin"}}`)
	if err := evidence.Validate(valid); err != nil {
		t.Fatalf("valid evidence rejected: %v", err)
	}
	invalid := decodeObservabilityJSON(t, `{"schema_version":1,"kind":"proposal","evidence_id":"ev-1","window_id":"`+windowID+`","scope_sha256":"`+scope+`","evidence_authority":"caller_asserted_shadow","recorded_at":"2026-07-30T12:00:00Z","prompt":"secret","proposal":{"proposal_sha256":"`+strings.Repeat("a", 64)+`","proposal_kind":"policy_calibration","status":"draft","author_role":"darwin"}}`)
	if err := evidence.Validate(invalid); err == nil {
		t.Fatal("evidence schema accepted prompt content")
	}
	window := Window{ID: windowID, ScopeSHA256: scope, Start: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)}
	report, err := BuildWeekly([]Record{selectionRecord(t, "ev-schema", window.ID, activationpolicy.D0Direct)}, window)
	if err != nil {
		t.Fatal(err)
	}
	reportBody, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	validReport := decodeObservabilityJSON(t, string(reportBody))
	if err := scorecard.Validate(validReport); err != nil {
		t.Fatalf("valid scorecard rejected: %v", err)
	}
}

func compileObservabilitySchema(t *testing.T, path string) *jsonschema.Schema {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(filepath.Base(path), document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func decodeObservabilityJSON(t *testing.T, raw string) any {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}
