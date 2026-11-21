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
		if err != nil || strings.Contains(string(body), "versioned_slots") || strings.Contains(string(body), "selection") || strings.Contains(string(body), "owner_review_channel") || strings.Contains(string(body), "target") {
			t.Fatal("absent extension changed legacy JSON", err)
		}
	}
}

func TestClientOwnerReviewChannelRejectsUnknownAndUnsupportedProfiles(t *testing.T) {
	for _, mode := range []string{"", "local-browser", "native-first", "model-approved"} {
		for _, experimental := range []bool{false, true} {
			for _, versioned := range []bool{false, true} {
				err := validateClientOwnerReviewChannel(mode, experimental, versioned)
				want := mode == "" || mode == "local-browser" && experimental && versioned
				if (err == nil) != want {
					t.Fatalf("mode=%q experimental=%t versioned=%t: %v", mode, experimental, versioned, err)
				}
			}
		}
	}
}

func TestClientGenerationsRetainOwnerReviewChannel(t *testing.T) {
	var record ClientRecord
	if err := json.Unmarshal([]byte(`{"materialization":{"owner_review_channel":"local-browser"}}`), &record); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(ClientRequestFromRecord(record))
	if err != nil || !strings.Contains(string(body), `"owner_review_channel":"local-browser"`) {
		t.Fatalf("cold reconstruction lost the reviewed owner channel: %s %v", body, err)
	}
}

func TestDesktopClientGenerationHasNoCodeSlot(t *testing.T) {
	var request ClientRequest
	body, err := json.Marshal(ClientRequest{Target: "desktop", CarrierID: strings.Repeat("a", 32), DesktopPath: filepath.Join(t.TempDir(), "maestro.mcpb"), VersionedSlots: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	material, err := ClientGenerationPaths(request, strings.Repeat("b", 64))
	if err != nil {
		t.Fatalf("explicit Desktop target must not require a Code slot: %v", err)
	}
	if material.CodeRoot != "" || material.Selection == nil || material.Selection.CodeRoot != "" || material.DesktopPath == request.DesktopPath {
		t.Fatalf("Desktop target fabricated Code or lost versioned custody: %+v", material)
	}
	encoded, err := json.Marshal(ClientRequestFromRecord(ClientRecord{CarrierID: request.CarrierID, Materialization: material}))
	if err != nil || !strings.Contains(string(encoded), `"target":"desktop"`) {
		t.Fatalf("cold reconstruction lost the explicit target: %s %v", encoded, err)
	}
}

func TestDesktopClientGenerationRejectsCodeAndUnknownTargets(t *testing.T) {
	base := ClientRequest{Target: "desktop", VersionedSlots: true, CarrierID: strings.Repeat("a", 32), DesktopPath: filepath.Join(t.TempDir(), "desktop.mcpb")}
	for _, kind := range []string{"unknown", "unversioned", "code_root", "code_scope", "code_executable", "code_config", "native", "lifecycle"} {
		t.Run(kind, func(t *testing.T) {
			r := base
			switch kind {
			case "unknown":
				r.Target = "automatic"
			case "unversioned":
				r.VersionedSlots = false
			case "code_root":
				r.CodeRoot = filepath.Join(t.TempDir(), "fake-code")
			case "code_scope":
				r.CodeScope = "user"
			case "code_executable":
				r.CodeExecutable = "selected-code"
			case "code_config":
				r.CodeConfigurationDirectory = "private-profile"
			case "native":
				r.NativeEnforcement = true
			case "lifecycle":
				r.LifecycleContext = true
			}
			if _, err := ClientGenerationPaths(r, strings.Repeat("b", 64)); err == nil {
				t.Fatal("unknown or unselected Code authority admitted")
			}
		})
	}
	both := base
	both.Target = ""
	both.CodeRoot = filepath.Join(t.TempDir(), "code")
	d, err := ClientGenerationPaths(base, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	b, err := ClientGenerationPaths(both, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	if d.DesktopPath == b.DesktopPath {
		t.Fatal("generation identity did not bind target")
	}
}
func TestDesktopClientManagementHasNoCodeSelection(t *testing.T) {
	o := ClientOptions{SeededOperationOptions: SeededOperationOptions{Self: "/Developer/maestro-install", ManagedRoot: "/Developer/product", UserHome: "/Developer/owner"}, Request: ClientRequest{Target: "desktop", VersionedSlots: true, CarrierID: strings.Repeat("a", 32), Workspace: "/Developer/workspace", DesktopPath: "/Developer/clients/maestro.mcpb"}}
	for _, goos := range []string{"darwin", "windows"} {
		_, body := clientManagementLauncherForOS(o, true, goos)
		text := string(body)
		if !strings.Contains(text, "--client-target") || !strings.Contains(text, "desktop") || strings.Contains(text, "--client-code") || strings.Contains(text, "CLAUDE_CONFIG_DIR") {
			t.Fatalf("Desktop management fabricated Code on %s: %s", goos, text)
		}
	}
}

func TestDesktopClientRecordRejectsHiddenCodeState(t *testing.T) {
	request := ClientRequest{Target: "desktop", VersionedSlots: true, CarrierID: strings.Repeat("a", 32), DesktopPath: filepath.Join(t.TempDir(), "desktop.mcpb")}
	material, err := ClientGenerationPaths(request, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	valid := ClientRecord{SchemaVersion: 2, Materialization: material, Code: CodeObservation{State: "not_selected", EvidenceClass: "not_applicable"}}
	if err := validateClientTargetRecord(valid); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*ClientRecord){
		"old schema":              func(r *ClientRecord) { r.SchemaVersion = 1 },
		"unknown target":          func(r *ClientRecord) { r.Materialization.Target = "both" },
		"hidden material Code":    func(r *ClientRecord) { r.Materialization.CodeRoot = "unexpected" },
		"hidden selected Code":    func(r *ClientRecord) { r.Materialization.Selection.CodeRoot = "unexpected" },
		"hidden Code observation": func(r *ClientRecord) { r.Code.Executable = "unexpected" },
		"hidden provider":         func(r *ClientRecord) { r.Materialization.CodeProviderRoot = "unexpected" },
		"hidden hooks":            func(r *ClientRecord) { r.Materialization.NativeEnforcement = true },
		"backup":                  func(r *ClientRecord) { r.Materialization.DesktopBackupSHA256 = strings.Repeat("c", 64) },
		"selection target":        func(r *ClientRecord) { r.Materialization.Selection.Target = "" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			r := valid
			selection := *valid.Materialization.Selection
			r.Materialization.Selection = &selection
			change(&r)
			if validateClientTargetRecord(r) == nil {
				t.Fatal("invalid current target admitted")
			}
		})
	}
	legacy := ClientRecord{SchemaVersion: 1, Materialization: ClientMaterialization{Selection: &ClientPacketSelection{Target: "desktop"}}}
	if validateClientTargetRecord(legacy) == nil {
		t.Fatal("legacy both record acquired new selected target")
	}
}
