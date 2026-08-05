package ownerctx

import "strings"

// TechCoreRecommendation is a bounded, deterministic suggestion derived only
// from the function the collaborator explicitly declared. It never activates
// a bundle; activation still requires an explicit owner selection.
type TechCoreRecommendation struct {
	State           string   `json:"state"`
	Bundle          string   `json:"bundle"`
	MatchedDomains  []string `json:"matched_domains,omitempty"`
	SuggestedTracks []string `json:"suggested_tracks,omitempty"`
	Reason          string   `json:"reason"`
	Question        string   `json:"question,omitempty"`
}

// RecommendTechCore maps explicit function language to the unified tech-core
// bundle. It deliberately recommends rather than infers or mutates state.
func RecommendTechCore(function string) TechCoreRecommendation {
	value := normalizeFunction(function)
	if value == "" {
		return askForTechCore()
	}

	domains := make([]string, 0, 3)
	tracks := make([]string, 0, 3)
	if containsAny(value, "engenheiro de software", "engenheira de software", "engenharia de software", "software engineer", "software developer", "desenvolvedor", "desenvolvedora", "developer", "programador", "programadora", "tech lead", "devops", "sre") {
		domains = append(domains, "engineering")
		tracks = append(tracks, "software-engineering")
	}
	if containsAny(value, "inteligencia artificial", "artificial intelligence", "machine learning", "ml engineer", "engenharia de ia", "engenheiro de ia", "genai", "llm", "ia aplicada", "ai engineer", "ai engineering") {
		domains = append(domains, "ai")
		tracks = append(tracks, "ai-engineering")
	}
	if containsAny(value, "cientista de dados", "cientista em dados", "data scientist", "data science", "analista de dados", "data analyst", "analytics", "estatistica", "modelagem estatistica") {
		domains = append(domains, "data")
		tracks = append(tracks, "data-science")
	}
	if containsAny(value, "engenheiro de dados", "engenheira de dados", "engenharia de dados", "data engineer", "data engineering", "data platform", "plataforma de dados", "pipeline de dados") {
		domains = append(domains, "data")
		tracks = append(tracks, "data-engineering")
	}
	if len(domains) == 0 {
		return askForTechCore()
	}
	return TechCoreRecommendation{
		State:           "recommended",
		Bundle:          "tech-core",
		MatchedDomains:  domains,
		SuggestedTracks: uniqueStrings(tracks),
		Reason:          "A função declarada tem uma frente técnica de " + strings.Join(domains, ", ") + "; o Tech Core reúne engineering, data e AI em um único bundle.",
		Question:        "Deseja incluir o bundle Tech Core no seu workspace? A ativação é opcional e explícita.",
	}
}

func askForTechCore() TechCoreRecommendation {
	return TechCoreRecommendation{
		State:    "ask",
		Bundle:   "tech-core",
		Reason:   "A função declarada não permite recomendar uma frente técnica com segurança.",
		Question: "Você deseja incluir skills de tecnologia (engineering, data e AI) no seu workspace? A ativação é opcional e explícita.",
	}
}

func normalizeFunction(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("á", "a", "à", "a", "ã", "a", "â", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o", "ú", "u", "ç", "c").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func containsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
