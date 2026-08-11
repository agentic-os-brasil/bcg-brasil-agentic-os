package ownerctx

import (
	"strings"
	"unicode"
)

// TechCoreRecommendation is a bounded, deterministic suggestion derived only
// from the function the collaborator explicitly declared. It never changes
// installation state; the included bundle is already available and the
// recommendation only suggests a routing track.
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
	if containsRole(value, "engenheiro de software", "engenheira de software", "engenharia de software", "software engineer", "software developer", "desenvolvedor", "desenvolvedora", "developer", "programador", "programadora", "tech lead", "devops", "sre") {
		domains = append(domains, "engineering")
		tracks = append(tracks, "software-engineering")
	}
	if containsRole(value, "inteligencia artificial", "artificial intelligence", "machine learning", "ml engineer", "engenharia de ia", "engenheiro de ia", "genai", "llm", "ia aplicada", "ai engineer", "ai engineering") {
		domains = append(domains, "ai")
		tracks = append(tracks, "ai-engineering")
	}
	if containsRole(value, "cientista de dados", "cientista em dados", "data scientist", "data science", "analista de dados", "data analyst", "analytics", "estatistica", "modelagem estatistica") {
		domains = append(domains, "data")
		tracks = append(tracks, "data-science")
	}
	if containsRole(value, "engenheiro de dados", "engenheira de dados", "engenharia de dados", "data engineer", "data engineering", "data platform", "plataforma de dados", "pipeline de dados") {
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
		Question:        "O Tech Core já está incluído no workspace. Deseja registrar a trilha mais aderente para personalizar o roteamento?",
	}
}

func askForTechCore() TechCoreRecommendation {
	return TechCoreRecommendation{
		State:    "ask",
		Bundle:   "tech-core",
		Reason:   "A função declarada não permite recomendar uma frente técnica com segurança.",
		Question: "O Tech Core (engineering, data e AI) já está incluído. Deseja registrar uma trilha técnica para personalizar o roteamento?",
	}
}

func normalizeFunction(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("á", "a", "à", "a", "ã", "a", "â", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o", "ú", "u", "ç", "c").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func containsRole(value string, terms ...string) bool {
	for _, term := range terms {
		for offset := 0; offset < len(value); {
			found := strings.Index(value[offset:], term)
			if found < 0 {
				break
			}
			start := offset + found
			end := start + len(term)
			if tokenBoundary(value, start, end) && !negatedRole(value, start) && !businessRole(value, end) {
				return true
			}
			offset = end
		}
	}
	return false
}

func tokenBoundary(value string, start, end int) bool {
	isWord := func(char byte) bool {
		return char == '_' || unicode.IsLetter(rune(char)) || unicode.IsDigit(rune(char))
	}
	return (start == 0 || !isWord(value[start-1])) && (end == len(value) || !isWord(value[end]))
}

func negatedRole(value string, start int) bool {
	prefix := strings.TrimSpace(value[maxInt(0, start-32):start])
	for _, marker := range []string{"nao sou", "nao trabalho como", "nao atuo como", "nao exerco", "fora de"} {
		if strings.HasSuffix(prefix, marker) {
			return true
		}
	}
	return false
}

func businessRole(value string, end int) bool {
	suffix := strings.TrimSpace(value[end:])
	return strings.HasPrefix(suffix, "de negocio") || strings.HasPrefix(suffix, "de negocios")
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
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
