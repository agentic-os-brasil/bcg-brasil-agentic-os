package acquisition

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const proofInventorySHA = "a8fcda46a1bc4a3f526508032f06cbf6e5dd2af042b14ae18a13edfd3ff78400"
const proofArchiveSHA = "8135ae5bc55a0c258cb9efd818b2d29859ef7e5fdc48ebeebccaae219944d579"
const proofArchiveBytes int64 = 131466640

type publicPreflight struct {
	Scope           string            `json:"scope"`
	Options         Options           `json:"options"`
	Plan            Plan              `json:"plan"`
	Metadata        map[string]string `json:"metadata_sha256"`
	SourceRevision  string            `json:"source_revision"`
	NetworkRequests int               `json:"network_requests"`
}

// TestPublicAcquisitionConstruction is a separately authorized opt-in host
// construction proof, not an alternative application entry. Preflight generates
// only synthetic public metadata and performs read-only Review. Acquisition
// requires the exact externally approved preflight digest. The exported Acquire
// native guard is unchanged; acquired code and the synthetic server never run.
func TestPublicAcquisitionConstruction(t *testing.T) {
	mode := os.Getenv("MAESTRO_ACQUISITION_PROOF_MODE")
	if mode == "" {
		t.Skip("explicit preflight or separately authorized public-acquisition proof required")
	}
	if mode != "preflight" && mode != "acquire" {
		t.Fatal("unknown proof mode")
	}
	root := proofPath(t, "MAESTRO_ACQUISITION_PROOF_ROOT")
	cache := proofPath(t, "MAESTRO_ACQUISITION_PROOF_CACHE")
	inventory := proofPath(t, "MAESTRO_ACQUISITION_PROOF_INVENTORY")
	descriptor, err := docling.DescribeProfile("windows", "amd64", docling.DocumentsProfile)
	if err != nil {
		t.Fatal(err)
	}
	inventoryBody, err := readBounded(inventory, 1<<20)
	if err != nil || digest(inventoryBody) != proofInventorySHA {
		t.Fatal("independently approved inventory unavailable", err)
	}
	var approved struct {
		Inputs     []docling.Input `json:"inputs"`
		InputCount int             `json:"input_count"`
		Total      int64           `json:"total_input_bytes"`
	}
	if err = json.Unmarshal(inventoryBody, &approved); err != nil {
		t.Fatal(err)
	}
	inputs := append([]docling.Input{descriptor.Python}, descriptor.Inputs...)
	total := int64(0)
	for _, input := range inputs {
		total += input.Size
	}
	if !reflect.DeepEqual(inputs, approved.Inputs) || approved.InputCount != len(inputs) || approved.Total != total || len(inputs) != 57 || total != 132964926 {
		t.Fatal("reviewed public input closure changed")
	}
	if mode == "preflight" {
		archive := proofPath(t, "MAESTRO_ACQUISITION_PROOF_ARCHIVE")
		archiveBody, err := readBounded(archive, proofArchiveBytes)
		if err != nil || int64(len(archiveBody)) != proofArchiveBytes || digest(archiveBody) != proofArchiveSHA {
			t.Fatal("approved offline archive unavailable", err)
		}
		if err = mkdirPrivateNew(root); err != nil {
			t.Fatal("new proof root required; existing metadata preserved", err)
		}
		if err = privateDirectory(filepath.Dir(cache), true); err != nil {
			t.Fatal(err)
		}
		options := createPublicProof(t, root, cache, descriptor, archiveBody)
		metadata := proofMetadata(t, options)
		plan, err := Review(context.Background(), options)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(metadata, proofMetadata(t, options)) {
			t.Fatal("Review changed public metadata")
		}
		if _, err = os.Lstat(cache); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("preflight created cache")
		}
		report := publicPreflight{Scope: "transient-fixture-authority; synthetic nonexecuted server/bundle; host construction only", Options: options, Plan: plan, Metadata: metadata, SourceRevision: os.Getenv("MAESTRO_ACQUISITION_PROOF_SOURCE_REVISION"), NetworkRequests: 0}
		body, err := releasecontract.CanonicalJSON(report)
		if err != nil {
			t.Fatal(err)
		}
		if err = scheduler.WriteNewPrivateFile(filepath.Join(root, "preflight.json"), body); err != nil {
			t.Fatal(err)
		}
		t.Logf("preflight_sha256=%s descriptor_sha256=%s plan_sha256=%s valid_until=%s input_count=%d input_bytes=%d cache_absent=true network_requests=0", digest(body), plan.Recipe.DescriptorSHA256, plan.SHA256, plan.ValidUntil, len(inputs), total)
		return
	}
	// The external authorization binds this exact stored public preflight. No
	// private key is retained or needed after the preflight process exits.
	approval := os.Getenv("MAESTRO_ACQUISITION_APPROVED_PREFLIGHT_SHA256")
	preflightBody, err := readBounded(filepath.Join(root, "preflight.json"), 1<<20)
	if !validDigest(approval) || err != nil || digest(preflightBody) != approval {
		t.Fatal("separately approved exact preflight required")
	}
	var preflight publicPreflight
	if err = releasecontract.DecodeStrictJSON(preflightBody, &preflight); err != nil {
		t.Fatal(err)
	}
	if preflight.Options.CacheRoot != cache || preflight.Options.ReleaseDirectory != filepath.Join(root, "carrier", "release") || preflight.Options.RecipeDirectory != filepath.Join(root, "carrier", "recipe") || preflight.Options.RegistryPath != filepath.Join(root, "carrier", "trust", "registry.json") || preflight.Options.ServerPath != filepath.Join(preflight.Options.ReleaseDirectory, "server_0.1.0_windows_amd64.exe") {
		t.Fatal("approved proof paths changed")
	}
	options := preflight.Options
	if !reflect.DeepEqual(preflight.Metadata, proofMetadata(t, options)) {
		t.Fatal("approved public metadata changed")
	}
	reviewedAt, err := time.Parse(time.RFC3339, preflight.Plan.ReviewedAt)
	if err != nil {
		t.Fatal(err)
	}
	current, err := reviewAt(context.Background(), options, reviewedAt, time.Now().UTC())
	if err != nil || current.SHA256 != preflight.Plan.SHA256 {
		t.Fatal("approved review stale or changed", err)
	}
	if _, err = os.Lstat(cache); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("fresh acquisition cache required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	var requests, connections atomic.Int64
	trace := &httptrace.ClientTrace{GetConn: func(string) {
		if connections.Add(1) > 228 {
			cancel()
		}
	}, WroteRequest: func(info httptrace.WroteRequestInfo) {
		if info.Err == nil {
			requests.Add(1)
		}
	}}
	ctx = httptrace.WithClientTrace(ctx, trace)
	if err = privateDirectory(cache, true); err != nil {
		t.Fatal(err)
	}
	held, err := holdPrivateDirectory(cache)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	started := time.Now()
	var verified releaseverify.VerifiedRelease
	var resumed Plan
	err = scheduler.WithPrivateLockContext(ctx, filepath.Join(cache, ".acquisition.lock"), func() error {
		checked, err := reviewAt(ctx, options, reviewedAt, time.Now().UTC())
		if err != nil || checked.SHA256 != preflight.Plan.SHA256 {
			return errors.New("approved review changed before effects")
		}
		if err = privateDirectory(filepath.Join(cache, "blobs"), true); err != nil {
			return err
		}
		stage := filepath.Join(cache, "attempt-0")
		if err = mkdirPrivateNew(stage); err != nil {
			return err
		}
		first := inputs[0]
		pending := filepath.Join(stage, first.Filename+".part")
		if err = stillAuthorized(ctx, options, checked); err != nil {
			return err
		}
		if err = fetchInput(ctx, first, pending, publicClient(first.URL)); err != nil {
			return err
		}
		if err = stillAuthorized(ctx, options, checked); err != nil {
			return err
		}
		if err = publishBlob(cache, first, pending); err != nil {
			return err
		}
		// This is a partial overall acquisition: one complete verified blob exists.
		// The explicit fresh review proves resume without refetching that input.
		resumed, err = Review(ctx, options)
		if err != nil {
			return err
		}
		cached := 0
		for _, state := range resumed.Inputs {
			if state.Cached {
				cached++
			}
		}
		if cached != 1 || !resumed.Inputs[0].Cached || resumed.Attempt != 1 {
			return errors.New("verified first-input resume mismatch")
		}
		verified, err = prepare(ctx, options, resumed)
		return err
	})
	if err != nil {
		t.Fatalf("public acquisition stopped; private attempt preserved; requests=%d: %v", requests.Load(), err)
	}
	if requests.Load() < 57 || requests.Load() > 228 || connections.Load() > 228 {
		t.Fatal("actual HTTPS request budget mismatch")
	}
	if !reflect.DeepEqual(preflight.Metadata, proofMetadata(t, options)) {
		t.Fatal("original public carrier changed")
	}
	for _, input := range inputs {
		cached, err := cachedInput(cache, input)
		if err != nil || !cached {
			t.Fatal("downloaded input failed final authentication", input.Filename, err)
		}
	}
	file, err := scheduler.OpenRegularFile(filepath.Join(verified.Directory, preflight.Plan.Recipe.RuntimeArtifact.Name))
	if err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	file.Close()
	if err != nil || info.Size() != proofArchiveBytes {
		t.Fatal("final archive size mismatch")
	}
	report := map[string]any{"scope": "transient fixture authority; real public download and host assembly only", "approved_preflight_sha256": approval, "descriptor_sha256": resumed.Recipe.DescriptorSHA256, "archive_sha256": proofArchiveSHA, "archive_bytes": proofArchiveBytes, "input_count": len(inputs), "input_bytes": total, "https_requests": requests.Load(), "connection_attempts": connections.Load(), "completed_input_reused": 1, "whole_release_verified": true, "manifest_sha256": verified.ManifestSHA256, "complete_directory": verified.Directory, "elapsed_seconds": time.Since(started).Seconds(), "host_os": runtime.GOOS, "host_arch": runtime.GOARCH, "native_windows_execution": "UNATTESTED", "runtime_or_server_execution": false}
	body, err := releasecontract.CanonicalJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	if err = scheduler.WriteNewPrivateFile(filepath.Join(root, "result.json"), body); err != nil {
		t.Fatal(err)
	}
	t.Log(string(body))
}

