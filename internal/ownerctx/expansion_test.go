package ownerctx

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestExpansionAsksOneDeterministicQuestionAndAppliesOnlyReviewedDraft(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if _, err := SelectOnboardingTrack(root, OnboardingTrackQuick); err != nil {
		t.Fatal(err)
	}
	for _, facet := range quickOnboardingFacets {
		path := filepath.Join(root, filepath.FromSlash(facets[facet].Record.Path))
		if err := os.WriteFile(path, []byte("# Confirmed\n\nexplicit owner answer for "+facet+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	status, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ConfirmOnboarding(root, status.Onboarding.ReviewDigest); err != nil {
		t.Fatal(err)
	}
	question, err := NextExpansionQuestion(root)
	if err != nil {
		t.Fatal(err)
	}
	if question.Facet != "voice" || question.Question == "" || question.AudioPrompt == "" || question.QuestionToken == "" {
		t.Fatalf("unexpected next question: %#v", question)
	}
	if _, err := DraftExpansion(root, digest("off-sequence"), "# Voice\n\n## Current\n\nwrong token\n", true, true); err == nil {
		t.Fatal("off-sequence question token was accepted")
	}
	draft, err := DraftExpansion(root, question.QuestionToken, "# Voice\n\n## Current\n\nDireta, humana e precisa.\n", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if draft.State != "drafted" || draft.ReviewDigest == "" {
		t.Fatalf("unexpected draft: %#v", draft)
	}
	if _, err := ConfirmExpansion(root, draft.ID, digest("wrong review"), true); err == nil {
		t.Fatal("tampered review digest was accepted")
	}
	applied, err := ConfirmExpansion(root, draft.ID, draft.ReviewDigest, true)
	if err != nil {
		t.Fatal(err)
	}
	if applied.State != "applied" || applied.RefinementID == "" {
		t.Fatalf("unexpected applied draft: %#v", applied)
	}
	body, err := os.ReadFile(filepath.Join(root, "owner", "self", "voice.md"))
	if err != nil || string(body) != draft.ProposedBody {
		t.Fatalf("canonical facet = %q, err=%v", body, err)
	}
}

func TestConcurrentExpansionDraftCreationAllowsExactlyOneOpenDraft(t *testing.T) {
	root := readyQuickOwner(t)
	question, err := NextExpansionQuestion(root)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := DraftExpansion(root, question.QuestionToken, "# Voice\n\n## Current\n\nConcise concurrent truth.\n", true, true)
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
		t.Fatalf("concurrent SELF draft successes = %d, want exactly 1", succeeded)
	}
}

func TestExpansionRefusesSilentOverwriteAfterBaseChanges(t *testing.T) {
	root := readyQuickOwner(t)
	question, err := NextExpansionQuestion(root)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := DraftExpansion(root, question.QuestionToken, "# Voice\n\n## Current\n\nreviewed draft\n", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "owner", "self", "voice.md"), []byte("# Voice\n\nnewer owner edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfirmExpansion(root, draft.ID, draft.ReviewDigest, true); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("conflict = %v, want ErrRevisionConflict", err)
	}
}

func TestExpansionFailsClosedForMalformedConfirmationMetadata(t *testing.T) {
	root := readyQuickOwner(t)
	path := filepath.Join(root, "owner", "interview", "confirmations.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"confirmations":{"psychological-profile":{"sha256":"bad","confirmed_at":"0001-01-01T00:00:00Z"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(root); err == nil {
		t.Fatal("malformed SELF confirmation metadata was accepted")
	}
}

func TestExpansionConfirmationRecoversAfterEveryPostCommitWriteFailure(t *testing.T) {
	for _, failure := range []string{"audit_close", "proposal_close", "confirmation_registry", "history_compaction", "draft_close"} {
		t.Run(failure, func(t *testing.T) {
			root := readyQuickOwner(t)
			question, err := NextExpansionQuestion(root)
			if err != nil {
				t.Fatal(err)
			}
			draft, err := DraftExpansion(root, question.QuestionToken, "# Voice\n\n## Current\n\nDireta e precisa.\n", true, true)
			if err != nil {
				t.Fatal(err)
			}
			original := expansionWriteJSON
			originalRefinement := refinementWriteJSON
			originalCompact := expansionCompactDrafts
			t.Cleanup(func() {
				expansionWriteJSON = original
				refinementWriteJSON = originalRefinement
				expansionCompactDrafts = originalCompact
			})
			failed := false
			expansionWriteJSON = func(path string, value any) error {
				candidate, _ := value.(ExpansionDraft)
				matches := failure == "confirmation_registry" && filepath.Base(path) == "confirmations.json" || failure == "draft_close" && candidate.State == "applied"
				if matches && !failed {
					failed = true
					return errors.New("injected post-commit write failure")
				}
				return original(path, value)
			}
			refinementWriteJSON = func(path string, value any) error {
				auditCandidate, _ := value.(audit)
				proposalCandidate, _ := value.(proposal)
				matches := failure == "audit_close" && auditCandidate.State == "applied" || failure == "proposal_close" && proposalCandidate.State == "applied"
				if matches && !failed {
					failed = true
					return errors.New("injected refinement post-commit write failure")
				}
				return originalRefinement(path, value)
			}
			expansionCompactDrafts = func(root string, draft ExpansionDraft) error {
				if failure == "history_compaction" && !failed {
					failed = true
					return errors.New("injected draft compaction failure")
				}
				return originalCompact(root, draft)
			}
			if _, err := ConfirmExpansion(root, draft.ID, draft.ReviewDigest, true); err == nil {
				t.Fatal("injected failure did not surface")
			}
			body, err := os.ReadFile(filepath.Join(root, "owner", "self", "voice.md"))
			if err != nil || string(body) != draft.ProposedBody {
				t.Fatalf("canonical commit missing after injected failure: %q %v", body, err)
			}
			expansionWriteJSON = original
			refinementWriteJSON = originalRefinement
			expansionCompactDrafts = originalCompact
			recovered, err := ConfirmExpansion(root, draft.ID, draft.ReviewDigest, true)
			if err != nil {
				t.Fatal(err)
			}
			if recovered.State != "applied" {
				t.Fatalf("recovered draft = %#v", recovered)
			}
		})
	}
}

func TestExpansionDraftMetadataAndCompactionFailClosed(t *testing.T) {
	root := readyQuickOwner(t)
	question, err := NextExpansionQuestion(root)
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"missing current": "# Voice\n\nplain prose\n",
		"transcript":      "# Voice\n\n## Current\n\nUser: keep this transcript\n",
		"duplicate":       "# Voice\n\n## Current\n\nSame statement.\n\nSame statement.\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DraftExpansion(root, question.QuestionToken, body, true, true); err == nil {
				t.Fatal("unbounded or transcript-like draft was accepted")
			}
		})
	}
	draft, err := DraftExpansion(root, question.QuestionToken, "# Voice\n\n## Current\n\nConcise current truth.\n", true, true)
	if err != nil {
		t.Fatal(err)
	}
	validID := draft.ID
	draft.ID = "self-draft-tampered"
	if err := writePrivateJSON(filepath.Join(root, "owner", "interview", "drafts", "tampered.json"), draft); err != nil {
		t.Fatal(err)
	}
	if _, err := ReviewExpansion(root, "tampered"); err == nil {
		t.Fatal("digest-unbound draft ID/path was accepted")
	}
	validPath := filepath.Join(root, "owner", "interview", "drafts", validID+".json")
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
	if _, err := ReviewExpansion(root, validID); err == nil {
		t.Fatal("trailing JSON in SELF draft was accepted")
	}
}

func readyQuickOwner(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if _, err := SelectOnboardingTrack(root, OnboardingTrackQuick); err != nil {
		t.Fatal(err)
	}
	for _, facet := range quickOnboardingFacets {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(facets[facet].Record.Path)), []byte("# Answer\n\n"+facet+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	status, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ConfirmOnboarding(root, status.Onboarding.ReviewDigest); err != nil {
		t.Fatal(err)
	}
	return root
}
