package installer

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

func experimentalRoleFixture(t *testing.T) (ExperimentalSeedRecord, []byte, ed25519.PrivateKey, time.Time) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	registry := []byte(fmt.Sprintf(`{"schema_version":1,"product":"maestro","authorities":[{"issuer":"maestro-engineering-pilot-20261117","key_id":"pilot-20261117","algorithm":"ed25519","public_key":"%s","status":"active","valid_from":"2026-11-17T00:00:00Z","valid_until":"2026-11-30T00:00:00Z"}]}`, base64.StdEncoding.EncodeToString(pub)))
	role := ExperimentalSeedRecord{SchemaVersion: 1, Product: "maestro", Purpose: ExperimentalSeedPurpose, Protocol: ExperimentalSeedProtocol, Profile: ExperimentalSeedProfile, RegistrySHA256: fmt.Sprintf("%x", sha256.Sum256(registry)), InstallerVersion: "0.1.51", InstallerSHA256: strings.Repeat("a", 64), TargetOS: "darwin", TargetArch: "arm64", Source: releasecontract.SourceTuple{SourceID: "team-pilot", Channel: "canary", OS: "darwin", Arch: "arm64"}, Issuer: "maestro-engineering-pilot-20261117", KeyID: "pilot-20261117", NotBefore: "2026-11-17T00:00:00Z", ExpiresAt: "2026-11-29T00:00:00Z"}
	for _, v := range []string{"0.1.51", "0.1.52"} {
		role.Releases = append(role.Releases, ExperimentalSeedRelease{Release: v, ManifestSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(v))), ClientInputsSHA256: strings.Repeat("b", 64), ServerSHA256: strings.Repeat("c", 64), BundleSHA256: strings.Repeat("d", 64), RuntimeSHA256: strings.Repeat("e", 64)})
	}
	return role, registry, key, now
}

func verifyExperimentalRoleFixture(t *testing.T, role ExperimentalSeedRecord, registry []byte, key ed25519.PrivateKey, clock func() time.Time) *VerifiedExperimentalSeed {
	t.Helper()
	body, err := json.Marshal(role)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyExperimentalSeedAdmission(body, ed25519.Sign(key, body), registry, role.RegistrySHA256, clock)
	if err != nil {
		t.Fatal(err)
	}
	return verified
}

func TestExperimentalSeedAdmissionOwnsInputsAndRechecksExpiry(t *testing.T) {
	role, registry, key, now := experimentalRoleFixture(t)
	body, _ := json.Marshal(role)
	signature := ed25519.Sign(key, body)
	v, err := VerifyExperimentalSeedAdmission(body, signature, registry, role.RegistrySHA256, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	expected := v.Identity()
	for _, b := range [][]byte{body, signature, registry} {
		b[0] ^= 1
	}
	exported, exportedSignature := v.RecordBytes()
	exported[0] ^= 1
	exportedSignature[0] ^= 1
	identity := v.Identity()
	identity.Source.SourceID = "other"
	if v.Identity() != expected {
		t.Fatal("external copies mutated admitted identity")
	}
	if err := v.Revalidate(); err != nil {
		t.Fatalf("input alias changed admission: %v", err)
	}
	if err := v.VerifySource(expected.Source); err != nil {
		t.Fatal(err)
	}
	if err := v.VerifySource(identity.Source); err == nil {
		t.Fatal("different source admitted")
	}
	for _, r := range role.Releases {
		if err := v.VerifyReleaseIdentity(r.ManifestSHA256, r.ClientInputsSHA256); err != nil {
			t.Fatal(err)
		}
	}
	if err := v.VerifyReleaseIdentity(role.Releases[0].ManifestSHA256, strings.Repeat("f", 64)); err == nil {
		t.Fatal("mixed release inputs accepted")
	}
	now, _ = time.Parse(time.RFC3339, role.ExpiresAt)
	if err := v.Revalidate(); err == nil {
		t.Fatal("admission survived exact role expiry")
	}
	if err := v.VerifySource(expected.Source); err == nil {
		t.Fatal("expired role permitted source use")
	}
	var zero VerifiedExperimentalSeed
	if zero.Revalidate() == nil || (*VerifiedExperimentalSeed)(nil).Revalidate() == nil {
		t.Fatal("unconstructed admission accepted")
	}
}

func TestExperimentalSeedAdmissionRegistryPurposeWindowAndRevocation(t *testing.T) {
	role, registry, key, now := experimentalRoleFixture(t)
	for name, mutate := range map[string]func(map[string]any){
		"revoked":               func(entry map[string]any) { entry["status"] = "revoked"; entry["revoked_at"] = "2026-11-17T06:00:00Z" },
		"long authority":        func(entry map[string]any) { entry["valid_from"] = "2026-11-01T00:00:00Z" },
		"role before authority": func(entry map[string]any) { entry["valid_from"] = "2026-11-17T01:00:00Z" },
		"role beyond authority": func(entry map[string]any) { entry["valid_until"] = "2026-11-28T00:00:00Z" },
	} {
		t.Run(name, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal(registry, &document); err != nil {
				t.Fatal(err)
			}
			mutate(document["authorities"].([]any)[0].(map[string]any))
			changedRegistry, _ := json.Marshal(document)
			changedRole := role
			changedRole.RegistrySHA256 = fmt.Sprintf("%x", sha256.Sum256(changedRegistry))
			body, _ := json.Marshal(changedRole)
			if _, err := VerifyExperimentalSeedAdmission(body, ed25519.Sign(key, body), changedRegistry, changedRole.RegistrySHA256, func() time.Time { return now }); err == nil {
				t.Fatal("unavailable or widened authority accepted")
			}
		})
	}
}

