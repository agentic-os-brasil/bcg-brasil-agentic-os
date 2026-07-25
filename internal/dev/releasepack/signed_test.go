package releasepack

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSignCandidateProducesClosedVerifiedRelease(t *testing.T) {
	candidate := unsignedCandidateFixture(t)
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(registryPath, authorityRegistryForKey(t, publicKey), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "signed")
	manifest, err := SignCandidate(SignCandidateOptions{
		Candidate: candidate, Output: output, Registry: registryPath,
		Issuer: "maestro-release", KeyID: "pilot-2026", PrivateKey: privateKey,
		Clock: func() time.Time { return time.Unix(2000, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("SignCandidate() error = %v", err)
	}
	if manifest.Issuer.ID != "maestro-release" || manifest.Issuer.KeyID != "pilot-2026" {
		t.Fatalf("unexpected signed issuer: %#v", manifest.Issuer)
	}
	notes, err := os.ReadFile(filepath.Join(output, manifest.ReleaseNotes.Name))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(notes, []byte("deterministic and unsigned")) ||
		!bytes.Contains(notes, []byte("signed prerelease")) ||
		!bytes.Contains(notes, []byte("not pilot-ready")) {
		t.Fatalf("signed release notes do not disclose the correct release state:\n%s", notes)
	}
	if got := SHA256(notes); got != manifest.ReleaseNotes.SHA256 {
		t.Fatalf("signed release notes digest = %s, want %s", got, manifest.ReleaseNotes.SHA256)
	}
	if err := VerifySignedRelease(
		output,
		registryPath,
		func() time.Time { return time.Unix(2000, 0).UTC() },
	); err != nil {
		t.Fatalf("VerifySignedRelease() error = %v", err)
	}
	for _, artifact := range manifest.Artifacts {
		if info, err := os.Lstat(filepath.Join(output, artifact.SignatureRef)); err != nil ||
			!info.Mode().IsRegular() ||
			info.Size() != ed25519.SignatureSize {
			t.Fatalf("signature %s is invalid: %v", artifact.SignatureRef, err)
		}
	}
	artifact := manifest.Artifacts[0]
	if err := os.WriteFile(filepath.Join(output, artifact.Name), []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := VerifySignedRelease(
		output,
		registryPath,
		func() time.Time { return time.Unix(2000, 0).UTC() },
	); err == nil {
		t.Fatal("VerifySignedRelease() accepted tampered signed artifact")
	}
}

func TestSignCandidateRejectsWrongAuthorityKey(t *testing.T) {
	candidate := unsignedCandidateFixture(t)
	publicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, wrongPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(registryPath, authorityRegistryForKey(t, publicKey), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "signed")
	if _, err := SignCandidate(SignCandidateOptions{
		Candidate: candidate, Output: output, Registry: registryPath,
		Issuer: "maestro-release", KeyID: "pilot-2026", PrivateKey: wrongPrivateKey,
		Clock: func() time.Time { return time.Unix(2000, 0).UTC() },
	}); err == nil {
		t.Fatal("SignCandidate() accepted a private key outside the approved registry")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("rejected signing created output: %v", err)
	}
}

func TestParseSigningSeedAcceptsOnlyOneExactSeed(t *testing.T) {
	seed := bytes.Repeat([]byte{7}, ed25519.SeedSize)
	encoded := base64.StdEncoding.EncodeToString(seed)
	privateKey, err := ParseSigningSeed(bytes.NewBufferString(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(privateKey.Seed(), seed) {
		t.Fatal("parsed signing seed changed")
	}
	for _, invalid := range []string{
		encoded + "\n",
		" " + encoded,
		base64.StdEncoding.EncodeToString(seed[:31]),
		"not-base64!",
	} {
		if _, err := ParseSigningSeed(bytes.NewBufferString(invalid)); err == nil {
			t.Fatalf("ParseSigningSeed() accepted %q", invalid)
		}
	}
}

func unsignedCandidateFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "bundles/base/runtime/capabilities.json", "{}\n")
	writeFile(
		t,
		root,
		"bundles/base/distribution.json",
		`{"schema_version":1,"files":[{"source":"bundles/base/runtime/capabilities.json","path":"runtime/capabilities.json"}]}`,
	)
	output := filepath.Join(t.TempDir(), "candidate")
	if _, err := BuildCandidate(context.Background(), CandidateOptions{
		Root: root, Output: output, Version: "0.2.0", Channel: "canary",
		Builder: fakeBinaryBuilder{},
	}); err != nil {
		t.Fatal(err)
	}
	return output
}

func authorityRegistryForKey(t *testing.T, publicKey ed25519.PublicKey) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"schema_version": 1,
		"product":        "maestro",
		"authorities": []map[string]any{{
			"issuer":      "maestro-release",
			"key_id":      "pilot-2026",
			"algorithm":   "ed25519",
			"public_key":  base64.StdEncoding.EncodeToString(publicKey),
			"status":      "active",
			"valid_from":  "1970-01-01T00:00:00Z",
			"valid_until": "2100-01-01T00:00:00Z",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return append(body, '\n')
}
