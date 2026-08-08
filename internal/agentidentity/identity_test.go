package agentidentity

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInitialInterviewExplainsNamesAvatarsAndOwnership(t *testing.T) {
	interview := InitialInterview()
	if interview.Kind != "agent_identity_setup" || interview.SchemaVersion != SchemaVersion || len(interview.Agents) < 7 {
		t.Fatalf("unexpected interview: %#v", interview)
	}
	if interview.OwnershipExplanation == "" || interview.AvatarExplanation == "" {
		t.Fatal("interview omitted ownership or avatar explanation")
	}
	if interview.ProfileInput.Command == "" || interview.ProfileInput.SchemaVersion != SchemaVersion ||
		len(interview.ProfileInput.RequiredFields) != 4 || len(interview.ProfileInput.SelectionFields) != 6 ||
		interview.ProfileInput.OwnershipScopes["quality_guardian"] != "quality_longitudinal" {
		t.Fatalf("interview omitted the canonical profile input contract: %#v", interview.ProfileInput)
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
		if (descriptor.Role == "walter" || descriptor.Role == "darwin") && len(descriptor.NarrativeSuggestions) < 9 {
			t.Fatalf("%s omitted its narrative repertoire: %#v", descriptor.Role, descriptor.NarrativeSuggestions)
		}
		if descriptor.Role == "quality_guardian" && descriptor.OwnershipScope != "quality_longitudinal" {
			t.Fatalf("Gamma ownership scope = %q", descriptor.OwnershipScope)
		}
	}
}

func TestNarrativeSuggestionsAreTransparentAndHALIsNotDefaultRecommended(t *testing.T) {
	interview := InitialInterview()
	seenHAL := false
	for _, descriptor := range interview.Agents {
		for _, suggestion := range descriptor.NarrativeSuggestions {
			if suggestion.Name == "" || suggestion.Reference == "" || suggestion.Story == "" || len(suggestion.BestFor) == 0 {
				t.Fatalf("incomplete narrative suggestion: %#v", suggestion)
			}
			if suggestion.Name == "HAL" {
				seenHAL = true
				if suggestion.AvoidWhen == "" {
					t.Fatal("HAL must carry an explicit non-default recommendation warning")
				}
			}
		}
	}
	if !seenHAL {
		t.Fatal("the preserved narrative repertoire omitted HAL")
	}
}

func TestNarrativeRecommendationsUseOnlyExplicitPreferences(t *testing.T) {
	if got := RecommendNarrativeSuggestions("walter", nil, 3); got != nil {
		t.Fatalf("recommendation without explicit preferences = %#v", got)
	}
	got := RecommendNarrativeSuggestions("walter", []string{"estratégia", "prudência"}, 3)
	if len(got) != 1 || got[0].Name != "Athena" {
		t.Fatalf("Walter recommendation = %#v, want Athena only", got)
	}
	got = RecommendNarrativeSuggestions("darwin", []string{"ficção científica clássica"}, 3)
	if len(got) != 0 {
		t.Fatalf("HAL must never be default recommended: %#v", got)
	}
}

func TestNarrativeRecommendationsCoverEveryPromptedPreferenceAndCapAtThree(t *testing.T) {
	cases := []struct {
		preference string
		role       string
		want       string
	}{
		{preference: "guia sereno", role: "walter", want: "Iroh"},
		{preference: "estrategista", role: "walter", want: "Athena"},
		{preference: "parceiro firme", role: "walter", want: "Samwise"},
		{preference: "advisor técnico", role: "walter", want: "Jarvis"},
		{preference: "arquiteto de sistemas", role: "darwin", want: "Ariadne"},
		{preference: "observador de evolução", role: "darwin", want: "Darwin"},
	}
	for _, test := range cases {
		t.Run(test.preference, func(t *testing.T) {
			got := RecommendNarrativeSuggestions(test.role, []string{test.preference}, 3)
			for _, candidate := range got {
				if candidate.Name == test.want {
					return
				}
			}
			t.Fatalf("%q did not suggest %q: %#v", test.preference, test.want, got)
		})
	}
	got := RecommendNarrativeSuggestions("darwin", []string{"arquitetura", "sistemas", "evolução", "continuidade", "resiliência", "pragmatismo"}, 99)
	if len(got) > 3 {
		t.Fatalf("recommendation must cap at three, got %#v", got)
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
	if len(managed) != 4 || managed[0].DisplayName != "Conductor" || managed[1].DisplayName != "Sentinel" || managed[2].DisplayName != "Darwin" || managed[3].DisplayName != "Gamma Guardian" {
		t.Fatalf("managed identity projection = %#v", managed)
	}
}
