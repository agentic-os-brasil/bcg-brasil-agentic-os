// Package releaseverify authenticates a complete Maestro release directory.
package releaseverify

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	Directory      string
	Manifest       releasecontract.Manifest
	ManifestSHA256 string
	PublicKey      ed25519.PublicKey
}

func VerifyDirectory(directory string, registry KeyRegistry) (VerifiedRelease, error) {
	if registry == nil {
		return VerifiedRelease{}, errors.New("approved release-key registry is required")
	}
	manifestBody, err := readRegular(filepath.Join(directory, ManifestName), 1<<20)
	if err != nil {
		return VerifiedRelease{}, err
	}
	manifestSignature, err := readRegular(filepath.Join(directory, ManifestSignatureName), ed25519.SignatureSize)
	if err != nil {
		return VerifiedRelease{}, err
	}
	manifest, publicKey, err := VerifyManifest(manifestBody, manifestSignature, registry)
	if err != nil {
		return VerifiedRelease{}, err
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
	return VerifiedRelease{
		Directory: directory, Manifest: manifest,
		ManifestSHA256: digest(manifestBody), PublicKey: keyCopy,
	}, nil
}

func VerifyManifest(manifestBody, signature []byte, registry KeyRegistry) (releasecontract.Manifest, ed25519.PublicKey, error) {
	if registry == nil {
		return releasecontract.Manifest{}, nil, errors.New("approved release-key registry is required")
	}
	issuer, keyID, err := parseIssuerHints(manifestBody)
	if err != nil {
		return releasecontract.Manifest{}, nil, err
	}
	publicKey, ok := registry.Lookup("maestro", issuer, keyID)
	if !ok || len(publicKey) != ed25519.PublicKeySize {
		return releasecontract.Manifest{}, nil, fmt.Errorf("release key maestro/%s/%s is not approved", issuer, keyID)
	}
	if len(signature) != ed25519.SignatureSize {
		return releasecontract.Manifest{}, nil, errors.New("release manifest signature has an invalid length")
	}
	if !ed25519.Verify(publicKey, manifestBody, signature) {
		return releasecontract.Manifest{}, nil, errors.New("release manifest signature verification failed")
	}
	manifest, err := releasecontract.Parse(bytes.NewReader(manifestBody))
	if err != nil {
		return releasecontract.Manifest{}, nil, err
	}
	return manifest, append(ed25519.PublicKey(nil), publicKey...), nil
}

// parseIssuerHints extracts only the routing identity needed to select a
// candidate public key. The complete manifest remains untrusted until its
// detached signature has verified over the exact downloaded bytes.
func parseIssuerHints(body []byte) (string, string, error) {
	if len(body) == 0 || len(body) > 1<<20 {
		return "", "", errors.New("release manifest exceeds its verification limit")
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return "", "", fmt.Errorf("decode release manifest issuer: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	var document map[string]json.RawMessage
	if err := decoder.Decode(&document); err != nil {
		return "", "", fmt.Errorf("decode release manifest issuer: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", "", errors.New("decode release manifest issuer: multiple JSON values")
		}
		return "", "", fmt.Errorf("decode release manifest issuer trailing content: %w", err)
	}
	rawIssuer, ok := document["issuer"]
	if !ok {
		return "", "", errors.New("release manifest issuer is missing")
	}
	issuerDecoder := json.NewDecoder(bytes.NewReader(rawIssuer))
	issuerDecoder.DisallowUnknownFields()
	var issuer struct {
		ID    string `json:"id"`
		KeyID string `json:"key_id"`
	}
	if err := issuerDecoder.Decode(&issuer); err != nil {
		return "", "", fmt.Errorf("decode release manifest issuer: %w", err)
	}
	if err := issuerDecoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", "", errors.New("decode release manifest issuer: multiple JSON values")
		}
		return "", "", fmt.Errorf("decode release manifest issuer trailing content: %w", err)
	}
	if issuer.ID == "" || issuer.KeyID == "" {
		return "", "", errors.New("release manifest issuer identity is incomplete")
	}
	return issuer.ID, issuer.KeyID, nil
}

func rejectDuplicateJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := walkJSONValue(decoder, token); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func walkJSONValue(decoder *json.Decoder, token json.Token) error {
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = true
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := walkJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return errors.New("unterminated JSON object")
		}
	case '[':
		for decoder.More() {
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := walkJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("unterminated JSON array")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
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
