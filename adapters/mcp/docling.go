package product

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

// DoclingRuntime binds both availability and use to this admitted owner's
// current authenticated source. Ordinary uninstalled packets retain their
// original verifier and cannot acquire installed placement authority.
func (b *Bound) DoclingRuntime() *docling.Runtime { return docling.New(b.doclingOptions()) }
func (b *Bound) doclingOptions() docling.Options {
	return b.doclingOptionsWithReader(b.readInstalledSourceState)
}

func (b *Bound) doclingOptionsWithReader(read sourceStateReader) docling.Options {
	o := docling.Options{DataRoot: filepath.Join(b.home, "state"), AuthorityRegistryPath: b.options.AuthorityRegistryPath, AuthorityRegistrySHA256: b.options.AuthorityRegistrySHA256, CLIVersion: b.options.CLIVersion}
	owning, err := b.sourceContextWithReader(read)
	if err != nil {
		// A failed owning observation cannot acquire authority by being retried
		// later inside this same runtime. A fresh request may construct a new one.
		o.ResolveRequirement = func(context.Context) (docling.Requirement, error) { return docling.Requirement{}, err }
		o.GuardSelection = func(func() error) error { return err }
	} else if owning.required {
		o.ResolveRequirement = func(context.Context) (docling.Requirement, error) { return b.currentDoclingRequirement() }
		o.GuardSelection = func(publish func() error) error { return b.guardQualifiedPublication(publish) }
	} else {
		// A retained schema4 activation already supplies an authenticated Current
		// requirement, independently of later source enrollment. Raw/uninstalled
		// calls without that owning state retain their original verifier.
		if state := owning.state; state != nil && state.SchemaVersion == 4 && state.Source == nil {
			o.ResolveRequirement = func(context.Context) (docling.Requirement, error) { return b.currentDoclingRequirement() }
			o.GuardSelection = func(publish func() error) error {
				return installtx.GuardActivatedCurrent(b.options.ProductRoot, filepath.Join(b.options.UserHome, ".maestro-install"), state.Binding, func() error {
					if err := b.validate(); err != nil {
						return err
					}
					return b.withExperimentalSeedFence(publish)
				})
			}
		}
	}
	return o
}
func (b *Bound) currentDoclingRequirement() (docling.Requirement, error) {
	if err := b.validateInstalledSource(); err != nil {
		return docling.Requirement{}, err
	}
	state, err := b.readInstalledSourceState(filepath.Join(b.options.UserHome, ".maestro-install"), b.options.ProductRoot)
	if err != nil {
		return docling.Requirement{}, err
	}
	if state.Source == nil {
		state, err = b.validatedActivatedState(state.Binding)
		if err != nil {
			return docling.Requirement{}, err
		}
		if b.options.GuardProcess {
			identity, e := processidentity.Current()
			if e != nil {
				return docling.Requirement{}, e
			}
			name := "maestro-mcp"
			if runtime.GOOS == "windows" {
				name += ".exe"
			}
			if identity.Path != filepath.Join(b.options.ProductRoot, "bin", name) || identity.SHA256 != state.Server.SHA256 || b.options.CLIVersion != state.Server.Version {
				return docling.Requirement{}, errors.New("initial Current consumer identity changed")
			}
		}
	}
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(b.options.AuthorityRegistryPath, b.options.AuthorityRegistrySHA256, time.Now)
	if err != nil {
		return docling.Requirement{}, err
	}
	signature, err := base64.StdEncoding.DecodeString(state.Binding.ManifestSignature)
	if err != nil {
		return docling.Requirement{}, err
	}
	manifest, _, err := releaseverify.VerifyManifest([]byte(state.Binding.ManifestJSON), signature, registry)
	if err != nil {
		return docling.Requirement{}, err
	}
	manifestSum := sha256.Sum256([]byte(state.Binding.ManifestJSON))
	if hex.EncodeToString(manifestSum[:]) != state.Binding.ManifestSHA256 || manifest.Release != state.Release || manifest.Channel != state.Channel || manifest.Bundle.Version != state.BundleVersion || manifest.Delivery.ClientInputsSHA256 != state.Binding.ClientInputsSHA256 {
		return docling.Requirement{}, errors.New("current signed release identity differs")
	}
	var serverMatches, bundleMatches bool
	for _, a := range manifest.Artifacts {
		if a.Kind == "mcp_server" && a.OS == state.TargetOS && a.Arch == state.TargetArch {
			serverMatches = a.SHA256 == state.Server.SHA256 && a.Size == state.Server.Size
		}
		if a.Kind == "bundle" {
			bundleMatches = a.SHA256 == state.Binding.BundleSHA256 && a.Size == state.Binding.BundleSize
		}
	}
	if !serverMatches || !bundleMatches {
		return docling.Requirement{}, errors.New("current signed component identity differs")
	}
	required, err := docling.RequirementForRelease(manifest, state.Binding.ManifestSHA256)
	if err != nil || required.ServerVersion != state.Server.Version {
		return docling.Requirement{}, errors.New("current converter source/server differs")
	}
	if err := b.fenceExperimentalState(state); err != nil {
		return docling.Requirement{}, err
	}
	return required, nil
}

