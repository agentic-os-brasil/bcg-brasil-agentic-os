package releasepack

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

type PublishGenerationOptions struct {
	Root             string
	ReleaseDirectory string
	Registry         string
	Generation       releasecontract.Generation
	PrivateKey       ed25519.PrivateKey
	Clock            func() time.Time
}

// PublishGeneration closes immutable release and generation directories before
// promoting the same signed bytes to current. Partial pointer pairs fail closed
// in the participant reader; an identical complete retry is idempotent.
func PublishGeneration(o PublishGenerationOptions) (releaseverify.VerifiedGeneration, error) {
	if o.Root == "" || o.ReleaseDirectory == "" || o.Registry == "" || len(o.PrivateKey) != ed25519.PrivateKeySize {
		return releaseverify.VerifiedGeneration{}, errors.New("generation publisher requires explicit paths and approved signing key")
	}
	clock := o.Clock
	if clock == nil {
		clock = time.Now
	}
	if err := o.Generation.CheckTime(clock(), time.Time{}); err != nil {
		return releaseverify.VerifiedGeneration{}, err
	}
	registry, err := releaseverify.LoadAuthorityRegistry(o.Registry, clock)
	if err != nil {
		return releaseverify.VerifiedGeneration{}, err
	}
	key, ok := registry.Lookup("maestro", o.Generation.Issuer.ID, o.Generation.Issuer.KeyID)
	if !ok || !bytes.Equal(key, o.PrivateKey.Public().(ed25519.PublicKey)) {
		return releaseverify.VerifiedGeneration{}, errors.New("generation signing key is not the active approved authority")
	}
	body, err := releasecontract.CanonicalJSON(o.Generation)
	if err != nil {
		return releaseverify.VerifiedGeneration{}, err
	}
	sig := ed25519.Sign(o.PrivateKey, body)
	verified, err := releaseverify.VerifyGeneration(body, sig, registry)
	if err != nil {
		return verified, err
	}
	release, err := releaseverify.VerifyDirectory(o.ReleaseDirectory, registry)
	if err != nil {
		return verified, err
	}
	m := release.Manifest
	g := o.Generation
	if !m.Modern() || m.Server.ExecutionContract != releasecontract.SyncedSourceContract || m.Release != g.Release || m.Channel != g.Source.Channel || release.ManifestSHA256 != g.ManifestSHA256 || m.Delivery.ClientInputsSHA256 != g.ClientInputsSHA256 {
		return verified, errors.New("generation does not bind the exact source-aware release")
	}
	foundTarget := false
	for _, artifact := range m.Artifacts {
		if artifact.Kind == "mcp_server" && artifact.OS == g.Source.OS && artifact.Arch == g.Source.Arch {
			foundTarget = true
		}
	}
	if !foundTarget {
		return verified, errors.New("generation release lacks exact target server")
	}
	if err := os.MkdirAll(o.Root, 0700); err != nil {
		return verified, err
	}
	rootHandle, err := scheduler.OpenDirectory(o.Root)
	if err != nil {
		return verified, err
	}
	rootHandle.Close()
	releaseLock, err := privatelock.Acquire(filepath.Join(o.Root, ".publish.lock"))
	if err != nil {
		return verified, err
	}
	defer releaseLock()
	if current, err := releaseprovider.ReadSyncedMetadata(o.Root, g.Source, registry); err == nil {
		if current.Generation.Sequence > g.Sequence {
			return verified, errors.New("publisher generation replay refused")
		}
		if current.Generation.Sequence == g.Sequence && current.SHA256 != verified.SHA256 {
			return verified, errors.New("publisher generation equivocation refused")
		}
	} else if _, statErr := os.Lstat(filepath.Join(o.Root, releaseprovider.CurrentGenerationName)); !errors.Is(statErr, os.ErrNotExist) {
		return verified, fmt.Errorf("existing current pointer is unavailable: %w", err)
	}
	generationParent := filepath.Join(o.Root, "generations")
	if err := os.MkdirAll(generationParent, 0700); err != nil {
		return verified, err
	}
	entries, err := os.ReadDir(generationParent)
	if err != nil {
		return verified, err
	}
	prefix := fmt.Sprintf("%d-", g.Sequence)
	generationName := g.GenerationDirectory(verified.SHA256)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) && entry.Name() != generationName {
			return verified, errors.New("immutable generation sequence already has another body")
		}
	}
	releaseDestination := releaseprovider.SyncedReleaseDirectory(o.Root, g)
	if err := publishImmutableRelease(o.ReleaseDirectory, releaseDestination, release, registry); err != nil {
		return verified, err
	}
	generationDirectory := filepath.Join(generationParent, generationName)
	if err := publishImmutableGeneration(generationDirectory, body, sig); err != nil {
		return verified, err
	}
	if err := promoteGenerationFile(o.Root, releaseprovider.CurrentGenerationSignatureName, sig); err != nil {
		return verified, err
	}
	if err := promoteGenerationFile(o.Root, releaseprovider.CurrentGenerationName, body); err != nil {
		return verified, err
	}
	return releaseprovider.ReadSyncedMetadata(o.Root, g.Source, registry)
}

func publishImmutableRelease(source, destination string, release releaseverify.VerifiedRelease, registry releaseverify.KeyRegistry) error {
	if _, err := os.Lstat(destination); err == nil {
		existing, err := releaseverify.VerifyDirectory(destination, registry)
		if err != nil {
			return err
		}
		if existing.ManifestSHA256 != release.ManifestSHA256 {
			return errors.New("immutable release version already has different bytes")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	parent, err := scheduler.OpenDirectory(filepath.Dir(destination))
	if err != nil {
		return err
	}
	parent.Close()
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".generation-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		body, _, err := readSeedInput(filepath.Join(source, entry.Name()), 1<<30)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(stage, entry.Name()), body, 0600); err != nil {
			return err
		}
	}
	copy, err := releaseverify.VerifyDirectory(stage, registry)
	if err != nil {
		return err
	}
	if copy.ManifestSHA256 != release.ManifestSHA256 {
		return errors.New("publisher release changed while copying")
	}
	return os.Rename(stage, destination)
}

func publishImmutableGeneration(destination string, body, sig []byte) error {
	if _, err := os.Lstat(destination); err == nil {
		for name, want := range map[string][]byte{releaseprovider.GenerationName: body, releaseprovider.GenerationSignatureName: sig} {
			f, err := scheduler.OpenRegularFile(filepath.Join(destination, name))
			if err != nil {
				return err
			}
			got, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
			f.Close()
			if err != nil {
				return err
			}
			if !bytes.Equal(got, want) {
				return errors.New("immutable generation retry differs")
			}
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".generation-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = os.WriteFile(filepath.Join(stage, releaseprovider.GenerationName), body, 0600); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(stage, releaseprovider.GenerationSignatureName), sig, 0600); err != nil {
		return err
	}
	return os.Rename(stage, destination)
}

func promoteGenerationFile(root, name string, body []byte) error {
	f, err := os.CreateTemp(root, ".current-generation-")
	if err != nil {
		return err
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	if _, err = f.Write(body); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporary, filepath.Join(root, name))
}
