package priorwork

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var ErrExplicitIntentRequired = errors.New("explicit prior-work intent is required")

type Query struct {
	Text                    string
	ExplicitPriorWorkIntent bool
	Limit                   int
	Access                  AccessContext
}

type SearchResult struct {
	ItemRef           string    `json:"item_ref"`
	Root              RootRef   `json:"root"`
	Name              string    `json:"name"`
	SourceURL         string    `json:"source_url"`
	Facets            Facets    `json:"facets"`
	ModifiedAt        time.Time `json:"modified_at"`
	Score             int       `json:"score"`
	MatchedTerms      []string  `json:"matched_terms"`
	CatalogFreshness  string    `json:"catalog_freshness"`
	AuthorizationNote string    `json:"authorization_note"`
}

type SearchResponse struct {
	SchemaVersion int            `json:"schema_version"`
	State         string         `json:"state"`
	Watermark     string         `json:"watermark"`
	Fingerprint   string         `json:"fingerprint"`
	Results       []SearchResult `json:"results"`
}

var queryStopwords = map[string]bool{
	"a": true, "ao": true, "aos": true, "apresentei": true, "da": true,
	"das": true, "de": true, "do": true, "dos": true, "e": true, "em": true,
	"encontre": true, "material": true, "me": true, "na": true, "nas": true,
	"no": true, "nos": true, "o": true, "os": true, "para": true, "por": true,
	"procure": true, "pro": true, "quero": true, "que": true, "recuperar": true,
	"sobre": true, "um": true, "uma": true,
}

func normalize(value string) string {
	var output strings.Builder
	output.Grow(len(value))
	space := true
	for _, character := range strings.ToLower(value) {
		switch character {
		case 'á', 'à', 'â', 'ã', 'ä':
			character = 'a'
		case 'é', 'è', 'ê', 'ë':
			character = 'e'
		case 'í', 'ì', 'î', 'ï':
			character = 'i'
		case 'ó', 'ò', 'ô', 'õ', 'ö':
			character = 'o'
		case 'ú', 'ù', 'û', 'ü':
			character = 'u'
		case 'ç':
			character = 'c'
		}
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			output.WriteRune(character)
			space = false
			continue
		}
		if !space {
			output.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(output.String())
}

func queryTerms(text string) []string {
	seen := map[string]bool{}
	var terms []string
	for _, term := range strings.Fields(normalize(text)) {
		if queryStopwords[term] || len(term) < 2 || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
	}
	return terms
}

func scoreItem(item Item, terms []string) (int, []string) {
	fields := []struct {
		weight int
		values []string
	}{
		{10, item.Facets.Clients},
		{10, yearsAsStrings(item.Facets.Years)},
		{9, item.Facets.Themes},
		{8, item.Facets.Projects},
		{8, item.Facets.Audiences},
		{7, item.Facets.Presenters},
		{6, []string{item.Name}},
		{5, item.Facets.People},
		{4, item.SearchTerms},
		{3, item.PathSegments},
	}
	normalizedFields := make([][]string, len(fields))
	for index, field := range fields {
		for _, value := range field.values {
			normalizedFields[index] = append(normalizedFields[index], normalize(value))
		}
	}

	score := 0
	var matched []string
	for _, term := range terms {
		termScore := 0
		for index, field := range fields {
			for _, value := range normalizedFields[index] {
				if containsTerm(value, term) && field.weight > termScore {
					termScore = field.weight
				}
			}
		}
		if termScore > 0 {
			score += termScore
			matched = append(matched, term)
		}
	}
	return score, matched
}

func containsTerm(value, term string) bool {
	for _, token := range strings.Fields(value) {
		if token == term {
			return true
		}
	}
	return false
}

func yearsAsStrings(years []int) []string {
	values := make([]string, 0, len(years))
	for _, year := range years {
		values = append(values, strconv.Itoa(year))
	}
	return values
}

func (store Store) Find(query Query) (SearchResponse, error) {
	if !query.ExplicitPriorWorkIntent {
		return SearchResponse{}, ErrExplicitIntentRequired
	}
	if strings.TrimSpace(query.Text) == "" {
		return SearchResponse{}, errors.New("prior-work query cannot be empty")
	}
	if query.Limit == 0 {
		query.Limit = 5
	}
	if query.Limit < 1 || query.Limit > 20 {
		return SearchResponse{}, errors.New("prior-work result limit must be between 1 and 20")
	}
	releaseLock, err := store.acquireImportLock()
	if err != nil {
		return SearchResponse{}, err
	}
	defer releaseLock()
	currentTime := store.now()

	enrollment, err := store.loadEnrollment()
	if err != nil {
		return SearchResponse{}, err
	}
	if err := authorize(enrollment, query.Access); err != nil {
		return SearchResponse{}, err
	}
	if !currentTime.Before(enrollment.AuthorizationExpiresAt) {
		return SearchResponse{}, errors.New("prior-work enrollment authorization has expired")
	}
	manifest, catalog, err := store.loadActive()
	if err != nil {
		return SearchResponse{}, err
	}
	if manifest.TenantRef != enrollment.TenantRef || catalog.TenantRef != enrollment.TenantRef ||
		catalog.PolicyVersion != enrollment.PolicyVersion || !rootsEqual(catalog.Roots, enrollment.Roots) {
		return SearchResponse{}, errors.New("active catalog no longer matches the enrolled policy")
	}
	enrollmentFingerprint, err := fingerprintEnrollment(enrollment)
	if err != nil {
		return SearchResponse{}, err
	}
	if manifest.EnrollmentFingerprint != enrollmentFingerprint {
		return SearchResponse{}, errors.New("active catalog enrollment fingerprint is stale")
	}
	state := freshnessState(manifest.PublishedAt, enrollment, currentTime)
	terms := queryTerms(query.Text)
	if len(terms) == 0 {
		return SearchResponse{}, errors.New("prior-work query has no searchable terms")
	}
	barriers, err := store.loadBarriers()
	if err != nil {
		return SearchResponse{}, err
	}

	response := SearchResponse{
		SchemaVersion: 1,
		State:         state,
		Watermark:     manifest.Watermark,
		Fingerprint:   manifest.Fingerprint,
		Results:       []SearchResult{},
	}
	for _, item := range catalog.Items {
		if item.Kind != "file" || item.Status != "active" || barriers[item.key()] {
			continue
		}
		score, matched := scoreItem(item, terms)
		if score == 0 {
			continue
		}
		response.Results = append(response.Results, SearchResult{
			ItemRef:           item.ItemRef,
			Root:              item.Root,
			Name:              item.Name,
			SourceURL:         item.SourceURL,
			Facets:            item.Facets,
			ModifiedAt:        item.ModifiedAt,
			Score:             score,
			MatchedTerms:      matched,
			CatalogFreshness:  state,
			AuthorizationNote: "Opening this SharePoint source rechecks the current source authorization.",
		})
	}
	sort.Slice(response.Results, func(i, j int) bool {
		left, right := response.Results[i], response.Results[j]
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if !left.ModifiedAt.Equal(right.ModifiedAt) {
			return left.ModifiedAt.After(right.ModifiedAt)
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.Root.key() != right.Root.key() {
			return left.Root.key() < right.Root.key()
		}
		return left.ItemRef < right.ItemRef
	})
	if len(response.Results) > query.Limit {
		response.Results = response.Results[:query.Limit]
	}
	return response, nil
}

func (response SearchResponse) Validate() error {
	if response.SchemaVersion != 1 || response.State == "" || response.Watermark == "" || response.Fingerprint == "" {
		return fmt.Errorf("invalid prior-work search response")
	}
	return nil
}
