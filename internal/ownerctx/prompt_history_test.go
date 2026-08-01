package ownerctx

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPromptHistoryRetainsOnlyUserPromptsAndSeparatesReceipts(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	receipt, err := RecordUserPrompt(root, promptInput("owner prompt", PromptScopeWorkspace, "workspace-a", time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SHA256 == "" || receipt.Bytes == 0 || strings.Contains(string(encoded), "owner prompt") {
		t.Fatalf("receipt exposed prompt body: %#v", receipt)
	}
	entries, err := ExportPromptHistory(root)
	if err != nil || len(entries) != 1 || entries[0].Prompt != "owner prompt" || entries[0].ContentKind != "user_prompt" {
		t.Fatalf("export = %#v, err = %v", entries, err)
	}
	if strings.Contains(PromptHistoryEntriesPath(root), string(filepath.Separator)+"runtime"+string(filepath.Separator)) {
		t.Fatal("prompt history path entered runtime receipts")
	}
	if _, err := os.Stat(filepath.Join(root, "bundles")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prompt history created a managed bundle path: %v", err)
	}
}

func promptInput(body string, scope PromptScopeKind, scopeID string, recordedAt time.Time) PromptHistoryInput {
	return PromptHistoryInput{OwnerID: "owner", Prompt: body, Language: "pt-BR", Source: "owner", SessionID: "session-a", ScopeKind: scope, ScopeID: scopeID, RecordedAt: recordedAt, ContentKind: "user_prompt"}
}

func TestPromptHistoryRejectsNonUserContentAndSymlinkStore(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	bad := promptInput("assistant text", PromptScopeGlobal, "owner", time.Now().UTC())
	bad.ContentKind = "assistant_output"
	if _, err := RecordUserPrompt(root, bad); err == nil || !strings.Contains(err.Error(), "user prompts only") {
		t.Fatalf("assistant content was accepted: %v", err)
	}
	entriesPath := PromptHistoryEntriesPath(root)
	backup := entriesPath + ".backup"
	if err := os.Rename(entriesPath, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(backup, entriesPath); err != nil {
		_ = os.Rename(backup, entriesPath)
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation is unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if _, err := RecordUserPrompt(root, promptInput("must fail", PromptScopeGlobal, "owner", time.Now().UTC())); err == nil {
		t.Fatal("symlinked prompt store was accepted")
	}
}

func TestPromptHistoryBoundsScopesAgeAndCurrentSelection(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := ConfigurePromptHistory(root, PromptHistoryConfig{SchemaVersion: 1, MaxEntries: 3, MaxBytes: 128, MaxAgeSeconds: int64((48 * time.Hour) / time.Second)}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	items := []struct {
		body  string
		kind  PromptScopeKind
		scope string
		age   time.Duration
	}{
		{"global context", PromptScopeGlobal, "owner", time.Hour},
		{"case context", PromptScopeCase, "case-a", 2 * time.Hour},
		{"other case", PromptScopeCase, "case-b", 3 * time.Hour},
		{"expired", PromptScopeCase, "case-a", 72 * time.Hour},
	}
	for _, item := range items {
		_, err := RecordUserPrompt(root, promptInput(item.body, item.kind, item.scope, now.Add(-item.age)))
		if item.age > 48*time.Hour {
			if err == nil {
				t.Fatal("expired prompt was retained")
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
	selected, err := SelectPromptHistory(root, PromptHistorySelectionLimits{OwnerID: "owner", MaxCount: 2, MaxBytes: 64, MaxAge: 24 * time.Hour, ScopeKind: PromptScopeCase, ScopeID: "case-a", CurrentPrompt: "case alpha decision"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].Prompt != "case context" || selected[1].Prompt != "global context" {
		t.Fatalf("bounded relevant selection = %#v", selected)
	}
}

func TestPromptHistoryDeleteResetRequireConfirmation(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	receipt, err := RecordUserPrompt(root, promptInput("delete me", PromptScopeGlobal, "owner", time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	if err := DeletePromptHistory(root, receipt.ID, false); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatal("delete without confirmation succeeded")
	}
	if err := DeletePromptHistory(root, receipt.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := ResetPromptHistory(root, false); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatal("reset without confirmation succeeded")
	}
}

func TestPromptHistoryRelevanceRanksOlderRelevantPromptAboveRecentNoise(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := RecordUserPrompt(root, promptInput("unrelated vacation logistics", PromptScopeGlobal, "owner", now.Add(-time.Minute))); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordUserPrompt(root, promptInput("case alpha decision tradeoff", PromptScopeGlobal, "owner", now.Add(-2*time.Hour))); err != nil {
		t.Fatal(err)
	}
	selected, err := SelectRelevantPromptHistory(root, PromptHistorySelectionLimits{OwnerID: "owner", MaxCount: 2, MaxBytes: 1024, MaxAge: 24 * time.Hour, ScopeKind: PromptScopeGlobal, ScopeID: "owner", CurrentPrompt: "case alpha decision"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].Entry.Prompt != "case alpha decision tradeoff" || selected[0].Score <= selected[1].Score || len(selected[0].Reasons) == 0 {
		t.Fatalf("relevance selection = %#v", selected)
	}
}

func TestPromptHistoryOwnerBindingAndConcurrentWriters(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordUserPrompt(root, promptInput("owner one", PromptScopeGlobal, "owner", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordUserPrompt(root, PromptHistoryInput{OwnerID: "owner-two", Prompt: "cross owner", Language: "pt-BR", Source: "owner", SessionID: "session-b", ScopeKind: PromptScopeGlobal, ScopeID: "owner-two", RecordedAt: time.Now().UTC(), ContentKind: "user_prompt"}); err == nil {
		t.Fatal("mixed-owner prompt was accepted")
	}
	const writers = 12
	errs := make(chan error, writers)
	var wait sync.WaitGroup
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := RecordUserPrompt(root, promptInput("concurrent-"+string(rune('a'+index)), PromptScopeGlobal, "owner", time.Now().UTC().Add(time.Duration(index)*time.Millisecond)))
			errs <- err
		}(index)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent writer failed: %v", err)
		}
	}
	entries, err := ExportPromptHistory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != writers+1 {
		t.Fatalf("lost concurrent updates: got %d entries, want %d", len(entries), writers+1)
	}
}

func TestPromptHistoryOccurrenceIsIdempotentAndContentBound(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	input := PromptHistoryInput{OwnerID: "owner", OccurrenceID: "dispatch-a", Prompt: "same prompt", Language: "en-US", Source: "cli", SessionID: "session-a", ScopeKind: PromptScopeCase, ScopeID: "case-a", ContentKind: "user_prompt"}
	first, err := RecordUserPrompt(root, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RecordUserPrompt(root, input)
	if err != nil || first.ID != second.ID {
		t.Fatalf("same occurrence was not idempotent: first=%#v second=%#v err=%v", first, second, err)
	}
	entries, err := ExportPromptHistory(root)
	if err != nil || len(entries) != 1 || entries[0].OccurrenceID != "dispatch-a" {
		t.Fatalf("occurrence history = %#v, err=%v", entries, err)
	}
	input.Prompt = "mutated prompt"
	if _, err := RecordUserPrompt(root, input); err == nil {
		t.Fatal("occurrence reuse with different prompt was accepted")
	}
}
