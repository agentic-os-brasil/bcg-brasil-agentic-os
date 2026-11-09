package ingest_test

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"testing"
)

func TestPolicyRoutesRemainClosedAndMarkItDownOptIn(t *testing.T) {
	for ext := range ingest.DefaultPolicy().AllowedExts {
		r, err := ingest.SelectFormat(ext, false)
		if err != nil || r.Parser == "" || r.Route == "markitdown_fallback" {
			t.Fatalf("%s: %+v %v", ext, r, err)
		}
	}
	for _, ext := range []string{".pdf", ".png", ".pptx", ".jats", ".xbrl", ".DOCX"} {
		if _, err := ingest.SelectFormat(ext, false); err == nil {
			t.Fatalf("unadmitted route %s", ext)
		}
	}
	literal, _ := ingest.SelectFormat(".json", false)
	native, _ := ingest.SelectFormat(".json", true)
	if literal.Fidelity != ingest.FidelityTextual || native.Parser != "docling.json" || native.Fidelity != ingest.FidelityStructured {
		t.Fatalf("JSON routes: %+v %+v", literal, native)
	}
	if _, err := ingest.SelectFallback(ingest.PrimaryReady, "markitdown"); err == nil {
		t.Fatal("implicit fallback")
	}
}
