package releaseverify

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

func TestVerifyDirectoryAuthenticatesClosedRelease(t *testing.T) {
	directory, registry := signedReleaseFixture(t)
	verified, err := VerifyDirectory(directory, registry)
	if err != nil {
		t.Fatalf("VerifyDirectory() error = %v", err)
	}
	if verified.Manifest.Release != "0.1.0" || len(verified.Manifest.Artifacts) != 2 {
		t.Fatalf("unexpected verified release: %#v", verified.Manifest)
	}
	if len(verified.ManifestSHA256) != 64 {
		t.Fatalf("authenticated manifest digest = %q", verified.ManifestSHA256)
	}
}

func TestVerifyDirectoryFailsClosedOnTamperingAndUnknownKeys(t *testing.T) {
	for name, mutate := range map[string]func(*testing.T, string, StaticRegistry){
		"artifact tamper": func(t *testing.T, directory string, _ StaticRegistry) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(directory, "bcgos_0.1.0_darwin_arm64"), []byte("tampered"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"manifest tamper": func(t *testing.T, directory string, _ StaticRegistry) {
			t.Helper()
			path := filepath.Join(directory, ManifestName)
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			body = []byte(strings.Replace(string(body), `"channel": "canary"`, `"channel": "stable"`, 1))
			if err := os.WriteFile(path, body, 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"unknown key": func(_ *testing.T, _ string, registry StaticRegistry) {
			clear(registry)
		},
		"extra file": func(t *testing.T, directory string, _ StaticRegistry) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(directory, "unlisted.txt"), []byte("no"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			directory, registry := signedReleaseFixture(t)
			mutate(t, directory, registry)
			if _, err := VerifyDirectory(directory, registry); err == nil {
				t.Fatal("VerifyDirectory() accepted an untrusted release")
			}
		})
	}
}

func TestVerifyManifestAuthenticatesIssuerBeforeSemanticValidation(t *testing.T) {
	body := []byte(`{"issuer":{"id":"unknown","key_id":"unknown"},"surprise":true}`)
	_, _, err := VerifyManifest(body, make([]byte, ed25519.SignatureSize), StaticRegistry{})
	if err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Fatalf("VerifyManifest() error = %v, want unapproved issuer before semantic validation", err)
	}
}

func signedReleaseFixture(t *testing.T) (string, StaticRegistry) {
	t.Helper()
	directory := t.TempDir()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := []struct {
		kind string
		os   string
		arch string
		name string
		body []byte
	}{
		{kind: "cli", os: "darwin", arch: "arm64", name: "bcgos_0.1.0_darwin_arm64", body: []byte("#!/bin/sh\necho 'bcgos 0.1.0'\n")},
		{kind: "bundle", os: "any", arch: "any", name: "maestro-base_0.1.0.tar.gz", body: []byte("bundle")},
	}
	manifest := releasecontract.Manifest{
		SchemaVersion: 1,
		Product:       "maestro",
		Release:       "0.1.0",
		Channel:       "canary",
		Issuer:        releasecontract.Issuer{ID: "maestro-release", KeyID: "pilot-2026"},
		CLI:           releasecontract.CLIComponent{Version: "0.1.0", CompatibleBundle: ">=0.1.0 <0.1.1"},
		Bundle:        releasecontract.BundleComponent{Version: "0.1.0", CompatibleCLI: ">=0.1.0 <0.1.1"},
		Migrations:    []releasecontract.Migration{},
	}
	for _, artifact := range artifacts {
		path := filepath.Join(directory, artifact.name)
		if err := os.WriteFile(path, artifact.body, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest.Artifacts = append(manifest.Artifacts, releasecontract.Artifact{
			Kind: artifact.kind, OS: artifact.os, Arch: artifact.arch, Name: artifact.name,
			Size: int64(len(artifact.body)), SHA256: digest(artifact.body), SignatureRef: artifact.name + ".sig",
		})
		if err := os.WriteFile(filepath.Join(directory, artifact.name+".sig"), ed25519.Sign(privateKey, artifact.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	notes := []byte("# release\n")
	if err := os.WriteFile(filepath.Join(directory, "release-notes-0.1.0.md"), notes, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest.ReleaseNotes = releasecontract.ReleaseNotes{Name: "release-notes-0.1.0.md", SHA256: digest(notes)}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(filepath.Join(directory, ManifestName), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ManifestSignatureName), ed25519.Sign(privateKey, body), 0o644); err != nil {
		t.Fatal(err)
	}
	return directory, StaticRegistry{"maestro/maestro-release/pilot-2026": publicKey}
}
