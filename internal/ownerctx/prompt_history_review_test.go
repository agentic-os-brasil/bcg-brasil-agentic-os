package ownerctx

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPromptHistoryReviewedChangesFenceReplayAndPruning(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	input := promptInput("one explicit synthetic prompt", PromptScopeWorkspace, "workspace-a", now)
	input.OccurrenceID = "once"
	review, err := ReviewPromptHistory(root, PromptHistoryChange{Operation: "record", Input: &input})
	if err != nil {
		t.Fatal(err)
	}
	if err = ApplyReviewedPromptHistory(root, review, false); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatal(err)
	}
	if entries, err := ExportPromptHistory(root); err != nil || len(entries) != 0 {
		t.Fatal("decline changed store", err)
	}
	if err = ApplyReviewedPromptHistory(root, review, true); err != nil {
		t.Fatal(err)
	}
	if err = ApplyReviewedPromptHistory(root, review, true); err != nil {
		t.Fatal("exact restart retry failed", err)
	}
	input.Prompt = "changed content"
	if _, err = ReviewPromptHistory(root, PromptHistoryChange{Operation: "record", Input: &input}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatal("conflicting occurrence accepted", err)
	}
	config := DefaultPromptHistoryConfig()
	config.MaxEntries = 1
	prune, err := ReviewPromptHistory(root, PromptHistoryChange{Operation: "configure", Config: &config})
	if err != nil {
		t.Fatal(err)
	}
	later := promptInput("second explicit prompt", PromptScopeWorkspace, "workspace-a", now.Add(time.Second))
	later.OccurrenceID = "twice"
	add, err := ReviewPromptHistory(root, PromptHistoryChange{Operation: "record", Input: &later})
	if err != nil {
		t.Fatal(err)
	}
	if err = ApplyReviewedPromptHistory(root, add, true); err != nil {
		t.Fatal(err)
	}
	if err = ApplyReviewedPromptHistory(root, prune, true); !errors.Is(err, ErrRevisionConflict) {
		t.Fatal("stale review pruned a new record", err)
	}
	prune, err = ReviewPromptHistory(root, PromptHistoryChange{Operation: "configure", Config: &config})
	if err != nil || len(prune.Removed) != 1 {
		t.Fatal("pruning consequence missing", prune.Removed, err)
	}
	if err = ApplyReviewedPromptHistory(root, prune, true); err != nil {
		t.Fatal(err)
	}
	entries, err := ExportPromptHistory(root)
	if err != nil || len(entries) != 1 || entries[0].Prompt != later.Prompt {
		t.Fatal("wrong retained result", entries, err)
	}
}
func TestPromptHistoryInterruptedConfigurationRetainsPreimagesAndNeedsExactRecovery(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	for n, body := range []string{"older owned prompt", "newer owned prompt"} {
		in := promptInput(body, PromptScopeWorkspace, "workspace-a", time.Now().UTC().Add(time.Duration(n)*time.Second))
		in.OccurrenceID = body[:5]
		if _, err := RecordUserPrompt(root, in); err != nil {
			t.Fatal(err)
		}
	}
	originalConfigPath := filepath.Join(filepath.Dir(PromptHistoryEntriesPath(root)), "config.json")
	originalConfig, err := os.ReadFile(originalConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	var compactConfig any
	if err = json.Unmarshal(originalConfig, &compactConfig); err != nil {
		t.Fatal(err)
	}
	originalConfig, err = json.Marshal(compactConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(originalConfigPath, originalConfig, 0600); err != nil {
		t.Fatal(err)
	}
	originalEntries, err := os.ReadFile(PromptHistoryEntriesPath(root))
	if err != nil {
		t.Fatal(err)
	}
	originalEntries = append([]byte("  "), originalEntries...)
	if err = os.WriteFile(PromptHistoryEntriesPath(root), originalEntries, 0600); err != nil {
		t.Fatal(err)
	}
	config := DefaultPromptHistoryConfig()
	config.MaxEntries = 1
	review, err := ReviewPromptHistory(root, PromptHistoryChange{Operation: "configure", Config: &config})
	if err != nil {
		t.Fatal(err)
	}
	err = withPromptHistoryLockPolicy(root, true, func(entriesPath, _ string) error {
		dir, err := scheduler.HoldDirectory(filepath.Dir(entriesPath), nil)
		if err != nil {
			return err
		}
		defer dir.Close()
		before, err := readHistoryPairHeld(dir)
		if err != nil {
			return err
		}
		return publishHistoryPair(dir, promptHistoryJournal{Version: 1, Review: review, Before: before, After: promptHistoryPair{review.ProposedConfig, review.after}}, func(stage string) error {
			if stage == "config.json" {
				return errors.New("synthetic interruption")
			}
			return nil
		})
	})
	if err == nil || !strings.Contains(err.Error(), "synthetic interruption") {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(PromptHistoryEntriesPath(root))
	if err != nil || !strings.Contains(string(raw), "older owned prompt") || !strings.Contains(string(raw), "newer owned prompt") {
		t.Fatal("pruned before recoverable commit", err)
	}
	if _, err = ExportPromptHistory(root); !errors.Is(err, ErrPromptHistoryRecovery) {
		t.Fatal("partial state was exposed", err)
	}
	if err = ResetPromptHistory(root, true); !errors.Is(err, ErrPromptHistoryRecovery) {
		t.Fatal("another mutation hid pending transition", err)
	}
	journalBytes, err := os.ReadFile(filepath.Join(filepath.Dir(PromptHistoryEntriesPath(root)), promptHistoryPending))
	if err != nil {
		t.Fatal(err)
	}
	var journal promptHistoryJournal
	if err = decodeHistoryJournal(journalBytes, &journal); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(journal.OriginalConfig, originalConfig) || !bytes.Equal(journal.OriginalEntries, originalEntries) {
		t.Fatal("interruption did not preserve literal preimage bytes")
	}
	recovered, err := ReviewPendingPromptHistory(root)
	if err != nil || recovered.Digest != review.Digest {
		t.Fatal("exact recovery unavailable", err)
	}
	if err = ApplyReviewedPromptHistory(root, recovered, false); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatal("unreviewed recovery", err)
	}
	if err = ApplyReviewedPromptHistory(root, recovered, true); err != nil {
		t.Fatal(err)
	}
	entries, err := ExportPromptHistory(root)
	if err != nil || len(entries) != 1 || entries[0].Prompt != "newer owned prompt" {
		t.Fatal("recovery not exact", entries, err)
	}
	if _, err = os.Lstat(filepath.Join(filepath.Dir(PromptHistoryEntriesPath(root)), promptHistoryPending)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("completed journal retained", err)
	}
}
func TestPromptHistoryReviewedFreshAndChangedGuard(t *testing.T) {
	root := filepath.Join(t.TempDir(), "fresh")
	view, err := PromptHistorySnapshot(root)
	if err != nil || view.Initialized {
		t.Fatal(view, err)
	}
	config := DefaultPromptHistoryConfig()
	review, err := ReviewPromptHistory(root, PromptHistoryChange{Operation: "configure", Config: &config})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("review initialized owner", err)
	}
	// No domain write is admitted once the external binding changed.
	if err = ApplyReviewedPromptHistoryGuarded(root, review, true, func() error { return ErrRevisionConflict }); !errors.Is(err, ErrRevisionConflict) {
		t.Fatal(err)
	}
	if _, err = os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("changed binding created owner state", err)
	}
}

