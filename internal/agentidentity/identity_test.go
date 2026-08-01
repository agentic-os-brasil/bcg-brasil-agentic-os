package agentidentity

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInitialInterviewExplainsNamesAvatarsAndOwnership(t *testing.T) {
	interview := InitialInterview()
	if interview.Kind != "agent_identity_setup" || interview.SchemaVersion != SchemaVersion || len(interview.Agents) < 6 {
		t.Fatalf("unexpected interview: %#v", interview)
	}
	if interview.OwnershipExplanation == "" || interview.AvatarExplanation == "" {
		t.Fatal("interview omitted ownership or avatar explanation")
	}
	if len(interview.CapabilityTracks) < 4 || interview.CapabilityTracks[0].ID == "" {
		t.Fatalf("interview omitted capability track selection: %#v", interview.CapabilityTracks)
	}
	for _, descriptor := range interview.Agents {
		if descriptor.DefaultName == "" || descriptor.DefaultEmoji == "" || len(descriptor.Suggestions) < 2 || len(descriptor.EmojiSuggestions) < 2 || descriptor.Purpose == "" {
			t.Fatalf("incomplete descriptor: %#v", descriptor)
		}
		if descriptor.Role == "walter" && descriptor.DefaultEmoji != "🦉" {
			t.Fatalf("Walter default emoji = %q, want owl", descriptor.DefaultEmoji)
		}
	}
}

func TestProfileValidatesCanonicalRolesAndPersistsAtomically(t *testing.T) {
	root := t.TempDir()
	profile := Profile{
		SchemaVersion:    SchemaVersion,
		OwnerID:          "daniel",
		Confirmed:        true,
		UpdatedAt:        time.Now().UTC(),
		CapabilityTracks: []string{"software-engineering"},
		Selections: []Selection{
			{Role: "client_account_agent", AgentID: "client-account-agent-acme", DisplayName: "Compass", Emoji: "🧭", OwnerID: "daniel", OwnershipScope: "account"},
			{Role: "case_agent", AgentID: "case-agent-pricing", DisplayName: "Forge", Emoji: "⚙️", OwnerID: "daniel", OwnershipScope: "case"},
		},
	}
	if err := Save(root, profile); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if selected, ok := Resolve(loaded, "case_agent", "case-agent-pricing"); !ok || selected.DisplayName != "Forge" {
		t.Fatalf("resolved profile = %#v, ok=%v", selected, ok)
	}
	if len(loaded.CapabilityTracks) != 1 || loaded.CapabilityTracks[0] != "software-engineering" {
		t.Fatalf("capability tracks = %#v", loaded.CapabilityTracks)
	}
	if _, err := os.Stat(filepath.Join(root, "agents", "personalization.json")); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyRolesResolveToCanonicalRoles(t *testing.T) {
	if got := CanonicalRole("account_agent"); got != "client_account_agent" {
		t.Fatalf("account alias = %q", got)
	}
	if got := CanonicalRole("workspace_agent"); got != "case_agent" {
		t.Fatalf("workspace alias = %q", got)
	}
	if IsCanonicalRole("practice_agent") || IsCanonicalRole("subject_specialist") {
		t.Fatal("retired practice roles remain canonical identities")
	}
	if _, ok := Default("practice_agent"); ok {
		t.Fatal("retired practice role still has a default identity")
	}
}

func TestLoadRejectsTrailingJSON(t *testing.T) {
	root := t.TempDir()
	profile := Profile{
		SchemaVersion: SchemaVersion, OwnerID: "daniel", Confirmed: true, UpdatedAt: time.Now().UTC(),
		Selections: []Selection{{Role: "maestro", DisplayName: "Maestro", Emoji: "🎼", OwnerID: "daniel", OwnershipScope: "system"}},
	}
	if err := Save(root, profile); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "agents", "personalization.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, []byte("{}\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
}

func TestSaveNormalizesLegacyRoles(t *testing.T) {
	root := t.TempDir()
	profile := Profile{
		SchemaVersion: SchemaVersion,
		OwnerID:       "daniel",
		Confirmed:     true,
		UpdatedAt:     time.Now().UTC(),
		Selections: []Selection{{
			Role: "workspace_agent", AgentID: "case-agent-pricing", DisplayName: "Forge", Emoji: "⚙️",
			OwnerID: "daniel", OwnershipScope: "case",
		}},
	}
	if err := Save(root, profile); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Selections[0].Role; got != "case_agent" {
		t.Fatalf("saved role = %q, want canonical case_agent", got)
	}
}

func TestManagedProjectionConsumesConfirmedCorePersonalization(t *testing.T) {
	profile := Profile{
		SchemaVersion: SchemaVersion, OwnerID: "daniel", Confirmed: true, UpdatedAt: time.Now().UTC(),
		Selections: []Selection{
			{Role: "maestro", DisplayName: "Conductor", Emoji: "🎹", OwnerID: "daniel", OwnershipScope: "system"},
			{Role: "walter", DisplayName: "Sentinel", Emoji: "🛡️", OwnerID: "daniel", OwnershipScope: "governance"},
		},
	}
	if err := profile.Validate(); err != nil {
		t.Fatal(err)
	}
	managed := ResolveManaged(profile)
	if len(managed) != 3 || managed[0].DisplayName != "Conductor" || managed[1].DisplayName != "Sentinel" || managed[2].DisplayName != "Darwin" {
		t.Fatalf("managed identity projection = %#v", managed)
	}
}
