package yodaselfreview

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

func TestWeeklyCompactionInputHasNoInteractiveTaskAndBindsPrivateSources(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "owner/self/communication-style.md"), []byte("# Communication style\n\n## Current\n\nUse concise messages.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	options := ownerctx.WeeklySelfOptions{OccurrenceID: strings.Repeat("c", 64), OwnerID: "synthetic-owner", ScheduledFor: now, Facets: []string{"communication-style"}, Limits: ownerctx.DefaultWeeklySelfLimits(), WorkingLanguage: "en", TranslatorID: "synthetic-identity", TranslatorVersion: "1"}
	config := ownerctx.DefaultPromptHistoryConfig()
	config.OwnerID = options.OwnerID
	if err := ownerctx.ConfigurePromptHistory(root, config); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.RecordUserPrompt(root, ownerctx.PromptHistoryInput{OwnerID: options.OwnerID, OccurrenceID: "weekly-history", Prompt: "A synthetic private user interaction.", Language: "en", Source: "owner", SessionID: "synthetic-session", ScopeKind: ownerctx.PromptScopeGlobal, ScopeID: "owner", RecordedAt: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	frozen, err := ownerctx.FreezeWeeklySelf(root, options, func(text, _, _ string) (string, error) { return text, nil }, now)
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildWeeklyCompactionInput(frozen)
	if err != nil {
		t.Fatal(err)
	}
	if input.InputSHA256 != frozen.InputSHA256 || input.CanonicalSHA256 != frozen.Facets.CanonicalSourceDigest || input.PromptHistorySHA256 != frozen.PromptHistorySHA256 || input.TranslationSHA256 != frozen.TranslationSHA256 || input.JobID != WeeklyJobID || input.Mode != "quoted_data_no_tools" || len(input.PromptHistory) != 1 || !input.PromptHistory[0].QuotedData || input.PromptHistory[0].Instructional {
		t.Fatalf("model input binding invalid: %+v", input)
	}
	body, _ := json.Marshal(input)
	for _, forbidden := range []string{"current_original", "current_normalized", "current_prompt", "user_channel", "tools"} {
		if bytes.Contains(body, []byte("\""+forbidden+"\":")) {
			t.Fatal("weekly model input exposed interactive/tool field", forbidden)
		}
	}
	tampered := frozen
	tampered.InputSHA256 = strings.Repeat("f", 64)
	if _, err := BuildWeeklyCompactionInput(tampered); err == nil {
		t.Fatal("tampered frozen identity accepted")
	}
	candidate := ownerctx.WeeklySelfCandidate{SchemaVersion: 1, InputSHA256: input.InputSHA256, CanonicalSHA256: input.CanonicalSHA256, PromptHistorySHA256: input.PromptHistorySHA256, PolicySHA256: input.PolicySHA256, TranslationSHA256: input.TranslationSHA256, Facets: map[string]string{"communication-style": "Synthetic bounded working text."}}
	candidateBody, _ := json.Marshal(candidate)
	if _, err := input.ParseCandidate(candidateBody); err != nil {
		t.Fatal(err)
	}
	if _, err := (WeeklyCompactionInput{}).ParseCandidate(candidateBody); err == nil {
		t.Fatal("unbuilt adapter input accepted")
	}
	input.PromptHistory[0].OriginalText = "changed local adapter copy"
	if _, err := input.ParseCandidate(candidateBody); err == nil {
		t.Fatal("mutated adapter input accepted")
	}

	if frozen.PromptHistory[0].Source.Prompt != "A synthetic private user interaction." {
		t.Fatal("model projection mutated frozen source")
	}
	delete(input.CanonicalSnapshot.Facets, "communication-style")
	if len(frozen.Facets.Facets) != 1 {
		t.Fatal("model projection mutated the frozen facet map")
	}
	priorCandidate := ownerctx.WeeklySelfCandidate{SchemaVersion: 1, InputSHA256: frozen.InputSHA256, CanonicalSHA256: frozen.Facets.CanonicalSourceDigest, PromptHistorySHA256: frozen.PromptHistorySHA256, PolicySHA256: frozen.PolicySHA256, TranslationSHA256: frozen.TranslationSHA256, Facets: map[string]string{"communication-style": "Previously compacted private owner knowledge."}}
	if _, err := ownerctx.PublishWeeklySelf(context.Background(), root, frozen.InputSHA256, priorCandidate, frozen.ExpectedRevision, func(write func() error) error { return write() }, now); err != nil {
		t.Fatal(err)
	}
	options.OccurrenceID = strings.Repeat("d", 64)
	options.ScheduledFor = now.Add(time.Second)
	next, err := ownerctx.FreezeWeeklySelf(root, options, func(text, _, _ string) (string, error) { return text, nil }, options.ScheduledFor)
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := BuildWeeklyCompactionInput(next)
	if err != nil {
		t.Fatal(err)
	}
	if consumer.PreviousWorking["communication-style"] != priorCandidate.Facets["communication-style"] || consumer.PreviousWorkingSHA256 != next.PreviousWorking.Receipt.WorkingSHA256 {
		t.Fatal("production model input dropped prior working knowledge")
	}
	consumer.PreviousWorking["communication-style"] = "changed local model copy"
	if next.PreviousWorking.Facets["communication-style"] != priorCandidate.Facets["communication-style"] {
		t.Fatal("model input mutated previous private working source")
	}

}
