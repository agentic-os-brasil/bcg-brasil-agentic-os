package releaseprovider

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const CurrentGenerationName = "current.json"
const CurrentGenerationSignatureName = "current.json.sig"
const GenerationName = "generation.json"
const GenerationSignatureName = "generation.json.sig"

// ReadSyncedMetadata is a read-only authenticated observation. It deliberately
// neither copies payload nor rejects expired authentic denial information.
func ReadSyncedMetadata(root string, expected releasecontract.SourceTuple, registry releaseverify.KeyRegistry) (releaseverify.VerifiedGeneration, error) {
	if err := expected.Validate(); err != nil {
		return releaseverify.VerifiedGeneration{}, err
	}
	body, err := readSyncedFile(filepath.Join(root, CurrentGenerationName), 1<<20)
	if err != nil {
		return releaseverify.VerifiedGeneration{}, err
	}
	sig, err := readSyncedFile(filepath.Join(root, CurrentGenerationSignatureName), ed25519.SignatureSize)
	if err != nil {
		return releaseverify.VerifiedGeneration{}, err
	}
	v, err := releaseverify.VerifyGeneration(body, sig, registry)
	if err != nil {
		return v, err
	}
	if v.Generation.Source != expected {
		return releaseverify.VerifiedGeneration{}, errors.New("signed generation source/channel/target mismatch")
	}
	dir := filepath.Join(root, "generations", v.Generation.GenerationDirectory(v.SHA256))
	immutable, err := readSyncedFile(filepath.Join(dir, GenerationName), 1<<20)
	if err != nil {
		return releaseverify.VerifiedGeneration{}, err
	}
	immutableSig, err := readSyncedFile(filepath.Join(dir, GenerationSignatureName), ed25519.SignatureSize)
	if err != nil {
		return releaseverify.VerifiedGeneration{}, err
	}
	if !bytes.Equal(body, immutable) || !bytes.Equal(sig, immutableSig) {
		return releaseverify.VerifiedGeneration{}, errors.New("current and immutable generation pair differ")
	}
	return v, nil
}

// ReadSyncedManifest performs bounded review without durable copying. The
// generation determines the only permitted immutable release directory.
func ReadSyncedManifest(root string, g releaseverify.VerifiedGeneration, registry releaseverify.KeyRegistry) (releasecontract.Manifest, error) {
	authenticated, err := revalidateGeneration(g, registry)
	if err != nil {
		return releasecontract.Manifest{}, err
	}
	dir := SyncedReleaseDirectory(root, authenticated.Generation)
	body, err := readSyncedFile(filepath.Join(dir, ManifestName), 1<<20)
	if err != nil {
		return releasecontract.Manifest{}, err
	}
	sig, err := readSyncedFile(filepath.Join(dir, ManifestSignatureName), ed25519.SignatureSize)
	if err != nil {
		return releasecontract.Manifest{}, err
	}
	m, _, err := releaseverify.VerifyManifest(body, sig, registry)
	if err != nil {
		return m, err
	}
	if err = matchSyncedManifest(authenticated, m, body); err != nil {
		return releasecontract.Manifest{}, err
	}
	return m, nil
}

func SyncedReleaseDirectory(root string, g releasecontract.Generation) string {
	return filepath.Join(root, "releases", g.Release, g.Source.OS+"-"+g.Source.Arch)
}

