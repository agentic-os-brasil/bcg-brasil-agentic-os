package ownerctx

import (
	"errors"
	"path/filepath"
	"sort"
)

var ErrSessionContextUnconfirmed = errors.New("owner session context requires current confirmed onboarding")
var ErrSessionContextChanged = errors.New("owner session context changed during projection")

// AcceptedSessionContext is data for the bound owner session, not effect authority.
type AcceptedSessionContext struct {
	Reader             string
	Track              string
	ConfirmationSHA256 string
	Generation         string
	Facets             []SnapshotFacet
}

// ProjectAcceptedSessionContext selects explicit, confirmed, session-readable facets.
// It does not accept a claimed reader and never repairs missing owner state.
func ProjectAcceptedSessionContext(root string) (result AcceptedSessionContext, err error) {
	unlock, err := acquireOwnerTransitionFile(filepath.Join(root, "owner", "interview", ".transition.lock"), false)
	if err != nil {
		return result, err
	}
	defer func() {
		if closeErr := unlock(); closeErr != nil {
			result = AcceptedSessionContext{}
			err = errors.Join(err, closeErr)
		}
	}()
	registry, err := readRegistry(root)
	if err != nil {
		return result, err
	}
	if registry.OnboardingConfirmedAt == "" || registry.OnboardingConfirmedSHA256 == "" {
		return result, ErrSessionContextUnconfirmed
	}
	before, err := reviewOnboarding(root)
	if err != nil {
		return result, err
	}
	if before.State != "complete" {
		return result, ErrSessionContextUnconfirmed
	}
	ids := make([]string, 0, len(before.Facets))
	for _, facet := range before.Facets {
		for _, reader := range facet.Readers {
			if reader == "session" {
				ids = append(ids, facet.Facet)
				break
			}
		}
	}
	if len(ids) == 0 {
		return result, ErrSessionContextUnconfirmed
	}
	sort.Strings(ids)
	snapshot, err := ProjectSnapshot(root, ids)
	if err != nil {
		return result, err
	}
	after, err := reviewOnboarding(root)
	if err != nil {
		return result, err
	}
	if after.State != "complete" || before.ReviewDigest != after.ReviewDigest {
		return result, ErrSessionContextChanged
	}
	current := make(map[string]OnboardingFacetReview, len(after.Facets))
	for _, facet := range after.Facets {
		for _, reader := range facet.Readers {
			if reader == "session" {
				current[facet.Facet] = facet
				break
			}
		}
	}
	if len(current) != len(ids) {
		return result, ErrSessionContextChanged
	}
	selected := make([]SnapshotFacet, 0, len(ids))
	for _, id := range ids {
		facet := snapshot.Facets[id]
		approved, ok := current[id]
		if !ok || facet.Content != approved.Body || facet.Sensitivity != approved.Sensitivity || facet.Refinement != approved.Policy || !sameStrings(facet.Readers, approved.Readers) {
			return result, ErrSessionContextChanged
		}
		selected = append(selected, facet)
	}
	return AcceptedSessionContext{Reader: "session", Track: after.Track, ConfirmationSHA256: after.ReviewDigest, Generation: digest("accepted-owner-session-context-v1\x00" + after.ReviewDigest + "\x00" + snapshot.CanonicalSourceDigest), Facets: selected}, nil
}
