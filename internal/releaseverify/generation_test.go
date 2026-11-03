package releaseverify

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

func generationFixture() releasecontract.Generation {
	return releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: releasecontract.Issuer{ID: "engineering", KeyID: "test"}, Source: releasecontract.SourceTuple{SourceID: "team", Channel: "canary", OS: "darwin", Arch: "arm64"}, Sequence: 11, PublishedAt: "2026-01-01T00:00:00Z", ExpiresAt: "2026-01-08T00:00:00Z", Release: "0.1.0", ManifestSHA256: strings.Repeat("a", 64), ClientInputsSHA256: strings.Repeat("b", 64), Recalls: []releasecontract.ReleaseRecall{{Release: "0.0.9", ManifestSHA256: strings.Repeat("c", 64)}}, MinimumReaderContract: releasecontract.SyncedSourceContract}
}

func TestVerifyGenerationRetainsAuthenticatedExpiredDenial(t *testing.T) {
	private := ed25519.NewKeyFromSeed(bytes32(7))
	registry := StaticRegistry{"maestro/engineering/test": private.Public().(ed25519.PublicKey)}
	body, _ := json.Marshal(generationFixture())
	signature := ed25519.Sign(private, body)
	verified, err := VerifyGeneration(body, signature, registry)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Generation.Sequence != 11 || len(verified.Generation.Recalls) != 1 {
		t.Fatal("authenticated denial lost")
	}
	if err = verified.Generation.CheckTime(time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC), time.Time{}); err == nil {
		t.Fatal("expired generation grants freshness")
	}
	corrupt := append([]byte(nil), body...)
	corrupt[len(corrupt)/2] ^= 1
	if _, err = VerifyGeneration(corrupt, signature, registry); err == nil {
		t.Fatal("changed signed bytes accepted")
	}
}

func TestVerifyGenerationAuthenticatesBeforeSemanticValidation(t *testing.T) {
	private := ed25519.NewKeyFromSeed(bytes32(8))
	registry := StaticRegistry{"maestro/engineering/test": private.Public().(ed25519.PublicKey)}
	g := generationFixture()
	g.Protocol = "untrusted-protocol"
	body, _ := json.Marshal(g)
	_, err := VerifyGeneration(body, make([]byte, ed25519.SignatureSize), registry)
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("semantic parsing preceded authentication: %v", err)
	}
	if _, err = VerifyGeneration(body, ed25519.Sign(private, body), registry); err == nil {
		t.Fatal("authenticated unknown protocol accepted")
	}
}

func bytes32(value byte) []byte {
	b := make([]byte, 32)
	for i := range b {
		b[i] = value
	}
	return b
}

func TestGenerationClosedShapeLifetimeAndClock(t *testing.T) {
	g := generationFixture()
	body, _ := json.Marshal(g)
	for name, invalid := range map[string][]byte{
		"unknown":   bytes.Replace(body, []byte(`"product":"maestro"`), []byte(`"product":"maestro","unapproved":true`), 1),
		"duplicate": bytes.Replace(body, []byte(`"sequence":11`), []byte(`"sequence":11,"sequence":12`), 1),
		"trailing":  append(append([]byte(nil), body...), []byte(`{}`)...),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := releasecontract.ParseGeneration(invalid); err == nil {
				t.Fatal("open or ambiguous generation accepted")
			}
		})
	}
	for name, change := range map[string]func(*releasecontract.Generation){
		"ttl cap":          func(g *releasecontract.Generation) { g.ExpiresAt = "2026-02-02T00:00:00Z" },
		"no positive ttl":  func(g *releasecontract.Generation) { g.ExpiresAt = g.PublishedAt },
		"no exact target":  func(g *releasecontract.Generation) { g.Source.OS = "any" },
		"no recalls array": func(g *releasecontract.Generation) { g.Recalls = nil },
		"bad recall": func(g *releasecontract.Generation) {
			g.Recalls = []releasecontract.ReleaseRecall{{Release: "latest", ManifestSHA256: strings.Repeat("a", 64)}}
		},
		"noncanonical time": func(g *releasecontract.Generation) { g.PublishedAt = "2026-01-01T00:00:00+00:00" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := generationFixture()
			change(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid generation accepted")
			}
		})
	}
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := g.CheckTime(now, now.Add(time.Second)); err == nil {
		t.Fatal("backward clock accepted")
	}
	g.PublishedAt = "2026-01-02T00:05:01Z"
	g.ExpiresAt = "2026-01-03T00:00:00Z"
	if err := g.CheckTime(now, time.Time{}); err == nil {
		t.Fatal("future publication beyond bounded skew accepted")
	}
}

func TestGenerationPublishedSchemaMatchesSignedWire(t *testing.T) {
	body, _ := json.Marshal(generationFixture())
	schema := "../../schemas/release-generation.schema.json"
	if err := releasecontract.ValidateSchemaDocument(schema, body); err != nil {
		t.Fatalf("valid generation schema: %v", err)
	}
	for name, invalid := range map[string][]byte{
		"unknown":      bytes.Replace(body, []byte(`"product":"maestro"`), []byte(`"product":"maestro","unknown":true`), 1),
		"target":       bytes.Replace(body, []byte(`"os":"darwin"`), []byte(`"os":"any"`), 1),
		"recall shape": bytes.Replace(body, []byte(`"release":"0.0.9"`), []byte(`"release":"0.0.9","path":"private"`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := releasecontract.ValidateSchemaDocument(schema, invalid); err == nil {
				t.Fatal("invalid generation accepted by published schema")
			}
		})
	}
}
