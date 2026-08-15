package reviewcustody

import (
	"crypto/ed25519"
	"testing"
)

func TestReviewCustodyRejectsWrongScopeAndInvalidIdentity(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	signer, err := NewEd25519Signer(privateKey, "yoda-review-key", "install-alpha")
	if err != nil {
		t.Fatal(err)
	}
	if signer.Scope() != YodaReviewScope || ValidateSigner(signer) != nil {
		t.Fatal("valid installation-scoped signer was rejected")
	}
	provider, err := NewProvider(func(scope string) (Signer, error) {
		if scope != YodaReviewScope {
			return nil, ErrUnavailable
		}
		return signer, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Load("release-signing"); err == nil {
		t.Fatal("release-signing scope crossed into Yoda custody")
	}
	wrongScope := &scopeOverrideSigner{Signer: signer}
	wrongScope.scope = "release-signing"
	if err := ValidateSigner(wrongScope); err == nil {
		t.Fatal("wrong-scope signer was accepted")
	}
}

type scopeOverrideSigner struct {
	Signer
	scope string
}

func (signer *scopeOverrideSigner) Scope() string { return signer.scope }
