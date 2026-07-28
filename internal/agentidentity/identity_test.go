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
	for _, descriptor := range interview.Agents {
		if descriptor.DefaultName == "" || descriptor.DefaultEmoji == "" || len(descriptor.Suggestions) < 2 || descriptor.Purpose == "" {
			t.Fatalf("incomplete descriptor: %#v", descriptor)
		}
	}
}

func TestProfileValidatesCanonicalRolesAndPersistsAtomically(t *testing.T) {
	root := t.TempDir()
	profile := Profile{
		SchemaVersion: SchemaVersion,
		OwnerID:       "daniel",
		Confirmed:     true,
		UpdatedAt:     time.Now().UTC(),
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
