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
		{name: "data science", input: "cientista de dados", domain: "data", track: "data-science"},
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
	for _, function := range []string{"Consultora de estratégia", "Profissional de cuidados paliativos"} {
		got := RecommendTechCore(function)
		if got.State != "ask" || got.Bundle != "tech-core" || got.Question == "" || len(got.SuggestedTracks) != 0 {
			t.Fatalf("recommendation for %q = %#v", function, got)
		}
	}
}

func TestRecommendTechCoreSeparatesDataScienceAndDataEngineering(t *testing.T) {
	for _, test := range []struct {
		function string
		track    string
	}{
		{function: "Cientista de dados", track: "data-science"},
		{function: "Engenheiro de dados", track: "data-engineering"},
	} {
		got := RecommendTechCore(test.function)
		if got.State != "recommended" || len(got.SuggestedTracks) != 1 || got.SuggestedTracks[0] != test.track {
			t.Fatalf("recommendation for %q = %#v", test.function, got)
		}
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
