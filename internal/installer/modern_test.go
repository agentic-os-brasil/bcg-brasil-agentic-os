package installer

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Native trust is an engineering-only test composition. Signed inputs still
// cross Prepare's canonical exact-byte verifier.
func TestModernSignedFirstInstallReview(t *testing.T) {
	root := t.TempDir()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	registry := []byte(fmt.Sprintf(`{"schema_version":1,"product":"maestro","authorities":[{"issuer":"engineering","key_id":"fixture","algorithm":"ed25519","public_key":"%s","status":"active","valid_from":"2026-01-01T00:00:00Z","valid_until":"2030-01-01T00:00:00Z"}]}`, base64.StdEncoding.EncodeToString(public)))
	registryPath := filepath.Join(root, "registry.json")
	writeInstallerFile(t, registryPath, registry)
	registryDigest := fmt.Sprintf("%x", sha256.Sum256(registry))
	release := filepath.Join(root, "release")
	if err := os.Mkdir(release, 0700); err != nil {
		t.Fatal(err)
	}
	artifacts := []map[string]any{}
	for _, item := range []struct{ kind, os, arch, name, body string }{
		{"bundle", "any", "any", "maestro-base_0.2.0.tar.gz", "signed bundle fixture"},
		{"mcp_server", "darwin", "arm64", "maestro-mcp_0.2.0_darwin_arm64", "signed server fixture"},
	} {
		body := []byte(item.body)
		writeInstallerFile(t, filepath.Join(release, item.name), body)
		writeInstallerFile(t, filepath.Join(release, item.name+".sig"), ed25519.Sign(private, body))
		artifacts = append(artifacts, map[string]any{"kind": item.kind, "os": item.os, "arch": item.arch, "name": item.name, "size": len(body), "sha256": fmt.Sprintf("%x", sha256.Sum256(body)), "signature_ref": item.name + ".sig"})
	}
	notes := []byte("engineering fixture")
	writeInstallerFile(t, filepath.Join(release, "notes_0.2.0.md"), notes)
	embedded := strings.Repeat("a", 64)
	vector := map[string]any{"profile": "mcp-local-v1", "server_protocol": "maestro-mcp-identity-v1", "installer_protocol": "maestro-install-seed-v1", "installer_version": "0.3.0", "embedded_inputs_sha256": embedded, "artifacts": artifacts}
	vectorBody, _ := json.Marshal(vector)
	manifest := map[string]any{"schema_version": 2, "product": "maestro", "release": "0.2.0", "channel": "canary", "issuer": map[string]any{"id": "engineering", "key_id": "fixture"}, "server": map[string]any{"version": "0.2.0", "compatible_bundle": ">=0.2.0 <0.3.0", "protocol": "maestro-mcp-identity-v1", "embedded_inputs_sha256": embedded}, "bundle": map[string]any{"version": "0.2.0", "compatible_server": ">=0.2.0 <0.3.0"}, "delivery": map[string]any{"profile": "mcp-local-v1", "installer_version": "0.3.0", "installer_protocol": "maestro-install-seed-v1", "client_inputs_sha256": fmt.Sprintf("%x", sha256.Sum256(vectorBody))}, "artifacts": artifacts, "migrations": []any{}, "release_notes": map[string]any{"name": "notes_0.2.0.md", "sha256": fmt.Sprintf("%x", sha256.Sum256(notes))}}
	body, _ := json.Marshal(manifest)
	writeInstallerFile(t, filepath.Join(release, "release-manifest.json"), body)
	writeInstallerFile(t, filepath.Join(release, "release-manifest.json.sig"), ed25519.Sign(private, body))
	seed := filepath.Join(root, "maestro-install_0.3.0_darwin_arm64")
	writeInstallerFile(t, seed, []byte("engineering native seed fixture"))
	options := Options{UserHome: root, ReleaseDir: release, Bootstrapper: seed, AuthorityRegistry: registryPath, ManagedRoot: filepath.Join(root, "managed"), DataRoot: filepath.Join(root, ".maestro-install"), TargetOS: "darwin", TargetArch: "arm64", Clock: func() time.Time { return time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC) }, VerifyNative: func(context.Context, string) error { return nil }, Run: func(context.Context, string, ...string) ([]byte, error) {
		return []byte(fmt.Sprintf(`{"schema_version":2,"product":"maestro","component":"installer","protocol":"maestro-install-seed-v1","installer_version":"0.3.0","authority_registry_sha256":"%s"}`, registryDigest)), nil
	}}
	plan, _, err := Prepare(options)
	if err != nil {
		t.Fatalf("modern signed first-install review: %v", err)
	}
	if plan.PlanDigest == "" {
		t.Fatal("review lacks an exact confirmation digest")
	}
	for name, mutate := range map[string]func(map[string]any){
		"mixed CLI": func(v map[string]any) {
			v["cli"] = map[string]any{"version": "0.2.0", "compatible_bundle": ">=0.2.0 <0.3.0"}
		},
		"unknown component field":     func(v map[string]any) { v["server"].(map[string]any)["callback"] = "healthy" },
		"unsupported profile":         func(v map[string]any) { v["delivery"].(map[string]any)["profile"] = "legacy-renamed" },
		"unsupported server protocol": func(v map[string]any) { v["server"].(map[string]any)["protocol"] = "bcgos-version" },
		"mixed bundle":                func(v map[string]any) { v["bundle"].(map[string]any)["compatible_cli"] = "" },
		"adapter on server":           func(v map[string]any) { v["artifacts"].([]any)[1].(map[string]any)["adapter"] = "" },
		"embedded digest binding": func(v map[string]any) {
			v["server"].(map[string]any)["embedded_inputs_sha256"] = strings.Repeat("b", 64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			var v map[string]any
			if err := json.Unmarshal(body, &v); err != nil {
				t.Fatal(err)
			}
			mutate(v)
			modified, _ := json.Marshal(v)
			writeInstallerFile(t, filepath.Join(release, "release-manifest.json"), modified)
			writeInstallerFile(t, filepath.Join(release, "release-manifest.json.sig"), ed25519.Sign(private, modified))
			defer func() {
				writeInstallerFile(t, filepath.Join(release, "release-manifest.json"), body)
				writeInstallerFile(t, filepath.Join(release, "release-manifest.json.sig"), ed25519.Sign(private, body))
			}()
			if _, _, err := Prepare(options); err == nil {
				t.Fatal("signed malformed modern variant accepted")
			}
		})
	}
	t.Run("unsupported target", func(t *testing.T) {
		changed := options
		changed.TargetArch = "amd64"
		if _, _, err := Prepare(changed); err == nil || !strings.Contains(err.Error(), "exact target") {
			t.Fatalf("target selected another artifact: %v", err)
		}
	})
	t.Run("private state custody", func(t *testing.T) {
		changed := options
		changed.DataRoot = filepath.Join(root, ".maestro", "state")
		if _, _, err := Prepare(changed); err == nil {
			t.Fatal("transaction state was admitted inside owner home")
		}
	})
	t.Run("no beta trust inheritance", func(t *testing.T) {
		changed := options
		changed.NativeTrustMode = NativeTrustCanarySimple
		if _, _, err := Prepare(changed); err == nil {
			t.Fatal("modern profile accepted legacy beta trust mode")
		}
	})
	t.Run("mixed legacy seed status", func(t *testing.T) {
		changed := options
		changed.Run = func(context.Context, string, ...string) ([]byte, error) {
			return []byte(fmt.Sprintf(`{"schema_version":2,"product":"maestro","component":"installer","protocol":"maestro-install-seed-v1","installer_version":"0.3.0","bootstrapper_version":"","authority_registry_sha256":"%s"}`, registryDigest)), nil
		}
		if _, _, err := Prepare(changed); err == nil {
			t.Fatal("modern seed accepted a legacy wire field")
		}
	})
	t.Run("duplicate seed status", func(t *testing.T) {
		changed := options
		changed.Run = func(context.Context, string, ...string) ([]byte, error) {
			return []byte(`{"schema_version":2,"schema_version":2}`), nil
		}
		if _, _, err := Prepare(changed); err == nil {
			t.Fatal("duplicate seed status fields accepted")
		}
	})
	if _, err := os.Stat(filepath.Join(root, "managed")); !os.IsNotExist(err) {
		t.Fatalf("review wrote managed root: %v", err)
	}
}
