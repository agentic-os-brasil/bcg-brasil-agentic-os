package ownerctx

import "testing"

func TestRecommendTechCoreForTechnicalFunctions(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		domain string
		track  string
	}{
		{name: "engineering", input: "Engenheira de software", domain: "engineering", track: "software-engineering"},
		{name: "data", input: "cientista de dados", domain: "data", track: "data-engineering"},
		{name: "ai", input: "AI engineer trabalhando com LLMs", domain: "ai", track: "ai-engineering"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := RecommendTechCore(test.input)
			if got.State != "recommended" || got.Bundle != "tech-core" || len(got.MatchedDomains) != 1 || got.MatchedDomains[0] != test.domain {
				t.Fatalf("recommendation = %#v", got)
			}
			if len(got.SuggestedTracks) != 1 || got.SuggestedTracks[0] != test.track {
				t.Fatalf("suggested tracks = %#v", got.SuggestedTracks)
			}
		})
	}
}

func TestRecommendTechCoreAsksWhenFunctionIsAmbiguous(t *testing.T) {
	got := RecommendTechCore("Consultora de estratégia")
	if got.State != "ask" || got.Bundle != "tech-core" || got.Question == "" || len(got.SuggestedTracks) != 0 {
		t.Fatalf("recommendation = %#v", got)
	}
}

func TestRecommendTechCoreNeverActivatesState(t *testing.T) {
	got := RecommendTechCore("Engenheiro de software")
	if got.State != "recommended" {
		t.Fatalf("recommendation state = %#v", got)
	}
	// The result is a recommendation contract only; callers must still use the
	// explicit profile-selection flow to activate the bundle.
	if got.Question == "" {
		t.Fatal("recommendation omitted explicit confirmation question")
	}
}