func proofPath(t *testing.T, name string) string {
	t.Helper()
	path := os.Getenv(name)
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		t.Fatal("explicit clean absolute proof path required", name)
	}
	return path
}

func createPublicProof(t *testing.T, root, cache string, d docling.ProfileDescriptor, archive []byte) Options {
	t.Helper()
	for _, relative := range []string{"carrier", "carrier/release", "carrier/recipe", "carrier/trust"} {
		if err := mkdirPrivateNew(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
			t.Fatal(err)
		}
	}
	o := Options{ReleaseDirectory: filepath.Join(root, "carrier", "release"), RecipeDirectory: filepath.Join(root, "carrier", "recipe"), RegistryPath: filepath.Join(root, "carrier", "trust", "registry.json"), CacheRoot: cache}
	o.ServerPath = filepath.Join(o.ReleaseDirectory, "server_0.1.0_windows_amd64.exe")
	public, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	issuer := releasecontract.Issuer{ID: "maestro-engineering-pilot-" + now.Format("20060102"), KeyID: "pilot-" + now.Format("20060102")}
	put := func(name string, body []byte) {
		t.Helper()
		if err := scheduler.WriteNewPrivateFile(name, body); err != nil {
			t.Fatal(err)
		}
	}
	canonical := func(value any) []byte {
		body, err := releasecontract.CanonicalJSON(value)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	registry := canonical(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": issuer.ID, "key_id": issuer.KeyID, "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": now.Add(-time.Minute).Format(time.RFC3339), "valid_until": now.Add(6 * time.Hour).Format(time.RFC3339)}}})
	put(o.RegistryPath, registry)
	o.RegistrySHA256 = digest(registry)
	notes := []byte("Synthetic public acquisition construction proof; no server, runtime or client execution.\n")
	m := releasecontract.Manifest{SchemaVersion: 2, Product: "maestro", Release: "0.1.0", Channel: "canary", Issuer: issuer, Server: releasecontract.ServerComponent{Version: "0.1.0", CompatibleBundle: ">=0.1.0 <0.1.1", Protocol: releasecontract.ServerProtocol, EmbeddedInputsSHA256: digest([]byte("synthetic public inventory"))}, Delivery: releasecontract.DeliveryComponent{Profile: releasecontract.ModernProfile, InstallerVersion: "0.1.0", InstallerProtocol: releasecontract.InstallerProtocol}, Bundle: releasecontract.BundleComponent{Version: "0.1.0", CompatibleServer: ">=0.1.0 <0.1.1"}, Migrations: []releasecontract.Migration{}, ReleaseNotes: releasecontract.ReleaseNotes{Name: "notes_0.1.0.md", SHA256: digest(notes)}}
	for _, entry := range []struct {
		kind, name, goos, arch string
		body                   []byte
	}{{"bundle", "base_0.1.0.tar.gz", "any", "any", []byte("synthetic public base bundle; not installable")}, {"mcp_server", "server_0.1.0_windows_amd64.exe", "windows", "amd64", []byte("synthetic public signed server fixture; never execute")}, {"runtime_pack", "runtime_0.1.0_windows_amd64.tar.gz", "windows", "amd64", archive}} {
		artifact := releasecontract.Artifact{Kind: entry.kind, OS: entry.goos, Arch: entry.arch, Name: entry.name, Size: int64(len(entry.body)), SHA256: digest(entry.body), SignatureRef: entry.name + ".sig"}
		if artifact.Kind == "runtime_pack" {
			artifact.Adapter = "docling"
		} else {
			put(filepath.Join(o.ReleaseDirectory, artifact.Name), entry.body)
		}
		put(filepath.Join(o.ReleaseDirectory, artifact.SignatureRef), ed25519.Sign(key, entry.body))
		m.Artifacts = append(m.Artifacts, artifact)
	}
	m.Delivery.ClientInputsSHA256, err = m.ClientInputsDigest()
	if err != nil {
		t.Fatal(err)
	}
	manifest := canonical(m)
	put(filepath.Join(o.ReleaseDirectory, releaseverify.ManifestName), manifest)
	put(filepath.Join(o.ReleaseDirectory, releaseverify.ManifestSignatureName), ed25519.Sign(key, manifest))
	put(filepath.Join(o.ReleaseDirectory, m.ReleaseNotes.Name), notes)
	total := d.Python.Size
	for _, input := range d.Inputs {
		total += input.Size
	}
	recipe := Recipe{SchemaVersion: 1, Purpose: Purpose, Issuer: issuer, ManifestSHA256: digest(manifest), ServerSHA256: m.Artifacts[1].SHA256, OS: d.OS, Arch: d.Arch, Profile: d.Profile, DescriptorSHA256: digest(canonical(d)), InputCount: len(d.Inputs) + 1, TotalInputBytes: total, RuntimeArtifact: m.Artifacts[2], IssuedAt: now.Format(time.RFC3339), ExpiresAt: now.Add(6 * time.Hour).Format(time.RFC3339)}
	body := canonical(recipe)
	put(filepath.Join(o.RecipeDirectory, RecipeName), body)
	put(filepath.Join(o.RecipeDirectory, RecipeSignatureName), ed25519.Sign(key, body))
	return o
}

func proofMetadata(t *testing.T, o Options) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, directory := range []string{o.ReleaseDirectory, o.RecipeDirectory, filepath.Dir(o.RegistryPath)} {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				t.Fatal("unexpected public metadata directory")
			}
			name := filepath.Join(directory, entry.Name())
			body, err := readBounded(name, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			result[name] = digest(body)
		}
	}
	return result
}
