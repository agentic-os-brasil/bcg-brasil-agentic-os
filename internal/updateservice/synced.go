package updateservice

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateplan"
)

type SyncedOptions struct {
	UserHome, ManagedRoot, DataRoot, SourceRoot string
	ExpectedSource                              releasecontract.SourceTuple
	Registry                                    releaseverify.KeyRegistry
	Clock                                       func() time.Time
	// Trusted in-process publication composition only. The callback wraps the
	// source observation while a maintenance live grant remains current.
	BeforeObserve func(func() error) error
	BeforeEffect  func(func() error) error
	AfterObserve  func(installtx.State) error
}

// SyncedReview is private trusted-form data. Ordinary MCP results must project
// its bounded outcome rather than expose paths, keys or reusable confirmation.
type SyncedReview struct {
	Generation         releasecontract.Generation  `json:"generation"`
	ManagedRoot        string                      `json:"managed_root"`
	DataRoot           string                      `json:"data_root"`
	Destinations       []string                    `json:"destinations"`
	Digest             string                      `json:"digest"`
	CurrentSHA256      string                      `json:"current_sha256"`
	SourceRoot         string                      `json:"source_root"`
	Source             releasecontract.SourceTuple `json:"source"`
	GenerationSHA256   string                      `json:"generation_sha256"`
	Sequence           uint64                      `json:"sequence"`
	CandidateRelease   string                      `json:"candidate_release"`
	ManifestSHA256     string                      `json:"manifest_sha256"`
	ClientInputsSHA256 string                      `json:"client_inputs_sha256"`
	ExpiresAt          string                      `json:"expires_at"`
	Effects            []string                    `json:"effects"`
}

func ReviewSynced(o SyncedOptions) (SyncedReview, error) {
	if o.Registry == nil || o.ManagedRoot == "" || o.DataRoot == "" || o.SourceRoot == "" || o.ExpectedSource.Validate() != nil {
		return SyncedReview{}, errors.New("fixed source, exact target and independently admitted registry required")
	}
	if !filepath.IsAbs(o.SourceRoot) || filepath.Clean(o.SourceRoot) != o.SourceRoot {
		return SyncedReview{}, errors.New("source locator must be canonical absolute local folder")
	}
	current, err := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
	if err != nil {
		return SyncedReview{}, err
	}
	if current.SchemaVersion != 4 || current.Server.ExecutionContract != releasecontract.SyncedSourceContract {
		return SyncedReview{}, errors.New("source enrollment upgrade/migration unavailable: authenticate and install a compatible source-aware release")
	}
	if o.DataRoot != filepath.Join(o.UserHome, ".maestro-install") || current.TargetOS != o.ExpectedSource.OS || current.TargetArch != o.ExpectedSource.Arch || current.Channel != o.ExpectedSource.Channel {
		return SyncedReview{}, errors.New("source target differs from fixed installed owner")
	}
	if current.Source != nil && (current.Source.Tuple != o.ExpectedSource || current.Source.Locator != o.SourceRoot) {
		return SyncedReview{}, errors.New("source review cannot replace enrolled source")
	}
	g, err := releaseprovider.ReadSyncedMetadata(o.SourceRoot, o.ExpectedSource, o.Registry)
	if err != nil {
		return SyncedReview{}, err
	}
	r := SyncedReview{Generation: g.Generation, CurrentSHA256: installtx.StateDigest(current), SourceRoot: o.SourceRoot, Source: o.ExpectedSource, GenerationSHA256: g.SHA256, Sequence: g.Generation.Sequence, CandidateRelease: g.Generation.Release, ManifestSHA256: g.Generation.ManifestSHA256, ClientInputsSHA256: g.Generation.ClientInputsSHA256, ExpiresAt: g.Generation.ExpiresAt, Effects: []string{"installation_source_custody", "verified_candidate_snapshot", "local_update_plan"}}
	r.ManagedRoot = o.ManagedRoot
	r.DataRoot = o.DataRoot
	r.Destinations = []string{filepath.Join(o.DataRoot, "config", "install-state.json"), filepath.Join(o.DataRoot, "updates", "synced-"+g.SHA256), filepath.Join(o.DataRoot, "config", "pending-update.json"), filepath.Join(o.DataRoot, "updates")}
	r.Digest = reviewDigest(r)
	return r, nil
}

func reviewDigest(r SyncedReview) string {
	r.Digest = ""
	b, _ := json.Marshal(r)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum)
}

