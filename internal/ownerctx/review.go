package ownerctx

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maximumOwnerFacetBytes = 12 << 10
const maximumOwnerEvidenceBytes = 4 << 10

func readOwnerFacet(path string) ([]byte, error) {
	body, err := readOwnerFile(path, maximumOwnerFacetBytes)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(body) {
		return nil, errors.New("owner facet text must be valid UTF-8")
	}
	// Onboarding digests use NUL framing; facet text must not contain it.
	if strings.ContainsRune(string(body), 0) {
		return nil, errors.New("owner facet text cannot contain NUL")
	}
	return body, nil
}

// RefinementReview contains private text for an explicit operator or trusted
// client review interaction. It must never be returned as an ordinary receipt.
type RefinementReview struct {
	ID             string   `json:"id"`
	Facet          string   `json:"facet"`
	Evidence       string   `json:"evidence"`
	ProposedBody   string   `json:"proposed_body"`
	CurrentBody    string   `json:"current_body"`
	Policy         string   `json:"policy"`
	Sensitivity    string   `json:"sensitivity"`
	Readers        []string `json:"readers"`
	BaseSHA256     string   `json:"base_sha256"`
	ProposalSHA256 string   `json:"proposal_sha256"`
	ReviewDigest   string   `json:"review_digest"`
}

type OnboardingFacetReview struct {
	Facet       string   `json:"facet"`
	Body        string   `json:"body"`
	Sensitivity string   `json:"sensitivity"`
	Readers     []string `json:"readers"`
	Policy      string   `json:"policy"`
}

// OnboardingReview is private UI content bound to the selected track and exact
// canonical bodies. Status and confirmation receipts remain metadata-only.
type OnboardingReview struct {
	Track        string                  `json:"track"`
	State        string                  `json:"state"`
	Facets       []OnboardingFacetReview `json:"facets"`
	ReviewDigest string                  `json:"review_digest"`
}

func ReviewRefinement(root, id string) (RefinementReview, error) {
	var result RefinementReview
	err := withOwnerTransition(root, func() error {
		var err error
		result, err = reviewRefinement(root, id)
		return err
	})
	return result, err
}

func reviewRefinement(root, id string) (RefinementReview, error) {
	p, err := readProposal(root, id)
	if err != nil {
		return RefinementReview{}, err
	}
	definition, err := facetDefinition(root, p.Facet)
	if err != nil {
		return RefinementReview{}, err
	}
	if p.State != "proposed" || p.Policy != definition.Refinement || p.Sensitivity != definition.Sensitivity || !sameStrings(p.Readers, definition.Readers) {
		return RefinementReview{}, ErrRevisionConflict
	}
	current, err := readOwnerFacet(filepath.Join(root, filepath.FromSlash(definition.Path)))
	if err != nil {
		return RefinementReview{}, err
	}
	if digest(string(current)) != p.SourceSHA256 {
		// A prepared audit can recover the exact reviewed base after a crash
		// between canonical publication and the applied receipt.
		item, err := readAudit(root, "audit-"+p.ID)
		expectedBefore := filepath.ToSlash(filepath.Join("owner", "refinement", "versions", p.Facet, p.ID+".before.md"))
		if err != nil || (item.State != "prepared" && item.State != "applied") || item.ProposalID != p.ID || item.Facet != p.Facet || item.BeforePath != expectedBefore || item.BeforeSHA256 != p.SourceSHA256 || item.AfterSHA256 != digest(p.ProposedBody) || item.AfterSHA256 != digest(string(current)) || item.EvidenceSHA256 != digest(p.Evidence) {
			return RefinementReview{}, ErrRevisionConflict
		}
		current, err = readOwnerFacet(filepath.Join(root, filepath.FromSlash(expectedBefore)))
		if err != nil {
			return RefinementReview{}, err
		}
		if digest(string(current)) != p.SourceSHA256 {
			return RefinementReview{}, ErrRevisionConflict
		}
	}
	r := RefinementReview{ID: p.ID, Facet: p.Facet, Evidence: p.Evidence, ProposedBody: p.ProposedBody, CurrentBody: string(current), Policy: definition.Refinement, Sensitivity: definition.Sensitivity, Readers: append([]string(nil), definition.Readers...), BaseSHA256: p.SourceSHA256, ProposalSHA256: digestJSON(p)}
	r.ReviewDigest = digestJSON(r)
	return r, nil
}

// ApplyReviewedRefinement re-reads the exact envelope while holding the owner
// transition lock. A stale digest/base grants no write authority.
func ApplyReviewedRefinement(root, id, reviewDigest string, confirmed bool) (RefinementReceipt, error) {
	if !confirmed {
		return RefinementReceipt{}, ErrConfirmationRequired
	}
	var result RefinementReceipt
	err := withOwnerTransition(root, func() error {
		review, err := reviewRefinement(root, id)
		if err != nil {
			return err
		}
		if !secureDigestEqual(reviewDigest, review.ReviewDigest) {
			return ErrRevisionConflict
		}
		result, err = applyRefinementLocked(root, id, true)
		return err
	})
	return result, err
}

func ReviewOnboarding(root string) (OnboardingReview, error) {
	var result OnboardingReview
	err := withOwnerTransition(root, func() error {
		var err error
		result, err = reviewOnboarding(root)
		return err
	})
	return result, err
}

func reviewOnboarding(root string) (OnboardingReview, error) {
	value, err := readRegistry(root)
	if err != nil {
		return OnboardingReview{}, err
	}
	definition, ok := onboardingTracks[value.OnboardingTrack]
	if !ok {
		return OnboardingReview{}, errors.New("select an onboarding track before review")
	}
	result := OnboardingReview{Track: definition.ID, State: "review_required", Facets: make([]OnboardingFacetReview, 0, len(definition.Facets))}
	parts := []string{"track\x00" + definition.ID}
	for _, id := range definition.Facets {
		record, ok := value.Facets[id]
		if !ok {
			return OnboardingReview{}, errors.New("onboarding facet is unavailable")
		}
		body, err := readOwnerFacet(filepath.Join(root, filepath.FromSlash(record.Path)))
		if err != nil {
			return OnboardingReview{}, err
		}
		if strings.TrimSpace(string(body)) == "" || strings.TrimSpace(string(body)) == strings.TrimSpace(facets[id].Body) {
			return OnboardingReview{}, errors.New("onboarding answers are not ready for owner confirmation")
		}
		result.Facets = append(result.Facets, OnboardingFacetReview{Facet: id, Body: string(body), Sensitivity: record.Sensitivity, Readers: append([]string(nil), record.Readers...), Policy: record.Refinement})
		parts = append(parts, id+"\x00"+string(body))
	}
	result.ReviewDigest = digest(strings.Join(parts, "\x00"))
	if value.OnboardingConfirmedAt != "" && secureDigestEqual(value.OnboardingConfirmedSHA256, result.ReviewDigest) {
		result.State = "complete"
	}
	return result, nil
}

func withOwnerTransition(root string, operation func() error) error {
	// Do not bootstrap owner state as a side effect of review or mutation.
	if _, err := readRegistry(root); err != nil {
		return err
	}
	unlock, err := acquireOwnerTransition(filepath.Join(root, "owner", "interview", ".transition.lock"))
	if err != nil {
		return err
	}
	return errors.Join(operation(), unlock())
}
