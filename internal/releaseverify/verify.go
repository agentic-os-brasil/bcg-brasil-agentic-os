// Package releaseverify authenticates a complete Maestro release directory.
package releaseverify

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

const (
	ManifestName          = "release-manifest.json"
	ManifestSignatureName = "release-manifest.json.sig"
	maximumArtifactBytes  = 1 << 30
	maximumNotesBytes     = 8 << 20
)

type KeyRegistry interface {
	Lookup(product, issuer, keyID string) (ed25519.PublicKey, bool)
}

type StaticRegistry map[string]ed25519.PublicKey

func (registry StaticRegistry) Lookup(product, issuer, keyID string) (ed25519.PublicKey, bool) {
	key, ok := registry[product+"/"+issuer+"/"+keyID]
	return key, ok
}

type VerifiedRelease struct {
	Directory string
	Manifest  releasecontract.Manifest
	PublicKey ed25519.PublicKey
}

func VerifyDirectory(directory string, registry KeyRegistry) (VerifiedRelease, error) {
	if registry == nil {
		return VerifiedRelease{}, errors.New("approved release-key registry is required")
	}
	manifestBody, err := readRegular(filepath.Join(directory, ManifestName), 1<<20)
	if err != nil {
		return VerifiedRelease{}, err
	}
	manifest, err := releasecontract.Parse(bytes.NewReader(manifestBody))
	if err != nil {
		return VerifiedRelease{}, err
	}
	publicKey, ok := registry.Lookup(manifest.Product, manifest.Issuer.ID, manifest.Issuer.KeyID)
	if !ok || len(publicKey) != ed25519.PublicKeySize {
		return VerifiedRelease{}, fmt.Errorf("release key %s/%s/%s is not approved", manifest.Product, manifest.Issuer.ID, manifest.Issuer.KeyID)
	}
	manifestSignature, err := readRegular(filepath.Join(directory, ManifestSignatureName), ed25519.SignatureSize)
	if err != nil {
		return VerifiedRelease{}, err
	}
	if len(manifestSignature) != ed25519.SignatureSize {
		return VerifiedRelease{}, errors.New("release manifest signature has an invalid length")
	}
	if !ed25519.Verify(publicKey, manifestBody, manifestSignature) {
		return VerifiedRelease{}, errors.New("release manifest signature verification failed")
	}

	expected := map[string]bool{
		ManifestName:               true,
		ManifestSignatureName:      true,
		manifest.ReleaseNotes.Name: true,
	}
	notesBody, err := readRegular(filepath.Join(directory, manifest.ReleaseNotes.Name), maximumNotesBytes)
	if err != nil {
		return VerifiedRelease{}, err
	}
	if digest(notesBody) != manifest.ReleaseNotes.SHA256 {
		return VerifiedRelease{}, errors.New("release notes digest verification failed")
	}
	for _, artifact := range manifest.Artifacts {
		expected[artifact.Name] = true
		expected[artifact.SignatureRef] = true
		if artifact.Size > maximumArtifactBytes {
			return VerifiedRelease{}, fmt.Errorf("artifact exceeds verification limit: %s", artifact.Name)
		}
		body, err := readRegular(filepath.Join(directory, artifact.Name), artifact.Size)
		if err != nil {
			return VerifiedRelease{}, err
		}
		if int64(len(body)) != artifact.Size || digest(body) != artifact.SHA256 {
			return VerifiedRelease{}, fmt.Errorf("artifact size or digest verification failed for %s", artifact.Name)
		}
		signature, err := readRegular(filepath.Join(directory, artifact.SignatureRef), ed25519.SignatureSize)
		if err != nil {
			return VerifiedRelease{}, err
		}
		if len(signature) != ed25519.SignatureSize {
			return VerifiedRelease{}, fmt.Errorf("artifact signature has an invalid length for %s", artifact.Name)
		}
		if !ed25519.Verify(publicKey, body, signature) {
			return VerifiedRelease{}, fmt.Errorf("artifact signature verification failed for %s", artifact.Name)
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return VerifiedRelease{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !expected[entry.Name()] {
			return VerifiedRelease{}, fmt.Errorf("unexpected release entry %s", entry.Name())
		}
	}
	if len(entries) != len(expected) {
		return VerifiedRelease{}, errors.New("signed release directory is incomplete or has colliding names")
	}
	keyCopy := append(ed25519.PublicKey(nil), publicKey...)
	return VerifiedRelease{Directory: directory, Manifest: manifest, PublicKey: keyCopy}, nil
}

func readRegular(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("release entry must be a regular file: %s", filepath.Base(path))
	}
	if maximum < 0 || info.Size() > maximum {
		return nil, fmt.Errorf("release entry exceeds its size limit: %s", filepath.Base(path))
	}
	return os.ReadFile(path)
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
