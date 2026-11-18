package ownerctx

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWeeklySelfNextInputConsumesWorkingAndRetainsDeletionDependencies(t *testing.T) {
	root, options, now := weeklyCoreRoot(t)
	config := DefaultPromptHistoryConfig()
	config.OwnerID = options.OwnerID
	if err := ConfigurePromptHistory(root, config); err != nil {
		t.Fatal(err)
	}
	old, err := RecordUserPrompt(root, PromptHistoryInput{OwnerID: options.OwnerID, OccurrenceID: "prior-source", Prompt: "An older source retained only through working compaction.", Language: "en", Source: "owner", SessionID: "prior-session", ScopeKind: PromptScopeGlobal, ScopeID: "owner", RecordedAt: now.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	normalize := func(text, _, _ string) (string, error) { return text, nil }
	first, err := FreezeWeeklySelf(root, options, normalize, now)
	if err != nil {
		t.Fatal(err)
	}
	previousText := "Private knowledge preserved from the previous weekly pass."
	candidate := weeklyCandidate(first)
	candidate.Facets["communication-style"] = previousText
	if _, err := PublishWeeklySelf(context.Background(), root, first.InputSHA256, candidate, 0, func(write func() error) error { return write() }, now); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := RecordUserPrompt(root, PromptHistoryInput{OwnerID: options.OwnerID, OccurrenceID: fmt.Sprintf("new-source-%d", i), Prompt: fmt.Sprintf("New selected evidence %d.", i), Language: "en", Source: "owner", SessionID: "new-session", ScopeKind: PromptScopeGlobal, ScopeID: "owner", RecordedAt: now.Add(time.Duration(i) * time.Millisecond)}); err != nil {
			t.Fatal(err)
		}
	}
	options.OccurrenceID = strings.Repeat("e", 64)
	options.ScheduledFor = now.Add(time.Second)
	next, err := FreezeWeeklySelf(root, options, normalize, options.ScheduledFor)
	if err != nil {
		t.Fatal(err)
	}
	if next.PreviousWorking == nil || next.PreviousWorking.Facets["communication-style"] != previousText {
		t.Fatal("next weekly input did not consume current policy-bound working text")
	}
	for _, prompt := range next.PromptHistory {
		if prompt.Source.ID == old.ID {
			t.Fatal("fixture did not move old source outside newest raw window")
		}
	}
	state, err := readWeeklySelfState(root)
	if err != nil {
		t.Fatal(err)
	}
	if state.Pending.Input.PreviousWorking == nil || len(state.Pending.Input.PreviousWorking.Facets) != 0 {
		t.Fatal("recovery manifest duplicated private previous working text")
	}
	body, _ := os.ReadFile(weeklySelfPath(root))
	if bytes.Count(body, []byte(previousText)) != 1 {
		t.Fatal("fixed state duplicated previous working text")
	}
	replay, err := FreezeWeeklySelf(root, options, normalize, options.ScheduledFor)
	if err != nil || replay.InputSHA256 != next.InputSHA256 || replay.PreviousWorking.Facets["communication-style"] != previousText {
		t.Fatal("prior working input did not reconstruct exactly", err)
	}
	newer := weeklyCandidate(next)
	newer.Facets["communication-style"] = previousText + " Added current evidence."
	if _, err := PublishWeeklySelf(context.Background(), root, next.InputSHA256, newer, next.ExpectedRevision, func(write func() error) error { return write() }, options.ScheduledFor); err != nil {
		t.Fatal(err)
	}
	current, err := ReadWeeklySelf(root, options, options.ScheduledFor)
	if err != nil || current.Source.Prompts[old.ID] == "" {
		t.Fatal("compaction lost inherited deletion provenance", err)
	}
	if err := DeletePromptHistory(root, old.ID, true); err != nil {
		t.Fatal(err)
	}
	state, err = readWeeklySelfState(root)
	if err != nil || state.Working != nil {
		t.Fatal("deleting inherited source retained derived private knowledge", err)
	}
}
