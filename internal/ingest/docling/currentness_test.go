//go:build darwin && arm64

package docling

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"strings"
)

// Inert synthetic packs exercise the real signature/inventory/placement owner.
// They are never executed and assert no native conversion qualification.
func TestAuthenticatedChangedPackPlacementPreservesPrior(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Cleanup(func() {
		filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
			if e == nil && d.IsDir() {
				os.Chmod(p, 0700)
			}
			return e
		})
	})
	public, private, e := ed25519.GenerateKey(nil)
	if e != nil {
		t.Fatal(e)
	}
	registry, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "fixture", "key_id": "ephemeral", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": "2020-01-01T00:00:00Z", "valid_until": "2100-01-01T00:00:00Z"}}})
	registryPath := filepath.Join(root, "authority.json")
	mustWrite(t, registryPath, registry)
	data := filepath.Join(root, "state")
	if e = os.Mkdir(data, 0700); e != nil {
		t.Fatal(e)
	}
	options := Options{DataRoot: data, AuthorityRegistryPath: registryPath, AuthorityRegistrySHA256: digest(registry), CLIVersion: "0.1.0"}
	n := syntheticRelease(t, root, "0.1.0", "inert-n", private, registryPath, options.AuthorityRegistrySHA256)
	next := syntheticRelease(t, root, "0.1.1", "inert-n1", private, registryPath, options.AuthorityRegistrySHA256)
	current := n
	options.ResolveRequirement = func(context.Context) (Requirement, error) { return requirementForTest(current), nil }
	if e = PlaceVerified(ctx, options, n); e != nil {
		t.Fatal(e)
	}
	first := New(options).Status(ctx)
	if first.State != "ready" {
		t.Fatal(first)
	}
	current = next
	if got := New(options).Status(ctx); got.State != "unavailable" || got.Reason != "current_pack_pending" {
		t.Fatalf("obsolete compatible pack still current: %+v", got)
	}
	space := filepath.Join(root, "workspace")
	if e = os.Mkdir(space, 0700); e != nil {
		t.Fatal(e)
	}
	w, e := workspace.Initialize(workspace.Options{WorkspacePath: space, DataRoot: data})
	if e != nil {
		t.Fatal(e)
	}
	mustWrite(t, filepath.Join(space, "sample.txt"), []byte("synthetic source"))
	conversion, e := New(options).Convert(ctx, ingest.Request{SourcePath: "sample.txt", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, w.WorkspaceID)
	if e != nil || conversion.Result.Status != ingest.StatusUnavailable || len(conversion.Result.Warnings) != 1 || conversion.Result.Warnings[0] != "current_pack_pending" {
		t.Fatalf("obsolete converter usable: %+v %v", conversion, e)
	}
	if e = PlaceVerified(ctx, options, next); e != nil {
		t.Fatalf("different authenticated pack update refused: %v", e)
	}
	second := New(options).Status(ctx)
	if second.State != "ready" || second.PackSHA256 == first.PackSHA256 {
		t.Fatalf("changed pack not current: %+v -> %+v", first, second)
	}
	if _, e = os.Stat(filepath.Join(data, "ingestion/docling/installed", first.PackSHA256)); e != nil {
		t.Fatal("prior payload lost", e)
	}
	if e = PlaceVerified(ctx, options, next); e != nil {
		t.Fatal("identical repeat", e)
	}
	current = n
	if e = PlaceVerified(ctx, options, n); e != nil {
		t.Fatal("retained rollback", e)
	}
	if got := New(options).Status(ctx); got != first {
		t.Fatalf("rollback readback %+v", got)
	}
	selector := filepath.Join(data, "ingestion/docling/current.json")
	selected, e := os.ReadFile(selector)
	if e != nil {
		t.Fatal(e)
	}
	t.Run("same pack metadata refresh", func(t *testing.T) {
		refreshed := syntheticRelease(t, root, "0.1.2", "inert-n", private, registryPath, options.AuthorityRegistrySHA256)
		current = refreshed
		if e := PlaceVerified(ctx, options, refreshed); e != nil {
			t.Fatal(e)
		}
		if got := New(options).Status(ctx); got.State != "ready" || got.PackSHA256 != first.PackSHA256 {
			t.Fatalf("same-pack metadata refresh %+v", got)
		}
		for _, v := range []releaseverify.VerifiedRelease{n, next, refreshed} {
			body, e := os.ReadFile(filepath.Join(data, "ingestion/docling/metadata", v.ManifestSHA256, "release-manifest.json"))
			if e != nil || digest(body) != v.ManifestSHA256 {
				t.Fatalf("prior metadata changed %v", e)
			}
		}
		current = n
		if e := RequalifyRequired(ctx, options); e != nil {
			t.Fatal(e)
		}
	})
	for name, body := range map[string][]byte{
		"corrupt":            []byte("not json"),
		"unknown field":      []byte(`{"schema_version":1,"manifest_sha256":"` + n.ManifestSHA256 + `","other":true}`),
		"unknown generation": []byte(`{"schema_version":1,"manifest_sha256":"` + strings.Repeat("a", 64) + `"}`),
		"path traversal":     []byte(`{"schema_version":1,"manifest_sha256":"../legacy"}`),
		"duplicate":          []byte(`{"schema_version":1,"schema_version":1,"manifest_sha256":"` + n.ManifestSHA256 + `"}`),
	} {
		t.Run("selector "+name, func(t *testing.T) {
			mustWrite(t, selector, body)
			defer mustWrite(t, selector, selected)
			if got := New(options).Status(ctx); got.State != "unavailable" || got.Reason != "pack_selection_unavailable" {
				t.Fatal("unsafe selection admitted", got)
			}
			current = next
			defer func() { current = n }()
			if e := PlaceVerified(ctx, options, next); e == nil {
				t.Fatal("unsafe selection overwritten")
			}
			actual, _ := os.ReadFile(selector)
			if !bytes.Equal(body, actual) {
				t.Fatal("unknown selector changed")
			}
		})
	}
	t.Run("selector alias", func(t *testing.T) {
		saved := selector + ".retained"
		if e := os.Rename(selector, saved); e != nil {
			t.Fatal(e)
		}
		if e := os.Symlink(saved, selector); e != nil {
			t.Fatal(e)
		}
		defer func() { os.Remove(selector); os.Rename(saved, selector) }()
		if got := New(options).Status(ctx); got.Reason != "pack_selection_unavailable" {
			t.Fatal(got)
		}
		if e := PlaceVerified(ctx, options, n); e == nil {
			t.Fatal("aliased selector replaced")
		}
	})
	t.Run("current changes at publication", func(t *testing.T) {
		current = next
		defer func() { current = n; options.GuardSelection = nil }()
		options.GuardSelection = func(publish func() error) error { current = n; return publish() }
		if e := PlaceVerified(ctx, options, next); e == nil {
			t.Fatal("changed Current selected")
		}
		actual, _ := os.ReadFile(selector)
		if !bytes.Equal(actual, selected) {
			t.Fatal("failed candidate changed current selector")
		}
	})
	t.Run("candidate changes at publication", func(t *testing.T) {
		current = next
		defer func() { current = n; options.GuardSelection = nil }()
		name := filepath.Join(data, "ingestion/docling/metadata", next.ManifestSHA256, releaseverify.ManifestSignatureName)
		original, e := os.ReadFile(name)
		if e != nil {
			t.Fatal(e)
		}
		defer mustWrite(t, name, original)
		options.GuardSelection = func(publish func() error) error { mustWrite(t, name, bytes.Repeat([]byte{1}, 64)); return publish() }
		if e := PlaceVerified(ctx, options, next); e == nil {
			t.Fatal("changed candidate selected")
		}
		actual, _ := os.ReadFile(selector)
		if !bytes.Equal(actual, selected) {
			t.Fatal("changed candidate replaced prior selection")
		}
	})
	t.Run("cancelled candidate", func(t *testing.T) {
		current = next
		defer func() { current = n }()
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if e := PlaceVerified(cancelled, options, next); e == nil {
			t.Fatal("cancelled candidate selected")
		}
		actual, _ := os.ReadFile(selector)
		if !bytes.Equal(actual, selected) {
			t.Fatal("cancelled candidate changed selector")
		}
	})
	t.Run("narrowing cannot grant foreign pack", func(t *testing.T) {
		original := options.ResolveRequirement
		defer func() { options.ResolveRequirement = original }()
		options.ResolveRequirement = func(context.Context) (Requirement, error) {
			r := requirementForTest(n)
			r.PackSHA256 = requirementForTest(next).PackSHA256
			return r, nil
		}
		if got := New(options).Status(ctx); got.State != "unavailable" || got.Reason != "current_pack_pending" {
			t.Fatal(got)
		}
		if e := PlaceVerified(ctx, options, n); e == nil {
			t.Fatal("different required pack granted authority")
		}
	})
	t.Run("foreign artifact signature", func(t *testing.T) {
		current = next
		defer func() { current = n }()
		var name string
		for _, a := range next.Manifest.Artifacts {
			if a.Kind == "runtime_pack" {
				name = filepath.Join(next.Directory, a.SignatureRef)
			}
		}
		original, e := os.ReadFile(name)
		if e != nil {
			t.Fatal(e)
		}
		mustWrite(t, name, bytes.Repeat([]byte{1}, 64))
		defer mustWrite(t, name, original)
		if e := PlaceVerified(ctx, options, next); e == nil {
			t.Fatal("foreign signature placed")
		}
		actual, _ := os.ReadFile(selector)
		if !bytes.Equal(actual, selected) {
			t.Fatal("foreign signature changed selector")
		}
	})
	t.Run("actual server incompatible", func(t *testing.T) {
		original := options.ResolveRequirement
		defer func() { options.ResolveRequirement = original }()
		options.ResolveRequirement = func(context.Context) (Requirement, error) {
			r := requirementForTest(n)
			r.ServerVersion = "0.2.0"
			return r, nil
		}
		if got := New(options).Status(ctx); got.State != "unavailable" || got.Reason != "pack_incompatible" {
			t.Fatal(got)
		}
	})
	t.Run("expired registry", func(t *testing.T) {
		expired := bytes.ReplaceAll(registry, []byte("2100-01-01T00:00:00Z"), []byte("2020-01-02T00:00:00Z"))
		mustWrite(t, registryPath, expired)
		original := options.AuthorityRegistrySHA256
		options.AuthorityRegistrySHA256 = digest(expired)
		defer func() { mustWrite(t, registryPath, registry); options.AuthorityRegistrySHA256 = original }()
		if got := New(options).Status(ctx); got.State != "unavailable" || got.Reason != "authority_unavailable" {
			t.Fatal(got)
		}
		if e := PlaceVerified(ctx, options, n); e == nil {
			t.Fatal("expired issuer placed")
		}
		actual, _ := os.ReadFile(selector)
		if !bytes.Equal(actual, selected) {
			t.Fatal("expired issuer changed selector")
		}
	})
}
func mustWrite(t *testing.T, p string, b []byte) {
	t.Helper()
	if e := os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
}
func syntheticRelease(t *testing.T, root, version, marker string, key ed25519.PrivateKey, registryPath, pin string) releaseverify.VerifiedRelease {
	t.Helper()
	target, e := inputsForTarget(runtime.GOOS, runtime.GOARCH)
	if e != nil {
		t.Fatal(e)
	}
	entries := map[string]entry{"bridge.py": {target.bridge, 0444}, "pylock.toml": {target.lock, 0444}, "inputs.json": {target.inputs, 0444}, "parser-licenses.json": {target.notices, 0444}, "inert-fixture.txt": {[]byte(marker), 0444}}
	var inputs []Input
	json.Unmarshal(target.inputs, &inputs)
	pack := packManifest{SchemaVersion: 1, Adapter: "docling", Version: DoclingVersion, OS: runtime.GOOS, Arch: runtime.GOARCH, CompatibleCLI: ">=0.1.0 <0.2.0", Python: PythonVersion, PythonArchiveSHA256: target.pythonSHA, LockSHA256: target.lockSHA, Models: []string{}, Inputs: inputs}
	paths := make([]string, 0, len(entries))
	for p := range entries {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		f := entries[p]
		pack.Files = append(pack.Files, inventoryFile{p, digest(f.body), int64(len(f.body)), f.mode})
	}
	body, _ := json.Marshal(pack)
	entries["pack.json"] = entry{body, 0444}
	paths = append(paths, "pack.json")
	sort.Strings(paths)
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for _, p := range paths {
		f := entries[p]
		if e = tw.WriteHeader(&tar.Header{Name: p, Mode: f.mode, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); e != nil {
			t.Fatal(e)
		}
		tw.Write(f.body)
	}
	tw.Close()
	gz.Close()
	directory := filepath.Join(root, "release-"+version)
	if e = os.Mkdir(directory, 0700); e != nil {
		t.Fatal(e)
	}
	artifacts := []releasecontract.Artifact{}
	for _, a := range []struct {
		kind, name string
		body       []byte
	}{{"mcp_server", "server-" + version, []byte("inert server")}, {"bundle", "bundle-" + version + ".tar.gz", []byte("inert bundle")}, {"runtime_pack", "docling-" + version + ".tar.gz", archive.Bytes()}} {
		item := releasecontract.Artifact{Kind: a.kind, OS: runtime.GOOS, Arch: runtime.GOARCH, Name: a.name, Size: int64(len(a.body)), SHA256: digest(a.body), SignatureRef: a.name + ".sig"}
		if a.kind == "bundle" {
			item.OS = "any"
			item.Arch = "any"
		}
		if a.kind == "runtime_pack" {
			item.Adapter = "docling"
		}
		artifacts = append(artifacts, item)
		mustWrite(t, filepath.Join(directory, a.name), a.body)
		mustWrite(t, filepath.Join(directory, item.SignatureRef), ed25519.Sign(key, a.body))
	}
	manifest := releasecontract.Manifest{SchemaVersion: 2, Product: "maestro", Release: version, Channel: "canary", Issuer: releasecontract.Issuer{ID: "fixture", KeyID: "ephemeral"}, Server: releasecontract.ServerComponent{Version: version, CompatibleBundle: ">=0.1.0 <0.2.0", Protocol: releasecontract.ServerProtocol, EmbeddedInputsSHA256: digest([]byte("resources"))}, Delivery: releasecontract.DeliveryComponent{Profile: releasecontract.ModernProfile, InstallerVersion: "0.1.0", InstallerProtocol: releasecontract.InstallerProtocol}, Bundle: releasecontract.BundleComponent{Version: version, CompatibleServer: ">=0.1.0 <0.2.0"}, Migrations: []releasecontract.Migration{}, ReleaseNotes: releasecontract.ReleaseNotes{Name: "notes-" + version + ".md", SHA256: digest([]byte("synthetic fixture"))}, Artifacts: artifacts}
	manifest.Delivery.ClientInputsSHA256, e = manifest.ClientInputsDigest()
	if e != nil {
		t.Fatal(e)
	}
	body, e = json.Marshal(manifest)
	if e != nil {
		t.Fatal(e)
	}
	mustWrite(t, filepath.Join(directory, releaseverify.ManifestName), body)
	mustWrite(t, filepath.Join(directory, releaseverify.ManifestSignatureName), ed25519.Sign(key, body))
	mustWrite(t, filepath.Join(directory, manifest.ReleaseNotes.Name), []byte("synthetic fixture"))
	registry, e := releaseverify.LoadPinnedAuthorityRegistry(registryPath, pin, time.Now)
	if e != nil {
		t.Fatal(e)
	}
	v, e := releaseverify.VerifyDirectory(directory, registry)
	if e != nil {
		t.Fatal(e)
	}
	return v
}

func requirementForTest(v releaseverify.VerifiedRelease) Requirement {
	r := Requirement{ManifestSHA256: v.ManifestSHA256, ServerVersion: v.Manifest.Server.Version}
	for _, a := range v.Manifest.Artifacts {
		if a.Kind == "runtime_pack" {
			r.PackSHA256 = a.SHA256
		}
	}
	return r
}
