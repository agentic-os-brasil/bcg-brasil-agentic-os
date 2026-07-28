package ingest

import (
	"strings"
	"testing"
)

func TestSelectFallbackRequiresPrimaryRouteFailure(t *testing.T) {
	decision, err := SelectFallback(PrimaryUnavailable, "markitdown")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Adapter != "markitdown" || decision.Route != "markitdown_fallback" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestSelectFallbackDoesNotReplaceReadyPrimary(t *testing.T) {
	_, err := SelectFallback(PrimaryReady, "markitdown")
	if err == nil || !strings.Contains(err.Error(), "primary route is ready") {
		t.Fatalf("error = %v", err)
	}
}