func TestExperimentalSeedAdmissionPinsActualInstallerBytes(t *testing.T) {
	role, registry, key, now := experimentalRoleFixture(t)
	path := filepath.Join(t.TempDir(), "maestro-install")
	body := []byte("synthetic installer bytes; no executable qualification")
	if err := os.WriteFile(path, body, 0700); err != nil {
		t.Fatal(err)
	}
	role.InstallerSHA256 = fmt.Sprintf("%x", sha256.Sum256(body))
	v := verifyExperimentalRoleFixture(t, role, registry, key, func() time.Time { return now })
	if err := v.VerifyInstaller(path, role.InstallerVersion, role.TargetOS, role.TargetArch); err != nil {
		t.Fatal(err)
	}
	if err := v.VerifyInstaller(path, "0.1.99", role.TargetOS, role.TargetArch); err == nil {
		t.Fatal("other version admitted")
	}
	if err := v.VerifyInstaller(path, role.InstallerVersion, "windows", role.TargetArch); err == nil {
		t.Fatal("other target admitted")
	}
	if err := os.WriteFile(path, []byte("changed synthetic installer bytes"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := v.VerifyInstaller(path, role.InstallerVersion, role.TargetOS, role.TargetArch); err == nil {
		t.Fatal("changed executable admitted")
	}
}

func TestExperimentalSeedAdmissionChecksVerifiedReleaseBindings(t *testing.T) {
	role, registry, key, now := experimentalRoleFixture(t)
	v := verifyExperimentalRoleFixture(t, role, registry, key, func() time.Time { return now })
	r := role.Releases[0]
	// This is a typed metadata fixture at an already-verified boundary, not
	// evidence that a payload, installer or native host was actually qualified.
	release := releaseverify.VerifiedRelease{ManifestSHA256: r.ManifestSHA256, PublicKey: key.Public().(ed25519.PublicKey), Manifest: releasecontract.Manifest{
		SchemaVersion: 2, Product: "maestro", Release: r.Release, Channel: "canary", Issuer: releasecontract.Issuer{ID: role.Issuer, KeyID: role.KeyID},
		Server:   releasecontract.ServerComponent{ExecutionContract: releasecontract.SyncedSourceContract},
		Delivery: releasecontract.DeliveryComponent{InstallerVersion: role.InstallerVersion, ClientInputsSHA256: r.ClientInputsSHA256},
		Artifacts: []releasecontract.Artifact{
			{Kind: "mcp_server", OS: role.TargetOS, Arch: role.TargetArch, SHA256: r.ServerSHA256},
			{Kind: "bundle", OS: "any", Arch: "any", SHA256: r.BundleSHA256},
			{Kind: "runtime_pack", Adapter: "docling", OS: role.TargetOS, Arch: role.TargetArch, SHA256: r.RuntimeSHA256},
		},
	}}
	if err := v.VerifyRelease(release); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*releaseverify.VerifiedRelease){
		"foreign key":    func(r *releaseverify.VerifiedRelease) { r.PublicKey = make([]byte, ed25519.PublicKeySize) },
		"third manifest": func(r *releaseverify.VerifiedRelease) { r.ManifestSHA256 = strings.Repeat("f", 64) },
		"mixed client": func(r *releaseverify.VerifiedRelease) {
			r.Manifest.Delivery.ClientInputsSHA256 = strings.Repeat("f", 64)
		},
		"wrong release version":    func(r *releaseverify.VerifiedRelease) { r.Manifest.Release = "0.1.99" },
		"wrong issuer":             func(r *releaseverify.VerifiedRelease) { r.Manifest.Issuer.ID = "production" },
		"wrong installer":          func(r *releaseverify.VerifiedRelease) { r.Manifest.Delivery.InstallerVersion = "0.1.99" },
		"old execution contract":   func(r *releaseverify.VerifiedRelease) { r.Manifest.Server.ExecutionContract = "" },
		"production channel":       func(r *releaseverify.VerifiedRelease) { r.Manifest.Channel = "stable" },
		"different server":         func(r *releaseverify.VerifiedRelease) { r.Manifest.Artifacts[0].SHA256 = strings.Repeat("f", 64) },
		"different bundle":         func(r *releaseverify.VerifiedRelease) { r.Manifest.Artifacts[1].SHA256 = strings.Repeat("f", 64) },
		"different runtime":        func(r *releaseverify.VerifiedRelease) { r.Manifest.Artifacts[2].SHA256 = strings.Repeat("f", 64) },
		"missing runtime":          func(r *releaseverify.VerifiedRelease) { r.Manifest.Artifacts = r.Manifest.Artifacts[:2] },
		"different runtime target": func(r *releaseverify.VerifiedRelease) { r.Manifest.Artifacts[2].Arch = "amd64" },
		"ambiguous runtime": func(r *releaseverify.VerifiedRelease) {
			r.Manifest.Artifacts = append(r.Manifest.Artifacts, r.Manifest.Artifacts[2])
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := release
			changed.Manifest.Artifacts = append([]releasecontract.Artifact(nil), release.Manifest.Artifacts...)
			change(&changed)
			if err := v.VerifyRelease(changed); err == nil {
				t.Fatal("different or ambiguous release admitted")
			}
		})
	}
}

