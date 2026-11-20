package ownerctx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSelfObservationReviewIsOwnerBoundAndStaleSafe(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	for _, root := range []string{first, second} {
		if _, err := Initialize(root); err != nil {
			t.Fatal(err)
		}
	}
	declaration := SelfObservationDeclaration{Signal: SignalExplicitCorrection, Facet: "voice", Claim: "prefer.direct", Confidence: 1, ExpiresAt: time.Now().UTC().Add(time.Hour)}
	input, err := NewSelfObservationDeclaration(first, declaration)
	if err != nil {
		t.Fatal(err)
	}
	review, err := ReviewSelfObservation(first, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApproveSelfObservation(context.Background(), second, review, true); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("cross-owner=%v", err)
	}
	if _, err := os.Stat(observationDirectory(second)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid review created state: %v", err)
	}
	body, err := json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}
	var replay SelfObservationReview
	if err := json.Unmarshal(body, &replay); err != nil {
		t.Fatal(err)
	}
	if _, err := ApproveSelfObservation(context.Background(), first, replay, true); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("serialized review became authority: %v", err)
	}
	if _, err := os.Stat(observationDirectory(first)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("serialized review created state: %v", err)
	}
	other := declaration
	other.Claim = "prefer.other"
	otherInput, err := NewSelfObservationDeclaration(first, other)
	if err != nil {
		t.Fatal(err)
	}
	otherReview, err := ReviewSelfObservation(first, otherInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApproveSelfObservation(context.Background(), first, otherReview, true); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(selfObservationIntakePath(first))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApproveSelfObservation(context.Background(), first, review, true); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale=%v", err)
	}
	after, err := os.ReadFile(selfObservationIntakePath(first))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("stale review changed intake: %v", err)
	}
}

