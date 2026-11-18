package docling

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Requirement is an additional narrowing constraint from an independently
// authenticated current installation. It never replaces pack authentication.
// An empty PackSHA256 means that the current release declares no converter.
type Requirement struct {
	ManifestSHA256 string `json:"manifest_sha256"`
	PackSHA256     string `json:"pack_sha256"`
	ServerVersion  string `json:"server_version"`
}

var shaPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type currentSelection struct {
	SchemaVersion  int    `json:"schema_version"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

func decodeSelection(body []byte) (currentSelection, error) {
	var s currentSelection
	if releasecontract.DecodeStrictJSON(body, &s) != nil || s.SchemaVersion != 1 || !shaPattern.MatchString(s.ManifestSHA256) {
		return s, errors.New("converter selection is invalid; preserved")
	}
	canonical, _ := json.Marshal(s)
	if !bytes.Equal(canonical, body) {
		return s, errors.New("converter selection is noncanonical; preserved")
	}
	return s, nil
}
func metadataBase(o Options) (string, error) {
	base := filepath.Join(o.DataRoot, "ingestion", "docling")
	if o.legacyMetadata {
		return base, nil
	}
	if o.metadataGeneration != "" {
		return filepath.Join(base, "metadata", o.metadataGeneration), nil
	}
	body, err := scheduler.ReadPrivateStateFile(filepath.Join(base, "current.json"), 4096)
	if errors.Is(err, os.ErrNotExist) {
		return base, nil
	}
	if err != nil {
		return "", err
	}
	selection, err := decodeSelection(body)
	if err != nil {
		return "", err
	}
	generation := filepath.Join(base, "metadata", selection.ManifestSHA256)
	if err = scheduler.ValidatePrivateDirectory(generation); err != nil {
		return "", err
	}
	manifest, _, err := readFile(filepath.Join(generation, "release-manifest.json"), 1<<20)
	if err != nil || digest(manifest) != selection.ManifestSHA256 {
		return "", errors.New("converter selected metadata is unavailable; preserved")
	}
	signature, _, err := readFile(filepath.Join(generation, releaseverify.ManifestSignatureName), ed25519.SignatureSize)
	if err != nil {
		return "", err
	}
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(o.AuthorityRegistryPath, o.AuthorityRegistrySHA256, time.Now)
	if err != nil {
		return "", err
	}
	if _, _, err = releaseverify.VerifyManifest(manifest, signature, registry); err != nil {
		return "", errors.New("converter selected metadata is untrusted; preserved")
	}
	return generation, nil
}

func (r *Runtime) currentRequirement(ctx context.Context) (Requirement, error) {
	if r.options.ResolveRequirement == nil {
		return Requirement{}, nil
	}
	required, err := r.options.ResolveRequirement(ctx)
	if err != nil {
		return required, err
	}
	if !shaPattern.MatchString(required.ManifestSHA256) || required.PackSHA256 != "" && !shaPattern.MatchString(required.PackSHA256) {
		return required, errors.New("invalid authenticated current requirement")
	}
	if !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(required.ServerVersion) {
		err = errors.New("invalid current server version")
		return required, err
	}
	return required, nil
}

func selectMetadata(ctx context.Context, o Options, expected Requirement) error {
	manifest := expected.ManifestSHA256
	base := filepath.Join(o.DataRoot, "ingestion", "docling")
	body, _ := json.Marshal(currentSelection{1, manifest})
	guard := o.GuardSelection
	if guard == nil {
		guard = func(publish func() error) error { return publish() }
	}
	return scheduler.UpdatePrivateFileGuarded(filepath.Join(base, "current.json"), 4096, func(publish func() error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return guard(func() error {
			required, err := New(o).currentRequirement(ctx)
			if err != nil || required != expected {
				return errors.New("current converter requirement changed before selection")
			}
			status := New(o).Status(ctx)
			if status.State != "ready" || status.PackSHA256 != expected.PackSHA256 {
				return errors.New("verified converter candidate changed before selection; prior selection preserved")
			}
			return publish()
		})
	}, func(previous []byte) ([]byte, error) {
		if len(previous) > 0 {
			selectionOptions := o
			selectionOptions.metadataGeneration = ""
			selectionOptions.legacyMetadata = false
			if _, err := metadataBase(selectionOptions); err != nil {
				return nil, err
			}
			if _, err := decodeSelection(previous); err != nil {
				return nil, err
			}
		}
		return body, nil
	})
}
