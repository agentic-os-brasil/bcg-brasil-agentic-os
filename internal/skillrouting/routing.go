// Package skillrouting selects a bounded set of installed, policy-allowed
// methods from one ephemeral user prompt. It returns pointers only and never
// persists or returns prompt text or skill bodies.
package skillrouting

import (
	"errors"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

const MaximumSelections = 2

type InstalledSkill struct {
	ID      string
	Pointer string
}

type Request struct {
	Prompt    string
	Role      string
	Catalog   skillsindex.Catalog
	Policy    skillpolicy.Policy
	Installed []InstalledSkill
}

type Selection struct {
	ID      string `json:"id"`
	Reason  string `json:"reason"`
	Pointer string `json:"pointer"`
}

type candidate struct {
	skill   skillsindex.Skill
	pointer string
	tokens  map[string]bool
	score   int
	order   int
}

// Route selects explicit references first in prompt order, then fills the
// remaining bounded slots with deterministic lexical matches. Unknown intent
// is an empty selection, not a guessed method.
func Route(request Request) ([]Selection, error) {
	if err := request.Catalog.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, nil
	}
	installed, err := installedPointers(request.Installed)
	if err != nil {
		return nil, err
	}

	eligible := make([]candidate, 0, len(request.Catalog.Skills))
	for _, skill := range request.Catalog.Skills {
		pointer, ok := installed[skill.ID]
		if !ok || !request.Policy.AllowsDirect(request.Role, skill.ID) {
			continue
		}
		eligible = append(eligible, candidate{skill: skill, pointer: pointer, tokens: catalogTokens(skill)})
	}

	lowerPrompt := strings.ToLower(request.Prompt)
	selections := make([]Selection, 0, MaximumSelections)
	selected := map[string]bool{}
	for len(selections) < MaximumSelections {
		bestIndex, bestPosition := -1, len(lowerPrompt)+1
		for index := range eligible {
			if selected[eligible[index].skill.ID] {
				continue
			}
			position := explicitReferencePosition(lowerPrompt, eligible[index].skill.ID)
			if position >= 0 && (position < bestPosition || (position == bestPosition && eligible[index].skill.ID < eligible[bestIndex].skill.ID)) {
				bestIndex, bestPosition = index, position
			}
		}
		if bestIndex < 0 {
			break
		}
		chosen := eligible[bestIndex]
		selections = append(selections, Selection{ID: chosen.skill.ID, Reason: "explicit_skill_reference", Pointer: chosen.pointer})
		selected[chosen.skill.ID] = true
	}
	if len(selections) == MaximumSelections {
		return selections, nil
	}

	promptTokens := tokenSet(request.Prompt)
	frequency := map[string]map[string]int{}
	for _, item := range eligible {
		if frequency[item.skill.Bundle] == nil {
			frequency[item.skill.Bundle] = map[string]int{}
		}
		if selected[item.skill.ID] {
			continue
		}
		for token := range item.tokens {
			frequency[item.skill.Bundle][token]++
		}
	}
	for index := range eligible {
		if selected[eligible[index].skill.ID] {
			continue
		}
		for token := range eligible[index].tokens {
			if promptTokens[token] && frequency[eligible[index].skill.Bundle][token] == 1 {
				eligible[index].score++
			}
		}
	}
	sort.SliceStable(eligible, func(left, right int) bool {
		if eligible[left].score != eligible[right].score {
			return eligible[left].score > eligible[right].score
		}
		return eligible[left].skill.ID < eligible[right].skill.ID
	})
	for _, item := range eligible {
		if len(selections) == MaximumSelections || item.score < 2 || selected[item.skill.ID] {
			break
		}
		selections = append(selections, Selection{ID: item.skill.ID, Reason: "lexical_intent", Pointer: item.pointer})
		selected[item.skill.ID] = true
	}
	return selections, nil
}

func installedPointers(skills []InstalledSkill) (map[string]string, error) {
	result := make(map[string]string, len(skills))
	for _, skill := range skills {
		cleaned := path.Clean(strings.TrimSpace(skill.Pointer))
		if skill.ID == "" || cleaned == "." || path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") || !strings.HasSuffix(cleaned, "/"+skill.ID+"/SKILL.md") {
			return nil, errors.New("installed skill pointer is invalid")
		}
		if _, duplicate := result[skill.ID]; duplicate {
			return nil, errors.New("installed skill IDs must be unique")
		}
		result[skill.ID] = cleaned
	}
	return result, nil
}

func explicitReferencePosition(prompt, skillID string) int {
	wanted := "$" + skillID
	for offset := 0; offset < len(prompt); {
		index := strings.Index(prompt[offset:], wanted)
		if index < 0 {
			return -1
		}
		index += offset
		after := index + len(wanted)
		if after == len(prompt) || !isSkillIDByte(prompt[after]) {
			return index
		}
		offset = after
	}
	return -1
}

func isSkillIDByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '-'
}

func catalogTokens(skill skillsindex.Skill) map[string]bool {
	return tokenSet(skill.ID + " " + skill.DisplayName + " " + skill.Trigger + " " + skill.DefaultPrompt)
}

func tokenSet(value string) map[string]bool {
	tokens := map[string]bool{}
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if len([]rune(token)) >= 3 && !stopword(token) {
			tokens[token] = true
		}
	}
	return tokens
}

func stopword(token string) bool {
	switch token {
	case "and", "for", "from", "into", "the", "this", "that", "then", "use", "with", "para", "por", "uma", "esse", "essa", "este", "esta", "com", "sem":
		return true
	default:
		return false
	}
}