// SnapshotSyncedRelease copies only authenticated named bytes into a fresh
// private directory and verifies the complete copy. Synced inputs never execute.
func SnapshotSyncedRelease(root, destination string, g releaseverify.VerifiedGeneration, registry releaseverify.KeyRegistry) (releaseverify.VerifiedRelease, error) {
	current, err := ReadSyncedMetadata(root, g.Generation.Source, registry)
	if err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	if current.SHA256 != g.SHA256 || !bytes.Equal(current.Body, g.Body) || !bytes.Equal(current.Signature, g.Signature) {
		return releaseverify.VerifiedRelease{}, errors.New("synced generation changed since review")
	}
	m, err := ReadSyncedManifest(root, current, registry)
	if err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	if err := disjointSyncedDestination(root, destination); err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		return releaseverify.VerifiedRelease{}, errors.New("private snapshot destination must be new")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	parent, err := scheduler.OpenDirectory(filepath.Dir(destination))
	if err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	parent.Close()
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".synced-release-")
	if err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	// Preserve failed or interrupted material. Path-based cleanup cannot
	// prove custody if another writer has replaced or edited the stage.
	if err := scheduler.EnsurePrivateDirectory(stage); err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	type entry struct {
		name    string
		maximum int64
	}
	entries := []entry{{ManifestName, 1 << 20}, {ManifestSignatureName, ed25519.SignatureSize}, {m.ReleaseNotes.Name, 8 << 20}}
	for _, a := range m.Artifacts {
		if a.Size > 1<<30 {
			return releaseverify.VerifiedRelease{}, errors.New("synced artifact exceeds copy bound")
		}
		entries = append(entries, entry{a.Name, a.Size}, entry{a.SignatureRef, ed25519.SignatureSize})
	}
	dir := SyncedReleaseDirectory(root, current.Generation)
	for _, e := range entries {
		body, err := readSyncedFile(filepath.Join(dir, e.name), e.maximum)
		if err != nil {
			return releaseverify.VerifiedRelease{}, err
		}
		if err = scheduler.WriteNewPrivateFile(filepath.Join(stage, e.name), body); err != nil {
			return releaseverify.VerifiedRelease{}, err
		}
	}
	verified, err := releaseverify.VerifyDirectory(stage, registry)
	if err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	if verified.ManifestSHA256 != current.Generation.ManifestSHA256 {
		return releaseverify.VerifiedRelease{}, errors.New("snapshot manifest changed during copy")
	}
	latest, err := ReadSyncedMetadata(root, current.Generation.Source, registry)
	if err != nil || latest.SHA256 != current.SHA256 {
		return releaseverify.VerifiedRelease{}, errors.New("synced generation changed during copy")
	}
	if err = os.Rename(stage, destination); err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	verified.Directory = destination
	return verified, nil
}

func revalidateGeneration(v releaseverify.VerifiedGeneration, registry releaseverify.KeyRegistry) (releaseverify.VerifiedGeneration, error) {
	authenticated, err := releaseverify.VerifyGeneration(v.Body, v.Signature, registry)
	if err != nil {
		return authenticated, err
	}
	if authenticated.SHA256 != v.SHA256 {
		return releaseverify.VerifiedGeneration{}, errors.New("generation observation digest mismatch")
	}
	return authenticated, nil
}

func matchSyncedManifest(v releaseverify.VerifiedGeneration, m releasecontract.Manifest, body []byte) error {
	if !m.Modern() || m.Release != v.Generation.Release || m.Channel != v.Generation.Source.Channel || fmt.Sprintf("%x", sha256.Sum256(body)) != v.Generation.ManifestSHA256 || m.Delivery.ClientInputsSHA256 != v.Generation.ClientInputsSHA256 || m.Server.ExecutionContract != releasecontract.SyncedSourceContract {
		return errors.New("synced generation does not bind this source-aware release")
	}
	found := false
	for _, a := range m.Artifacts {
		if a.Kind == "mcp_server" && a.OS == v.Generation.Source.OS && a.Arch == v.Generation.Source.Arch {
			found = true
		}
	}
	if !found {
		return errors.New("synced release lacks exact target server")
	}
	return nil
}

func readSyncedFile(path string, maximum int64) ([]byte, error) {
	f, err := openSyncedFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > maximum || !syncedSingleLink(f, before) {
		return nil, fmt.Errorf("synced input must be bounded hydrated unaliased ordinary file: %s", filepath.Base(path))
	}
	body, err := io.ReadAll(io.LimitReader(f, maximum+1))
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) || before.Size() != int64(len(body)) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || !syncedSingleLink(f, after) {
		return nil, errors.New("synced input changed while reading")
	}
	return body, nil
}

func disjointSyncedDestination(root, destination string) error {
	source, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	dest, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	for _, pair := range [][2]string{{source, dest}, {dest, source}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)) {
			return errors.New("private snapshot must be disjoint from synced source")
		}
	}
	return nil
}
