package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"reflect"
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
