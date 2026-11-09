package releaseverify

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

// Exact signed input, including whitespace, consumes private custody capacity.
// These public verifier seams must reject over-budget authentic bytes before a
// caller can qualify an observation, without rewriting those signed bytes.
func TestSyncedMetadataExactByteAdmissionBudget(t *testing.T) {
	const limit = 65536
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{61}, ed25519.SeedSize))
	registry := StaticRegistry{"maestro/engineering/test": key.Public().(ed25519.PublicKey)}
	generationBody, err := json.Marshal(generationFixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{limit, limit + 1} {
		t.Run("generation-"+budgetSizeName(size), func(t *testing.T) {
			body := padSignedBudget(t, generationBody, size)
			verified, err := VerifyGeneration(body, ed25519.Sign(key, body), registry)
			if size > limit {
				if err == nil {
					t.Fatalf("authentic %d-byte generation exceeds the 65536-byte custody budget but was admitted", size)
				}
				return
			}
			if err != nil {
				t.Fatalf("exact budget generation refused: %v", err)
			}
			if !bytes.Equal(verified.Body, body) || verified.SHA256 != digest(body) {
				t.Fatal("verifier normalized exact signed generation bytes")
			}
		})
	}
	oversize := padSignedBudget(t, generationBody, limit+1)
	if _, err := VerifyGeneration(oversize, make([]byte, ed25519.SignatureSize), registry); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("generation size semantics preceded exact authentication: %v", err)
	}

	legacyDirectory, _ := signedReleaseFixture(t)
	legacyBody, err := os.ReadFile(filepath.Join(legacyDirectory, ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := releasecontract.Parse(bytes.NewReader(legacyBody))
	if err != nil {
		t.Fatal(err)
	}
	legacy.Issuer = generationFixture().Issuer
	for _, variant := range []string{"source-aware", "historical-modern", "legacy"} {
		manifest := legacy
		if variant != "legacy" {
			manifest.SchemaVersion = 2
			manifest.CLI = releasecontract.CLIComponent{}
			manifest.Server = releasecontract.ServerComponent{Version: "0.1.0", CompatibleBundle: ">=0.1.0 <0.1.1", Protocol: releasecontract.ServerProtocol, EmbeddedInputsSHA256: strings.Repeat("a", 64)}
			if variant == "source-aware" {
				manifest.Server.ExecutionContract = releasecontract.SyncedSourceContract
			}
			manifest.Delivery = releasecontract.DeliveryComponent{Profile: releasecontract.ModernProfile, InstallerVersion: "0.1.0", InstallerProtocol: releasecontract.InstallerProtocol}
			manifest.Bundle.CompatibleCLI = ""
			manifest.Bundle.CompatibleServer = ">=0.1.0 <0.1.1"
			manifest.Artifacts = append([]releasecontract.Artifact(nil), manifest.Artifacts...)
			manifest.Artifacts[0].Kind = "mcp_server"
			manifest.Delivery.ClientInputsSHA256, err = manifest.ClientInputsDigest()
			if err != nil {
				t.Fatal(err)
			}
		}
		body, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		for _, size := range []int{limit, limit + 1} {
			t.Run(variant+"-"+budgetSizeName(size), func(t *testing.T) {
				padded := padSignedBudget(t, body, size)
				_, _, err := VerifyManifest(padded, ed25519.Sign(key, padded), registry)
				if variant == "source-aware" && size > limit {
					if err == nil {
						t.Fatalf("authentic %d-byte source-aware manifest exceeds custody budget but was admitted", size)
					}
					return
				}
				if err != nil {
					t.Fatalf("supported %s manifest at %d bytes refused: %v", variant, size, err)
				}
			})
		}
		if variant == "source-aware" {
			padded := padSignedBudget(t, body, limit+1)
			if _, _, err := VerifyManifest(padded, make([]byte, ed25519.SignatureSize), registry); err == nil || !strings.Contains(err.Error(), "signature") {
				t.Fatalf("manifest size semantics preceded authentication: %v", err)
			}
		} else {
			padded := padSignedBudget(t, body, 1<<20)
			if _, _, err := VerifyManifest(padded, ed25519.Sign(key, padded), registry); err != nil {
				t.Fatalf("historical %s 1MiB contract regressed: %v", variant, err)
			}
		}
	}
}

func padSignedBudget(t *testing.T, body []byte, size int) []byte {
	t.Helper()
	if len(body) > size {
		t.Fatal("fixture already exceeds boundary")
	}
	return append(append([]byte(nil), body...), bytes.Repeat([]byte(" "), size-len(body))...)
}
func budgetSizeName(size int) string {
	if size == 65536 {
		return "65536"
	}
	return "65537"
}
