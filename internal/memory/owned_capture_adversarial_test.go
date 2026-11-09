//go:build !windows

package memory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestOwnedMemoryUnsafePersistence(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "fifo", "oversize", "duplicate", "trailing", "ancestor"} {
		t.Run(kind, func(t *testing.T) {
			e := testEngine(t)
			if err := e.PrepareOwnedCaptureAuthority("case-a"); err != nil {
				t.Fatal(err)
			}
			day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
			r, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "private-capture", day))
			if err != nil {
				t.Fatal(err)
			}
			path := e.ownedPath("case-a", r.ID)
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			external := filepath.Join(t.TempDir(), "external")
			if err := os.WriteFile(external, body, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "symlink":
				err = os.Symlink(external, path)
			case "hardlink":
				err = os.Link(external, path)
			case "fifo":
				err = unix.Mkfifo(path, 0600)
			case "oversize":
				err = os.WriteFile(path, []byte(strings.Repeat("x", MaxOwnedCaptureBytes+4097)), 0600)
			case "duplicate":
				err = os.WriteFile(path, append([]byte(`{"text":"other",`), body[1:]...), 0600)
			case "trailing":
				err = os.WriteFile(path, append(body, []byte(` {}`)...), 0600)
			case "ancestor":
				directory := filepath.Dir(path)
				err = os.Remove(directory)
				if err == nil {
					err = os.Symlink(filepath.Dir(external), directory)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.ReadCapture("case-a", r.ID); err == nil {
				t.Fatal("unsafe read admitted")
			}
			if _, err := e.ListCaptures("case-a", 0, 10); err == nil {
				t.Fatal("unsafe discovery admitted")
			}
			if _, err := e.DeleteCapture("case-a", r.ID, r.Revision); err == nil {
				t.Fatal("unsafe mutation admitted")
			}
			got, err := os.ReadFile(external)
			if err != nil || string(got) != string(body) {
				t.Fatalf("outside data changed: %v", err)
			}
		})
	}
}

