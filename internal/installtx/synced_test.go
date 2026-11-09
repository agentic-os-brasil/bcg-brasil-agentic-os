package installtx

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A supported new writer must never erase protected source custody by writing
// an older snapshot. This is deliberately a public writer test, not an assertion
// about the unchanged historical low-level writer in an old binary.
func TestWriteStateCannotDowngradeProtectedSourceCustody(t *testing.T) {
	data := t.TempDir()
	managed := filepath.Join(t.TempDir(), "managed")
	path := statePath(data)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	protected := []byte(`{"schema_version":4,"custody":"must not be overwritten"}`)
	if err := os.WriteFile(path, protected, 0600); err != nil {
		t.Fatal(err)
	}
	stale := State{SchemaVersion: 3, Profile: "mcp-local-v1", ManagedRoot: managed,
		Release: "0.2.0", Channel: "canary", CLIVersion: "0.2.0", BundleVersion: "0.2.0",
		TargetOS: "darwin", TargetArch: "arm64", ActivatedAt: time.Now().UTC(),
		Server: ExecutableIdentity{Version: "0.2.0", Protocol: "maestro-mcp-identity-v1", CompatibleBundle: ">=0.2.0 <0.3.0", CompatibleServer: ">=0.2.0 <0.3.0", EmbeddedInputsSHA256: strings.Repeat("a", 64), ArtifactName: "maestro-mcp_0.2.0_darwin_arm64", SHA256: strings.Repeat("b", 64), Size: 1}}
	if err := WriteState(data, stale); err == nil {
		t.Fatal("supported writer erased protected source custody with a stale schema3 snapshot")
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != string(protected) {
		t.Fatalf("protected state changed: %q %v", actual, err)
	}
}

func TestSupportedSourceWriterPreservesHighWaterRecallAndEnrollment(t *testing.T) {
	data := t.TempDir()
	managed := filepath.Join(t.TempDir(), "managed")
	now := time.Now().UTC().Truncate(time.Second)
	g := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: releasecontract.Issuer{ID: "engineering", KeyID: "fixture"}, Source: releasecontract.SourceTuple{SourceID: "pilot", Channel: "canary", OS: "darwin", Arch: "arm64"}, Sequence: 10, PublishedAt: now.Add(-time.Hour).Format(time.RFC3339), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339), Release: "0.2.0", ManifestSHA256: strings.Repeat("a", 64), ClientInputsSHA256: strings.Repeat("b", 64), Recalls: []releasecontract.ReleaseRecall{{Release: "0.1.0", ManifestSHA256: strings.Repeat("c", 64)}}, MinimumReaderContract: releasecontract.SyncedSourceContract}
	control := func(g releasecontract.Generation) *SourceControl {
		body, _ := json.Marshal(g)
		return &SourceControl{Locator: filepath.Join(data, "synced"), SourceBinding: SourceBinding{Tuple: g.Source, Sequence: g.Sequence, GenerationSHA256: fmt.Sprintf("%x", sha256.Sum256(body))}, Body: body, Signature: make([]byte, 64), Recalls: append([]releasecontract.ReleaseRecall{}, g.Recalls...), MinimumReaderContract: g.MinimumReaderContract, ObservedAt: now}
	}
	s := State{SchemaVersion: 4, ManagedRoot: managed, Profile: releasecontract.ModernProfile, Release: "0.2.0", Channel: "canary", CLIVersion: "0.2.0", BundleVersion: "0.2.0", TargetOS: "darwin", TargetArch: "arm64", ActivatedAt: now, Server: ExecutableIdentity{Version: "0.2.0", Protocol: releasecontract.ServerProtocol, ExecutionContract: releasecontract.SyncedSourceContract, CompatibleBundle: ">=0.2.0 <0.3.0", CompatibleServer: ">=0.2.0 <0.3.0", EmbeddedInputsSHA256: strings.Repeat("a", 64), ArtifactName: "server", SHA256: strings.Repeat("b", 64), Size: 1}, Binding: ReleaseBinding{ManifestSHA256: strings.Repeat("a", 64), ClientInputsSHA256: strings.Repeat("b", 64), ManifestJSON: "{}", ManifestSignature: strings.Repeat("a", 88), BundleSHA256: strings.Repeat("c", 64), BundleSize: 1, BundleArchive: filepath.Join(data, "updates", "bundle.tar.gz")}, Source: control(g)}
	if err := WriteState(data, s); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(statePath(data))
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*State){
		"replay": func(n *State) { copy := g; copy.Sequence = 9; n.Source = control(copy) },
		"equivocation": func(n *State) {
			copy := g
			copy.ExpiresAt = now.Add(2 * time.Hour).Format(time.RFC3339)
			n.Source = control(copy)
		},
		"forget recall": func(n *State) {
			copy := g
			copy.Sequence = 11
			copy.Recalls = []releasecontract.ReleaseRecall{}
			n.Source = control(copy)
		},
		"reenroll":       func(n *State) { copy := g; copy.Source.SourceID = "other"; n.Source = control(copy) },
		"remove source":  func(n *State) { n.Source = nil },
		"backward clock": func(n *State) { copy := *n.Source; copy.ObservedAt = now.Add(-time.Second); n.Source = &copy },
	} {
		t.Run(name, func(t *testing.T) {
			next := s
			change(&next)
			if err := WriteState(data, next); err == nil {
				t.Fatal("supported writer accepted source custody regression")
			}
			actual, _ := os.ReadFile(statePath(data))
			if string(actual) != string(before) {
				t.Fatal("refused writer changed source custody bytes")
			}
		})
	}
	newer := s
	copy := g
	copy.Sequence = 11
	newer.Source = control(copy)
	if err := WriteState(data, newer); err != nil {
		t.Fatal(err)
	}
	readback, err := ReadSourceState(data, managed)
	if err != nil || readback.Source.Sequence != 11 || len(readback.Source.Recalls) != 1 {
		t.Fatalf("monotonic supported write/readback failed: %#v %v", readback.Source, err)
	}
	// Even a later otherwise valid supported write cannot clear a durable
	// capacity denial. A fresh process must read it and refuse before execution.
	exhausted := readback
	exhaustedControl := *readback.Source
	exhaustedControl.RecallCapacityExhausted = true
	exhausted.Source = &exhaustedControl
	if err := WriteState(data, exhausted); err != nil {
		t.Fatal(err)
	}
	protected, err := os.ReadFile(statePath(data))
	if err != nil {
		t.Fatal(err)
	}
	cleared := exhausted
	later := g
	later.Sequence = 12
	cleared.Source = control(later)
	if err := WriteState(data, cleared); err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("supported writer cleared sticky capacity fence: %v", err)
	}
	if actual, err := os.ReadFile(statePath(data)); err != nil || string(actual) != string(protected) {
		t.Fatal("refused capacity clear changed protected bytes")
	}
	if _, err := ValidateQualifiedLaunch(managed, data, ReleaseBinding{}, now); err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("fresh source reader did not deny capacity exhaustion: %v", err)
	}
	advanced := exhausted
	advanced.Source = control(later)
	advanced.Source.RecallCapacityExhausted = true
	if err := WriteState(data, advanced); err != nil {
		t.Fatal(err)
	}
	readback, err = ReadSourceState(data, managed)
	if err != nil || readback.Source.Sequence != 12 || !readback.Source.RecallCapacityExhausted {
		t.Fatalf("higher metadata lost capacity fence: %#v %v", readback.Source, err)
	}
}

func TestQualifiedSourceReadNeverMigratesLegacyState(t *testing.T) {
	data := t.TempDir()
	managed := filepath.Join(t.TempDir(), "managed")
	path := statePath(data)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"schema_version":1,"release":"0.1.0","channel":"canary","cli_version":"0.1.0","bundle_version":"0.1.0","target_os":"darwin","target_arch":"arm64","activated_at":"2026-11-03T00:00:00Z"}`)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateQualifiedLaunch(managed, data, ReleaseBinding{}, time.Now()); err == nil {
		t.Fatal("legacy state granted source launch")
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != string(body) {
		t.Fatal("read-only launch mutated legacy state")
	}
}