func TestSelfObservationUnsafeStateFailsClosed(t *testing.T) {
	for _, kind := range []string{"log-symlink", "log-hardlink", "intake-symlink", "oversized-log", "oversized-intake", "legacy-lock", "trailing-json"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			if _, err := Initialize(root); err != nil {
				t.Fatal(err)
			}
			input, err := NewSelfObservationDeclaration(root, SelfObservationDeclaration{Signal: SignalExplicitInstruction, Facet: "voice", Claim: "prefer.safe", Confidence: 1, ExpiresAt: time.Now().UTC().Add(time.Hour)})
			if err != nil {
				t.Fatal(err)
			}
			review, err := ReviewSelfObservation(root, input)
			if err != nil {
				t.Fatal(err)
			}
			queued, err := ApproveSelfObservation(context.Background(), root, review, true)
			if err != nil {
				t.Fatal(err)
			}
			source, err := InspectSelfObservationSource(root, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			foreign := filepath.Join(t.TempDir(), "foreign.txt")
			marker := []byte("FOREIGN_SYNTHETIC_UNCHANGED")
			if err := os.WriteFile(foreign, marker, 0600); err != nil {
				t.Fatal(err)
			}
			logPath := filepath.Join(observationDirectory(root), "observations.jsonl")
			switch kind {
			case "log-symlink":
				if err := os.Symlink(foreign, logPath); err != nil {
					t.Fatal(err)
				}
			case "log-hardlink":
				if err := os.Link(foreign, logPath); err != nil {
					t.Fatal(err)
				}
			case "intake-symlink":
				if err := os.Remove(selfObservationIntakePath(root)); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(foreign, selfObservationIntakePath(root)); err != nil {
					t.Fatal(err)
				}
			case "oversized-log":
				if err := os.WriteFile(logPath, []byte(strings.Repeat("x", maximumObservationBytes+1)), 0600); err != nil {
					t.Fatal(err)
				}
			case "oversized-intake":
				if err := os.WriteFile(selfObservationIntakePath(root), []byte(strings.Repeat("x", maximumObservationBytes+1)), 0600); err != nil {
					t.Fatal(err)
				}
			case "legacy-lock":
				if err := os.WriteFile(filepath.Join(observationDirectory(root), promptHistoryLockName), []byte("legacy-owner-active"), 0600); err != nil {
					t.Fatal(err)
				}
			case "trailing-json":
				if _, err := CaptureSelfObservation(context.Background(), root, queued.ID, source.Fingerprint, func(write func() error) error { return write() }); err != nil {
					t.Fatal(err)
				}
				body, err := os.ReadFile(logPath)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(logPath, append(bytes.TrimSuffix(body, []byte{'\n'}), []byte("{}\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := InspectSelfObservationSource(root, time.Now().UTC()); err == nil {
				t.Fatal("unsafe source was admitted")
			}
			after, err := os.ReadFile(foreign)
			if err != nil || !bytes.Equal(marker, after) {
				t.Fatalf("foreign bytes changed: %v", err)
			}
		})
	}
}

func TestObservationAppendReplayReturnsCurrentTerminalState(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	input := observationInput(SignalExplicitCorrection, "prefer.concise", "owner-episode-one", true, true)
	captured, _, err := AppendObservation(root, input)
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := RejectObservation(root, ObservationTransitionInput{ObservationID: captured.ID, TransitionID: "owner-rejected-one", Next: ObservationRejected, ExpectedState: captured.State, ExpectedRevision: captured.Revision, OwnerAction: true})
	if err != nil {
		t.Fatal(err)
	}
	replay, _, err := AppendObservation(root, input)
	if err != nil {
		t.Fatal(err)
	}
	if replay.State != rejected.State || replay.Revision != rejected.Revision {
		t.Fatalf("replay invented state/revision: got %s %s; want canonical %s %s", replay.State, replay.Revision, rejected.State, rejected.Revision)
	}
	observations, err := ListObservations(root)
	if err != nil || len(observations) != 1 || observations[0].Revision != rejected.Revision {
		t.Fatalf("replay changed canonical log: %#v %v", observations, err)
	}
}

func TestReviewedSelfIntakeThenCanonicalCapture(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	submission, err := NewSelfObservationDeclaration(root, SelfObservationDeclaration{Signal: SignalExplicitCorrection, Facet: "voice", Claim: "prefer.concise", Confidence: 1, ExpiresAt: time.Now().UTC().Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	review, err := ReviewSelfObservation(root, submission)
	if err != nil {
		t.Fatal(err)
	}
	before, err := ListObservations(root)
	if err != nil || len(before) != 0 {
		t.Fatalf("review captured self: %#v %v", before, err)
	}
	if _, err := ApproveSelfObservation(context.Background(), root, review, false); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("decline=%v", err)
	}
	source, err := InspectSelfObservationSource(root, time.Now().UTC())
	if err != nil || len(source.Pending) != 0 {
		t.Fatalf("decline wrote intake: %#v %v", source, err)
	}
	queued, err := ApproveSelfObservation(context.Background(), root, review, true)
	if err != nil {
		t.Fatal(err)
	}
	before, err = ListObservations(root)
	if err != nil || len(before) != 0 {
		t.Fatalf("intake is not capture: %#v %v", before, err)
	}
	source, err = InspectSelfObservationSource(root, time.Now().UTC())
	if err != nil || len(source.Pending) != 1 {
		t.Fatalf("source=%#v %v", source, err)
	}
	calls := 0
	guard := func(write func() error) error { calls++; return write() }
	captured, err := CaptureSelfObservation(context.Background(), root, queued.ID, source.Fingerprint, guard)
	if err != nil {
		t.Fatal(err)
	}
	if !captured.Added || captured.Observation.State != ObservationCaptured || calls != 1 {
		t.Fatalf("capture=%#v calls=%d", captured, calls)
	}
	if err := VerifySelfObservationVersion(root, queued.ID, captured.Observation.ID, captured.Observation.Revision); err != nil {
		t.Fatal(err)
	}
	replay, err := CaptureSelfObservation(context.Background(), root, queued.ID, source.Fingerprint, guard)
	if err != nil || replay.Added || replay.Observation.Revision != captured.Observation.Revision {
		t.Fatalf("replay=%#v %v", replay, err)
	}
	after, err := InspectSelfObservationSource(root, time.Now().UTC())
	if err != nil || len(after.Pending) != 0 || after.Fingerprint != source.Fingerprint {
		t.Fatalf("capture changed input identity: %#v %v", after, err)
	}
	if _, err := os.Stat(filepath.Join(root, "owner", "self", "voice.md")); err != nil {
		t.Fatal(err)
	}
}

func TestSelfIntakeOwnerControlPreventsPendingAndCapturedReplay(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	submission, err := NewSelfObservationDeclaration(root, SelfObservationDeclaration{Signal: SignalExplicitInstruction, Facet: "preferences", Claim: "prefer.direct", Confidence: 1, ExpiresAt: time.Now().UTC().Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	review, err := ReviewSelfObservation(root, submission)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := ApproveSelfObservation(context.Background(), root, review, true)
	if err != nil {
		t.Fatal(err)
	}
	control, err := ReviewSelfObservationControl(root, queued.ID, "rejected")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplySelfObservationControl(context.Background(), root, control, false); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("decline=%v", err)
	}
	if _, err := ApplySelfObservationControl(context.Background(), root, control, true); err != nil {
		t.Fatal(err)
	}
	source, err := InspectSelfObservationSource(root, time.Now().UTC())
	if err != nil || len(source.Pending) != 0 {
		t.Fatalf("rejected pending=%#v %v", source, err)
	}
	guard := func(write func() error) error { return write() }
	if _, err := CaptureSelfObservation(context.Background(), root, queued.ID, source.Fingerprint, guard); err == nil {
		t.Fatal("rejected intake captured")
	}
	submission, err = NewSelfObservationDeclaration(root, SelfObservationDeclaration{Signal: SignalExplicitInstruction, Facet: "preferences", Claim: "prefer.capture", Confidence: 1, ExpiresAt: submission.ExpiresAt})
	if err != nil {
		t.Fatal(err)
	}
	review, err = ReviewSelfObservation(root, submission)
	if err != nil {
		t.Fatal(err)
	}
	queued, err = ApproveSelfObservation(context.Background(), root, review, true)
	if err != nil {
		t.Fatal(err)
	}
	source, err = InspectSelfObservationSource(root, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	captured, err := CaptureSelfObservation(context.Background(), root, queued.ID, source.Fingerprint, guard)
	if err != nil {
		t.Fatal(err)
	}
	control, err = ReviewSelfObservationControl(root, queued.ID, "redacted")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplySelfObservationControl(context.Background(), root, control, true); err != nil {
		t.Fatal(err)
	}
	actual, err := GetObservation(root, captured.Observation.ID)
	if err != nil || actual.State != ObservationRedacted {
		t.Fatalf("canonical=%#v %v", actual, err)
	}
	input, err := selfObservationInput(submission)
	if err != nil {
		t.Fatal(err)
	}
	replay, _, err := AppendObservation(root, input)
	if err != nil || replay.State != ObservationRedacted || replay.Revision != actual.Revision {
		t.Fatalf("terminal replay=%#v %v", replay, err)
	}
	submission, err = NewSelfObservationDeclaration(root, SelfObservationDeclaration{Signal: SignalExplicitInstruction, Facet: "preferences", Claim: "prefer.reset", Confidence: 1, ExpiresAt: submission.ExpiresAt})
	if err != nil {
		t.Fatal(err)
	}
	review, err = ReviewSelfObservation(root, submission)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApproveSelfObservation(context.Background(), root, review, true); err != nil {
		t.Fatal(err)
	}
	if err := ResetDerivedSelf(root, true); err != nil {
		t.Fatal(err)
	}
	source, err = InspectSelfObservationSource(root, time.Now().UTC())
	if err != nil || len(source.Pending) != 0 {
		t.Fatalf("reset retained pending=%#v %v", source, err)
	}
}

func TestObservationCorroborationIgnoresRejectedHistoricalEpisode(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	first := observationInput(SignalExplicitInstruction, "prefer.direct", "episode-old", true, true)
	a, _, err := AppendObservation(root, first)
	if err != nil {
		t.Fatal(err)
	}
	eligible, err := TransitionObservation(root, ObservationTransitionInput{ObservationID: a.ID, TransitionID: "old-eligible", Next: ObservationEligible, ExpectedState: a.State, ExpectedRevision: a.Revision, OwnerAction: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RejectObservation(root, ObservationTransitionInput{ObservationID: a.ID, TransitionID: "old-rejected", Next: ObservationRejected, ExpectedState: eligible.State, ExpectedRevision: eligible.Revision, OwnerAction: true}); err != nil {
		t.Fatal(err)
	}
	second := observationInput(SignalExplicitInstruction, "prefer.direct", "episode-new", true, true)
	b, _, err := AppendObservation(root, second)
	if err != nil {
		t.Fatal(err)
	}
	eligible, err = TransitionObservation(root, ObservationTransitionInput{ObservationID: b.ID, TransitionID: "new-eligible", Next: ObservationEligible, ExpectedState: b.State, ExpectedRevision: b.Revision, OwnerAction: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := TransitionObservation(root, ObservationTransitionInput{ObservationID: b.ID, TransitionID: "new-corroborated", Next: ObservationCorroborated, ExpectedState: eligible.State, ExpectedRevision: eligible.Revision, OwnerAction: true}); err == nil {
		t.Fatal("rejected old versions were counted as independent corroboration")
	}
}

func TestSelfDeclarationsNeverEstablishIndependentCorroboration(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	submission, err := NewSelfObservationDeclaration(root, SelfObservationDeclaration{Signal: SignalExplicitInstruction, Facet: "voice", Claim: "prefer.concise", Confidence: 1, ExpiresAt: time.Now().UTC().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	review, err := ReviewSelfObservation(root, submission)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := ApproveSelfObservation(context.Background(), root, review, true)
	if err != nil {
		t.Fatal(err)
	}
	source, err := InspectSelfObservationSource(root, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	captured, err := CaptureSelfObservation(context.Background(), root, queued.ID, source.Fingerprint, func(write func() error) error { return write() })
	if err != nil {
		t.Fatal(err)
	}
	legacy := observationInput(SignalExplicitInstruction, "prefer.concise", "actual-other-episode", true, true)
	other, _, err := AppendObservation(root, legacy)
	if err != nil {
		t.Fatal(err)
	}
	eligible, err := TransitionObservation(root, ObservationTransitionInput{ObservationID: other.ID, TransitionID: "legacy-eligible", Next: ObservationEligible, ExpectedState: other.State, ExpectedRevision: other.Revision, OwnerAction: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := TransitionObservation(root, ObservationTransitionInput{ObservationID: other.ID, TransitionID: "legacy-corroborated", Next: ObservationCorroborated, ExpectedState: eligible.State, ExpectedRevision: eligible.Revision, OwnerAction: true}); err == nil {
		t.Fatal("owner declaration was counted as independent native episode support")
	}
	eligible, err = TransitionObservation(root, ObservationTransitionInput{ObservationID: captured.Observation.ID, TransitionID: "declaration-eligible", Next: ObservationEligible, ExpectedState: captured.Observation.State, ExpectedRevision: captured.Observation.Revision, OwnerAction: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := TransitionObservation(root, ObservationTransitionInput{ObservationID: eligible.ID, TransitionID: "declaration-corroborated", Next: ObservationCorroborated, ExpectedState: eligible.State, ExpectedRevision: eligible.Revision, OwnerAction: true}); err == nil {
		t.Fatal("declaration inferred native corroboration")
	}
	if IsYodaWeeklyEligible(ObservationReceipt{State: ObservationCorroborated, Signal: SignalExplicitInstruction, OwnerConfirmed: true, SourceEvent: "owner.reviewed_declaration"}) {
		t.Fatal("declaration fabricated weekly eligibility")
	}
}

// Base token-lock writers remove .lock after a successful append. Preserve the
// exact old-format canonical bytes while modeling that normal released state.
func TestSelfObservationLegacyLogExplicitInitialization(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	legacy, _, err := AppendObservation(root, observationInput(SignalExplicitCorrection, "prefer.legacy", "legacy-episode", true, true))
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(observationDirectory(root), "observations.jsonl")
	before, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(observationDirectory(root), promptHistoryLockName)
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectSelfObservationSource(root, time.Now().UTC()); err == nil || !strings.Contains(err.Error(), "selection lock is missing") {
		t.Fatalf("missing legacy lock should require explicit setup: %v", err)
	}
	if _, err := os.Stat(lock); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("query repaired legacy lock: %v", err)
	}
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	source, err := InspectSelfObservationSource(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("explicit initialization did not migrate released legacy log: %v", err)
	}
	if len(source.Pending) != 0 || len(source.Intakes) != 0 {
		t.Fatal("migration invented pending intake")
	}
	after, err := os.ReadFile(log)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("migration changed canonical bytes: %v", err)
	}
	if body, err := readOwnerFile(lock, 1); err != nil || len(body) != 0 {
		t.Fatalf("migration did not admit private empty persistent lock: %v", err)
	}
	if _, err := Initialize(root); err != nil {
		t.Fatalf("repeat explicit initialization: %v", err)
	}
	again, err := os.ReadFile(log)
	if err != nil || !bytes.Equal(before, again) {
		t.Fatal("repeat initialization rewrote log")
	}
	input, err := NewSelfObservationDeclaration(root, SelfObservationDeclaration{Signal: SignalExplicitInstruction, Facet: "voice", Claim: "prefer.after-upgrade", Confidence: 1, ExpiresAt: time.Now().UTC().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	review, err := ReviewSelfObservation(root, input)
	if err != nil {
		t.Fatal(err)
	}
	intake, err := ApproveSelfObservation(context.Background(), root, review, true)
	if err != nil {
		t.Fatal(err)
	}
	source, err = InspectSelfObservationSource(root, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	captured, err := CaptureSelfObservation(context.Background(), root, intake.ID, source.Fingerprint, func(write func() error) error { return write() })
	if err != nil || !captured.Added {
		t.Fatalf("post-upgrade capture: %#v %v", captured, err)
	}
	if err := VerifySelfObservationVersion(root, intake.ID, captured.Observation.ID, captured.Observation.Revision); err != nil {
		t.Fatal(err)
	}
	preserved, err := GetObservation(root, legacy.ID)
	if err != nil || preserved.Revision != legacy.Revision {
		t.Fatalf("legacy record changed: %#v %v", preserved, err)
	}
	final, err := os.ReadFile(log)
	if err != nil || !bytes.HasPrefix(final, before) {
		t.Fatal("capture did not preserve legacy canonical prefix")
	}
}

func TestSelfObservationLegacySetupPreservesUnsafeState(t *testing.T) {
	for _, kind := range []string{"nonempty-token", "active-native", "log-symlink", "log-hardlink", "lock-symlink", "lock-hardlink", "malformed-log"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			if _, err := Initialize(root); err != nil {
				t.Fatal(err)
			}
			if _, _, err := AppendObservation(root, observationInput(SignalExplicitCorrection, "prefer.legacy", "legacy-episode", true, true)); err != nil {
				t.Fatal(err)
			}
			directory := observationDirectory(root)
			log := filepath.Join(directory, "observations.jsonl")
			lock := filepath.Join(directory, promptHistoryLockName)
			logBefore, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			foreign := filepath.Join(t.TempDir(), "foreign")
			foreignBody := []byte("FOREIGN_LEGACY_PRESERVED")
			if err := os.WriteFile(foreign, foreignBody, 0600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "nonempty-token":
				if err := os.WriteFile(lock, []byte("legacy-active-token"), 0600); err != nil {
					t.Fatal(err)
				}
			case "log-symlink", "lock-symlink", "log-hardlink", "lock-hardlink":
				target := log
				if strings.HasPrefix(kind, "lock-") {
					target = lock
				}
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if strings.HasSuffix(kind, "symlink") {
					err = os.Symlink(foreign, target)
				} else {
					err = os.Link(foreign, target)
				}
				if err != nil {
					t.Fatal(err)
				}
				if target == log {
					logBefore = foreignBody
				}
			case "malformed-log":
				logBefore = []byte("{invalid legacy log}\n")
				if err := os.WriteFile(log, logBefore, 0600); err != nil {
					t.Fatal(err)
				}
			}
			lockBefore, err := os.ReadFile(lock)
			if err != nil {
				t.Fatal(err)
			}
			var heldLock func() error
			if kind == "active-native" {
				heldLock, err = acquireOwnerTransitionFile(lock, false)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if heldLock != nil {
						_ = heldLock()
					}
				}()
			}
			if _, err := Initialize(root); err == nil {
				t.Fatal("explicit setup accepted unsafe/active legacy state")
			}
			// Windows byte-range locks exclude reads through another handle.
			if heldLock != nil {
				if err := heldLock(); err != nil {
					t.Fatal(err)
				}
				heldLock = nil
			}
			logAfter, err := os.ReadFile(log)
			if err != nil || !bytes.Equal(logBefore, logAfter) {
				t.Fatalf("setup changed legacy log: %v", err)
			}
			lockAfter, err := os.ReadFile(lock)
			if err != nil || !bytes.Equal(lockBefore, lockAfter) {
				t.Fatalf("setup changed active/foreign lock: %v", err)
			}
			after, err := os.ReadFile(foreign)
			if err != nil || !bytes.Equal(foreignBody, after) {
				t.Fatalf("setup changed foreign bytes: %v", err)
			}
		})
	}
}

func TestObservationLegacyDigestNormalizationUpgrade(t *testing.T) {
	for _, transitioned := range []bool{false, true} {
		t.Run(fmt.Sprintf("transitioned_%t", transitioned), func(t *testing.T) {
			root := t.TempDir()
			if _, err := Initialize(root); err != nil {
				t.Fatal(err)
			}
			if err := ensureOwnerDirectory(observationDirectory(root)); err != nil {
				t.Fatal(err)
			}
			input := observationInput(SignalExplicitCorrection, "prefer.legacy-case", "legacy-case-episode", true, true)
			input.SourceDigest = strings.ToUpper(input.SourceDigest)
			now := time.Now().UTC()
			first := observationRecord{ObservationInput: input, ID: observationID(input), State: ObservationCaptured, RecordedAt: now, StateChangedAt: now}
			body, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			body = append(body, '\n')
			// These are the exact normalization and transition rules in the base reader/writer.
			expected := first
			expected.SourceDigest = strings.ToLower(expected.SourceDigest)
			if transitioned {
				next := expected
				next.State = ObservationEligible
				next.ExpectedState = expected.State
				next.ExpectedRevision = observationRevision(expected)
				next.TransitionID = "legacy-case-transition"
				next.StateChangedAt = now.Add(time.Millisecond)
				line, err := json.Marshal(next)
				if err != nil {
					t.Fatal(err)
				}
				body = append(body, append(line, '\n')...)
				expected = next
			}
			path := filepath.Join(observationDirectory(root), "observations.jsonl")
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Initialize(root); err != nil {
				t.Fatalf("valid base-shaped normalization history cannot upgrade: %v", err)
			}
			got, err := GetObservation(root, first.ID)
			if err != nil || got.ID != first.ID || got.Revision != observationRevision(expected) || got.SourceDigest != strings.ToLower(input.SourceDigest) {
				t.Fatalf("base read identity/revision changed: %#v err=%v expected=%s", got, err, observationRevision(expected))
			}
			if _, err := InspectSelfObservationSource(root, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(body, after) {
				t.Fatal("upgrade rewrote canonical bytes", err)
			}
			if _, err := Initialize(root); err != nil {
				t.Fatalf("repeat normalized legacy setup: %v", err)
			}
			again, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(body, again) {
				t.Fatal("repeat setup changed legacy bytes", err)
			}
			submission, err := NewSelfObservationDeclaration(root, SelfObservationDeclaration{Signal: SignalExplicitInstruction, Facet: "voice", Claim: "prefer.after-case-upgrade", Confidence: 1, ExpiresAt: time.Now().UTC().Add(time.Hour)})
			if err != nil {
				t.Fatal(err)
			}
			review, err := ReviewSelfObservation(root, submission)
			if err != nil {
				t.Fatal(err)
			}
			intake, err := ApproveSelfObservation(context.Background(), root, review, true)
			if err != nil {
				t.Fatal(err)
			}
			source, err := InspectSelfObservationSource(root, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			captured, err := CaptureSelfObservation(context.Background(), root, intake.ID, source.Fingerprint, func(write func() error) error { return write() })
			if err != nil || !captured.Added {
				t.Fatalf("normalized legacy upgrade could not capture new declaration: %#v %v", captured, err)
			}
			if err := VerifySelfObservationVersion(root, intake.ID, captured.Observation.ID, captured.Observation.Revision); err != nil {
				t.Fatal(err)
			}
			final, err := os.ReadFile(path)
			if err != nil || !bytes.HasPrefix(final, body) {
				t.Fatal("new capture rewrote normalized legacy prefix", err)
			}
			if got, err := GetObservation(root, first.ID); err != nil || got.Revision != observationRevision(expected) {
				t.Fatal("new capture changed legacy revision", err)
			}
			body = final
			// Only an exact normalized history with actual prior CAS can use the anchor.
			for _, kind := range []string{"arbitrary-id", "changed-input", "wrong-cas"} {
				forged := expected
				forged.ExpectedState = expected.State
				forged.ExpectedRevision = observationRevision(expected)
				forged.State = ObservationRejected
				forged.TransitionID = "forged-legacy-transition"
				switch kind {
				case "arbitrary-id":
					forged.ID = "observation-" + strings.Repeat("0", 40)
				case "changed-input":
					forged.Claim = "prefer.changed-input"
				case "wrong-cas":
					forged.ExpectedRevision = strings.Repeat("0", 64)
				}
				line, _ := json.Marshal(forged)
				candidate := append(append([]byte(nil), body...), append(line, '\n')...)
				if err := os.WriteFile(path, candidate, 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := ListObservations(root); err == nil {
					t.Fatalf("%s borrowed legacy normalization compatibility", kind)
				}
			}
		})
	}
}

func TestSelfObservationCaptureCancellationAcrossHeldOwnerLock(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	submission, err := NewSelfObservationDeclaration(root, SelfObservationDeclaration{Signal: SignalExplicitInstruction, Facet: "voice", Claim: "prefer.cancelled-lock-capture", Confidence: 1, ExpiresAt: time.Now().UTC().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	review, err := ReviewSelfObservation(root, submission)
	if err != nil {
		t.Fatal(err)
	}
	intake, err := ApproveSelfObservation(context.Background(), root, review, true)
	if err != nil {
		t.Fatal(err)
	}
	source, err := InspectSelfObservationSource(root, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	before, err := readOwnerFile(selfObservationIntakePath(root), maximumObservationBytes)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := acquireOwnerTransitionFile(filepath.Join(observationDirectory(root), promptHistoryLockName), false)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			_ = unlock()
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := CaptureSelfObservation(ctx, root, intake.ID, source.Fingerprint, func(write func() error) error { close(entered); return write() })
		done <- err
	}()
	// The entry context check and enclosing guard passed, but publication cannot
	// acquire its native observation lock until this test releases it.
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("capture never reached its enclosing guard")
	}
	cancel()
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	released = true
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("late cancellation was not preserved: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled capture did not finish")
	}
	after, err := readOwnerFile(selfObservationIntakePath(root), maximumObservationBytes)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("cancelled capture changed pending source", err)
	}
	records, err := ListObservations(root)
	if err != nil || len(records) != 0 {
		t.Fatalf("cancelled capture committed canonical evidence: %#v %v", records, err)
	}
	if _, err := os.Stat(filepath.Join(observationDirectory(root), "observations.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled capture created domain evidence", err)
	}
}