// RuntimeUpdateReview is a local CLI review, never an MCP mutation. Paths and
// reusable confirmation remain in the participant's foreground manager.
type RuntimeUpdateReview struct {
	SchemaVersion    int                    `json:"schema_version"`
	State            string                 `json:"state"`
	Requirement      docling.Requirement    `json:"requirement"`
	ServerSHA256     string                 `json:"server_sha256"`
	StateSHA256      string                 `json:"installation_state_sha256"`
	ReleaseDirectory string                 `json:"release_directory,omitempty"`
	Destination      string                 `json:"destination"`
	Facts            docling.PlacementFacts `json:"facts"`
	Availability     docling.Availability   `json:"availability"`
	Effects          []string               `json:"effects"`
	Digest           string                 `json:"review_digest"`
}

// installedRuntimeBound admits only the actual installed current consumer.
// The source-free branch is the original first-setup activation, before source
// enrollment. It requires that activation's retained signed immutable binding;
// it cannot replace source-aware validation after enrollment.
func installedRuntimeBound(o Options) (*Bound, installtx.State, error) {
	state, err := (&Bound{options: o}).readInstalledSourceState(filepath.Join(o.UserHome, ".maestro-install"), o.ProductRoot)
	if err != nil {
		return nil, state, err
	}
	if state.Profile != releasecontract.ModernProfile || state.SchemaVersion != 4 || state.TargetOS != runtime.GOOS || state.TargetArch != runtime.GOARCH || o.InstalledManifestSHA256 == "" || o.InstalledClientInputsSHA256 == "" || state.Binding.ManifestSHA256 != o.InstalledManifestSHA256 || state.Binding.ClientInputsSHA256 != o.InstalledClientInputsSHA256 {
		return nil, state, errors.New("exact authenticated current installed runtime selection required")
	}
	identity, err := processidentity.Current()
	if err != nil {
		return nil, state, err
	}
	name := "maestro-mcp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if identity.Path != filepath.Join(o.ProductRoot, "bin", name) || identity.SHA256 != state.Server.SHA256 || o.CLIVersion != state.Server.Version {
		return nil, state, errors.New("runtime update must execute the actual current installed server")
	}
	if err = installtx.CheckActivatedServer(o.ProductRoot, state); err != nil {
		return nil, state, err
	}
	boundOptions := o
	if state.Source == nil {
		boundOptions.InstalledManifestSHA256 = ""
		boundOptions.InstalledClientInputsSHA256 = ""
	}
	b, err := Open(boundOptions)
	if err != nil {
		return nil, state, err
	}
	required, err := b.currentDoclingRequirement()
	if err != nil {
		return nil, state, err
	}
	if required.ManifestSHA256 != state.Binding.ManifestSHA256 || required.ServerVersion != state.Server.Version {
		return nil, state, errors.New("current signed runtime binding differs")
	}
	return b, state, nil
}

