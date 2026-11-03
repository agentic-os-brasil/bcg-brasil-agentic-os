package installtx

import (
	"bytes"
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

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

// This is a public persistence/cryptographic byte-budget proof, not activation.
// It deliberately uses the parser's worst admitted metadata envelope: raw '<'
// in an authenticated filename expands sixfold when retained inside state JSON.
// No executable or filesystem payload is declared healthy by this test.
func TestSourceCustodyWorstAdmittedExactByteEnvelopesRoundTrip(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	registry := releaseverify.StaticRegistry{"maestro/engineering/budget": public}
	data := t.TempDir()
	managed := filepath.Join(t.TempDir(), "managed")
	now := time.Now().UTC().Truncate(time.Second)
	makeRelease := func(version string) (releasecontract.Manifest, ReleaseBinding, ExecutableIdentity) {
		m := releasecontract.Manifest{SchemaVersion: 2, Product: "maestro", Release: version, Channel: "canary", Issuer: releasecontract.Issuer{ID: "engineering", KeyID: "budget"}, Server: releasecontract.ServerComponent{Version: version, Protocol: releasecontract.ServerProtocol, ExecutionContract: releasecontract.SyncedSourceContract, CompatibleBundle: ">=0.0.0 <1.0.0", EmbeddedInputsSHA256: strings.Repeat("a", 64)}, Bundle: releasecontract.BundleComponent{Version: version, CompatibleServer: ">=0.0.0 <1.0.0"}, Delivery: releasecontract.DeliveryComponent{Profile: releasecontract.ModernProfile, InstallerVersion: "0.3.7", InstallerProtocol: releasecontract.InstallerProtocol}, Artifacts: []releasecontract.Artifact{{Kind: "mcp_server", OS: "darwin", Arch: "arm64", Name: "maestro-mcp_" + version + "_darwin_arm64", Size: 1, SHA256: strings.Repeat("b", 64), SignatureRef: "server_" + version + ".sig"}, {Kind: "bundle", OS: "any", Arch: "any", Name: "maestro-base_" + version + ".tar.gz", Size: 1, SHA256: strings.Repeat("c", 64), SignatureRef: "bundle_" + version + ".sig"}}, Migrations: []releasecontract.Migration{}, ReleaseNotes: releasecontract.ReleaseNotes{Name: "notes_" + version + ".md", SHA256: strings.Repeat("d", 64)}}
		m.Delivery.ClientInputsSHA256, err = m.ClientInputsDigest()
		if err != nil {
			t.Fatal(err)
		}
		encode := func() []byte {
			var b bytes.Buffer
			e := json.NewEncoder(&b)
			e.SetEscapeHTML(false)
			if err := e.Encode(m); err != nil {
				t.Fatal(err)
			}
			body := bytes.TrimSuffix(b.Bytes(), []byte{'\n'})
			// Manifest's custom marshaler escapes HTML internally. Restore the
			// equivalent valid exact signed JSON bytes to exercise the maximum
			// state-string expansion rather than already escaped publisher bytes.
			for escaped, raw := range map[string]string{`\u003c`: "<", `\u003e`: ">", `\u0026`: "&"} {
				body = bytes.ReplaceAll(body, []byte(escaped), []byte(raw))
			}
			return body
		}
		body := encode()
		padding := releasecontract.MaximumSyncedMetadataBytes - len(body)
		if padding <= 0 {
			t.Fatal("manifest fixture exceeds exact admission envelope")
		}
		m.ReleaseNotes.Name = "notes_" + version + strings.Repeat("<", padding) + ".md"
		body = encode()
		if len(body) != releasecontract.MaximumSyncedMetadataBytes {
			t.Fatalf("manifest is not exact maximum: %d", len(body))
		}
		signature := ed25519.Sign(private, body)
		verified, _, err := releaseverify.VerifyManifest(body, signature, registry)
		if err != nil {
			t.Fatal(err)
		}
		binding := ReleaseBinding{ManifestSHA256: fmt.Sprintf("%x", sha256.Sum256(body)), ClientInputsSHA256: verified.Delivery.ClientInputsSHA256, ManifestJSON: string(body), ManifestSignature: base64.StdEncoding.EncodeToString(signature), BundleSHA256: verified.Artifacts[1].SHA256, BundleSize: verified.Artifacts[1].Size, BundleArchive: filepath.Join(data, "updates", "bundle-"+version+".tar.gz")}
		return verified, binding, serverForManifest(verified, verified.Artifacts[0])
	}
	current, currentBinding, currentServer := makeRelease("0.2.0")
	previous, previousBinding, previousServer := makeRelease("0.1.0")
	recalls := make([]releasecontract.ReleaseRecall, releasecontract.MaximumSourceRecalls)
	for i := range recalls {
		recalls[i] = releasecontract.ReleaseRecall{Release: fmt.Sprintf("0.0.%d", i), ManifestSHA256: fmt.Sprintf("%064x", i)}
	}
	g := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: current.Issuer, Source: releasecontract.SourceTuple{SourceID: "budget", Channel: "canary", OS: "darwin", Arch: "arm64"}, Sequence: 256, PublishedAt: now.Add(-time.Hour).Format(time.RFC3339), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339), Release: current.Release, ManifestSHA256: currentBinding.ManifestSHA256, ClientInputsSHA256: currentBinding.ClientInputsSHA256, Recalls: recalls, MinimumReaderContract: releasecontract.SyncedSourceContract}
	body, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	padding := releasecontract.MaximumSyncedMetadataBytes - len(body)
	if padding < 0 {
		t.Fatal("source fixture exceeds maximum envelope")
	}
	body = append(body, bytes.Repeat([]byte{'\t'}, padding)...)
	signature := ed25519.Sign(private, body)
	verified, err := releaseverify.VerifyGeneration(body, signature, registry)
	if err != nil {
		t.Fatal(err)
	}
	state := State{SchemaVersion: 4, Profile: releasecontract.ModernProfile, ManagedRoot: managed, Release: current.Release, Channel: "canary", CLIVersion: current.Server.Version, BundleVersion: current.Bundle.Version, TargetOS: "darwin", TargetArch: "arm64", ActivatedAt: now, Server: currentServer, Binding: currentBinding, Source: &SourceControl{Locator: filepath.Join(data, "source"), SourceBinding: SourceBinding{Tuple: g.Source, Sequence: g.Sequence, GenerationSHA256: verified.SHA256}, Body: verified.Body, Signature: verified.Signature, Recalls: recalls, MinimumReaderContract: g.MinimumReaderContract, ObservedAt: now}, Previous: &Snapshot{Profile: releasecontract.ModernProfile, Server: previousServer, Binding: previousBinding, Release: previous.Release, Channel: "canary", CLIVersion: previous.Server.Version, BundleVersion: previous.Bundle.Version, TargetOS: "darwin", TargetArch: "arm64", CLIBackup: filepath.Join(managed, "recovery", "previous")}}
	if err := WriteState(data, state); err != nil {
		t.Fatalf("maximum supported current/previous/source envelopes cannot be persisted: %v", err)
	}
	serialized, err := os.ReadFile(statePath(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(serialized) >= 1<<20 {
		t.Fatalf("worst admitted envelopes exhaust private state owner: %d", len(serialized))
	}
	readback, err := ReadSourceState(data, managed)
	if err != nil {
		t.Fatal(err)
	}
	if readback.Binding != currentBinding || readback.Previous == nil || readback.Previous.Binding != previousBinding || !bytes.Equal(readback.Source.Body, verified.Body) || len(readback.Source.Recalls) != 256 {
		t.Fatal("maximum envelope roundtrip changed exact signed bytes or custody")
	}
	if _, err := releaseverify.VerifyGeneration(readback.Source.Body, readback.Source.Signature, registry); err != nil {
		t.Fatal(err)
	}
	for _, binding := range []ReleaseBinding{readback.Binding, readback.Previous.Binding} {
		sig, err := base64.StdEncoding.DecodeString(binding.ManifestSignature)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := releaseverify.VerifyManifest([]byte(binding.ManifestJSON), sig, registry); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("current_signed_manifest_bytes=%d previous_signed_manifest_bytes=%d source_signed_body_bytes=%d cumulative_recalls=%d state_json_bytes=%d remaining_owner_bytes=%d", len(currentBinding.ManifestJSON), len(previousBinding.ManifestJSON), len(verified.Body), len(recalls), len(serialized), (1<<20)-len(serialized))
}
