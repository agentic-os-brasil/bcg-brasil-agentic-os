package installtx

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

func TestQualifiedPublicationCanceledBeforeLockKeepsForeignState(t *testing.T) {
	managed := t.TempDir()
	lock := filepath.Join(managed, ".activation.lock")
	expected := []byte("synthetic-foreign-owner\n")
	if err := os.WriteFile(lock, expected, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, guard := range map[string]func(func() error) error{
		"original": func(use func() error) error {
			return GuardQualifiedCurrentContext(ctx, managed, filepath.Join(managed, "state"), ReleaseBinding{}, time.Now(), use)
		},
		"current-state-callback": func(use func() error) error {
			return GuardQualifiedCurrentStateContext(ctx, managed, filepath.Join(managed, "state"), ReleaseBinding{}, time.Now(), func(State, func(ReleaseBinding, time.Time) error) error { return use() })
		},
	} {
		t.Run(name, func(t *testing.T) {
			published := false
			err := guard(func() error { published = true; return nil })
			if !errors.Is(err, context.Canceled) || published {
				t.Fatalf("canceled publication: called=%t err=%v", published, err)
			}
			got, err := os.ReadFile(lock)
			if err != nil || string(got) != string(expected) {
				t.Fatal("cancellation removed or changed another owner's lock")
			}
		})
	}
}

// This characterizes the existing late checks before the native startup
// optimization. The signed source can expire while current packet files are
// inspected; a successful guard-start check must not authorize later launch.
func TestQualifiedSourceLateFreshnessCharacterization(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	generation := releasecontract.Generation{
		SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol,
		Issuer:   releasecontract.Issuer{ID: "engineering", KeyID: "fixture"},
		Source:   releasecontract.SourceTuple{SourceID: "synthetic", Channel: "canary", OS: "darwin", Arch: "arm64"},
		Sequence: 1, PublishedAt: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339),
		Release: "0.2.0", ManifestSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ClientInputsSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Recalls:            []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract,
	}
	body, err := json.Marshal(generation)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := releaseverify.VerifyGeneration(body, ed25519.Sign(private, body), releaseverify.StaticRegistry{"maestro/engineering/fixture": public})
	if err != nil {
		t.Fatal(err)
	}
	if err := verified.Generation.CheckTime(now, now); err != nil {
		t.Fatalf("fresh signed source refused: %v", err)
	}
	if err := verified.Generation.CheckTime(now.Add(2*time.Minute), now); err == nil {
		t.Fatal("guard-start freshness incorrectly survives source expiry during packet inspection")
	}
	control := &SourceControl{MinimumReaderContract: releasecontract.SyncedSourceContract, Recalls: []releasecontract.ReleaseRecall{}}
	binding := ReleaseBinding{ManifestSHA256: generation.ManifestSHA256, ClientInputsSHA256: generation.ClientInputsSHA256}
	if err := releaseEligible(control, generation.Release, binding); err != nil {
		t.Fatal(err)
	}
	control.Recalls = append(control.Recalls, releasecontract.ReleaseRecall{Release: generation.Release, ManifestSHA256: generation.ManifestSHA256})
	if err := releaseEligible(control, generation.Release, binding); err == nil {
		t.Fatal("late source eligibility accepted the exact recalled release")
	}
}

// These are signed synthetic source-policy checks, never NativeHost admission or
// an installed-client positive fixture. Time advances deterministically after
// the guard-start check, with no sleep or change to a native hook deadline.
func TestQualifiedCurrentRecheckRetainsLateFences(t *testing.T) {

	for _, name := range []string{"expires-during-inspection", "signing-key-expires", "record-manifest-changed", "record-inputs-changed", "partial-pins", "empty-pins", "canceled", "after-callback", "recalled", "capacity-exhausted"} {
		t.Run(name, func(t *testing.T) {
			state, registry, clock := qualifiedCurrentTestSource(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var active atomic.Bool
			active.Store(true)
			if name == "recalled" {
				state.Source.Recalls = append(state.Source.Recalls, releasecontract.ReleaseRecall{Release: state.Release, ManifestSHA256: state.Binding.ManifestSHA256})
			}
			if name == "capacity-exhausted" {
				state.Source.RecallCapacityExhausted = true
			}
			check := qualifiedCurrentRecheck(ctx, state, registry, &active)
			if name != "recalled" && name != "capacity-exhausted" {
				if err := check(state.Binding, *clock); err != nil {
					t.Fatalf("guard-start source refused: %v", err)
				}
			}
			expected := state.Binding
			switch name {
			case "expires-during-inspection":
				*clock = clock.Add(2 * time.Minute)
			case "signing-key-expires":
				*clock = clock.Add(2 * time.Hour)
			case "record-manifest-changed":
				expected.ManifestSHA256 = strings.Repeat("c", 64)
			case "record-inputs-changed":
				expected.ClientInputsSHA256 = strings.Repeat("c", 64)
			case "partial-pins":
				expected.ManifestSHA256 = ""
			case "empty-pins":
				expected = ReleaseBinding{}
			case "canceled":
				cancel()
			case "after-callback":
				active.Store(false)
			}
			err := check(expected, *clock)
			if err == nil {
				t.Fatal("late source/selection/cancellation guard accepted invalid launch")
			}
			if name == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", err)
			}
			if name == "signing-key-expires" && !strings.Contains(err.Error(), "key is not approved") {
				t.Fatalf("signing authority not rechecked at lookup: %v", err)
			}
		})
	}
}