func ReviewInstalledRuntime(ctx context.Context, o Options, releaseDirectory string) (RuntimeUpdateReview, error) {
	b, state, err := installedRuntimeBound(o)
	if err != nil {
		return RuntimeUpdateReview{}, err
	}
	required, err := b.currentDoclingRequirement()
	if err != nil {
		return RuntimeUpdateReview{}, err
	}
	review := RuntimeUpdateReview{SchemaVersion: 1, State: "reviewable", Requirement: required, ServerSHA256: state.Server.SHA256, StateSHA256: installtx.StateDigest(state), Destination: filepath.Join(b.home, "state", "ingestion", "docling"), Availability: b.DoclingRuntime().Status(ctx), Effects: []string{}}
	if required.PackSHA256 == "" {
		review.State = "not_declared"
	} else if review.Availability.State != "ready" {
		retained := docling.RequiredRetainedStatus(ctx, b.doclingOptions())
		if retained.State == "ready" || retained.State == "unqualified" {
			review.Facts, err = docling.RequiredRetainedFacts(ctx, b.doclingOptions())
			if err != nil {
				return review, err
			}
			review.Effects = []string{"reverify_retained_authenticated_converter", "qualify_current_consumer_if_required", "select_current_authenticated_metadata"}
		} else {
			if releaseDirectory == "" && state.Source != nil {
				releaseDirectory = filepath.Join(o.UserHome, ".maestro-install", "updates", "synced-"+state.Source.GenerationSHA256)
			}
			if releaseDirectory == "" || !filepath.IsAbs(releaseDirectory) || filepath.Clean(releaseDirectory) != releaseDirectory {
				return review, errors.New("current authenticated private release snapshot required; retry manage update/reconcile")
			}
			registry, e := releaseverify.LoadPinnedAuthorityRegistry(o.AuthorityRegistryPath, o.AuthorityRegistrySHA256, time.Now)
			if e != nil {
				return review, e
			}
			v, e := releaseverify.VerifyDirectory(releaseDirectory, registry)
			if e != nil {
				return review, e
			}
			candidate, e := docling.RequirementForRelease(v.Manifest, v.ManifestSHA256)
			if e != nil || candidate != required {
				return review, errors.New("runtime release is not the authenticated current requirement")
			}
			review.Facts, e = docling.FactsForRelease(v)
			if e != nil {
				return review, e
			}
			review.ReleaseDirectory = releaseDirectory
			review.Effects = []string{"place_authenticated_docling_pack", "preserve_prior_converter_payloads_and_metadata", "qualify_current_consumer_if_required"}
			review.Effects = append(review.Effects, "select_current_authenticated_metadata")
		}
	}
	if review.Facts.Requirement.ManifestSHA256 == "" {
		review.Facts.Requirement = required
	}
	body, err := releasecontract.CanonicalJSON(review)
	if err != nil {
		return review, err
	}
	sum := sha256.Sum256(body)
	review.Digest = hex.EncodeToString(sum[:])
	if err := b.validateExperimentalSeed(); err != nil {
		return review, err
	}
	return review, nil
}

func ApplyInstalledRuntime(ctx context.Context, o Options, releaseDirectory, digest string, confirmed bool) (docling.Availability, error) {
	pending := docling.Availability{State: "unavailable", Reason: "runtime_pending"}
	if !confirmed || len(digest) != 64 {
		return pending, errors.New("exact runtime review and explicit confirmation required")
	}
	review, err := ReviewInstalledRuntime(ctx, o, releaseDirectory)
	if err != nil {
		return pending, err
	}
	if review.Digest != digest {
		return pending, errors.New("runtime review changed; review exact effects again")
	}
	b, state, err := installedRuntimeBound(o)
	if err != nil {
		return pending, err
	}
	if review.State == "not_declared" || len(review.Effects) == 0 {
		status := b.DoclingRuntime().Status(ctx)
		if err := b.validateExperimentalSeed(); err != nil {
			return pending, err
		}
		return status, nil
	}
	options := b.doclingOptions()
	if review.ReleaseDirectory == "" {
		err = docling.RequalifyRequired(ctx, options)
	} else {
		registry, e := releaseverify.LoadPinnedAuthorityRegistry(o.AuthorityRegistryPath, o.AuthorityRegistrySHA256, time.Now)
		if e != nil {
			return pending, e
		}
		v, e := releaseverify.VerifyDirectory(review.ReleaseDirectory, registry)
		if e != nil {
			return pending, e
		}
		if v.ManifestSHA256 != review.Requirement.ManifestSHA256 {
			return pending, errors.New("runtime release changed after review")
		}
		err = docling.PlaceVerified(ctx, options, v)
	}
	if err != nil {
		return pending, fmt.Errorf("source-updated/runtime-pending; prior converter bytes preserved; retry manage reconcile: %w", err)
	}
	_, _, e := installedRuntimeBound(o)
	if e != nil {
		return pending, e
	}
	latest, e := b.readInstalledSourceState(filepath.Join(o.UserHome, ".maestro-install"), o.ProductRoot)
	if e != nil || latest.Binding.ManifestSHA256 != state.Binding.ManifestSHA256 {
		return pending, errors.New("current changed during converter placement; retry manage reconcile")
	}
	result := b.DoclingRuntime().Status(ctx)
	if result.State != "ready" {
		return result, errors.New("source-updated/runtime-pending; retry manage reconcile")
	}
	if err := b.validateExperimentalSeed(); err != nil {
		return pending, err
	}
	return result, nil
}
