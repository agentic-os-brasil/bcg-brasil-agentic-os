//go:build darwin && arm64

package docling

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthenticatedStatusCancellationStopsBeforeRequirement(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				_ = os.Chmod(name, 0700)
			}
			return err
		})
	})
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "fixture", "key_id": "ephemeral", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": "2020-01-01T00:00:00Z", "valid_until": "2100-01-01T00:00:00Z"}}})
	registryPath := filepath.Join(root, "authority.json")
	mustWrite(t, registryPath, registry)
	options := Options{DataRoot: filepath.Join(root, "state"), AuthorityRegistryPath: registryPath, AuthorityRegistrySHA256: digest(registry), CLIVersion: "0.1.0"}
	if err := os.Mkdir(options.DataRoot, 0700); err != nil {
		t.Fatal(err)
	}
	release := syntheticRelease(t, root, "0.1.0", "cancel-preserves-this-pack", private, registryPath, options.AuthorityRegistrySHA256)
	requirement := requirementForTest(release)
	resolutions := 0
	options.ResolveRequirement = func(context.Context) (Requirement, error) {
		resolutions++
		return requirement, nil
	}
	if err := PlaceVerified(t.Context(), options, release); err != nil {
		t.Fatal(err)
	}
	runtime := New(options)
	if availability := runtime.Status(t.Context()); availability.State != "ready" || availability.PackSHA256 != requirement.PackSHA256 {
		t.Fatalf("authenticated fixture is not ready: %+v", availability)
	}
	selector := filepath.Join(options.DataRoot, "ingestion", "docling", "current.json")
	before, err := os.ReadFile(selector)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("already cancelled", func(t *testing.T) {
		resolutions = 0
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		availability := runtime.Status(ctx)
		if availability.State != "unavailable" || availability.Reason != "conversion_cancelled" || resolutions != 0 {
			t.Fatalf("cancelled status performed current validation: availability=%+v; resolutions=%d", availability, resolutions)
		}
	})
	t.Run("cancelled at the final Current fence", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		resolutions = 0
		selected := options
		selected.ResolveRequirement = func(context.Context) (Requirement, error) {
			resolutions++
			if resolutions == 2 {
				cancel()
			}
			return requirement, nil
		}
		availability := New(selected).Status(ctx)
		if availability.State != "unavailable" || availability.Reason != "conversion_cancelled" || resolutions != 2 {
			t.Fatalf("final cancellation reported a ready pack: availability=%+v; resolutions=%d", availability, resolutions)
		}
	})
	after, err := os.ReadFile(selector)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("cancelled status changed the selected pack: %v", err)
	}
	if availability := runtime.Status(t.Context()); availability.State != "ready" || availability.PackSHA256 != requirement.PackSHA256 {
		t.Fatalf("cancelled status changed the authenticated pack: %+v", availability)
	}
}