func TestExperimentalSeedAdmissionClosedRoleAndAuthority(t *testing.T) {
	role, registry, key, now := experimentalRoleFixture(t)
	body, _ := json.Marshal(role)
	verified, err := VerifyExperimentalSeedAdmission(body, ed25519.Sign(key, body), registry, role.RegistrySHA256, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if verified.Identity().Evidence != "engineering_only" || verified.Identity().AdmissionSHA256 != fmt.Sprintf("%x", sha256.Sum256(body)) {
		t.Fatal("engineering identity lost")
	}
	for name, change := range map[string]func(*ExperimentalSeedRecord){
		"runtime purpose":     func(r *ExperimentalSeedRecord) { r.Purpose = "engineering-runtime-pilot-v1" },
		"production profile":  func(r *ExperimentalSeedRecord) { r.Profile = "strict" },
		"wrong protocol":      func(r *ExperimentalSeedRecord) { r.Protocol = "maestro-install-seed-v1" },
		"wrong product":       func(r *ExperimentalSeedRecord) { r.Product = "other" },
		"wrong source target": func(r *ExperimentalSeedRecord) { r.Source.OS = "windows" },
		"production channel":  func(r *ExperimentalSeedRecord) { r.Source.Channel = "stable" },
		"missing release":     func(r *ExperimentalSeedRecord) { r.Releases = r.Releases[:1] },
		"duplicate release":   func(r *ExperimentalSeedRecord) { r.Releases[1] = r.Releases[0] },
		"missing runtime":     func(r *ExperimentalSeedRecord) { r.Releases[0].RuntimeSHA256 = "" },
		"expanded validity":   func(r *ExperimentalSeedRecord) { r.ExpiresAt = "2026-12-01T00:00:00Z" },
		"expired":             func(r *ExperimentalSeedRecord) { r.ExpiresAt = "2026-11-17T11:59:59Z" },
		"future":              func(r *ExperimentalSeedRecord) { r.NotBefore = "2026-11-18T00:00:00Z" },
		"foreign registry":    func(r *ExperimentalSeedRecord) { r.RegistrySHA256 = strings.Repeat("f", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			var changed ExperimentalSeedRecord
			if err := json.Unmarshal(body, &changed); err != nil {
				t.Fatal(err)
			}
			change(&changed)
			b, _ := json.Marshal(changed)
			if _, err := VerifyExperimentalSeedAdmission(b, ed25519.Sign(key, b), registry, role.RegistrySHA256, func() time.Time { return now }); err == nil {
				t.Fatal("invalid signed role admitted")
			}
		})
	}
	for name, b := range map[string][]byte{
		"unknown":   bytes.Replace(body, []byte(`"schema_version":1`), []byte(`"schema_version":1,"callback":"trust"`), 1),
		"duplicate": bytes.Replace(body, []byte(`"schema_version":1`), []byte(`"schema_version":1,"schema_version":1`), 1),
		"trailing":  append(append([]byte{}, body...), []byte(` {}`)...),
		"oversize":  bytes.Repeat([]byte(" "), 128<<10+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := VerifyExperimentalSeedAdmission(b, ed25519.Sign(key, b), registry, role.RegistrySHA256, func() time.Time { return now }); err == nil {
				t.Fatal("unclosed role accepted")
			}
		})
	}
	if _, err := VerifyExperimentalSeedAdmission(body, make([]byte, ed25519.SignatureSize), registry, role.RegistrySHA256, func() time.Time { return now }); err == nil {
		t.Fatal("unsigned role accepted")
	}
	if _, err := VerifyExperimentalSeedAdmission(body, ed25519.Sign(key, body), registry, role.RegistrySHA256, func() time.Time { return now.Add(20 * 24 * time.Hour) }); err == nil {
		t.Fatal("expired authority accepted")
	}
}
