package darwinobservability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestSchemasCompileAndRejectContent(t *testing.T) {
	root := filepath.Join("..", "..", "schemas")
	evidence := compileObservabilitySchema(t, filepath.Join(root, "darwin-evidence.schema.json"))
	scorecard := compileObservabilitySchema(t, filepath.Join(root, "darwin-scorecard.schema.json"))
	valid := decodeObservabilityJSON(t, `{"schema_version":1,"kind":"proposal","evidence_id":"ev-1","window_id":"week-1","recorded_at":"2026-07-30T12:00:00Z","proposal":{"proposal_sha256":"`+strings.Repeat("a", 64)+`","proposal_kind":"policy_calibration","status":"draft","author_role":"darwin"}}`)
	if err := evidence.Validate(valid); err != nil {
		t.Fatalf("valid evidence rejected: %v", err)
	}
	invalid := decodeObservabilityJSON(t, `{"schema_version":1,"kind":"proposal","evidence_id":"ev-1","window_id":"week-1","recorded_at":"2026-07-30T12:00:00Z","prompt":"secret","proposal":{"proposal_sha256":"`+strings.Repeat("a", 64)+`","proposal_kind":"policy_calibration","status":"draft","author_role":"darwin"}}`)
	if err := evidence.Validate(invalid); err == nil {
		t.Fatal("evidence schema accepted prompt content")
	}
	validReport := decodeObservabilityJSON(t, `{"schema_version":1,"report_kind":"weekly_operational","report_version":"weekly-v1","window":{"window_id":"week-1","start":"2026-07-27T00:00:00Z","end":"2026-08-03T00:00:00Z"},"input_sha256":"`+strings.Repeat("a", 64)+`","health":{"records":0,"current":0,"aging":0,"stale":0,"missed":0,"unavailable":0,"recovered":0,"recovery_failed":0,"recovery_blocked":0},"selection":{"records":0,"completed":0,"failed":0,"blocked":0,"routes":[],"missing_pa_coverage":0,"unavailable_pa_coverage":0,"capability_gap_count":0},"integrity":{"input_records":0,"accepted_records":0,"duplicate_records":0,"independence_violations":0},"recommendation_codes":["hold_current_posture"],"may_mutate_policy":false}`)
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