func TestPromptHistoryReviewedPreservesExistingSingleOwnerBinding(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	in := promptInput("owned legacy prompt", PromptScopeWorkspace, "workspace-a", time.Now().UTC())
	in.OccurrenceID = "legacy"
	if _, err := RecordUserPrompt(root, in); err != nil {
		t.Fatal(err)
	}
	config := DefaultPromptHistoryConfig()
	// A legacy unset config label may be reconciled only with its existing entries.
	if err := writePrivateJSON(filepath.Join(filepath.Dir(PromptHistoryEntriesPath(root)), "config.json"), config); err != nil {
		t.Fatal(err)
	}
	other := in
	other.OwnerID = "different-owner"
	other.OccurrenceID = "other"
	if _, err := ReviewPromptHistory(root, PromptHistoryChange{Operation: "record", Input: &other}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatal("record rebound legacy owner", err)
	}
	config.OwnerID = "different-owner"
	if _, err := ReviewPromptHistory(root, PromptHistoryChange{Operation: "configure", Config: &config}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatal("configure rebound legacy owner", err)
	}
	config.OwnerID = ""
	review, err := ReviewPromptHistory(root, PromptHistoryChange{Operation: "configure", Config: &config})
	if err != nil || review.ProposedConfig.OwnerID != "owner" {
		t.Fatal("original owner was not preserved", err)
	}
	if err = ApplyReviewedPromptHistory(root, review, true); err != nil {
		t.Fatal(err)
	}
}
