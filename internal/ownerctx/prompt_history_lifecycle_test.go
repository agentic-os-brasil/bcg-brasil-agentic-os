package ownerctx

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPromptHistoryFreshReadsNeverInitialize(t *testing.T) {
	root := t.TempDir()
	config, err := LoadPromptHistoryConfig(root)
	if err != nil || config != DefaultPromptHistoryConfig() {
		t.Fatalf("fresh configuration: %+v %v", config, err)
	}
	if receipts, err := InspectPromptHistory(root); err != nil || len(receipts) != 0 {
		t.Fatalf("fresh inspection: %v %v", receipts, err)
	}
	if entries, err := ExportPromptHistory(root); err != nil || len(entries) != 0 {
		t.Fatalf("fresh export: %v %v", entries, err)
	}
	if entries, err := SelectRelevantPromptHistory(root, PromptHistorySelectionLimits{OwnerID: "owner", ScopeKind: PromptScopeGlobal, ScopeID: "owner"}, time.Now().UTC()); err != nil || len(entries) != 0 {
		t.Fatalf("fresh selection: %v %v", entries, err)
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("read-only history initialized state: %v %v", entries, err)
	}
}

func TestPromptHistoryIncompleteReadsNeverRepair(t *testing.T) {
	for _, missing := range []string{"config.json", "entries.jsonl"} {
		t.Run(missing, func(t *testing.T) {
			root := t.TempDir()
			if _, err := Initialize(root); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "owner", "prompt-history", missing)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadPromptHistoryConfig(root); err == nil {
				t.Fatal("configuration admitted incomplete history")
			}
			if _, err := InspectPromptHistory(root); err == nil {
				t.Fatal("inspection admitted incomplete history")
			}
			if _, err := ExportPromptHistory(root); err == nil {
				t.Fatal("export admitted incomplete history")
			}
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatal("read repaired the missing history file", err)
			}
		})
	}
}

func TestPromptHistoryPreparationWaitsForExistingMutex(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "owner", "prompt-history", "config.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := acquirePromptHistoryLock(filepath.Join(filepath.Dir(path), promptHistoryLockName))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	// The public writer must reach the held mutex before it can prepare files.
	_, err = RecordUserPrompt(root, promptInput("after-release", PromptScopeGlobal, "owner", time.Now().UTC()))
	if !errors.Is(err, ErrPromptHistoryLocked) {
		t.Fatalf("writer bypassed held history mutex: %v", err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("blocked writer initialized a domain file before acquiring the mutex", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	unlock = nil
	// Only this explicit fixture restoration repairs the incomplete state.
	if err := publishOwnerFile(path, body, true); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordUserPrompt(root, promptInput("after-release", PromptScopeGlobal, "owner", time.Now().UTC())); err != nil {
		t.Fatal("released writer positive control failed", err)
	}
	if entries, err := ExportPromptHistory(root); err != nil || len(entries) != 1 {
		t.Fatalf("post-release record absent: %v %v", entries, err)
	}
}

func TestPromptHistoryExistingReadsPreserveDomainFiles(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	type snapshot struct {
		info os.FileInfo
		body string
	}
	take := func(path string) snapshot {
		t.Helper()
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		info, statErr := file.Stat() // eager Windows identity before Close
		if err := errors.Join(statErr, file.Close()); err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return snapshot{info, string(body)}
	}
	before := map[string]snapshot{}
	for _, name := range []string{"entries.jsonl", "config.json"} {
		path := filepath.Join(root, "owner", "prompt-history", name)
		before[path] = take(path)
	}
	if _, err := LoadPromptHistoryConfig(root); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectPromptHistory(root); err != nil {
		t.Fatal(err)
	}
	if _, err := ExportPromptHistory(root); err != nil {
		t.Fatal(err)
	}
	for path, prior := range before {
		after := take(path)
		if !os.SameFile(prior.info, after.info) || prior.info.Mode() != after.info.Mode() || !prior.info.ModTime().Equal(after.info.ModTime()) || prior.body != after.body {
			t.Fatal("read changed domain identity, attributes or bytes")
		}
	}
	config := filepath.Join(root, "owner", "prompt-history", "config.json")
	if err := atomicPrivateWrite(config, []byte("{corrupt")); err != nil {
		t.Fatal(err)
	}
	if _, err := ExportPromptHistory(root); err == nil {
		t.Fatal("corrupt configuration admitted")
	}
	if body, err := os.ReadFile(config); err != nil || string(body) != "{corrupt" {
		t.Fatal("read repaired corrupt configuration", err)
	}
}