func qualifiedCurrentTestSource(t *testing.T) (State, releaseverify.AuthorityRegistry, *time.Time) {
	t.Helper()
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	clock := now
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	document := map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "engineering", "key_id": "fixture", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": now.Add(-time.Hour).Format(time.RFC3339), "valid_until": now.Add(time.Hour).Format(time.RFC3339)}}}
	registryBody, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := releaseverify.ParseAuthorityRegistry(bytes.NewReader(registryBody), func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	generation := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: releasecontract.Issuer{ID: "engineering", KeyID: "fixture"}, Source: releasecontract.SourceTuple{SourceID: "synthetic", Channel: "canary", OS: "darwin", Arch: "arm64"}, Sequence: 1, PublishedAt: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339), Release: "0.2.0", ManifestSHA256: strings.Repeat("a", 64), ClientInputsSHA256: strings.Repeat("b", 64), Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}
	body, err := json.Marshal(generation)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := releaseverify.VerifyGeneration(body, ed25519.Sign(private, body), registry)
	if err != nil {
		t.Fatal(err)
	}
	state := State{SchemaVersion: 4, Release: generation.Release, Binding: ReleaseBinding{ManifestSHA256: generation.ManifestSHA256, ClientInputsSHA256: generation.ClientInputsSHA256}, Source: &SourceControl{Locator: t.TempDir(), SourceBinding: SourceBinding{Tuple: generation.Source, Sequence: generation.Sequence, GenerationSHA256: verified.SHA256}, Body: verified.Body, Signature: verified.Signature, Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract, ObservedAt: now}}
	return state, registry, &clock
}

func TestQualifiedCurrentMetadataFenceRefusesChangedState(t *testing.T) {
	state, _, clock := qualifiedCurrentTestSource(t)
	data, managed := t.TempDir(), t.TempDir()
	state.ManagedRoot, state.Profile, state.Channel = managed, releasecontract.ModernProfile, "canary"
	state.CLIVersion, state.BundleVersion = "0.2.0", "0.2.0"
	state.TargetOS, state.TargetArch = "darwin", "arm64"
	state.ActivatedAt = *clock
	state.Server = ExecutableIdentity{Version: "0.2.0", Protocol: releasecontract.ServerProtocol, ExecutionContract: releasecontract.SyncedSourceContract, CompatibleBundle: ">=0.2.0 <0.3.0", CompatibleServer: ">=0.2.0 <0.3.0", EmbeddedInputsSHA256: strings.Repeat("a", 64), ArtifactName: "server", SHA256: strings.Repeat("b", 64), Size: 1}
	state.Binding.ManifestJSON = "{}"
	state.Binding.ManifestSignature = strings.Repeat("a", 88)
	state.Binding.BundleSHA256 = strings.Repeat("c", 64)
	state.Binding.BundleSize = 1
	state.Binding.BundleArchive = filepath.Join(data, "updates", "bundle.tar.gz")
	if err := WriteState(data, state); err != nil {
		t.Fatal(err)
	}
	digest := StateDigest(state)
	if err := recheckQualifiedCurrentState(data, managed, digest); err != nil {
		t.Fatalf("unchanged current metadata refused: %v", err)
	}
	state.Source.Recalls = append(state.Source.Recalls, releasecontract.ReleaseRecall{Release: state.Release, ManifestSHA256: state.Binding.ManifestSHA256})
	if err := WriteState(data, state); err != nil {
		t.Fatal(err)
	}
	if err := recheckQualifiedCurrentState(data, managed, digest); err == nil {
		t.Fatal("late current metadata replacement escaped the held-state fence")
	}
}
