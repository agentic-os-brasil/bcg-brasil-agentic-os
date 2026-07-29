package releasereadiness

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestEvaluateReportsMultidimensionalLocalBoundary(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	report := Evaluate(Options{
		Root:           root,
		ProviderConfig: filepath.Join(root, "bundles", "base", "release", "provider.json"),
		Clock:          func() time.Time { return time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC) },
	})
	if report.State != ReadinessClaim || report.Promotion != "not_permitted" {
		t.Fatalf("report boundary = %#v", report)
	}
	if check := findCheck(report, "contract"); check.State != StatePass {
		t.Fatalf("contract check = %#v", check)
	}
	if check := findCheck(report, "provider_config"); check.State != StateUnavailable {
		t.Fatalf("provider check = %#v", check)
	}
	for _, id := range []string{"signature", "provider_auth", "clean_device"} {
		if check := findCheck(report, id); check.State == StatePass || check.State == StateConfigured {
			t.Fatalf("%s was promoted: %#v", id, check)
		}
	}
	if !sort.StringsAreSorted(report.Blockers) {
		t.Fatalf("blockers are not sorted: %#v", report.Blockers)
	}
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), root) || strings.Contains(string(body), "client_id") {
		t.Fatalf("report leaked path or provider identity: %s", body)
	}
}

