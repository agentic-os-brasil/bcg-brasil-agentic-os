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

func TestPrivateWikiCompatibleProducerAcrossActualBinaries(t *testing.T) {
	helper := os.Getenv("MAESTRO_PRIVATE_WIKI_COMPAT_PRODUCER")
	if helper == "" {
		t.Skip("actual compatible producer not selected")
	}
	if !filepath.IsAbs(helper) {
		t.Fatal("producer path must be absolute")
	}
	e, _, _ := privateWikiEngine(t)
	cfg := struct {
		Policy    Policy                   `json:"policy"`
		Budgets   map[string]int           `json:"budgets"`
		Freshness map[string]time.Duration `json:"freshness"`
	}{e.Policy, e.Budgets, e.Freshness}
	body, _ := json.Marshal(cfg)
	config := filepath.Join(t.TempDir(), "source-config.json")
	if os.WriteFile(config, body, 0600) != nil {
		t.Fatal("config fixture write")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, helper, e.Root, config).CombinedOutput()
	if err != nil {
		t.Fatalf("producer failed: %v %s", err, output)
	}
	var produced PrivateWikiResult
	if json.Unmarshal(output, &produced) != nil || !produced.Changed {
		t.Fatalf("producer did not build a real graph: %s", output)
	}
	manifestPath := filepath.Join(e.privateWikiRoot("case-a"), "versions", produced.Generation, "manifest.json")
	originalManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Implementation string `json:"implementation"`
	}
	if json.Unmarshal(originalManifest, &manifest) != nil {
		t.Fatal("manifest decode")
	}
	program, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(program)
	if manifest.Implementation != hex.EncodeToString(digest[:]) {
		t.Fatal("stored provenance is not the actual producer executable")
	}
	page, err := e.ReadPrivateWiki("case-a", produced.Generation, "index.md", 0, 65536)
	if err != nil || !strings.Contains(page.Content, "Compatible producer-only description") {
		t.Fatalf("compatible current-source historical graph refused: %v", err)
	}
	status, err := e.PrivateWikiStatus("case-a")
	if err != nil || status.State != "current" || status.ImplementationCurrent || !status.ConfigurationCurrent {
		t.Fatalf("producer provenance was substituted or treated as authority: %+v %v", status, err)
	}
	unchanged, err := os.ReadFile(manifestPath)
	if err != nil || string(unchanged) != string(originalManifest) {
		t.Fatal("reader rewrote historical evidence")
	}
	next, err := e.CompilePrivateWiki(context.Background(), "case-a", produced.Generation)
	if err != nil || !next.Changed || next.Generation == produced.Generation {
		t.Fatalf("current-build compilation did not create genuine new evidence: %+v %v", next, err)
	}
	status, err = e.PrivateWikiStatus("case-a")
	if err != nil || status.State != "current" || !status.ImplementationCurrent || !status.ConfigurationCurrent {
		t.Fatalf("new provenance not current: %+v %v", status, err)
	}
	unchanged, err = os.ReadFile(manifestPath)
	if err != nil || string(unchanged) != string(originalManifest) {
		t.Fatal("current build overwrote historical provenance")
	}
}
