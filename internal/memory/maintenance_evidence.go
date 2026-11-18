package memory

import (
	"errors"
	"os"
	"sort"
	"time"
)

// DailyInput identifies one actual eligible ingestion day, not a scheduling
// window. The cohort fingerprint is independent of outputs committed by dream.
type DailyInput struct {
	Period      string `json:"period"`
	Fingerprint string `json:"fingerprint"`
}

// DailyInputCohort uses the existing bounded canonical discovery owner. It
// preloads the owned captures once, applies the permanent denial barrier, and
// fingerprints exactly the same bytes as the actual daily engine.
func (engine *Engine) DailyInputCohort(workspaceID string) ([]DailyInput, error) {
	if err := engine.validateRuntime(workspaceID, false); err != nil {
		return nil, err
	}
	before, err := engine.readVisibility(workspaceID)
	if err != nil {
		return nil, err
	}
	journal, err := engine.journalCaptures(workspaceID)
	if err != nil {
		return nil, err
	}
	owned, err := engine.ownedCaptures(workspaceID)
	if err != nil {
		return nil, err
	}
	byID := map[string]Capture{}
	for _, capture := range append(journal, owned...) {
		byID[capture.ID] = capture
	}
	if len(byID) > 4096 {
		return nil, errors.New("memory captures exceed discovery bound")
	}
	dates := map[string]bool{}
	for _, c := range byID {
		if c.Revision > before.Denied[c.ID] && captureReceipt(c, before.Revision).Eligible {
			dates[c.RecordedAt.UTC().Format("2006-01-02")] = true
		}
	}
	periods := make([]string, 0, len(dates))
	for period := range dates {
		periods = append(periods, period)
	}
	sort.Strings(periods)
	inputs := make([]DailyInput, 0, len(periods))
	for _, period := range periods {
		sources, err := engine.dailySourcesWithOwned(workspaceID, period, "attested-captures", owned)
		if err != nil {
			return nil, err
		}
		if len(sources) > 0 {
			inputs = append(inputs, DailyInput{Period: period, Fingerprint: engine.sourceFingerprint("daily", sources)})
		}
	}
	after, err := engine.readVisibility(workspaceID)
	if err != nil {
		return nil, err
	}
	if before.Revision != after.Revision {
		return nil, ErrCaptureConflict
	}
	return inputs, nil
}

// PendingDailyInput selects at most one day from a witnessed input cohort. It
// has no lookback cutoff: a committed capture survives arbitrarily long client
// downtime while the existing capture bound limits work. Outputs only select
// work; they never change the input grant's identity.
func (engine *Engine) PendingDailyInput(workspaceID string, inputs []DailyInput) (*DailyInput, error) {
	if err := engine.validateRuntime(workspaceID, false); err != nil {
		return nil, err
	}
	manifest, _, err := engine.latestManifest(workspaceID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	missing := errors.Is(err, os.ErrNotExist)
	for _, input := range inputs {
		if missing {
			selected := input
			return &selected, nil
		}
		artifact, _, err := engine.readArtifactFromManifest(workspaceID, manifest, "L1/"+input.Period)
		if errors.Is(err, os.ErrNotExist) {
			selected := input
			return &selected, nil
		}
		if err != nil {
			return nil, err
		}
		if err = engine.validateArtifact(artifact); err != nil {
			return nil, err
		}
		if artifact.SourceFingerprint != input.Fingerprint || artifact.SynthesizerID != engine.SynthesizerID {
			selected := input
			return &selected, nil
		}
	}
	return nil, nil
}

// ActiveArtifact reads an artifact through the canonical visibility-aware
// manifest. It does not initialize state or expose a caller-supplied path.
func (engine *Engine) ActiveArtifact(workspaceID, key string) (Artifact, string, error) {
	if err := engine.validateRuntime(workspaceID, false); err != nil {
		return Artifact{}, "", err
	}
	artifact, path, err := engine.readArtifactByKey(workspaceID, key)
	if err != nil {
		return Artifact{}, "", err
	}
	if err := engine.validateArtifact(artifact); err != nil {
		return Artifact{}, "", err
	}
	return artifact, path, nil
}

// DailyInputIdentity uses the same eligible bytes as the actual daily handler.
// It creates no directories or signing authority.
func (engine *Engine) DailyInputIdentity(workspaceID string, day time.Time) (string, error) {
	if err := engine.validateRuntime(workspaceID, false); err != nil {
		return "", err
	}
	before, err := engine.readVisibility(workspaceID)
	if err != nil {
		return "", err
	}
	sources, err := engine.dailySources(workspaceID, day.UTC().Format("2006-01-02"), "attested-captures")
	if err != nil {
		return "", err
	}
	after, err := engine.readVisibility(workspaceID)
	if err != nil {
		return "", err
	}
	if before.Revision != after.Revision {
		return "", ErrCaptureConflict
	}
	return engine.sourceFingerprint("daily", sources), nil
}
