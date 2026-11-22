package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
)

// This private pipe exercises the actual fixture emit function. Tests never run
// main, construct a runtime or invoke a converter, interpreter or model.
func captureFixtureEmit(t *testing.T, value any) []byte {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal("private test pipe unavailable")
	}
	original := os.Stdout
	defer func() { os.Stdout = original; _ = reader.Close(); _ = writer.Close() }()
	os.Stdout = writer
	emit(value)
	if writer.Close() != nil {
		t.Fatal("private test pipe close failed")
	}
	os.Stdout = original
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal("private test pipe read failed")
	}
	return body
}

func TestFixturePlacementFailureProjectionIsClosedAndBodyFree(t *testing.T) {
	const status = "confined converter exit 3221225506 (status=0xc0000022 pid=123 creation_flags=0x00080004)"
	attrs := func(stage, process string) []slog.Attr {
		return []slog.Attr{slog.String("stage", stage), slog.String("process_status", process)}
	}
	for _, test := range []struct {
		name    string
		message string
		attrs   []slog.Attr
		want    bool
	}{
		{"actual_launch_status", "native converter qualification failed", attrs("launch", status), true},
		{"private_other_attribute_discarded", "native converter qualification failed", append(attrs("launch", status), slog.String("private", "PRIVATE_BODY_MARKER")), true},
		{"other_message", "PRIVATE_BODY_MARKER", attrs("launch", status), false},
		{"other_stage", "native converter qualification failed", attrs("response", status), false},
		{"other_status", "native converter qualification failed", attrs("launch", strings.ReplaceAll(status, "0xc0000022", "0xc0000005")), false},
		{"decimal_status_mismatch", "native converter qualification failed", attrs("launch", strings.ReplaceAll(status, "3221225506", "1")), false},
		{"missing_status", "native converter qualification failed", []slog.Attr{slog.String("stage", "launch")}, false},
		{"typed_status_required", "native converter qualification failed", []slog.Attr{slog.String("stage", "launch"), slog.Int("process_status", 1)}, false},
		{"duplicate_stage", "native converter qualification failed", append(attrs("launch", status), slog.String("stage", "launch")), false},
		{"duplicate_status", "native converter qualification failed", append(attrs("launch", status), slog.String("process_status", status)), false},
		{"zero_pid", "native converter qualification failed", attrs("launch", strings.ReplaceAll(status, "pid=123", "pid=0")), false},
		{"overflow_pid", "native converter qualification failed", attrs("launch", strings.ReplaceAll(status, "pid=123", "pid=4294967296")), false},
		{"noncanonical_flags", "native converter qualification failed", attrs("launch", strings.ReplaceAll(status, "0x00080004", "0x80004")), false},
		{"trailing_private_body", "native converter qualification failed", attrs("launch", status+" PRIVATE_BODY_MARKER"), false},
		{"over_bound", "native converter qualification failed", attrs("launch", status+strings.Repeat("x", 257)), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var projection placementFailureProjection
			slog.New(&projection).LogAttrs(context.Background(), slog.LevelDebug, test.message, test.attrs...)
			want := "synthetic_place_failed"
			if test.want {
				want = placementLaunchAccessDenied
			}
			if got := projection.message(); got != want || len(got) > 128 || strings.Contains(got, "PRIVATE_BODY_MARKER") {
				t.Fatal("placement failure lost its closed body-free projection")
			}
		})
	}
	for _, mode := range []string{"duplicate_record", "attached_attributes", "group"} {
		t.Run(mode, func(t *testing.T) {
			var projection placementFailureProjection
			logger := slog.New(&projection)
			logger.LogAttrs(context.Background(), slog.LevelDebug, "native converter qualification failed", attrs("launch", status)...)
			switch mode {
			case "duplicate_record":
				logger.LogAttrs(context.Background(), slog.LevelDebug, "native converter qualification failed", attrs("launch", status)...)
			case "attached_attributes":
				_ = logger.With("private", "PRIVATE_BODY_MARKER")
			case "group":
				_ = logger.WithGroup("private")
			}
			if projection.message() != "synthetic_place_failed" {
				t.Fatal("ambiguous placement diagnostic selected the loader trace")
			}
		})
	}
}

func TestFixtureConversionWirePreservesExactBodies(t *testing.T) {
	original := docling.Conversion{
		Result:     ingest.Result{Status: ingest.StatusUsable},
		Provenance: docling.Provenance{NetworkDenied: true, PackSHA256: "synthetic-pack", LockSHA256: "synthetic-lock"},
		Markdown:   []byte("  WIRE_PRIVATE_MARKER <text>\nsecond line\n"),
		Document:   json.RawMessage("{\n  \"wire_test\": \"WIRE_PRIVATE_MARKER <text>\"\n}\n"),
	}
	// Decode the explicit parent protocol, not production Conversion's json:-
	// fields. Byte slices preserve whitespace/escaping exactly through JSON.
	var wire struct {
		Metadata docling.Conversion `json:"metadata"`
		Markdown []byte             `json:"markdown"`
		Document []byte             `json:"document"`
	}
	if json.Unmarshal(captureFixtureEmit(t, original), &wire) != nil {
		t.Fatal("private conversion wire is invalid")
	}
	got := wire.Metadata
	got.Markdown, got.Document = wire.Markdown, json.RawMessage(wire.Document)
	if !reflect.DeepEqual(got, original) {
		t.Fatal("fixture wire lost original metadata or exact document bytes")
	}
}

func TestFixtureProductionJSONKeepsBodiesPrivate(t *testing.T) {
	conversion := docling.Conversion{Markdown: []byte("WIRE_PRIVATE_MARKER"), Document: json.RawMessage(`{"wire_test":"WIRE_PRIVATE_MARKER"}`)}
	body, err := json.Marshal(conversion)
	if err != nil {
		t.Fatal("production metadata encoding failed")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		t.Fatal("production metadata JSON invalid")
	}
	if len(fields) != 2 || fields["result"] == nil || fields["provenance"] == nil {
		t.Fatal("production JSON exposed non-metadata fields")
	}
	if _, exists := fields["markdown"]; exists {
		t.Fatal("production JSON exposed Markdown")
	}
	if _, exists := fields["document"]; exists {
		t.Fatal("production JSON exposed Document")
	}
	if bytes.Contains(body, []byte("WIRE_PRIVATE_MARKER")) {
		t.Fatal("production JSON exposed body contents")
	}
}

func TestFixtureNonConversionResponseUnchanged(t *testing.T) {
	availability := docling.Availability{State: "ready", Profile: "synthetic-profile", PackSHA256: "synthetic-pack"}
	expected, err := json.Marshal(availability)
	if err != nil {
		t.Fatal("metadata response encoding failed")
	}
	if !bytes.Equal(captureFixtureEmit(t, availability), append(expected, '\n')) {
		t.Fatal("non-conversion response changed")
	}
}
