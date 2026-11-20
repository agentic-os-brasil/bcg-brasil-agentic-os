package ownerctx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWeeklySelfFrozenInputAndSilentWorkingProjection(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(root, "owner/self/communication-style.md")
	original := []byte("# Communication style\n\n## Current\n\nUse concise messages and an explicit next step.\n")
	if err := os.WriteFile(canonical, original, 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	options := WeeklySelfOptions{OccurrenceID: strings.Repeat("a", 64), OwnerID: "synthetic-owner", ScheduledFor: now, Facets: []string{"communication-style"}, Limits: DefaultWeeklySelfLimits(), WorkingLanguage: "en", TranslatorID: "synthetic-identity", TranslatorVersion: "1"}
	input, err := FreezeWeeklySelf(root, options, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if input.InputSHA256 == "" || len(input.PromptHistory) != 0 || len(input.Facets.Facets) != 1 {
		t.Fatalf("unexpected frozen input: %+v", input)
	}
	replay, err := FreezeWeeklySelf(root, options, nil, now.Add(time.Hour))
	if err != nil || replay.InputSHA256 != input.InputSHA256 || !replay.FrozenAt.Equal(input.FrozenAt) {
		t.Fatalf("frozen replay changed: %+v, %v", replay, err)
	}
	candidate := WeeklySelfCandidate{SchemaVersion: 1, CanonicalSHA256: input.Facets.CanonicalSourceDigest, PromptHistorySHA256: input.PromptHistorySHA256, InputSHA256: input.InputSHA256, PolicySHA256: input.PolicySHA256, TranslationSHA256: input.TranslationSHA256, Facets: map[string]string{"communication-style": "Concise messages; explicit next step."}}
	receipt, err := PublishWeeklySelf(context.Background(), root, input.InputSHA256, candidate, 0, func(write func() error) error { return write() }, now)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.State != "committed" || receipt.Revision != 1 || receipt.WorkingSHA256 == "" {
		t.Fatalf("receipt = %+v", receipt)
	}
	working, err := ReadWeeklySelf(root, options, now)
	if err != nil || working.Facets["communication-style"] != "Concise messages; explicit next step." {
		t.Fatalf("working = %+v, %v", working, err)
	}
	actual, err := os.ReadFile(canonical)
	if err != nil || !bytes.Equal(actual, original) {
		t.Fatal("canonical SELF changed", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "owner/refinement/proposals")); len(entries) != 0 {
		t.Fatal("weekly compaction created a user-visible proposal")
	}
	body, err := os.ReadFile(filepath.Join(root, "owner/self/weekly/current.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("Use concise messages and an explicit next step.")) {
		t.Fatal("raw frozen canonical text retained after atomic working commit")
	}
	next, err := PublishWeeklySelf(context.Background(), root, input.InputSHA256, candidate, 0, func(write func() error) error { return write() }, now)
	if err != nil || next.Revision != receipt.Revision {
		t.Fatal("committed replay advanced working revision", err)
	}
}

func weeklyCoreRoot(t *testing.T) (string, WeeklySelfOptions, time.Time) {
	t.Helper()
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "owner/self/communication-style.md"), []byte("# Communication style\n\n## Current\n\nUse concise messages and an explicit next step.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	return root, WeeklySelfOptions{OccurrenceID: strings.Repeat("b", 64), OwnerID: "synthetic-owner", ScheduledFor: now, Facets: []string{"communication-style"}, Limits: DefaultWeeklySelfLimits(), WorkingLanguage: "en", TranslatorID: "synthetic-identity", TranslatorVersion: "1"}, now
}

func weeklyCandidate(input WeeklySelfInput) WeeklySelfCandidate {
	return WeeklySelfCandidate{SchemaVersion: 1, InputSHA256: input.InputSHA256, CanonicalSHA256: input.Facets.CanonicalSourceDigest, PromptHistorySHA256: input.PromptHistorySHA256, PolicySHA256: input.PolicySHA256, TranslationSHA256: input.TranslationSHA256, Facets: map[string]string{"communication-style": "Concise messages; explicit next step."}}
}

func TestWeeklySelfChangedSourceAndIgnoredWriteErrorPreservePending(t *testing.T) {
	root, options, now := weeklyCoreRoot(t)
	input, err := FreezeWeeklySelf(root, options, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(weeklySelfPath(root))
	if err := os.WriteFile(filepath.Join(root, "owner/self/communication-style.md"), []byte("# Current\n\nA newer owner correction.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = PublishWeeklySelf(context.Background(), root, input.InputSHA256, weeklyCandidate(input), 0, func(write func() error) error { _ = write(); return nil }, now)
	if !errors.Is(err, ErrWeeklySelfChanged) {
		t.Fatalf("ignored owning error became success: %v", err)
	}
	after, _ := os.ReadFile(weeklySelfPath(root))
	if !bytes.Equal(before, after) {
		t.Fatal("source denial altered pending/working data")
	}
}

func TestWeeklySelfPublicationBoundaryFailuresAreTruthful(t *testing.T) {
	for _, kind := range []string{"no_op", "late", "cancelled", "double", "failed_after_commit", "failed_readback"} {
		t.Run(kind, func(t *testing.T) {
			root, options, now := weeklyCoreRoot(t)
			input, err := FreezeWeeklySelf(root, options, nil, now)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(weeklySelfPath(root))
			var late func() error
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			boundary := func(write func() error) error {
				switch kind {
				case "no_op":
					return nil
				case "late":
					late = write
					return nil
				case "cancelled":
					cancel()
					return write()
				case "double":
					_ = write()
					_ = write()
					return nil
				case "failed_readback":
					if err := write(); err != nil {
						return err
					}
					return os.Link(weeklySelfPath(root), weeklySelfPath(root)+".readback-alias")
				default:
					if err := write(); err != nil {
						return err
					}
					return errors.New("synthetic boundary failure")
				}
			}
			receipt, err := PublishWeeklySelf(ctx, root, input.InputSHA256, weeklyCandidate(input), 0, boundary, now)
			if err == nil {
				t.Fatal("misbehaving boundary reported success")
			}
			if late != nil && !errors.Is(late(), ErrWeeklySelfBoundary) {
				t.Fatal("late write was admitted")
			}
			after, _ := os.ReadFile(weeklySelfPath(root))
			if kind == "double" || kind == "failed_after_commit" || kind == "failed_readback" {
				if receipt.State != "partial" || receipt.Verified || receipt.Revision != 1 || bytes.Equal(before, after) {
					t.Fatalf("durable partial result was hidden: %+v", receipt)
				}
				if kind == "failed_readback" {
					if err := os.Remove(weeklySelfPath(root) + ".readback-alias"); err != nil {
						t.Fatal(err)
					}
				}
				working, readErr := ReadWeeklySelf(root, options, now)
				if readErr != nil || working.Receipt.Revision != 1 {
					t.Fatal("durable first commit was rolled back", readErr)
				}
			} else if !bytes.Equal(before, after) {
				t.Fatal("pre-commit denial changed private bytes")
			}
		})
	}
}

func TestWeeklySelfPromptReplayDeletionAndOwningPurge(t *testing.T) {
	root, options, now := weeklyCoreRoot(t)
	config := DefaultPromptHistoryConfig()
	config.OwnerID = options.OwnerID
	if err := ConfigurePromptHistory(root, config); err != nil {
		t.Fatal(err)
	}
	oldText := "Synthetic selected interaction, kept private."
	first, err := RecordUserPrompt(root, PromptHistoryInput{OwnerID: options.OwnerID, OccurrenceID: "prompt-a", Prompt: oldText, Language: "en", Source: "owner", SessionID: "session-a", ScopeKind: PromptScopeGlobal, ScopeID: "owner", RecordedAt: now.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	normalize := func(text, _, _ string) (string, error) { return text, nil }
	input, err := FreezeWeeklySelf(root, options, normalize, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.PromptHistory) != 1 {
		t.Fatal("eligible history was not selected")
	}
	pendingBytes, _ := os.ReadFile(weeklySelfPath(root))
	if bytes.Contains(pendingBytes, []byte(oldText)) || bytes.Contains(pendingBytes, []byte("Use concise messages and an explicit next step.")) {
		t.Fatal("recovery manifest duplicated raw prompt or canonical SELF")
	}
	if _, err := RecordUserPrompt(root, PromptHistoryInput{OwnerID: options.OwnerID, OccurrenceID: "prompt-b", Prompt: "Later evidence belongs to a later occurrence.", Language: "en", Source: "owner", SessionID: "session-b", ScopeKind: PromptScopeGlobal, ScopeID: "owner", RecordedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	replay, err := FreezeWeeklySelf(root, options, normalize, time.Now().UTC())
	if err != nil || replay.InputSHA256 != input.InputSHA256 || len(replay.PromptHistory) != 1 {
		t.Fatal("replay picked up a later source", err)
	}
	if _, err := PublishWeeklySelf(context.Background(), root, input.InputSHA256, weeklyCandidate(input), 0, func(write func() error) error { return write() }, now); err != nil {
		t.Fatal(err)
	}
	if err := DeletePromptHistory(root, first.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadWeeklySelf(root, options, now); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owning prompt delete did not already remove derived view: %v", err)
	}
	changed, err := PruneWeeklySelf(root, options, now)
	if err != nil || changed {
		t.Fatal("already-purged view needed a later cleanup", err)
	}
	body, _ := os.ReadFile(weeklySelfPath(root))
	if bytes.Contains(body, []byte(oldText)) || bytes.Contains(body, []byte("Concise messages; explicit next step.")) {
		t.Fatal("owning source purge retained private working/input text")
	}
}

func TestWeeklySelfStrictCandidateAndSensitiveFacet(t *testing.T) {
	root, options, now := weeklyCoreRoot(t)
	input, err := FreezeWeeklySelf(root, options, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	candidate := weeklyCandidate(input)
	body, _ := json.Marshal(candidate)
	if _, err := ParseWeeklySelfCandidate(input, body); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"unknown", "duplicate", "trailing", "prose", "schema", "canonical", "history", "policy", "extra_facet", "missing_facet", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			c := weeklyCandidate(input)
			switch kind {
			case "schema":
				c.SchemaVersion = 2
			case "canonical":
				c.CanonicalSHA256 = strings.Repeat("f", 64)
			case "history":
				c.PromptHistorySHA256 = strings.Repeat("f", 64)
			case "policy":
				c.PolicySHA256 = strings.Repeat("f", 64)
			case "extra_facet":
				c.Facets["voice"] = "Unknown selected facet"
			case "missing_facet":
				delete(c.Facets, "communication-style")
			case "oversized":
				c.Facets["communication-style"] = strings.Repeat("x", options.Limits.MaxWorkingFacetBytes+1)
			}
			b, _ := json.Marshal(c)
			switch kind {
			case "unknown":
				b = []byte(strings.Replace(string(b), "{", "{\"extra\":true,", 1))
			case "duplicate":
				b = []byte(strings.Replace(string(b), "\"schema_version\":1", "\"schema_version\":1,\"schema_version\":1", 1))
			case "trailing":
				b = append(b, []byte("{}")...)
			case "prose":
				b = append([]byte("Here is the result:"), b...)
			}
			if _, err := ParseWeeklySelfCandidate(input, b); err == nil {
				t.Fatal("untrusted candidate accepted")
			}
		})
	}
	root, options, now = weeklyCoreRoot(t)
	options.Facets = []string{"psychological-profile"}
	if err := os.WriteFile(filepath.Join(root, "owner/self/psychological-profile.md"), []byte("# Authorized profile\n\n## Current\n\nSynthetic authorized context.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := FreezeWeeklySelf(root, options, nil, now); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("sensitive facet lacked exact authorization: %v", err)
	}
}

func TestWeeklySelfExpiredInputPurgesDespiteRemovedFacet(t *testing.T) {
	root, options, now := weeklyCoreRoot(t)
	options.Limits.RecoverySeconds = 1
	if _, err := FreezeWeeklySelf(root, options, nil, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "owner/self/communication-style.md")); err != nil {
		t.Fatal(err)
	}
	changed, err := PruneWeeklySelf(root, options, now.Add(2*time.Second))
	if err != nil || !changed {
		t.Fatalf("expired raw input could not be purged after source removal: %v", err)
	}
	body, _ := os.ReadFile(weeklySelfPath(root))
	if bytes.Contains(body, []byte("Use concise messages")) {
		t.Fatal("expired raw source retained")
	}
	if err := os.WriteFile(filepath.Join(root, "owner/self/communication-style.md"), []byte("# Current\n\nA later source version.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := FreezeWeeklySelf(root, options, nil, now.Add(3*time.Second)); !errors.Is(err, ErrWeeklySelfChanged) {
		t.Fatalf("purged occurrence was reconstructed from newer input: %v", err)
	}
}

func TestWeeklySelfCorroborationSourceRedactionInvalidatesWorking(t *testing.T) {
	root, options, _ := weeklyCoreRoot(t)
	observation := ObservationInput{SchemaVersion: 1, Signal: SignalExplicitInstruction, Facet: "communication-style", Claim: "concise_updates", EvidenceType: "owner_statement", SourceEvent: "owner.explicit_instruction", SourceDigest: strings.Repeat("a", 64), EpisodeID: "weekly-episode-a", ScopeKind: "global", ScopeID: "owner", Confidence: 1, Sensitivity: "professional", ExpiresAt: time.Now().UTC().Add(time.Hour), AuthenticatedOwner: true, Material: true, OwnerConfirmed: true, DeclassifiedGlobal: true}
	first, _, err := AppendObservation(root, observation)
	if err != nil {
		t.Fatal(err)
	}
	observation.EpisodeID = "weekly-episode-b"
	observation.SourceDigest = strings.Repeat("b", 64)
	second, _, err := AppendObservation(root, observation)
	if err != nil {
		t.Fatal(err)
	}
	first, err = TransitionObservation(root, ObservationTransitionInput{ObservationID: first.ID, TransitionID: "weekly-eligible", Next: ObservationEligible, ExpectedState: first.State, ExpectedRevision: first.Revision, OwnerAction: true})
	if err != nil {
		t.Fatal(err)
	}
	first, err = TransitionObservation(root, ObservationTransitionInput{ObservationID: first.ID, TransitionID: "weekly-corroborated", Next: ObservationCorroborated, ExpectedState: first.State, ExpectedRevision: first.Revision, OwnerAction: true})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	options.ScheduledFor = now
	input, err := FreezeWeeklySelf(root, options, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Observations) != 1 || input.ObservationSupport[second.ID] != second.Revision {
		t.Fatalf("independent support not bound: %+v", input.ObservationSupport)
	}
	if _, err := PublishWeeklySelf(context.Background(), root, input.InputSHA256, weeklyCandidate(input), 0, func(write func() error) error { return write() }, now); err != nil {
		t.Fatal(err)
	}
	if _, err := TransitionObservation(root, ObservationTransitionInput{ObservationID: second.ID, TransitionID: "weekly-redact-support", Next: ObservationRedacted, ExpectedState: second.State, ExpectedRevision: second.Revision, OwnerAction: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadWeeklySelf(root, options, now); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owning support redaction did not already remove derived view: %v", err)
	}
	if changed, err := PruneWeeklySelf(root, options, now); err != nil || changed {
		t.Fatal("already-purged support view needed a later cleanup", err)
	}
}

func TestWeeklySelfRawInputBoundPrecedesNormalization(t *testing.T) {
	root, options, now := weeklyCoreRoot(t)
	options.Limits.MaxInputBytes = 128
	config := DefaultPromptHistoryConfig()
	config.OwnerID = options.OwnerID
	if err := ConfigurePromptHistory(root, config); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordUserPrompt(root, PromptHistoryInput{OwnerID: options.OwnerID, OccurrenceID: "raw-bound", Prompt: "Synthetic selected source.", Language: "en", Source: "owner", SessionID: "bound-session", ScopeKind: PromptScopeGlobal, ScopeID: "owner", RecordedAt: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if _, err := FreezeWeeklySelf(root, options, func(text, _, _ string) (string, error) { calls++; return text, nil }, now); err == nil {
		t.Fatal("oversized complete raw input accepted")
	}
	if calls != 0 {
		t.Fatal("normalizer ran before the combined raw input bound")
	}
}

func TestWeeklySelfReadDoesNotInitializeAndOwnerOccurrencesContend(t *testing.T) {
	root, options, now := weeklyCoreRoot(t)
	if _, err := ReadWeeklySelf(root, options, now); err == nil {
		t.Fatal("missing working projection reported ready")
	}
	if _, err := os.Lstat(filepath.Join(root, "owner/self/weekly")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read initialized weekly state: %v", err)
	}
	type attempt struct {
		input WeeklySelfInput
		err   error
	}
	results := make(chan attempt, 2)
	for _, id := range []string{strings.Repeat("d", 64), strings.Repeat("e", 64)} {
		copy := options
		copy.OccurrenceID = id
		go func(o WeeklySelfOptions) {
			input, err := FreezeWeeklySelf(root, o, nil, now)
			results <- attempt{input, err}
		}(copy)
	}
	success, denied := 0, 0
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err == nil {
			success++
		} else if errors.Is(r.err, ErrWeeklySelfChanged) {
			denied++
		} else {
			t.Fatal(r.err)
		}
	}
	if success != 1 || denied != 1 {
		t.Fatalf("same owner/scheduled instant admitted multiple occurrences: success=%d denied=%d", success, denied)
	}
}

func TestWeeklySelfRejectsMissingCommitBoundary(t *testing.T) {
	_, err := PublishWeeklySelf(context.Background(), t.TempDir(), strings.Repeat("a", 64), WeeklySelfCandidate{}, 0, nil, time.Now().UTC())
	if !errors.Is(err, ErrWeeklySelfUnavailable) {
		t.Fatalf("missing boundary error = %v", err)
	}
}

func TestWeeklyWorkingReadRejectsMalformedReceiptMetadata(t *testing.T) {
	root, options, now := weeklyCoreRoot(t)
	input, err := FreezeWeeklySelf(root, options, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = PublishWeeklySelf(context.Background(), root, input.InputSHA256, weeklyCandidate(input), input.ExpectedRevision, func(write func() error) error { return write() }, now); err != nil {
		t.Fatal(err)
	}
	state, err := readWeeklySelfState(root)
	if err != nil {
		t.Fatal(err)
	}
	state.Working.Receipt.InputSHA256 = strings.Repeat("x", 4096)
	working := state.Working
	candidate := WeeklySelfCandidate{SchemaVersion: 1, InputSHA256: working.Receipt.InputSHA256, CanonicalSHA256: working.Source.CanonicalSHA256, PromptHistorySHA256: working.Source.PromptHistorySHA256, PolicySHA256: working.Source.PolicySHA256, TranslationSHA256: working.Source.TranslationSHA256, Facets: working.Facets}
	working.Receipt.WorkingSHA256 = weeklyCandidateDigest(candidate)
	if err = writeWeeklySelfState(root, state); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadWeeklySelf(root, options, now); err == nil {
		t.Fatal("malformed receipt can escape the bounded metadata report")
	}
}