// CheckSynced executes only the exact just-reviewed source observation. A valid
// higher expired record is committed before temporal or payload readiness can
// fail. The returned Current exposes that durable denial honestly on failure.
func CheckSynced(o SyncedOptions, confirmedDigest string) (CheckResult, error) {
	r, err := ReviewSynced(o)
	if err != nil {
		return CheckResult{}, err
	}
	if len(confirmedDigest) != 64 || r.Digest != confirmedDigest {
		return CheckResult{}, errors.New("exact trusted source review confirmation required")
	}
	g, err := releaseprovider.ReadSyncedMetadata(o.SourceRoot, o.ExpectedSource, o.Registry)
	if err != nil {
		return CheckResult{}, err
	}
	if g.SHA256 != r.GenerationSHA256 {
		return CheckResult{}, errors.New("source metadata changed after trusted review")
	}
	now := time.Now().UTC()
	if o.Clock != nil {
		now = o.Clock().UTC()
	}
	var current installtx.State
	observe := func() error {
		var err error
		current, err = installtx.ObserveSourceWithRegistry(o.DataRoot, o.ManagedRoot, o.SourceRoot, g, r.CurrentSHA256, now, o.Registry)
		if err != nil {
			return err
		}
		if o.AfterObserve != nil {
			return o.AfterObserve(current)
		}
		return nil
	}
	if o.BeforeEffect != nil {
		err = o.BeforeEffect(observe)
	} else if o.BeforeObserve != nil {
		err = o.BeforeObserve(observe)
	} else {
		err = observe()
	}
	if err != nil {
		return CheckResult{Current: current, Generation: g}, err
	}
	result := CheckResult{Current: current, Generation: g}
	if current.Source.RecallCapacityExhausted {
		return result, errors.New("source recall capacity exhausted; signed observation retained, candidate readiness unavailable")
	}
	if err := g.Generation.CheckTime(now, current.Source.ObservedAt); err != nil {
		return result, err
	}
	comparison, err := updateplan.CompareVersions(g.Generation.Release, current.Release)
	if err != nil {
		return result, err
	}
	if comparison == 0 {
		if g.Generation.ManifestSHA256 != current.Binding.ManifestSHA256 || g.Generation.ClientInputsSHA256 != current.Binding.ClientInputsSHA256 {
			return result, errors.New("source redefines an immutable installed release")
		}
		for _, r := range current.Source.Recalls {
			if r.Release == current.Release && r.ManifestSHA256 == current.Binding.ManifestSHA256 {
				return result, errors.New("current release recalled; independent installer upgrade/recovery required")
			}
		}
		return result, ErrCurrent
	}
	if comparison < 0 {
		return result, errors.New("source candidate is older than installed release; explicit compatible rollback required")
	}
	for _, recall := range current.Source.Recalls {
		if recall.Release == g.Generation.Release && recall.ManifestSHA256 == g.Generation.ManifestSHA256 {
			return result, errors.New("candidate release is recalled")
		}
	}
	destination := filepath.Join(o.DataRoot, "updates", "synced-"+g.SHA256)
	var verified releaseverify.VerifiedRelease
	snapshot := func() error {
		var err error
		if _, statErr := os.Lstat(destination); statErr == nil {
			verified, err = releaseverify.VerifyDirectory(destination, o.Registry)
		} else if errors.Is(statErr, os.ErrNotExist) {
			verified, err = releaseprovider.SnapshotSyncedRelease(o.SourceRoot, destination, g, o.Registry)
		} else {
			err = statErr
		}
		return err
	}
	if o.BeforeEffect != nil {
		err = o.BeforeEffect(snapshot)
	} else if o.BeforeObserve != nil {
		err = o.BeforeObserve(snapshot)
	} else {
		err = snapshot()
	}
	if err != nil {
		return result, err
	}
	if verified.ManifestSHA256 != g.Generation.ManifestSHA256 || verified.Manifest.Delivery.ClientInputsSHA256 != g.Generation.ClientInputsSHA256 {
		return result, errors.New("private candidate differs from authenticated generation")
	}
	plan, err := updateplan.Build(current, verified.Manifest, current.TargetOS, current.TargetArch, updateplan.SourceBinding{Provider: "synced-folder", ManifestSHA256: verified.ManifestSHA256, Modern: &current.Source.SourceBinding})
	if err != nil {
		return result, err
	}
	result.Plan = plan
	result.Verified = verified
	return result, nil
}

func modernSource(p updateplan.Plan) *installtx.SourceBinding {
	if p.SchemaVersion != 3 {
		return nil
	}
	return &p.Source
}
func validateSyncedPending(current installtx.State, pending Pending, registry releaseverify.KeyRegistry) error {
	if current.Source != nil && current.Source.RecallCapacityExhausted {
		return errors.New("source recall capacity exhausted; pending update unavailable")
	}
	if current.Source == nil || current.Source.SourceBinding != pending.Plan.Source {
		return errors.New("pending signed source changed; review again")
	}
	g, err := releaseverify.VerifyGeneration(current.Source.Body, current.Source.Signature, registry)
	if err != nil {
		return err
	}
	if g.SHA256 != pending.Plan.Source.GenerationSHA256 || g.Generation.ManifestSHA256 != pending.Plan.ManifestSHA256 || g.Generation.ClientInputsSHA256 != pending.Plan.ClientInputsSHA256 {
		return errors.New("pending signed generation identity drift")
	}
	if err := g.Generation.CheckTime(time.Now(), current.Source.ObservedAt); err != nil {
		return err
	}
	for _, r := range current.Source.Recalls {
		if r.Release == pending.Plan.ToRelease && r.ManifestSHA256 == pending.Plan.ManifestSHA256 {
			return errors.New("pending candidate recalled")
		}
	}
	return nil
}