func TestOwnedMemoryTrustedInputCannotBeRetargetedOrForged(t *testing.T) {
	e := testEngine(t)
	if err := e.PrepareOwnedCaptureAuthority("case-a"); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	in := ownedInput(t, e, "", 0, "reviewed", day)
	tampered := in
	tampered.Text = "unreviewed"
	if _, err := e.CaptureOwned("case-a", tampered); err == nil {
		t.Fatal("changed review accepted")
	}
	if _, err := e.CaptureOwned("case-b", in); err == nil {
		t.Fatal("cross-workspace review accepted")
	}
	if _, err := e.CaptureOwned("case-a", OwnedCaptureInput{Text: "model", At: day}); err == nil {
		t.Fatal("model-shaped input accepted")
	}
	r, err := e.CaptureOwned("case-a", in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := e.CaptureOwned("case-a", in)
	if err != nil || again.ID != r.ID {
		t.Fatalf("same reviewed request not idempotent: %#v %v", again, err)
	}
	v, err := e.VisibilityRevision("case-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "intervening", day)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ApplyRetention("case-a", day.Add(time.Hour), v); !errors.Is(err, ErrCaptureConflict) {
		t.Fatalf("stale retained review applied: %v", err)
	}
	replacement := ownedInput(t, e, r.ID, r.Revision, "replacement", day)
	if _, err := e.CorrectCapture("case-a", again.ID+"a", 1, replacement); err == nil {
		t.Fatal("replacement retargeted")
	}
}

func TestOwnedMemoryLegacyIdentityAndEligibility(t *testing.T) {
	e := testEngine(t)
	if err := e.PrepareOwnedCaptureAuthority("case-a"); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	if _, err := e.Capture(Capture{WorkspaceID: "case-a", RecordedAt: day, Kind: OwnedCaptureKind, ProducerID: OwnedCaptureProducerID, Text: "private-legacy-marker", Sanitized: true}); err != nil {
		t.Fatal(err)
	}
	attestor := CaptureAttestor{Root: e.Root}
	v2, err := attestor.Seal(Capture{WorkspaceID: "case-a", RecordedAt: day, Kind: "skill_route", Text: "skill-route-marker", Sanitized: true, ProducerID: "claude.context-injection", SanitizerID: SkillRouteSanitizerID, SourceDigest: strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Capture(v2); err != nil {
		t.Fatal(err)
	}
	page, err := e.ListCaptures("case-a", 0, 10)
	if err != nil || page.Total != 2 {
		t.Fatalf("legacy discovery: %#v %v", page, err)
	}
	var legacy, trusted CaptureReceipt
	for _, r := range page.Captures {
		if r.Eligible {
			trusted = r
		} else {
			legacy = r
		}
	}
	if trusted.ID == "" || legacy.ID == "" {
		t.Fatal("legacy strings granted trust")
	}
	page, err = e.SearchCaptures("case-a", "private-legacy-marker", 0, 10)
	if err != nil || page.Total != 0 {
		t.Fatalf("untrusted legacy searchable: %#v %v", page, err)
	}
	restarted := *e
	record, err := restarted.ReadCapture("case-a", trusted.ID)
	if err != nil || record.ProducerID != v2.ProducerID || record.Revision != 1 {
		t.Fatalf("v2 provenance changed: %#v %v", record, err)
	}
	r, err := e.CorrectCapture("case-a", trusted.ID, 1, ownedInput(t, e, trusted.ID, 1, "reviewed-new-marker", day))
	if err != nil || r.ID != trusted.ID || r.Revision != 2 {
		t.Fatalf("legacy correction: %#v %v", r, err)
	}
	if _, err = e.DeleteCapture("case-a", legacy.ID, 1); err != nil {
		t.Fatal(err)
	}
	page, err = restarted.SearchCaptures("case-a", "skill-route-marker", 0, 10)
	if err != nil || page.Total != 0 {
		t.Fatalf("old source remains searchable: %#v %v", page, err)
	}
	page, err = restarted.SearchCaptures("case-a", "reviewed-new-marker", 0, 10)
	if err != nil || page.Total != 1 {
		t.Fatalf("new reviewed source missing: %#v %v", page, err)
	}
}

func TestOwnedMemoryFallbackWeeklyAndStaleActivation(t *testing.T) {
	e := testEngine(t)
	if err := e.PrepareOwnedCaptureAuthority("case-a"); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	for _, layer := range []string{"L1", "L2", "L3", "lifetime"} {
		e.Budgets[layer] = 16000
	}
	e.Synthesizer = DeterministicWeeklySynthesizer{Daily: DeterministicL1Synthesizer{MaxRunes: 4000, MaxEntries: 64, MaxInputBytes: 1 << 20, MaxInputEntries: 256, Attestor: CaptureAttestor{Root: e.Root}}, MaxRunes: e.Budgets, MaxInputBytes: 1 << 20}
	e.Eligibility = EligibilityFunc(func(context.Context, Artifact) (bool, string, error) {
		return true, "synthetic explicit eligibility", nil
	})
	e.EligibilityPolicyID = "test-explicit-policy"
	r, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "removed-weekly-marker", day))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "surviving-marker", day)); err != nil {
		t.Fatal(err)
	}
	if _, err = e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
	if _, err = e.DreamWeekly(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
	assertContextMarker(t, e, "removed-weekly-marker", true)
	manifest, _, err := e.latestManifest("case-a")
	if err != nil {
		t.Fatal(err)
	}
	stale, _, err := e.readArtifactFromManifest("case-a", manifest, "L3/current")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.DeleteCapture("case-a", r.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err = e.activate("case-a", []Artifact{stale}); !errors.Is(err, ErrCaptureConflict) {
		t.Fatalf("stale writer accepted: %v", err)
	}
	commits := filepath.Join(e.workspaceRoot("case-a"), "commits")
	if err = os.WriteFile(filepath.Join(commits, "99999999T999999.999999999Z-broken.json"), []byte(`{"corrupt":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	assertContextMarker(t, e, "removed-weekly-marker", false)
	if _, err = e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
	if _, err = e.DreamWeekly(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
	assertContextMarker(t, e, "removed-weekly-marker", false)
	assertContextMarker(t, e, "surviving-marker", true)
	restarted := *e
	page, err := restarted.SearchCaptures("case-a", "removed-weekly-marker", 0, 10)
	if err != nil || page.Total != 0 {
		t.Fatalf("deleted capture search: %#v %v", page, err)
	}
	manifest, _, err = e.latestManifest("case-a")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.VisibilityRevision == 0 {
		t.Fatal("rebuilt manifest lost generation")
	}
	body, _ := json.Marshal(manifest)
	if strings.Contains(string(body), "removed-weekly-marker") {
		t.Fatal("manifest retained text")
	}
	if _, err = e.DeleteCapture("case-a", r.ID, 1); err != nil {
		t.Fatalf("idempotent delete failed: %v", err)
	}
	assertContextMarker(t, e, "surviving-marker", true)
}

func TestOwnedMemoryInterruptedRevocationAndRetentionSources(t *testing.T) {
	e := testEngine(t)
	if err := e.PrepareOwnedCaptureAuthority("case-a"); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	e.Synthesizer = DeterministicL1Synthesizer{MaxRunes: 4000, MaxEntries: 64, MaxInputBytes: 1 << 20, MaxInputEntries: 256, Attestor: CaptureAttestor{Root: e.Root}}
	old, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "interrupted-denied-marker", day))
	if err != nil {
		t.Fatal(err)
	}
	cutoff := day.Add(time.Hour)
	keep, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "cutoff-boundary-survivor", cutoff))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
	e.FaultPoint = func(point string) error {
		if point == "after_visibility_barrier" {
			return errors.New("simulated crash before cleanup")
		}
		return nil
	}
	if _, err = e.DeleteCapture("case-a", old.ID, 1); err == nil {
		t.Fatal("interruption ignored")
	}
	e.FaultPoint = nil
	if _, err := os.Stat(e.ownedPath("case-a", old.ID)); err != nil {
		t.Fatal("fixture did not leave denied physical plaintext")
	}
	assertContextMarker(t, e, "interrupted-denied-marker", false)
	if _, err = e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
	assertContextMarker(t, e, "interrupted-denied-marker", false)
	assertContextMarker(t, e, "cutoff-boundary-survivor", true)
	v, err := e.VisibilityRevision("case-a")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := e.ApplyRetention("case-a", cutoff, v)
	if err != nil || !receipt.Complete {
		t.Fatalf("cleanup retry: %#v %v", receipt, err)
	}
	if _, err = os.Stat(e.ownedPath("case-a", old.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("denied plaintext not purged: %v", err)
	}
	if _, err = e.ReadCapture("case-a", keep.ID); err != nil {
		t.Fatal("cutoff equality pruned", err)
	}
	assertContextMarker(t, e, "cutoff-boundary-survivor", true)
	v, err = e.VisibilityRevision("case-a")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err = e.ApplyRetention("case-a", cutoff.Add(time.Nanosecond), v)
	if err != nil || receipt.Removed != 1 {
		t.Fatalf("explicit retention cutoff: %#v %v", receipt, err)
	}
	page, err := e.SearchCaptures("case-a", "cutoff-boundary-survivor", 0, 10)
	if err != nil || page.Total != 0 {
		t.Fatalf("retention search: %#v %v", page, err)
	}
	assertContextMarker(t, e, "cutoff-boundary-survivor", false)
	if _, err = e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
}

func TestOwnedMemoryConcurrentMutationHonorsActivationLock(t *testing.T) {
	e := testEngine(t)
	if err := e.PrepareOwnedCaptureAuthority("case-a"); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	r, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "lock-survivor", day))
	if err != nil {
		t.Fatal(err)
	}
	release, err := e.acquireCycleLock("case-a", "activation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.DeleteCapture("case-a", r.ID, 1); !errors.Is(err, ErrDreamInProgress) {
		t.Fatalf("mutation raced activation: %v", err)
	}
	release()
	if _, err = e.ReadCapture("case-a", r.ID); err != nil {
		t.Fatal(err)
	}
}
