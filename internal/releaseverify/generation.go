package releaseverify

import (
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

type VerifiedGeneration struct {
	Generation releasecontract.Generation
	Body       []byte
	Signature  []byte
	SHA256     string
	PublicKey  ed25519.PublicKey
}

// VerifyGeneration uses the exact same live issuer/key registry and exact-byte
// signature policy as manifests. Freshness is deliberately a later decision.
func VerifyGeneration(body, signature []byte, registry KeyRegistry) (VerifiedGeneration, error) {
	if registry == nil {
		return VerifiedGeneration{}, errors.New("approved release-key registry is required")
	}
	issuer, keyID, err := parseIssuerHints(body)
	if err != nil {
		return VerifiedGeneration{}, err
	}
	key, ok := registry.Lookup("maestro", issuer, keyID)
	if !ok || len(key) != ed25519.PublicKeySize {
		return VerifiedGeneration{}, errors.New("generation signing key is not approved")
	}
	if len(signature) != ed25519.SignatureSize || !ed25519.Verify(key, body, signature) {
		return VerifiedGeneration{}, errors.New("generation signature verification failed")
	}
	g, err := releasecontract.ParseGeneration(body)
	if err != nil {
		return VerifiedGeneration{}, err
	}
	if policy, ok := registry.(interface {
		PermitsGeneration(string, string, time.Time, time.Time) bool
	}); ok {
		published, _ := time.Parse(time.RFC3339, g.PublishedAt)
		expires, _ := time.Parse(time.RFC3339, g.ExpiresAt)
		if !policy.PermitsGeneration(issuer, keyID, published, expires) {
			return VerifiedGeneration{}, errors.New("generation exceeds admitted signing key validity")
		}
	}
	return VerifiedGeneration{Generation: g, Body: append([]byte(nil), body...), Signature: append([]byte(nil), signature...), SHA256: digest(body), PublicKey: append(ed25519.PublicKey(nil), key...)}, nil
}

// PermitsGeneration bounds publisher-issued lifetimes to the existing authority
// validity window, without introducing any replacement registry or clock.
func (r AuthorityRegistry) PermitsGeneration(issuer, keyID string, published, expires time.Time) bool {
	k, ok := r.keys[issuer+"/"+keyID]
	return ok && !published.Before(k.validFrom) && !expires.After(k.validUntil)
}
