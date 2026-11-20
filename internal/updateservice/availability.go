package updateservice

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateplan"
)

// SyncedAvailability describes signed metadata only. It never establishes
// payload readiness, source custody, a pending plan or release activation.
type SyncedAvailability struct {
	Outcome            string `json:"outcome"`
	PayloadState       string `json:"payload_state"`
	CurrentRelease     string `json:"current_release"`
	SourceStateSHA256  string `json:"source_state_sha256"`
	Sequence           uint64 `json:"sequence,omitempty"`
	GenerationSHA256   string `json:"generation_sha256,omitempty"`
	CandidateRelease   string `json:"candidate_release,omitempty"`
	ManifestSHA256     string `json:"manifest_sha256,omitempty"`
	ClientInputsSHA256 string `json:"client_inputs_sha256,omitempty"`
}

func (a SyncedAvailability) MetadataReady() bool {
	return a.Outcome == "current_signed_metadata" || a.Outcome == "update_available_metadata_only"
}

func (a SyncedAvailability) Validate() error {
	digest := func(s string) bool { return len(s) == 64 && strings.Trim(s, "0123456789abcdef") == "" }
	if a.PayloadState != "not_checked" || !digest(a.SourceStateSHA256) {
		return errors.New("invalid availability observation binding")
	}
	if _, err := updateplan.CompareVersions(a.CurrentRelease, a.CurrentRelease); err != nil {
		return err
	}
	switch a.Outcome {
	case "source_unavailable", "source_custody_denied":
		if a.Sequence == 0 && a.GenerationSHA256 == "" && a.CandidateRelease == "" && a.ManifestSHA256 == "" && a.ClientInputsSHA256 == "" {
			return nil
		}
		return errors.New("unavailable observation carries unverified metadata")
	case "current_signed_metadata", "update_available_metadata_only", "source_expired", "source_time_invalid", "source_replayed_or_changed", "installed_release_recalled", "candidate_release_recalled", "older_release_metadata", "immutable_release_changed":
	default:
		return errors.New("unknown availability outcome")
	}
	if a.Sequence == 0 || !digest(a.GenerationSHA256) || !digest(a.ManifestSHA256) || !digest(a.ClientInputsSHA256) {
		return errors.New("availability lacks authenticated metadata identity")
	}
	_, err := updateplan.CompareVersions(a.CandidateRelease, a.CandidateRelease)
	return err
}

// ObserveSyncedAvailability uses only the already enrolled source and the
// existing authenticated review. It performs no filesystem writes.
func ObserveSyncedAvailability(o SyncedOptions) (SyncedAvailability, error) {
	current, err := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
	if err != nil {
		return SyncedAvailability{}, err
	}
	if current.Source == nil || o.DataRoot != filepath.Join(o.UserHome, ".maestro-install") || current.Source.Locator != o.SourceRoot || current.Source.Tuple != o.ExpectedSource {
		return SyncedAvailability{}, errors.New("availability requires the exact enrolled source")
	}
	a := SyncedAvailability{Outcome: "source_unavailable", PayloadState: "not_checked", CurrentRelease: current.Release, SourceStateSHA256: installtx.StateDigest(current)}
	if current.Source.RecallCapacityExhausted {
		a.Outcome = "source_custody_denied"
		return a, nil
	}
	review, err := ReviewSynced(o)
	if err != nil {
		return a, nil
	}
	a.Sequence, a.GenerationSHA256, a.CandidateRelease = review.Sequence, review.GenerationSHA256, review.CandidateRelease
	a.ManifestSHA256, a.ClientInputsSHA256 = review.ManifestSHA256, review.ClientInputsSHA256
	if review.CurrentSHA256 != a.SourceStateSHA256 {
		return SyncedAvailability{}, errors.New("installation changed during availability observation")
	}
	after, err := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
	if err != nil || installtx.StateDigest(after) != a.SourceStateSHA256 {
		return SyncedAvailability{}, errors.New("installation changed during availability observation")
	}
	if a.Sequence < current.Source.Sequence || a.Sequence == current.Source.Sequence && a.GenerationSHA256 != current.Source.GenerationSHA256 {
		a.Outcome = "source_replayed_or_changed"
		return a, nil
	}
	for _, recalls := range [][]releasecontract.ReleaseRecall{current.Source.Recalls, review.Generation.Recalls} {
		for _, recall := range recalls {
			if recall.Release == current.Release && recall.ManifestSHA256 == current.Binding.ManifestSHA256 {
				a.Outcome = "installed_release_recalled"
				return a, nil
			}
			if recall.Release == a.CandidateRelease && recall.ManifestSHA256 == a.ManifestSHA256 {
				a.Outcome = "candidate_release_recalled"
				return a, nil
			}
		}
	}
	now := time.Now().UTC()
	if o.Clock != nil {
		now = o.Clock().UTC()
	}
	if err := review.Generation.CheckTime(now, current.Source.ObservedAt); err != nil {
		a.Outcome = "source_time_invalid"
		expires, _ := time.Parse(time.RFC3339, review.ExpiresAt)
		if !now.Before(expires) {
			a.Outcome = "source_expired"
		}
		return a, nil
	}
	comparison, err := updateplan.CompareVersions(a.CandidateRelease, current.Release)
	if err != nil {
		return SyncedAvailability{}, err
	}
	switch {
	case comparison == 0:
		a.Outcome = "current_signed_metadata"
		if a.ManifestSHA256 != current.Binding.ManifestSHA256 || a.ClientInputsSHA256 != current.Binding.ClientInputsSHA256 {
			a.Outcome = "immutable_release_changed"
		}
	case comparison > 0:
		a.Outcome = "update_available_metadata_only"
	default:
		a.Outcome = "older_release_metadata"
	}
	return a, a.Validate()
}
