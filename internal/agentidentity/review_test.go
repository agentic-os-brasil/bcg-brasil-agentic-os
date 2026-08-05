package agentidentity

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestGuidedIdentityAndDigestBoundProfileReview(t *testing.T) {
	root := t.TempDir()
	interview := GuidedIdentityInterview(root)
	if interview.State != "action_required" || interview.NextQuestion == nil || interview.NextQuestion.Role != "maestro" || interview.NextQuestion.AudioPrompt == "" || len(interview.Catalog.Steps) != 0 {
		t.Fatalf("unexpected interview: %#v", interview)
	}
	profile := Profile{SchemaVersion: SchemaVersion, OwnerID: "daniel", Confirmed: true, UpdatedAt: time.Now().UTC(), Selections: []Selection{{Role: "maestro", DisplayName: "Maestro", Emoji: "🎼", OwnerID: "daniel", OwnershipScope: "system"}}}
	draft, err := DraftProfile(root, profile, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ConfirmProfileDraft(root, draft.ID, "invalid", true); err == nil {
		t.Fatal("invalid digest was accepted")
	}
	applied, err := ConfirmProfileDraft(root, draft.ID, draft.ReviewDigest, true)
	if err != nil {
		t.Fatal(err)
	}
	if applied.State != "applied" {
		t.Fatalf("unexpected state: %#v", applied)
	}
}

func TestConcurrentIdentityDraftCreationAllowsExactlyOneOpenDraft(t *testing.T) {
	root := t.TempDir()
	profile := Profile{SchemaVersion: SchemaVersion, OwnerID: "daniel", Confirmed: true, UpdatedAt: time.Now().UTC(), Selections: []Selection{{Role: "maestro", DisplayName: "Maestro", Emoji: "🎼", OwnerID: "daniel", OwnershipScope: "system"}}}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := DraftProfile(root, profile, true, true)
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("concurrent identity draft successes = %d, want exactly 1", succeeded)
	}
}

func TestIdentityConfirmationRecoversAfterPostCommitDraftCloseFailure(t *testing.T) {
	root := t.TempDir()
	profile := Profile{SchemaVersion: SchemaVersion, OwnerID: "daniel", Confirmed: true, UpdatedAt: time.Now().UTC(), Selections: []Selection{{Role: "maestro", DisplayName: "Maestro", Emoji: "🎼", OwnerID: "daniel", OwnershipScope: "system"}}}
	draft, err := DraftProfile(root, profile, true, true)
	if err != nil {
		t.Fatal(err)
	}
	original := identityWriteJSON
	t.Cleanup(func() { identityWriteJSON = original })
	failed := false
	identityWriteJSON = func(path string, value any) error {
		candidate, _ := value.(ProfileDraft)
		if candidate.State == "applied" && !failed {
			failed = true
			return errors.New("injected draft close failure")
		}
		return original(path, value)
	}
	if _, err := ConfirmProfileDraft(root, draft.ID, draft.ReviewDigest, true); err == nil {
		t.Fatal("injected failure did not surface")
	}
	if _, err := Load(root); err != nil {
		t.Fatalf("profile was not committed before close failure: %v", err)
	}
	identityWriteJSON = original
	recovered, err := ConfirmProfileDraft(root, draft.ID, draft.ReviewDigest, true)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State != "applied" {
		t.Fatalf("recovered draft = %#v", recovered)
	}
}

func TestIdentityConfirmationRecoversAfterPostCommitCompactionFailure(t *testing.T) {
	root := t.TempDir()
	profile := Profile{SchemaVersion: SchemaVersion, OwnerID: "daniel", Confirmed: true, UpdatedAt: time.Now().UTC(), Selections: []Selection{{Role: "maestro", DisplayName: "Maestro", Emoji: "🎼", OwnerID: "daniel", OwnershipScope: "system"}}}
	draft, err := DraftProfile(root, profile, true, true)
	if err != nil {
		t.Fatal(err)
	}
	original := identityCompactDrafts
	t.Cleanup(func() { identityCompactDrafts = original })
	failed := false
	identityCompactDrafts = func(root, currentID string) error {
		if !failed {
			failed = true
			return errors.New("injected compaction failure")
		}
		return original(root, currentID)
	}
	if _, err := ConfirmProfileDraft(root, draft.ID, draft.ReviewDigest, true); err == nil {
		t.Fatal("injected compaction failure did not surface")
	}
	identityCompactDrafts = original
	if _, err := ConfirmProfileDraft(root, draft.ID, draft.ReviewDigest, true); err != nil {
		t.Fatal(err)
	}
}

func TestIdentityDraftRejectsUnboundIDPathAndLifecycle(t *testing.T) {
	root := t.TempDir()
	profile := Profile{SchemaVersion: SchemaVersion, OwnerID: "daniel", Confirmed: true, UpdatedAt: time.Now().UTC(), Selections: []Selection{{Role: "maestro", DisplayName: "Maestro", Emoji: "🎼", OwnerID: "daniel", OwnershipScope: "system"}}}
	draft, err := DraftProfile(root, profile, true, true)
	if err != nil {
		t.Fatal(err)
	}
	validID := draft.ID
	draft.ID, draft.State = "identity-draft-tampered", "unknown"
	path := filepath.Join(root, "agents", "interview", "drafts", "tampered.json")
	if err := writeIdentityJSON(path, draft); err != nil {
		t.Fatal(err)
	}
	if _, err := ReviewProfileDraft(root, "tampered"); err == nil {
		t.Fatal("invalid identity draft lifecycle was accepted")
	}
	validPath := filepath.Join(root, "agents", "interview", "drafts", validID+".json")
	file, err := os.OpenFile(validPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{}\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ReviewProfileDraft(root, validID); err == nil {
		t.Fatal("trailing JSON in identity draft was accepted")
	}
}

func TestIdentityDraftRefusesOverwriteOfNewerProfile(t *testing.T) {
	root := t.TempDir()
	profile := Profile{SchemaVersion: SchemaVersion, OwnerID: "daniel", Confirmed: true, UpdatedAt: time.Now().UTC(), Selections: []Selection{{Role: "maestro", DisplayName: "Maestro", Emoji: "🎼", OwnerID: "daniel", OwnershipScope: "system"}}}
	draft, err := DraftProfile(root, profile, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "agents"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agents", "personalization.json"), []byte("newer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfirmProfileDraft(root, draft.ID, draft.ReviewDigest, true); err == nil {
		t.Fatal("newer profile was silently overwritten")
	}
}

func TestIdentityDraftRejectsBatchedFutureMainAgents(t *testing.T) {
	root := t.TempDir()
	profile := Profile{SchemaVersion: SchemaVersion, OwnerID: "daniel", Confirmed: true, UpdatedAt: time.Now().UTC(), Selections: []Selection{
		{Role: "maestro", DisplayName: "Maestro", Emoji: "🎼", OwnerID: "daniel", OwnershipScope: "system"},
		{Role: "walter", DisplayName: "Walter", Emoji: "🦉", OwnerID: "daniel", OwnershipScope: "governance"},
	}}
	if _, err := DraftProfile(root, profile, true, true); err == nil {
		t.Fatal("one-question interview accepted a batched future main-agent answer")
	}
}
