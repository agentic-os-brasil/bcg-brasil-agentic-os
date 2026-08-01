package ownerctx

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	selected, err := SelectPromptHistory(root, PromptHistorySelectionLimits{MaxCount: 2, MaxBytes: 64, MaxAge: 24 * time.Hour, ScopeKind: PromptScopeCase, ScopeID: "case-a"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].Prompt != "global context" || selected[1].Prompt != "case context" {
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
