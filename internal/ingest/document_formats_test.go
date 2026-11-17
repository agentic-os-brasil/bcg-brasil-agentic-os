package ingest_test

import (
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
)

func TestAdmitNativeTextPDFAndModernPowerpoint(t *testing.T) {
	for ext, want := range map[string]ingest.FormatRoute{
		".pdf":  {Route: "docling_local", Parser: "docling.pdf_native_text", Fidelity: ingest.FidelityTextual},
		".pptx": {Route: "docling_local", Parser: "docling.pptx", Fidelity: ingest.FidelityStructured},
	} {
		if !ingest.DefaultPolicy().AllowedExts[ext] {
			t.Errorf("missing explicitly admitted document format %s", ext)
		}
		got, err := ingest.SelectFormat(ext, false)
		if err != nil || got != want {
			t.Errorf("%s: got %+v, %v; want %+v", ext, got, err, want)
		}
	}
	for _, ext := range []string{".ppt", ".doc", ".pptm", ".png", ".PDF"} {
		if _, err := ingest.SelectFormat(ext, false); err == nil {
			t.Errorf("unadmitted extension %s", ext)
		}
	}
}
