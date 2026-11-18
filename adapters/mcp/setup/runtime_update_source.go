package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// RuntimeRecoveryReview describes only the missing deterministic private
// snapshot of an already accepted Current source. It does not stage bytes,
// refresh source custody, activate anything, or grant converter execution.
type RuntimeRecoveryReview struct {
	SchemaVersion      int                         `json:"schema_version"`
	Current            docling.Requirement         `json:"current"`
	ClientInputsSHA256 string                      `json:"client_inputs_sha256"`
	ServerSHA256       string                      `json:"server_sha256"`
	StateSHA256        string                      `json:"installation_state_sha256"`
	Source             releasecontract.SourceTuple `json:"source"`
	GenerationSHA256   string                      `json:"generation_sha256"`
	RegistrySHA256     string                      `json:"registry_sha256"`
	SourceRoot         string                      `json:"source_root"`
	Destination        string                      `json:"destination"`
	Artifacts          []releasecontract.Artifact  `json:"authenticated_artifacts"`
	Effects            []string                    `json:"effects"`
	Digest             string                      `json:"review_digest"`
}

type RuntimeRecoveryResult struct {
	State          string `json:"state"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Destination    string `json:"destination"`
}

func ReviewRuntimeRecoverySource(ctx context.Context, o installer.ClientOptions) (RuntimeRecoveryReview, error) {
	launch, err := runtimeUpdateLaunch(ctx, o, "runtime-review")
	if err != nil {
		return RuntimeRecoveryReview{}, err
	}
	state := launch.State
	if state.Source == nil {
		return RuntimeRecoveryReview{}, errors.New("enrolled Current source is required for first-setup snapshot recovery")
	}
	if err = scheduler.ValidatePrivateDirectory(o.DataRoot); err != nil {
		return RuntimeRecoveryReview{}, err
	}
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(filepath.Join(o.ManagedRoot, "trust", "release-authority-registry.json"), o.AuthorityRegistrySHA256, time.Now)
	if err != nil {
		return RuntimeRecoveryReview{}, err
	}
	generation, err := releaseprovider.ReadSyncedMetadata(state.Source.Locator, state.Source.Tuple, registry)
	if err != nil {
		return RuntimeRecoveryReview{}, err
	}
	if generation.SHA256 != state.Source.GenerationSHA256 || generation.Generation.ManifestSHA256 != state.Binding.ManifestSHA256 || generation.Generation.ClientInputsSHA256 != state.Binding.ClientInputsSHA256 || generation.Generation.Release != state.Release {
		return RuntimeRecoveryReview{}, errors.New("recovery source no longer names exact accepted Current; use the existing source update/review owner")
	}
	if err = generation.Generation.CheckTime(time.Now(), state.Source.ObservedAt); err != nil {
		return RuntimeRecoveryReview{}, err
	}
	manifest, err := releaseprovider.ReadSyncedManifest(state.Source.Locator, generation, registry)
	if err != nil {
		return RuntimeRecoveryReview{}, err
	}
	required, err := docling.RequirementForRelease(manifest, state.Binding.ManifestSHA256)
	if err != nil {
		return RuntimeRecoveryReview{}, err
	}
	if required.PackSHA256 == "" || required.ServerVersion != state.Server.Version {
		return RuntimeRecoveryReview{}, errors.New("Current source has no matching converter recovery requirement")
	}
	destination := filepath.Join(o.DataRoot, "updates", "synced-"+generation.SHA256)
	if _, err = os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		return RuntimeRecoveryReview{}, errors.New("existing runtime snapshot or unknown bytes preserved; automatic restaging is unavailable")
	}
	review := RuntimeRecoveryReview{SchemaVersion: 1, Current: required, ClientInputsSHA256: state.Binding.ClientInputsSHA256, ServerSHA256: state.Server.SHA256, StateSHA256: installtx.StateDigest(state), Source: state.Source.Tuple, GenerationSHA256: generation.SHA256, RegistrySHA256: o.AuthorityRegistrySHA256, SourceRoot: state.Source.Locator, Destination: destination, Artifacts: manifest.Artifacts, Effects: []string{"stage_current_authenticated_release_snapshot", "preserve_failed_staging_for_inspection", "leave_source_custody_activation_and_owner_data_unchanged"}}
	if o.Request.Reconcile {
		review.Effects = append(review.Effects, "reclaim_only_dead_installation_lock_before_reviewed_snapshot_staging")
	}
	body, err := releasecontract.CanonicalJSON(review)
	if err != nil {
		return review, err
	}
	sum := sha256.Sum256(body)
	review.Digest = hex.EncodeToString(sum[:])
	return review, nil
}

func ApplyRuntimeRecoverySource(ctx context.Context, o installer.ClientOptions, digest string, confirmed bool) (RuntimeRecoveryResult, error) {
	pending := RuntimeRecoveryResult{State: "runtime_snapshot_pending"}
	if !confirmed || len(digest) != 64 {
		return pending, errors.New("exact snapshot staging review and explicit confirmation required")
	}
	review, err := ReviewRuntimeRecoverySource(ctx, o)
	if err != nil {
		return pending, err
	}
	if review.Digest != digest {
		return pending, errors.New("Current source snapshot review changed; review exact effects again")
	}
	expected := installtx.ReleaseBinding{ManifestSHA256: review.Current.ManifestSHA256, ClientInputsSHA256: review.ClientInputsSHA256}
	// SnapshotSyncedRelease owns exact named copying and source rechecks through
	// its final publication. Hold the existing installation guard while composing
	// it: this does not add a second transaction or mutate source observation.
	guard := installtx.GuardQualifiedCurrent
	if o.Request.Reconcile {
		guard = installtx.GuardQualifiedReconciliationCurrent
	}
	err = guard(o.ManagedRoot, o.DataRoot, expected, time.Now(), func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		fresh, err := ReviewRuntimeRecoverySource(ctx, o)
		if err != nil {
			return err
		}
		if fresh.Digest != digest {
			return errors.New("Current/source changed before reviewed snapshot staging")
		}
		registry, err := releaseverify.LoadPinnedAuthorityRegistry(filepath.Join(o.ManagedRoot, "trust", "release-authority-registry.json"), review.RegistrySHA256, time.Now)
		if err != nil {
			return err
		}
		generation, err := releaseprovider.ReadSyncedMetadata(review.SourceRoot, review.Source, registry)
		if err != nil {
			return err
		}
		if generation.SHA256 != review.GenerationSHA256 {
			return errors.New("accepted Current source changed before staging")
		}
		verified, err := releaseprovider.SnapshotSyncedRelease(review.SourceRoot, review.Destination, generation, registry)
		if err != nil {
			return err
		}
		if verified.ManifestSHA256 != review.Current.ManifestSHA256 || verified.Manifest.Delivery.ClientInputsSHA256 != review.ClientInputsSHA256 {
			return errors.New("staged snapshot differs from exact reviewed Current; staged bytes preserved")
		}
		return nil
	})
	if err != nil {
		return pending, fmt.Errorf("source-updated/runtime-pending; snapshot staging failed or changed; existing/staged bytes preserved; retry manage reconcile after inspection: %w", err)
	}
	return RuntimeRecoveryResult{State: "current_snapshot_staged_runtime_pending", ManifestSHA256: review.Current.ManifestSHA256, Destination: review.Destination}, nil
}
