//go:build !windows

package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const privateWikiLegacyRevision = "d1e8f18dd13d680f81a552567c869bcc4f57ac73"

type wikiFixtureProof struct {
	Binary              string `json:"binary"`
	SourceRevision      string `json:"source_revision"`
	SourceArchiveSHA256 string `json:"source_archive_sha256"`
	FixtureSHA256       string `json:"fixture_sha256"`
	ExecutableSHA256    string `json:"executable_sha256"`
}
type wikiFixtureReceipt struct {
	SourceRevision      string `json:"source_revision"`
	SourceArchiveSHA256 string `json:"source_archive_sha256"`
	FixtureSHA256       string `json:"fixture_sha256"`
	ExecutableSHA256    string `json:"executable_sha256"`
	ConfigSHA256        string `json:"config_sha256"`
}

func privateWikiBinaryFixture(t *testing.T, kind, variable string) (string, wikiFixtureProof) {
	t.Helper()
	helper := os.Getenv(variable)
	if helper == "" {
		t.Skip("actual " + kind + " binary not provisioned")
	}
	manifestName := os.Getenv("MAESTRO_PRIVATE_WIKI_FIXTURE_MANIFEST")
	if !filepath.IsAbs(helper) || !filepath.IsAbs(manifestName) || !filepath.IsAbs(os.Getenv("MAESTRO_PRIVATE_WIKI_BINARY_EVIDENCE")) {
		t.Fatal("actual binaries require absolute manifest and evidence paths")
	}
	body, err := os.ReadFile(manifestName)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SourceRevision string                      `json:"source_revision"`
		LegacyRevision string                      `json:"legacy_revision"`
		Helpers        map[string]wikiFixtureProof `json:"helpers"`
	}
	if json.Unmarshal(body, &manifest) != nil || manifest.LegacyRevision != privateWikiLegacyRevision {
		t.Fatal("actual historical source provenance missing")
	}
	proof, ok := manifest.Helpers[kind]
	if !ok || proof.Binary != helper {
		t.Fatal("selected helper differs from provisioned executable")
	}
	program, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(program)
	if proof.ExecutableSHA256 != hex.EncodeToString(sum[:]) || len(proof.SourceArchiveSHA256) != 64 || len(proof.FixtureSHA256) != 64 {
		t.Fatal("actual binary/source hashes unavailable")
	}
	want := manifest.SourceRevision
	if kind == "old_writer" {
		want = privateWikiLegacyRevision
	}
	if proof.SourceRevision != want {
		t.Fatal("helper selected the wrong immutable source")
	}
	return helper, proof
}
func verifyWikiFixtureReceipt(t *testing.T, receipt wikiFixtureReceipt, proof wikiFixtureProof, config []byte) {
	t.Helper()
	sum := sha256.Sum256(config)
	if receipt.SourceRevision != proof.SourceRevision || receipt.SourceArchiveSHA256 != proof.SourceArchiveSHA256 || receipt.FixtureSHA256 != proof.FixtureSHA256 || receipt.ExecutableSHA256 != proof.ExecutableSHA256 || receipt.ConfigSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("actual helper/source/config provenance changed")
	}
}
func retainWikiFixtureEvidence(t *testing.T, label string, config, output []byte) {
	t.Helper()
	root := os.Getenv("MAESTRO_PRIVATE_WIKI_BINARY_EVIDENCE")
	if !filepath.IsAbs(root) {
		t.Fatal("actual binary evidence destination missing")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	name := strings.ReplaceAll(t.Name(), "/", "_") + "-" + label
	for suffix, body := range map[string][]byte{".config.json": config, ".stdout.json": output} {
		if err := os.WriteFile(filepath.Join(root, name+suffix), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

type wikiOldResult struct {
	wikiFixtureReceipt
	Refused  bool             `json:"refused"`
	Code     string           `json:"code"`
	Capture  *CaptureReceipt  `json:"capture,omitempty"`
	Record   *CaptureRecord   `json:"record,omitempty"`
	Mutation *MutationReceipt `json:"mutation,omitempty"`
}

func invokeOldWikiFixture(t *testing.T, helper string, proof wikiFixtureProof, operation, root, id, configName, label string) wikiOldResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, helper, operation, root, id, configName).CombinedOutput()
	if err != nil {
		t.Fatalf("real legacy operation failed: %s %v", output, err)
	}
	var result wikiOldResult
	if json.Unmarshal(output, &result) != nil {
		t.Fatal("invalid real legacy result")
	}
	config, err := os.ReadFile(configName)
	if err != nil {
		t.Fatal(err)
	}
	verifyWikiFixtureReceipt(t, result.wikiFixtureReceipt, proof, config)
	retainWikiFixtureEvidence(t, label, config, output)
	return result
}
