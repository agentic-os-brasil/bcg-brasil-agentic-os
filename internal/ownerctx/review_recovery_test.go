package ownerctx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReviewedRefinementRecoversInterruptedCanonicalPublication(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	r, err := SubmitRefinement(root, RefinementInput{Facet: "owner-identity", Evidence: "explicit synthetic answer", ProposedBody: "## Current\nSynthetic reviewed identity.\n"})
	if err != nil {
		t.Fatal(err)
	}
	review, err := ReviewRefinement(root, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	original := refinementWriteJSON
	defer func() { refinementWriteJSON = original }()
	interrupted := errors.New("synthetic interruption after canonical publication")
	refinementWriteJSON = func(path string, value any) error {
		if item, ok := value.(audit); ok && item.State == "applied" {
			return interrupted
		}
		return original(path, value)
	}
	if _, err := ApplyReviewedRefinement(root, r.ID, review.ReviewDigest, true); !errors.Is(err, interrupted) {
		t.Fatalf("missing interruption: %v", err)
	}
	refinementWriteJSON = original
	if _, err := ApplyReviewedRefinement(root, r.ID, review.ReviewDigest, true); err != nil {
		t.Fatalf("reviewed retry failed: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "owner", "self", "owner-identity.md"))
	if err != nil || string(body) != review.ProposedBody {
		t.Fatalf("reviewed answer not recovered: %q %v", body, err)
	}
}

func TestNULFacetsInvalidateCurrentAndLegacyOnboardingConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name, track string
		schema      int
		ids         []string
		framedTrack bool
	}{
		{"current", OnboardingTrackQuick, 3, quickOnboardingFacets, true},
		{"legacy-quick", OnboardingTrackQuick, 3, legacyQuickOnboardingFacets, true},
		{"legacy-complete", OnboardingTrackComplete, 3, legacyCompleteOnboardingFacets, true},
		{"schema-v2", "", 2, legacyOnboardingFacets, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if _, err := Initialize(root); err != nil {
				t.Fatal(err)
			}
			body := "Synthetic persisted answer\x00ambiguous"
			for _, template := range facets {
				if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(template.Record.Path)), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			parts := []string{}
			if tc.framedTrack {
				parts = append(parts, "track\x00"+tc.track)
			}
			for _, id := range tc.ids {
				parts = append(parts, id+"\x00"+body)
			}
			value, err := readRegistry(root)
			if err != nil {
				t.Fatal(err)
			}
			value.SchemaVersion = tc.schema
			value.OnboardingTrack = tc.track
			value.OnboardingConfirmedAt = time.Now().UTC().Format(time.RFC3339Nano)
			value.OnboardingConfirmedSHA256 = digest(strings.Join(parts, "\x00"))
			if err := writeOwnerRegistry(root, value); err != nil {
				t.Fatal(err)
			}
			status, err := Inspect(root)
			if err != nil {
				t.Fatal(err)
			}
			if status.Onboarding.State == "complete" || status.Onboarding.ReviewDigest != "" {
				t.Fatalf("malformed persisted %s profile retained confirmation/digest: %#v", tc.name, status.Onboarding)
			}
			if _, err := ConfirmOnboarding(root, value.OnboardingConfirmedSHA256); err == nil {
				t.Fatal("malformed persisted profile confirmed")
			}
		})
	}
}
