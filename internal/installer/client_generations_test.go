package installer

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientGenerationsRetainSelectionsAndSeparateReleases(t *testing.T) {
	r := ClientRequest{CarrierID: strings.Repeat("a", 32), CodeRoot: filepath.Join(t.TempDir(), "code"), DesktopPath: filepath.Join(t.TempDir(), "desktop.mcpb"), VersionedSlots: true}
	r.GenerationID = strings.Repeat("d", 32)
	first, err := ClientGenerationPaths(r, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	second, err := ClientGenerationPaths(r, strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	if first.CodeRoot == second.CodeRoot || first.DesktopPath == second.DesktopPath || first.Selection == nil || *first.Selection != (ClientPacketSelection{CodeRoot: r.CodeRoot, DesktopPath: r.DesktopPath}) {
		t.Fatal("release slots or selected roots conflated")
	}
	record := ClientRecord{CarrierID: r.CarrierID, Materialization: first, Code: CodeObservation{Scope: "user", ConfigurationDirectory: filepath.Join(t.TempDir(), "selected-profile")}}
	got := ClientRequestFromRecord(record)
	if got.CodeConfigurationDirectory != record.Code.ConfigurationDirectory || !got.VersionedSlots || got.GenerationID != r.GenerationID || got.CodeRoot != r.CodeRoot || got.DesktopPath != r.DesktopPath || ClientMarketplaceRoot(record) == first.CodeRoot {
		t.Fatal("restart lost stable source selection")
	}
	for _, bad := range []string{"", "../escape", strings.Repeat("x", 64)} {
		if _, err := ClientGenerationPaths(r, bad); err == nil {
			t.Fatal("unbound generation accepted")
		}
	}
}

func TestClientGenerationsLeaveLegacyJSONAndLocationsUnchanged(t *testing.T) {
	r := ClientRequest{CodeRoot: "legacy-code", DesktopPath: "legacy.mcpb"}
	m, err := ClientGenerationPaths(r, "")
	if err != nil || m.CodeRoot != r.CodeRoot || m.DesktopPath != r.DesktopPath || m.Selection != nil {
		t.Fatal("legacy selection changed", err)
	}
	for _, value := range []any{r, m} {
		body, err := json.Marshal(value)
		if err != nil || strings.Contains(string(body), "versioned_slots") || strings.Contains(string(body), "selection") {
			t.Fatal("absent extension changed legacy JSON", err)
		}
	}
}
