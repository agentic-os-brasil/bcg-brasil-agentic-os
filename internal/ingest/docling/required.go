package docling

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// RequirementForRelease narrows an already authenticated manifest to this host.
// Callers must authenticate current installation custody independently.
func RequirementForRelease(manifest releasecontract.Manifest, manifestSHA string) (Requirement, error) {
	r := Requirement{ManifestSHA256: manifestSHA, ServerVersion: manifest.Server.Version}
	if !manifest.Modern() || !shaPattern.MatchString(manifestSHA) {
		return r, errors.New("modern authenticated converter requirement required")
	}
	for _, a := range manifest.Artifacts {
		if a.Kind == "runtime_pack" && a.OS == runtime.GOOS && a.Arch == runtime.GOARCH {
			if a.Adapter != "docling" || r.PackSHA256 != "" {
				return r, errors.New("ambiguous current converter requirement")
			}
			r.PackSHA256 = a.SHA256
		}
	}
	return r, nil
}

// RequalifyRequired uses only retained authenticated metadata for the current
// release. It cannot select a pack from an argument or grant installation trust.
func RequalifyRequired(ctx context.Context, o Options) error {
	if o.ResolveRequirement == nil {
		return errors.New("authenticated current requirement required")
	}
	required, err := New(o).currentRequirement(ctx)
	if err != nil {
		return err
	}
	if required.PackSHA256 == "" {
		return errors.New("current converter not declared")
	}
	if _, err = metadataBase(o); err != nil {
		return err
	}
	o, err = retainedOptions(ctx, o, required)
	if err != nil {
		return err
	}
	if o.legacyMetadata {
		if err = copyLegacyMetadata(o, required); err != nil {
			return err
		}
		o.legacyMetadata = false
	}
	o.metadataGeneration = required.ManifestSHA256
	if _, reason := New(o).verify(ctx); reason != "" {
		return fmt.Errorf("retained current converter unavailable: %s", reason)
	}
	if err = qualifyPlaced(ctx, o); err != nil {
		return err
	}
	if status := New(o).Status(ctx); status.State != "ready" {
		return fmt.Errorf("retained current converter unavailable: %s", status.Reason)
	}
	return selectMetadata(ctx, o, required)
}

// RequiredRetainedStatus is read-only and never falls back to legacy metadata.
func RequiredRetainedStatus(ctx context.Context, o Options) Availability {
	required, err := New(o).currentRequirement(ctx)
	if err != nil || o.ResolveRequirement == nil {
		return Availability{State: "unavailable", Reason: "current_release_unavailable"}
	}
	o, err = retainedOptions(ctx, o, required)
	if err != nil {
		return Availability{State: "unavailable", Reason: "retained_pack_unavailable"}
	}
	return New(o).Status(ctx)
}

// PlacementFacts are exact input/license identities for local human review.
// Complete signature, inventory, confinement and qualification checks remain
// mandatory in PlaceVerified and on every subsequent use.
type PlacementFacts struct {
	Requirement            Requirement `json:"requirement"`
	Profile                string      `json:"profile"`
	LockSHA256             string      `json:"lock_sha256"`
	LicenseInventorySHA256 string      `json:"license_inventory_sha256"`
	Inputs                 int         `json:"inputs"`
}