func TestEvaluateClassifiesMissingAndMalformedInputsWithoutFallback(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	providerPath := filepath.Join(t.TempDir(), "provider.json")
	if err := os.WriteFile(providerPath, []byte(`{"schema_version":1,"state":"approved"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	report := Evaluate(Options{
		Root:                    root,
		ProviderConfig:          providerPath,
		AuthorityRegistry:       filepath.Join(t.TempDir(), "missing.json"),
		AuthorityRegistrySHA256: strings.Repeat("a", 64),
		Candidate:               filepath.Join(t.TempDir(), "missing-candidate"),
	})
	if check := findCheck(report, "provider_config"); check.State != StateBlocked {
		t.Fatalf("malformed provider was not blocked: %#v", check)
	}
	if check := findCheck(report, "authority_registry"); check.State != StateUnavailable {
		t.Fatalf("missing authority was not unavailable: %#v", check)
	}
	if check := findCheck(report, "candidate_manifest"); check.State != StateUnavailable {
		t.Fatalf("missing candidate was not unavailable: %#v", check)
	}
	if report.State != ReadinessClaim {
		t.Fatalf("report state changed to a generic readiness claim: %#v", report)
	}
}

func TestEvaluateIsStableForIdenticalInputs(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	options := Options{Root: root, Clock: func() time.Time { return time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC) }}
	left, right := Evaluate(options), Evaluate(options)
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("identical inputs produced different reports:\nleft=%#v\nright=%#v", left, right)
	}
}

func TestEvaluateAcceptsPinnedRegistryWithActiveKey(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	registryPath := filepath.Join(t.TempDir(), "registry.json")
	body := []byte(validRegistryJSON("2026-01-01T00:00:00Z", "2027-01-01T00:00:00Z"))
	if err := os.WriteFile(registryPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	report := Evaluate(Options{
		Root:                    root,
		AuthorityRegistry:       registryPath,
		AuthorityRegistrySHA256: hex.EncodeToString(sum[:]),
		Clock:                   func() time.Time { return time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC) },
	})
	if check := findCheck(report, "authority_registry"); check.State != StateConfigured {
		t.Fatalf("active registry = %#v", check)
	}
}

func TestEvaluateBlocksRegistryWithoutActiveKey(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	registryPath := filepath.Join(t.TempDir(), "registry.json")
	body := []byte(validRegistryJSON("2025-01-01T00:00:00Z", "2026-01-01T00:00:00Z"))
	if err := os.WriteFile(registryPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	report := Evaluate(Options{
		Root:                    root,
		AuthorityRegistry:       registryPath,
		AuthorityRegistrySHA256: hex.EncodeToString(sum[:]),
		Clock:                   func() time.Time { return time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC) },
	})
	if check := findCheck(report, "authority_registry"); check.State != StateBlocked {
		t.Fatalf("expired registry = %#v", check)
	}
}

func TestEvaluateBlocksTamperedCandidateManifest(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	candidate := t.TempDir()
	if err := os.WriteFile(filepath.Join(candidate, "release-manifest.json"), []byte(`{"schema_version":1,"surprise":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	report := Evaluate(Options{Root: root, Candidate: candidate})
	if check := findCheck(report, "candidate_manifest"); check.State != StateBlocked {
		t.Fatalf("tampered candidate = %#v", check)
	}
}

func TestEvaluateAcceptsUnsignedCandidatePlaceholderAgainstPinnedRegistry(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	registryPath := filepath.Join(t.TempDir(), "registry.json")
	registryBody := []byte(validRegistryJSON("2026-01-01T00:00:00Z", "2027-01-01T00:00:00Z"))
	if err := os.WriteFile(registryPath, registryBody, 0o600); err != nil {
		t.Fatal(err)
	}
	candidate := t.TempDir()
	manifest := `{"schema_version":1,"product":"maestro","release":"0.1.0","channel":"canary","issuer":{"id":"maestro-release-candidate","key_id":"candidate-unavailable"},"cli":{"version":"0.1.0","compatible_bundle":">=0.1.0 <0.2.0"},"bundle":{"version":"0.1.0","compatible_cli":">=0.1.0 <0.2.0"},"artifacts":[{"kind":"cli","os":"darwin","arch":"arm64","name":"maestro-cli_0.1.0_darwin_arm64","size":1,"sha256":"` + strings.Repeat("a", 64) + `","signature_ref":"maestro-cli_0.1.0_darwin_arm64.sig"},{"kind":"bundle","os":"any","arch":"any","name":"maestro-base_0.1.0.tar.gz","size":1,"sha256":"` + strings.Repeat("b", 64) + `","signature_ref":"maestro-base_0.1.0.tar.gz.sig"}],"migrations":[],"release_notes":{"name":"release-notes-0.1.0.md","sha256":"` + strings.Repeat("c", 64) + `"}}`
	if err := os.WriteFile(filepath.Join(candidate, "release-manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(registryBody)
	report := Evaluate(Options{
		Root:                    root,
		AuthorityRegistry:       registryPath,
		AuthorityRegistrySHA256: hex.EncodeToString(sum[:]),
		Candidate:               candidate,
		Clock:                   func() time.Time { return time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC) },
	})
	if check := findCheck(report, "authority_registry"); check.State != StateConfigured {
		t.Fatalf("unsigned placeholder issuer was treated as an authority mismatch: %#v", check)
	}
}

func TestEvaluateDoesNotReadWorkflowsFromCurrentDirectoryWhenRootMissing(t *testing.T) {
	report := Evaluate(Options{})
	if check := findCheck(report, "release_workflows"); check.State != StateBlocked {
		t.Fatalf("missing root workflow check = %#v", check)
	}
}

func TestEvaluateRejectsSymlinkedProviderConfig(t *testing.T) {
	target := filepath.Join(t.TempDir(), "provider.json")
	link := filepath.Join(t.TempDir(), "provider-link.json")
	body := `{"schema_version":1,"state":"unavailable","provider":"github","auth_base":"https://github.com","api_base":"https://api.github.com","client_id":"","owner":"","repository":"","reason":"not enrolled"}`
	if err := os.WriteFile(target, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if check := evaluateProvider(link); check.State != StateBlocked {
		t.Fatalf("symlinked provider config = %#v", check)
	}
}

func findCheck(report Report, id string) Check {
	for _, check := range report.Checks {
		if check.ID == id {
			return check
		}
	}
	return Check{ID: id, State: "missing"}
}

func validRegistryJSON(validFrom, validUntil string) string {
	return `{"schema_version":1,"product":"maestro","authorities":[{"issuer":"maestro-release","key_id":"pilot-active","algorithm":"ed25519","public_key":"` + base64.StdEncoding.EncodeToString(make([]byte, 32)) + `","status":"active","valid_from":"` + validFrom + `","valid_until":"` + validUntil + `"}]}`
}
