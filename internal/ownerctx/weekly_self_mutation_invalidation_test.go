package ownerctx

import (
	"bytes"
	"context"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func assertMutationPurgedWeekly(t *testing.T, root, watermark string) {
	t.Helper()
	state, err := readWeeklySelfState(root)
	if err != nil || state.Pending != nil || state.Working != nil || state.LastInputSHA256 != watermark {
		t.Fatalf("source mutation left private weekly text or lost occurrence watermark: pending=%t working=%t err=%v", state.Pending != nil, state.Working != nil, err)
	}
}

func TestWeeklyMutationSelectedSourcesPurgeImmediately(t *testing.T) {
	for _, kind := range []string{"facet_body", "prompt_delete"} {
		t.Run(kind, func(t *testing.T) {
			root, options, now := weeklyCoreRoot(t)
			var prompt PromptHistoryReceipt
			if kind == "prompt_delete" {
				config := DefaultPromptHistoryConfig()
				config.OwnerID = options.OwnerID
				if err := ConfigurePromptHistory(root, config); err != nil {
					t.Fatal(err)
				}
				var err error
				prompt, err = RecordUserPrompt(root, PromptHistoryInput{OwnerID: options.OwnerID, OccurrenceID: "selected-prompt", Prompt: "Private selected prompt for owning purge.", Language: "en", Source: "owner", SessionID: "selected-session", ScopeKind: PromptScopeGlobal, ScopeID: "owner", RecordedAt: now.Add(-time.Hour)})
				if err != nil {
					t.Fatal(err)
				}
			}
			input, err := FreezeWeeklySelf(root, options, func(text, _, _ string) (string, error) { return text, nil }, now)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "facet_body" {
				if _, err := PublishWeeklySelf(context.Background(), root, input.InputSHA256, weeklyCandidate(input), 0, func(write func() error) error { return write() }, now); err != nil {
					t.Fatal(err)
				}
				proposal, err := SubmitRefinement(root, RefinementInput{Facet: "communication-style", Evidence: "Explicit professional correction", ProposedBody: "# Current\n\nUse a different concise professional presentation.\n"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := ApplyRefinement(root, proposal.ID, true); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := DeletePromptHistory(root, prompt.ID, true); err != nil {
					t.Fatal(err)
				}
			}
			assertMutationPurgedWeekly(t, root, input.InputSHA256)
		})
	}
}

func weeklyMutationBytes(t *testing.T, root string) []byte {
	t.Helper()
	b, err := os.ReadFile(weeklySelfPath(root))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func assertWeeklyMutationUnchanged(t *testing.T, root string, before []byte) {
	t.Helper()
	if !bytes.Equal(before, weeklyMutationBytes(t, root)) {
		t.Fatal("unrelated mutation purged or rewrote selected weekly state")
	}
}

func TestWeeklyMutationHistoryOwnersAndUnrelatedAdds(t *testing.T) {
	for _, kind := range []string{"reviewed_delete", "direct_reset", "reviewed_reset", "configuration", "direct_retention", "reviewed_retention", "unrelated_append_delete"} {
		t.Run(kind, func(t *testing.T) {
			root, o, now := weeklyCoreRoot(t)
			cfg := DefaultPromptHistoryConfig()
			cfg.OwnerID = o.OwnerID
			if strings.Contains(kind, "retention") {
				cfg.MaxEntries = 1
			}
			if err := ConfigurePromptHistory(root, cfg); err != nil {
				t.Fatal(err)
			}
			in := PromptHistoryInput{OwnerID: o.OwnerID, OccurrenceID: "selected", Prompt: "Selected private history.", Language: "en", Source: "owner", SessionID: "session", ScopeKind: PromptScopeGlobal, ScopeID: "owner", RecordedAt: now.Add(-time.Hour)}
			selected, err := RecordUserPrompt(root, in)
			if err != nil {
				t.Fatal(err)
			}
			input, err := FreezeWeeklySelf(root, o, func(text, _, _ string) (string, error) { return text, nil }, now)
			if err != nil || len(input.PromptHistory) != 1 {
				t.Fatalf("fixture did not select prompt: %v", err)
			}
			before := weeklyMutationBytes(t, root)
			switch kind {
			case "reviewed_delete":
				r, err := ReviewPromptHistory(root, PromptHistoryChange{Operation: "delete", IDs: []string{selected.ID}})
				if err != nil {
					t.Fatal(err)
				}
				if err := ApplyReviewedPromptHistory(root, r, false); !errors.Is(err, ErrConfirmationRequired) {
					t.Fatal(err)
				}
				assertWeeklyMutationUnchanged(t, root, before)
				if err := ApplyReviewedPromptHistory(root, r, true); err != nil {
					t.Fatal(err)
				}
			case "direct_reset":
				if err := ResetPromptHistory(root, true); err != nil {
					t.Fatal(err)
				}
			case "reviewed_reset":
				r, err := ReviewPromptHistory(root, PromptHistoryChange{Operation: "reset"})
				if err != nil {
					t.Fatal(err)
				}
				if err := ApplyReviewedPromptHistory(root, r, true); err != nil {
					t.Fatal(err)
				}
			case "configuration":
				cfg.MaxAgeSeconds--
				if err := ConfigurePromptHistory(root, cfg); err != nil {
					t.Fatal(err)
				}
			default:
				in.OccurrenceID = "later"
				in.RecordedAt = now
				in.Prompt = "Later unrelated prompt."
				if kind == "reviewed_retention" {
					r, err := ReviewPromptHistory(root, PromptHistoryChange{Operation: "record", Input: &in})
					if err != nil {
						t.Fatal(err)
					}
					if err := ApplyReviewedPromptHistory(root, r, true); err != nil {
						t.Fatal(err)
					}
				} else {
					r, err := RecordUserPrompt(root, in)
					if err != nil {
						t.Fatal(err)
					}
					if kind == "unrelated_append_delete" {
						assertWeeklyMutationUnchanged(t, root, before)
						if err := DeletePromptHistory(root, r.ID, true); err != nil {
							t.Fatal(err)
						}
						assertWeeklyMutationUnchanged(t, root, before)
						return
					}
				}
			}
			assertMutationPurgedWeekly(t, root, input.InputSHA256)
		})
	}
}

func TestWeeklyMutationFacetPoliciesAndRecovery(t *testing.T) {
	for _, kind := range []string{"readers", "sensitivity", "refinement", "unrelated_policy", "owner_metadata", "apply_recovery", "revert", "reset"} {
		t.Run(kind, func(t *testing.T) {
			root, o, now := weeklyCoreRoot(t)
			var proposal RefinementReceipt
			var review RefinementReview
			var err error
			if kind == "apply_recovery" || kind == "revert" {
				proposal, err = SubmitRefinement(root, RefinementInput{Facet: "communication-style", Evidence: "Exact owner correction", ProposedBody: "# Current\n\nA newly reviewed concise style.\n"})
				if err != nil {
					t.Fatal(err)
				}
				review, err = ReviewRefinement(root, proposal.ID)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "revert" {
					proposal, err = ApplyReviewedRefinement(root, proposal.ID, review.ReviewDigest, true)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			input, err := FreezeWeeklySelf(root, o, nil, now)
			if err != nil {
				t.Fatal(err)
			}
			before := weeklyMutationBytes(t, root)
			switch kind {
			case "apply_recovery":
				original := refinementWriteJSON
				defer func() { refinementWriteJSON = original }()
				interrupted := errors.New("synthetic after-facet publication")
				refinementWriteJSON = func(path string, value any) error {
					if a, ok := value.(audit); ok && a.State == "applied" {
						return interrupted
					}
					return original(path, value)
				}
				if _, err := ApplyReviewedRefinement(root, proposal.ID, review.ReviewDigest, true); !errors.Is(err, interrupted) {
					t.Fatal(err)
				}
				refinementWriteJSON = original
				assertMutationPurgedWeekly(t, root, input.InputSHA256)
				next := o
				next.OccurrenceID = strings.Repeat("d", 64)
				next.ScheduledFor = now.Add(time.Second)
				input, err = FreezeWeeklySelf(root, next, nil, now.Add(time.Second))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := ApplyReviewedRefinement(root, proposal.ID, review.ReviewDigest, true); err != nil {
					t.Fatal(err)
				}
			case "revert":
				if _, err := RevertRefinement(root, proposal.AuditID, false); !errors.Is(err, ErrConfirmationRequired) {
					t.Fatal(err)
				}
				assertWeeklyMutationUnchanged(t, root, before)
				if _, err := RevertRefinement(root, proposal.AuditID, true); err != nil {
					t.Fatal(err)
				}
			case "reset":
				if err := ResetDerivedSelf(root, true); err != nil {
					t.Fatal(err)
				}
			default:
				err = withOwnerTransition(root, func() error {
					r, err := readRegistry(root)
					if err != nil {
						return err
					}
					id := "communication-style"
					if kind == "unrelated_policy" {
						id = "voice"
					}
					f := r.Facets[id]
					switch kind {
					case "readers", "unrelated_policy":
						f.Readers = []string{"session"}
					case "sensitivity":
						f.Sensitivity = "sensitive"
					case "refinement":
						f.Refinement = "proposal_only"
					case "owner_metadata":
						r.OnboardingTrackRevision++
					}
					r.Facets[id] = f
					return writeOwnerRegistry(root, r)
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			if kind == "unrelated_policy" || kind == "owner_metadata" {
				assertWeeklyMutationUnchanged(t, root, before)
			} else {
				assertMutationPurgedWeekly(t, root, input.InputSHA256)
			}
		})
	}
}

func TestWeeklyMutationJournalInterruptionAndGuardRecovery(t *testing.T) {
	for _, kind := range []string{"after_config", "guard_after_purge"} {
		t.Run(kind, func(t *testing.T) {
			root, o, now := weeklyCoreRoot(t)
			cfg := DefaultPromptHistoryConfig()
			cfg.OwnerID = o.OwnerID
			if err := ConfigurePromptHistory(root, cfg); err != nil {
				t.Fatal(err)
			}
			r, err := RecordUserPrompt(root, PromptHistoryInput{OwnerID: o.OwnerID, OccurrenceID: "selected", Prompt: "Private selected recovery prompt.", Language: "en", Source: "owner", SessionID: "session", ScopeKind: PromptScopeGlobal, ScopeID: "owner", RecordedAt: now.Add(-time.Hour)})
			if err != nil {
				t.Fatal(err)
			}
			input, err := FreezeWeeklySelf(root, o, func(text, _, _ string) (string, error) { return text, nil }, now)
			if err != nil {
				t.Fatal(err)
			}
			review, err := ReviewPromptHistory(root, PromptHistoryChange{Operation: "delete", IDs: []string{r.ID}})
			if err != nil {
				t.Fatal(err)
			}
			if kind == "guard_after_purge" {
				calls := 0
				err = ApplyReviewedPromptHistoryGuarded(root, review, true, func() error {
					calls++
					if calls == 3 {
						return context.Canceled
					}
					return nil
				})
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("late request guard not preserved: %v", err)
				}
			} else {
				err = withPromptHistoryLockPolicy(root, true, func(entries, _ string) error {
					dir, err := scheduler.HoldDirectory(filepath.Dir(entries), nil)
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
							return context.Canceled
						}
						return nil
					})
				})
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			}
			assertMutationPurgedWeekly(t, root, input.InputSHA256)
			literal, err := os.ReadFile(PromptHistoryEntriesPath(root))
			if err != nil || !bytes.Contains(literal, []byte("Private selected recovery prompt.")) {
				t.Fatal("partial operation hid unchanged primary source", err)
			}
			recovered, err := ReviewPendingPromptHistory(root)
			if err != nil || recovered.Digest != review.Digest {
				t.Fatal(err)
			}
			if err := ApplyReviewedPromptHistory(root, recovered, true); err != nil {
				t.Fatal(err)
			}
			entries, err := ExportPromptHistory(root)
			if err != nil || len(entries) != 0 {
				t.Fatal("exact recovery failed", err)
			}
			assertMutationPurgedWeekly(t, root, input.InputSHA256)
		})
	}
}

func TestWeeklyMutationObservationSupportUsesPreservedIDs(t *testing.T) {
	root, o, _ := weeklyCoreRoot(t)
	in := ObservationInput{SchemaVersion: 1, Signal: SignalExplicitInstruction, Facet: "communication-style", Claim: "concise_updates", EvidenceType: "owner_statement", SourceEvent: "owner.explicit_instruction", SourceDigest: strings.Repeat("a", 64), EpisodeID: "selected-episode", ScopeKind: "global", ScopeID: "owner", Confidence: 1, Sensitivity: "professional", ExpiresAt: time.Now().UTC().Add(time.Hour), AuthenticatedOwner: true, Material: true, OwnerConfirmed: true, DeclassifiedGlobal: true}
	selected, _, err := AppendObservation(root, in)
	if err != nil {
		t.Fatal(err)
	}
	in.EpisodeID = "legacy-support-episode"
	in.SourceDigest = strings.Repeat("B", 64)
	support, _, err := AppendObservation(root, in)
	if err != nil {
		t.Fatal(err)
	}
	support, err = GetObservation(root, support.ID)
	if err != nil {
		t.Fatal(err)
	}
	selected, err = TransitionObservation(root, ObservationTransitionInput{ObservationID: selected.ID, TransitionID: "selected-eligible", Next: ObservationEligible, ExpectedState: selected.State, ExpectedRevision: selected.Revision, OwnerAction: true})
	if err != nil {
		t.Fatal(err)
	}
	selected, err = TransitionObservation(root, ObservationTransitionInput{ObservationID: selected.ID, TransitionID: "selected-corroborated", Next: ObservationCorroborated, ExpectedState: selected.State, ExpectedRevision: selected.Revision, OwnerAction: true})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	o.ScheduledFor = now
	input, err := FreezeWeeklySelf(root, o, nil, now)
	if err != nil || input.ObservationSupport[support.ID] != support.Revision {
		t.Fatal("support fixture not bound", err)
	}
	before := weeklyMutationBytes(t, root)
	in.EpisodeID = "unrelated-episode"
	in.SourceDigest = strings.Repeat("c", 64)
	in.Claim = "different_preference"
	unrelated, _, err := AppendObservation(root, in)
	if err != nil {
		t.Fatal(err)
	}
	assertWeeklyMutationUnchanged(t, root, before)
	if _, err := RejectObservation(root, ObservationTransitionInput{ObservationID: unrelated.ID, TransitionID: "reject-unrelated", Next: ObservationRejected, ExpectedState: unrelated.State, ExpectedRevision: unrelated.Revision, OwnerAction: true}); err != nil {
		t.Fatal(err)
	}
	assertWeeklyMutationUnchanged(t, root, before)
	if _, err := RejectObservation(root, ObservationTransitionInput{ObservationID: support.ID, TransitionID: "redact-preserved-support", Next: ObservationRedacted, ExpectedState: support.State, ExpectedRevision: support.Revision, OwnerAction: true}); err != nil {
		t.Fatal(err)
	}
	assertMutationPurgedWeekly(t, root, input.InputSHA256)
}

func TestWeeklyMutationAbsentUnsafeAndConcurrentPublication(t *testing.T) {
	t.Run("explicit_missing_facet_restore", func(t *testing.T) {
		root, o, now := weeklyCoreRoot(t)
		input, err := FreezeWeeklySelf(root, o, nil, now)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(root, "owner/self/communication-style.md")); err != nil {
			t.Fatal(err)
		}
		if _, err := Initialize(root); err != nil {
			t.Fatal(err)
		}
		assertMutationPurgedWeekly(t, root, input.InputSHA256)
	})
	t.Run("absent_stays_absent", func(t *testing.T) {
		root, _, _ := weeklyCoreRoot(t)
		proposal, err := SubmitRefinement(root, RefinementInput{Facet: "communication-style", Evidence: "Explicit correction", ProposedBody: "# Current\n\nNew professional style.\n"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ApplyRefinement(root, proposal.ID, true); err != nil {
			t.Fatal(err)
		}
		if err := ResetPromptHistory(root, true); err != nil {
			t.Fatal(err)
		}
		if err := ResetDerivedSelf(root, true); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Dir(weeklySelfPath(root))); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("source mutations bootstrapped weekly store", err)
		}
	})
	t.Run("missing_weekly_lock_refuses_primary_write", func(t *testing.T) {
		root, o, now := weeklyCoreRoot(t)
		if _, err := FreezeWeeklySelf(root, o, nil, now); err != nil {
			t.Fatal(err)
		}
		before := weeklyMutationBytes(t, root)
		path := filepath.Join(root, "owner/self/communication-style.md")
		canonical, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := SubmitRefinement(root, RefinementInput{Facet: "communication-style", Evidence: "Explicit correction", ProposedBody: "# Current\n\nChanged style.\n"})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(filepath.Dir(weeklySelfPath(root)), ".lock")); err != nil {
			t.Fatal(err)
		}
		if _, err := ApplyRefinement(root, proposal.ID, true); err == nil {
			t.Fatal("unsafe weekly state allowed canonical mutation")
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(canonical, after) {
			t.Fatal("failed purge changed primary facet", err)
		}
		assertWeeklyMutationUnchanged(t, root, before)
	})
	t.Run("inflight_publication_cannot_revive_deleted_source", func(t *testing.T) {
		root, o, now := weeklyCoreRoot(t)
		cfg := DefaultPromptHistoryConfig()
		cfg.OwnerID = o.OwnerID
		if err := ConfigurePromptHistory(root, cfg); err != nil {
			t.Fatal(err)
		}
		prompt, err := RecordUserPrompt(root, PromptHistoryInput{OwnerID: o.OwnerID, OccurrenceID: "inflight", Prompt: "Selected private inflight prompt.", Language: "en", Source: "owner", SessionID: "session", ScopeKind: PromptScopeGlobal, ScopeID: "owner", RecordedAt: now.Add(-time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
		input, err := FreezeWeeklySelf(root, o, func(text, _, _ string) (string, error) { return text, nil }, now)
		if err != nil {
			t.Fatal(err)
		}
		ready, release := make(chan struct{}), make(chan struct{})
		done := make(chan error, 1)
		go func() {
			_, err := PublishWeeklySelf(context.Background(), root, input.InputSHA256, weeklyCandidate(input), 0, func(write func() error) error { close(ready); <-release; return write() }, now)
			done <- err
		}()
		<-ready
		if err := DeletePromptHistory(root, prompt.ID, true); err != nil {
			close(release)
			t.Fatal(err)
		}
		close(release)
		if err := <-done; err == nil {
			t.Fatal("late concurrent publication revived deleted private source")
		}
		assertMutationPurgedWeekly(t, root, input.InputSHA256)
	})
}