func FactsForRelease(v releaseverify.VerifiedRelease) (PlacementFacts, error) {
	required, err := RequirementForRelease(v.Manifest, v.ManifestSHA256)
	if err != nil {
		return PlacementFacts{}, err
	}
	var selected releasecontract.Artifact
	for _, a := range v.Manifest.Artifacts {
		if a.Kind == "runtime_pack" && a.OS == runtime.GOOS && a.Arch == runtime.GOARCH && a.SHA256 == required.PackSHA256 {
			selected = a
		}
	}
	if selected.Name == "" {
		return PlacementFacts{Requirement: required}, nil
	}
	body, _, err := readFile(filepath.Join(v.Directory, selected.Name), selected.Size)
	if err != nil || digest(body) != selected.SHA256 {
		return PlacementFacts{}, errors.New("reviewed converter archive changed")
	}
	entries, err := archiveEntries(body)
	if err != nil {
		return PlacementFacts{}, err
	}
	var pack packManifest
	if decode(entries["pack.json"].body, &pack) != nil {
		return PlacementFacts{}, errors.New("reviewed converter metadata invalid")
	}
	target, err := inputsForLock(runtime.GOOS, runtime.GOARCH, pack.LockSHA256)
	if err != nil {
		return PlacementFacts{}, err
	}
	if !bytes.Equal(entries["parser-licenses.json"].body, target.notices) || !bytes.Equal(entries["pylock.toml"].body, target.lock) || !bytes.Equal(entries["inputs.json"].body, target.inputs) || !bytes.Equal(entries["bridge.py"].body, target.bridge) {
		return PlacementFacts{}, errors.New("reviewed converter input/license identities differ")
	}
	return PlacementFacts{required, target.profile, target.lockSHA, digest(target.notices), target.count}, nil
}

// A legacy candidate is reusable only if its complete signature/inventory
// independently matches the current requirement. A broken present generation
// or selector never triggers fallback.
func retainedOptions(ctx context.Context, o Options, required Requirement) (Options, error) {
	if _, err := metadataBase(o); err != nil {
		return o, err
	}
	candidate := filepath.Join(o.DataRoot, "ingestion", "docling", "metadata", required.ManifestSHA256)
	if _, err := os.Lstat(candidate); err == nil {
		o.metadataGeneration = required.ManifestSHA256
		return o, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return o, err
	}
	o.legacyMetadata = true
	if _, reason := New(o).verify(ctx); reason != "" {
		return o, fmt.Errorf("legacy current candidate unavailable: %s", reason)
	}
	return o, nil
}
func copyLegacyMetadata(o Options, required Requirement) error {
	base := filepath.Join(o.DataRoot, "ingestion", "docling")
	body, _, err := readFile(filepath.Join(base, releaseverify.ManifestName), 1<<20)
	if err != nil {
		return err
	}
	var manifest releasecontract.Manifest
	if releasecontract.DecodeStrictJSON(body, &manifest) != nil {
		return errors.New("legacy manifest changed")
	}
	var selected releasecontract.Artifact
	for _, a := range manifest.Artifacts {
		if a.Kind == "runtime_pack" && a.OS == runtime.GOOS && a.Arch == runtime.GOARCH && a.SHA256 == required.PackSHA256 {
			selected = a
		}
	}
	if selected.Name == "" {
		return errors.New("legacy current artifact unavailable")
	}
	destination := filepath.Join(base, "metadata", required.ManifestSHA256)
	if err = scheduler.EnsurePrivateDirectory(destination); err != nil {
		return err
	}
	for _, name := range []string{releaseverify.ManifestName, releaseverify.ManifestSignatureName, selected.Name, selected.SignatureRef} {
		limit := int64(1 << 20)
		if name == selected.Name {
			limit = selected.Size
		}
		content, _, e := readFile(filepath.Join(base, name), limit)
		if e != nil {
			return e
		}
		if e = placeExact(destination, name, content); e != nil {
			return e
		}
	}
	return nil
}

func RequiredRetainedFacts(ctx context.Context, o Options) (PlacementFacts, error) {
	required, err := New(o).currentRequirement(ctx)
	if err != nil || o.ResolveRequirement == nil {
		return PlacementFacts{}, errors.New("current authenticated requirement required")
	}
	o, err = retainedOptions(ctx, o, required)
	if err != nil {
		return PlacementFacts{}, err
	}
	pack, reason := New(o).verify(ctx)
	if reason != "" {
		return PlacementFacts{}, fmt.Errorf("retained current converter unavailable: %s", reason)
	}
	return PlacementFacts{required, pack.target.profile, pack.target.lockSHA, digest(pack.target.notices), pack.target.count}, nil
}
