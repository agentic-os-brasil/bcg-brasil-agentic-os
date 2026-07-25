package releasepack

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

type SignCandidateOptions struct {
	Candidate  string
	Output     string
	Registry   string
	Issuer     string
	KeyID      string
	PrivateKey ed25519.PrivateKey
	Clock      func() time.Time
}

func ParseSigningSeed(reader io.Reader) (ed25519.PrivateKey, error) {
	body, err := io.ReadAll(io.LimitReader(reader, 257))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || len(body) > 256 || bytes.ContainsAny(body, " \t\r\n") {
		return nil, errors.New("signing seed must be one bounded base64 value without whitespace")
	}
	seed, err := base64.StdEncoding.Strict().DecodeString(string(body))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("signing seed must decode to exactly 32 bytes")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func SignCandidate(options SignCandidateOptions) (releasecontract.Manifest, error) {
	if options.Candidate == "" || options.Output == "" || options.Registry == "" ||
		options.Issuer == "" || options.KeyID == "" {
		return releasecontract.Manifest{}, errors.New("signed candidate paths and authority identity are required")
	}
	if len(options.PrivateKey) != ed25519.PrivateKeySize {
		return releasecontract.Manifest{}, errors.New("approved Ed25519 private key is required")
	}
	if err := VerifyCandidate(options.Candidate); err != nil {
		return releasecontract.Manifest{}, fmt.Errorf("verify unsigned candidate: %w", err)
	}
	manifestFile, err := os.Open(filepath.Join(options.Candidate, ManifestName))
	if err != nil {
		return releasecontract.Manifest{}, err
	}
	manifest, err := releasecontract.Parse(manifestFile)
	closeErr := manifestFile.Close()
	if err != nil {
		return releasecontract.Manifest{}, err
	}
	if closeErr != nil {
		return releasecontract.Manifest{}, closeErr
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	registry, err := releaseverify.LoadAuthorityRegistry(options.Registry, clock)
	if err != nil {
		return releasecontract.Manifest{}, err
	}
	publicKey, ok := registry.Lookup("maestro", options.Issuer, options.KeyID)
	if !ok || !bytes.Equal(publicKey, options.PrivateKey.Public().(ed25519.PublicKey)) {
		return releasecontract.Manifest{}, errors.New("signing key does not match the active approved authority")
	}
	manifest.Issuer = releasecontract.Issuer{ID: options.Issuer, KeyID: options.KeyID}
	if err := manifest.Validate(); err != nil {
		return releasecontract.Manifest{}, err
	}
	if _, err := os.Stat(options.Output); err == nil {
		return releasecontract.Manifest{}, fmt.Errorf("signed output already exists: %s", options.Output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return releasecontract.Manifest{}, err
	}
	parent := filepath.Dir(options.Output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return releasecontract.Manifest{}, err
	}
	staging, err := os.MkdirTemp(parent, ".maestro-signed-release-")
	if err != nil {
		return releasecontract.Manifest{}, err
	}
	defer os.RemoveAll(staging)
	for _, artifact := range manifest.Artifacts {
		mode := os.FileMode(0o644)
		if artifact.Kind == "cli" {
			mode = 0o755
		}
		body, err := copySignedInput(
			filepath.Join(options.Candidate, artifact.Name),
			filepath.Join(staging, artifact.Name),
			artifact.Size,
			artifact.SHA256,
			mode,
		)
		if err != nil {
			return releasecontract.Manifest{}, err
		}
		if err := os.WriteFile(
			filepath.Join(staging, artifact.SignatureRef),
			ed25519.Sign(options.PrivateKey, body),
			0o600,
		); err != nil {
			return releasecontract.Manifest{}, err
		}
	}
	notesBody := signedPrereleaseNotes(manifest)
	manifest.ReleaseNotes.SHA256 = SHA256(notesBody)
	if err := os.WriteFile(
		filepath.Join(staging, manifest.ReleaseNotes.Name),
		notesBody,
		0o644,
	); err != nil {
		return releasecontract.Manifest{}, err
	}
	manifestBody, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return releasecontract.Manifest{}, err
	}
	manifestBody = append(manifestBody, '\n')
	if err := os.WriteFile(filepath.Join(staging, ManifestName), manifestBody, 0o600); err != nil {
		return releasecontract.Manifest{}, err
	}
	if err := os.WriteFile(
		filepath.Join(staging, releaseverify.ManifestSignatureName),
		ed25519.Sign(options.PrivateKey, manifestBody),
		0o600,
	); err != nil {
		return releasecontract.Manifest{}, err
	}
	if _, err := releaseverify.VerifyDirectory(staging, registry); err != nil {
		return releasecontract.Manifest{}, fmt.Errorf("verify signed release: %w", err)
	}
	if err := os.Rename(staging, options.Output); err != nil {
		return releasecontract.Manifest{}, err
	}
	return manifest, nil
}

func signedPrereleaseNotes(manifest releasecontract.Manifest) []byte {
	return []byte(fmt.Sprintf(
		"# Maestro %s signed prerelease\n\n"+
			"Channel: `%s`\n\n"+
			"This release set is cryptographically closed with approved Maestro "+
			"manifest and artifact signatures. The governed publication workflow "+
			"also requires native Windows/macOS code-signing identities and an "+
			"authenticated immutable GitHub prerelease.\n\n"+
			"This prerelease is not pilot-ready. Installer/notarization evidence, "+
			"clean corporate Windows and macOS install/update/rollback acceptance, "+
			"and support-owner approval remain separate gates.\n",
		manifest.Release,
		manifest.Channel,
	))
}

func VerifySignedRelease(directory, registryPath string, clock func() time.Time) error {
	if clock == nil {
		return errors.New("signed release verification clock is required")
	}
	registry, err := releaseverify.LoadAuthorityRegistry(registryPath, clock)
	if err != nil {
		return err
	}
	_, err = releaseverify.VerifyDirectory(directory, registry)
	return err
}

func copySignedInput(
	source, destination string,
	expectedSize int64,
	expectedSHA256 string,
	mode os.FileMode,
) ([]byte, error) {
	body, digest, err := readSeedInput(source, 1<<30)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) != expectedSize || digest != expectedSHA256 {
		return nil, fmt.Errorf("candidate input identity changed for %s", filepath.Base(source))
	}
	if err := os.WriteFile(destination, body, mode); err != nil {
		return nil, err
	}
	return body, nil
}
