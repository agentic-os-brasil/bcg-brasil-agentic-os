// Package reviewcustody defines the installation-scoped signing boundary for
// Walter review. It is deliberately separate from release signing: a review
// signer can authenticate one local review installation, but cannot sign
// release manifests or authorize distribution.
package reviewcustody

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"regexp"
)

const WalterReviewScope = "maestro/walter-review"

var ErrUnavailable = errors.New("Walter review signing custody is unavailable")

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,95}$`)

// Signer is the only capability the review core needs from local custody.
// Implementations must keep private key material outside packets, receipts,
// logs and repository files.
type Signer interface {
	Sign(payload []byte) ([]byte, error)
	PublicKey() ed25519.PublicKey
	KeyID() string
	InstallationID() string
	Scope() string
}

// Provider loads a signer from installation-local custody. OS keychain,
// managed secret-store and attended qualification implementations can satisfy
// this interface without changing the review protocol.
type Provider interface {
	Load(scope string) (Signer, error)
}

// LocalProvider is a small adapter seam for an approved local custody
// implementation. The callback is intentionally not serialized or logged.
type LocalProvider struct {
	load func(string) (Signer, error)
}

func NewProvider(load func(string) (Signer, error)) (LocalProvider, error) {
	if load == nil {
		return LocalProvider{}, errors.New("review custody loader is required")
	}
	return LocalProvider{load: load}, nil
}

func (provider LocalProvider) Load(scope string) (Signer, error) {
	if provider.load == nil || scope != WalterReviewScope {
		return nil, ErrUnavailable
	}
	signer, err := provider.load(scope)
	if err != nil {
		return nil, err
	}
	if signer == nil || signer.Scope() != WalterReviewScope {
		return nil, errors.New("review custody returned a signer for the wrong scope")
	}
	return signer, nil
}

// Ed25519Signer is an in-memory implementation intended for tests and a
// narrow attended qualification harness. Production adapters should load the
// same interface from OS-managed custody rather than constructing it from
// repository or packet data.
type Ed25519Signer struct {
	privateKey     ed25519.PrivateKey
	publicKey      ed25519.PublicKey
	keyID          string
	installationID string
	scope          string
}

func NewEd25519Signer(privateKey ed25519.PrivateKey, keyID, installationID string) (*Ed25519Signer, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("Walter review private key has an invalid size")
	}
	if !identifierPattern.MatchString(keyID) || !identifierPattern.MatchString(installationID) {
		return nil, errors.New("Walter review custody identity is invalid")
	}
	keyCopy := append(ed25519.PrivateKey(nil), privateKey...)
	publicKey := append(ed25519.PublicKey(nil), privateKey.Public().(ed25519.PublicKey)...)
	return &Ed25519Signer{
		privateKey: keyCopy, publicKey: publicKey, keyID: keyID,
		installationID: installationID, scope: WalterReviewScope,
	}, nil
}

func (signer *Ed25519Signer) Sign(payload []byte) ([]byte, error) {
	if signer == nil || len(signer.privateKey) != ed25519.PrivateKeySize {
		return nil, ErrUnavailable
	}
	return ed25519.Sign(signer.privateKey, payload), nil
}

func (signer *Ed25519Signer) PublicKey() ed25519.PublicKey {
	if signer == nil {
		return nil
	}
	return append(ed25519.PublicKey(nil), signer.publicKey...)
}

func (signer *Ed25519Signer) KeyID() string {
	if signer == nil {
		return ""
	}
	return signer.keyID
}

func (signer *Ed25519Signer) InstallationID() string {
	if signer == nil {
		return ""
	}
	return signer.installationID
}

func (signer *Ed25519Signer) Scope() string {
	if signer == nil {
		return ""
	}
	return signer.scope
}

func ValidateSigner(signer Signer) error {
	if signer == nil || signer.Scope() != WalterReviewScope ||
		!identifierPattern.MatchString(signer.KeyID()) ||
		!identifierPattern.MatchString(signer.InstallationID()) ||
		len(signer.PublicKey()) != ed25519.PublicKeySize {
		return fmt.Errorf("Walter review signer is missing or outside the installation scope")
	}
	return nil
}
