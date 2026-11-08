package ownerctx

import "errors"

// OnboardingTrackReview binds a requested track to the exact prior onboarding
// state and selection revision. Even a same-value selection advances revision.
type OnboardingTrackReview struct {
	Track             string           `json:"track"`
	CurrentOnboarding OnboardingStatus `json:"current_onboarding"`
	TrackRevision     uint64           `json:"track_revision"`
	ReviewDigest      string           `json:"review_digest"`
}

func ReviewOnboardingTrack(root, track string) (OnboardingTrackReview, error) {
	var result OnboardingTrackReview
	err := withOwnerTransition(root, func() error {
		var err error
		result, err = reviewOnboardingTrack(root, track)
		return err
	})
	return result, err
}

func reviewOnboardingTrack(root, track string) (OnboardingTrackReview, error) {
	if _, ok := onboardingTracks[track]; !ok {
		return OnboardingTrackReview{}, errors.New("onboarding track must be quick or complete")
	}
	value, err := readRegistry(root)
	if err != nil {
		return OnboardingTrackReview{}, err
	}
	status, err := Inspect(root)
	if err != nil {
		return OnboardingTrackReview{}, err
	}
	review := OnboardingTrackReview{Track: track, CurrentOnboarding: status.Onboarding, TrackRevision: value.OnboardingTrackRevision}
	review.ReviewDigest = digestJSON(review)
	return review, nil
}

// ApplyReviewedOnboardingTrack compares and selects under the same canonical
// transition lock, so a later committed choice cannot be overwritten by review.
func ApplyReviewedOnboardingTrack(root, track, reviewDigest string, confirmed bool) (Status, error) {
	if !confirmed {
		return Status{}, ErrConfirmationRequired
	}
	var result Status
	err := withOwnerTransition(root, func() error {
		review, err := reviewOnboardingTrack(root, track)
		if err != nil {
			return err
		}
		if !secureDigestEqual(reviewDigest, review.ReviewDigest) {
			return ErrRevisionConflict
		}
		result, err = selectOnboardingTrackLocked(root, track)
		return err
	})
	return result, err
}
