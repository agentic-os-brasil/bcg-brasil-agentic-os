package ownerctx

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPromptHistoryLegalMetadataEnvelope(t *testing.T) {
	entry := PromptHistoryEntry{SchemaVersion: 1, ID: strings.Repeat("I", 128), OwnerID: strings.Repeat("o", 128), OccurrenceID: strings.Repeat("e", 128), SessionID: strings.Repeat("s", 128), ScopeID: strings.Repeat("w", 128), ScopeKind: PromptScopeWorkspace, Language: "abcdefgh-abcdefgh", Source: "claude", RecordedAt: time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.FixedZone("<>&\"\\", 23*3600+59*60)), ContentKind: "user_prompt", Prompt: "<>&\"\\\x00\n"}
	entry.SHA256 = digest(entry.Prompt)
	if err := validatePromptHistoryEntry(entry); err != nil {
		t.Fatal(err)
	}
	// Identifier/language/enumeration grammars exclude JSON-escaped characters;
	// time.MarshalJSON emits only the bounded RFC3339 numeric representation.
	for _, prompt := range []string{"<", ">", "&", "\"", "\\", "\x00", "\n"} {
		encoded, err := json.Marshal(prompt)
		if err != nil || len(encoded)-2 > 6*len(prompt) {
			t.Fatal("accepted prompt escaping exceeds six bytes per input byte", err)
		}
	}
	entry.Prompt = ""
	body, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if len(body)+1 != 975 {
		t.Fatalf("largest legal metadata grammar changed: %d", len(body)+1)
	}
}

func TestPromptHistoryNearMaxRawBytesSurviveEscapedSerialization(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := ConfigurePromptHistory(root, PromptHistoryConfig{SchemaVersion: 1, MaxEntries: 129, MaxBytes: 8 << 20, MaxAgeSeconds: 86400}); err != nil {
		t.Fatal(err)
	}
	owner := strings.Repeat("o", 128)
	base := time.Now().UTC().Add(-time.Minute)
	ids := map[string]bool{}
	first := ""
	for index := 0; index < 128; index++ {
		text := strings.Repeat("x", 64<<10)
		if index == 127 {
			text = strings.Repeat("<", 64<<10)
		}
		input := promptInput(text, PromptScopeGlobal, owner, base.Add(time.Duration(index)*time.Millisecond))
		input.OwnerID = owner
		input.SessionID = strings.Repeat("s", 128)
		input.Language = "abcdefgh-abcdefgh"
		input.Source = "claude"
		input.OccurrenceID = fmt.Sprintf("event-%03d-%s", index, strings.Repeat("e", 118))
		receipt, err := RecordUserPrompt(root, input)
		if err != nil {
			t.Fatalf("legal public append %d failed: %v", index, err)
		}
		ids[receipt.ID] = true
		if index == 0 {
			first = receipt.ID
		}
	}
	info, err := os.Stat(PromptHistoryEntriesPath(root))
	if err != nil || info.Size() <= 8<<20 {
		t.Fatal("public fixture did not exceed the raw budget in serialized bytes", err)
	}
	entries, err := ExportPromptHistory(root)
	if err != nil || len(entries) != 128 {
		t.Fatalf("near-capacity history unreadable: count=%d err=%v", len(entries), err)
	}
	raw := 0
	for _, entry := range entries {
		raw += len(entry.Prompt)
		if !ids[entry.ID] {
			t.Fatal("history lost public receipt identity")
		}
	}
	if raw != 8<<20 || entries[127].Prompt != strings.Repeat("<", 64<<10) {
		t.Fatal("raw budget or escaped prompt changed")
	}
	input := promptInput("tail", PromptScopeGlobal, owner, base.Add(time.Second))
	input.OwnerID = owner
	last, err := RecordUserPrompt(root, input)
	if err != nil {
		t.Fatal("append after maximum readable store failed", err)
	}
	delete(ids, first)
	ids[last.ID] = true
	entries, err = ExportPromptHistory(root)
	if err != nil || len(entries) != 128 {
		t.Fatalf("raw-byte retention changed: %d %v", len(entries), err)
	}
	for _, entry := range entries {
		if !ids[entry.ID] {
			t.Fatal("serialized budget dropped extra history")
		}
		delete(ids, entry.ID)
	}
	if len(ids) != 0 {
		t.Fatal("raw-byte retention lost an expected entry")
	}
	if err := ResetPromptHistory(root, true); err != nil {
		t.Fatal("maximum history cannot reset", err)
	}
	if entries, err := ExportPromptHistory(root); err != nil || len(entries) != 0 {
		t.Fatal("reset did not leave readable empty history", err)
	}
}
