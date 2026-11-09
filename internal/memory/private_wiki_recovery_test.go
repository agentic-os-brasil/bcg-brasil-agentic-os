//go:build !windows

package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestPrivateWikiMissingEventRecoveryUsesActualRecoveryTime(t *testing.T) {
	e, _, day := privateWikiEngine(t)
	r, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		t.Fatal(err)
	}
	_, name, err := e.latestManifest("case-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(e.privateWikiEventPath("case-a", name)); err != nil {
		t.Fatal(err)
	}
	recovery := day.Add(3 * time.Hour)
	e.Now = func() time.Time { return recovery }
	before := privateWikiTree(t, e.privateWikiRoot("case-a"))
	next, err := e.CompilePrivateWiki(context.Background(), "case-a", r.Generation)
	if err != nil || !next.Recovered || next.Changed || next.Generation != r.Generation {
		t.Fatalf("recovery manufactured a graph: %+v %v", next, err)
	}
	if !reflect.DeepEqual(before, privateWikiTree(t, e.privateWikiRoot("case-a"))) {
		t.Fatal("event recovery changed immutable graph bytes")
	}
	b, err := os.ReadFile(e.privateWikiEventPath("case-a", name))
	if err != nil {
		t.Fatal(err)
	}
	var event privateWikiSourceEvent
	if json.Unmarshal(b, &event) != nil || !event.Recovered || event.Reason != "reconcile_requested" || !event.CreatedAt.Equal(recovery) {
		t.Fatal("recovered event invented original durability/time")
	}
	if strings.Contains(string(b), "unique-wiki-launch-marker") {
		t.Fatal("source body reached outbox")
	}
}

func TestPrivateWikiOrphanIntentCannotActivateACommit(t *testing.T) {
	e, _, day := privateWikiEngine(t)
	r, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "uncommitted-second-route", day.AddDate(0, 0, 1))); err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("synthetic source interruption after durable intent")
	e.FaultPoint = func(point string) error {
		if point == "wiki_after_commit_intent" {
			return interrupted
		}
		return nil
	}
	if _, err := e.DreamDailyAttested(context.Background(), "case-a", day.AddDate(0, 0, 1)); !errors.Is(err, interrupted) {
		t.Fatalf("source interruption absent: %v", err)
	}
	e.FaultPoint = nil
	entries, err := os.ReadDir(filepath.Join(e.workspaceRoot("case-a"), ".wiki-outbox"))
	if err != nil {
		t.Fatal(err)
	}
	orphans := 0
	for _, entry := range entries {
		if _, err := os.Stat(filepath.Join(e.workspaceRoot("case-a"), "commits", entry.Name())); os.IsNotExist(err) {
			orphans++
		}
	}
	if orphans != 1 {
		t.Fatalf("missing durable orphan intent: %d", orphans)
	}
	status, err := e.PrivateWikiStatus("case-a")
	if err != nil || status.State != "current" || status.Generation != r.Generation || status.Concepts != 1 {
		t.Fatalf("orphan became source authority: %+v %v", status, err)
	}
	if next, err := e.CompilePrivateWiki(context.Background(), "case-a", r.Generation); err != nil || next.Changed {
		t.Fatalf("orphan produced graph evidence: %+v %v", next, err)
	}
}

func TestPrivateWikiInterruptedVersionResumesWithoutReplacingFiles(t *testing.T) {
	e, _, _ := privateWikiEngine(t)
	interrupted := errors.New("synthetic immutable file publication interruption")
	e.FaultPoint = func(point string) error {
		if point == "wiki_after_version_file" {
			return interrupted
		}
		return nil
	}
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", ""); !errors.Is(err, interrupted) {
		t.Fatalf("missing version interruption: %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.privateWikiRoot("case-a"), "current.json")); !os.IsNotExist(err) {
		t.Fatal("partial graph activated")
	}
	entries, err := os.ReadDir(filepath.Join(e.privateWikiRoot("case-a"), "versions"))
	if err != nil || len(entries) != 1 {
		t.Fatal("missing bounded partial generation")
	}
	first := filepath.Join(e.privateWikiRoot("case-a"), "versions", entries[0].Name(), "backlinks.json")
	before, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	e.FaultPoint = nil
	r, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil || !r.Changed || r.Generation != entries[0].Name() {
		t.Fatalf("interrupted version recovery %+v %v", r, err)
	}
	after, err := os.Stat(first)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("recovery replaced an existing immutable file")
	}
	if _, err := e.ReadPrivateWiki("case-a", r.Generation, "index.md", 0, 65536); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateWikiHardlinkRetirementIsIncompleteAndRetryable(t *testing.T) {
	e, capture, _ := privateWikiEngine(t)
	r, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		t.Fatal(err)
	}
	index, err := e.ReadPrivateWiki("case-a", r.Generation, "index.md", 0, 65536)
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`\]\((concepts/[0-9a-f]{64}[.]md)\)`).FindStringSubmatch(index.Content)
	if len(match) != 2 {
		t.Fatal("concept route absent")
	}
	alias := filepath.Join(t.TempDir(), "outside-alias")
	if err := os.Link(filepath.Join(e.privateWikiRoot("case-a"), "versions", r.Generation, filepath.FromSlash(match[1])), alias); err != nil {
		t.Fatal(err)
	}
	outside, err := os.ReadFile(alias)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := e.DeleteCapture("case-a", capture.ID, capture.Revision)
	if !errors.Is(err, ErrPrivateWikiRetirement) || removed.Complete {
		t.Fatalf("unsafe cleanup claimed complete: %+v %v", removed, err)
	}
	after, err := os.ReadFile(alias)
	if err != nil || !bytes.Equal(outside, after) {
		t.Fatal("retirement touched outside alias bytes")
	}
	for _, ref := range []string{"index.md", "log.md", "backlinks.json", match[1]} {
		if page, err := e.ReadPrivateWiki("case-a", r.Generation, ref, 0, 65536); err == nil || page.Content != "" {
			t.Fatal("failed cleanup reopened a denied graph")
		}
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	removed, err = e.DeleteCapture("case-a", capture.ID, capture.Revision)
	if err != nil || !removed.Complete {
		t.Fatalf("cleanup retry failed: %+v %v", removed, err)
	}
	if _, err := os.Lstat(e.privateWikiRoot("case-a")); !os.IsNotExist(err) {
		t.Fatal("retry left derived bodies")
	}
}

func TestPrivateWikiPostBarrierFailureCannotReopenOldGraph(t *testing.T) {
	e, capture, _ := privateWikiEngine(t)
	r, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("synthetic interruption after authoritative denial")
	e.FaultPoint = func(point string) error {
		if point == "after_visibility_barrier" {
			return interrupted
		}
		return nil
	}
	if removed, err := e.DeleteCapture("case-a", capture.ID, capture.Revision); !errors.Is(err, interrupted) || removed.Complete {
		t.Fatal("post-barrier interruption absent")
	}
	if page, err := e.ReadPrivateWiki("case-a", r.Generation, "index.md", 0, 65536); err == nil || page.Content != "" {
		t.Fatal("failure bypassed denial")
	}
	e.FaultPoint = nil
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", r.Generation); err == nil {
		t.Fatal("empty denied source produced a graph")
	}
	if _, err := os.Lstat(e.privateWikiRoot("case-a")); !os.IsNotExist(err) {
		t.Fatal("owning retry did not retire stale bodies before admission")
	}
	b, err := e.readVisibility("case-a")
	if err != nil || b.PrivateWiki == nil || b.PrivateWiki.DenialEvent == nil || b.PrivateWiki.RetiredThrough != b.Generation || b.Denied[capture.ID] != capture.Revision {
		t.Fatal("cleanup lost cumulative denial/event authority")
	}
}

func TestPrivateWikiMissingOwnerKeyNeverFallsBackOrBootstraps(t *testing.T) {
	e, _, _ := privateWikiEngine(t)
	directory := filepath.Join(e.workspaceRoot("case-a"), ".attestation")
	if err := os.Remove(filepath.Join(directory, "owner-reviewed.key")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "capture-v2.key"), bytes.Repeat([]byte{1}, 32), 0600); err != nil {
		t.Fatal(err)
	}
	before := privateWikiTree(t, e.Root)
	if _, err := e.PrivateWikiStatus("case-a"); err == nil {
		t.Fatal("other producer key supplied owner identity")
	}
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", ""); err == nil {
		t.Fatal("owning compile created or substituted authority")
	}
	if !reflect.DeepEqual(before, privateWikiTree(t, e.Root)) {
		t.Fatal("missing authority caused bootstrap/repair")
	}
}

func TestPrivateWikiLivePublicationGuardCanRefuseAfterSnapshot(t *testing.T) {
	e, _, _ := privateWikiEngine(t)
	revoked := false
	refusal := errors.New("synthetic live grant revoked")
	e.PublicationGuard = func(publish func() error) error {
		if revoked {
			return refusal
		}
		return publish()
	}
	e.FaultPoint = func(point string) error {
		if point == "wiki_after_snapshot" {
			revoked = true
		}
		return nil
	}
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", ""); !errors.Is(err, refusal) {
		t.Fatalf("revoked grant admitted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.privateWikiRoot("case-a"), "current.json")); !os.IsNotExist(err) {
		t.Fatal("revoked work activated")
	}
}

func TestPrivateWikiOldCohortProcessCannotReadCaptureOrDelete(t *testing.T) {
	helper, proof := privateWikiBinaryFixture(t, "old_writer", "MAESTRO_PRIVATE_WIKI_OLD_WRITER")
	e, capture, day := privateWikiEngine(t)
	config := struct {
		Policy    Policy                   `json:"policy"`
		Budgets   map[string]int           `json:"budgets"`
		Freshness map[string]time.Duration `json:"freshness"`
		At        time.Time                `json:"at"`
		Text      string                   `json:"text"`
	}{e.Policy, e.Budgets, e.Freshness, day, "legacy positive-control marker"}
	body, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configName := filepath.Join(t.TempDir(), "legacy-config.json")
	if os.WriteFile(configName, body, 0600) != nil {
		t.Fatal("legacy config write")
	}
	// Actual unprotected successes prove the historical consumer is functional,
	// rather than interpreting an unrelated broken executable as a cohort refusal.
	positiveRoot := filepath.Join(t.TempDir(), "brain", "memory")
	seed := invokeOldWikiFixture(t, helper, proof, "seed", positiveRoot, "", configName, "positive-seed")
	if seed.Refused || seed.Capture == nil || seed.Capture.Revision != 1 || !seed.Capture.Eligible {
		t.Fatalf("legacy seed unavailable: %+v", seed)
	}
	read := invokeOldWikiFixture(t, helper, proof, "read", positiveRoot, seed.Capture.ID, configName, "positive-read")
	if read.Refused || read.Record == nil || read.Record.Text != config.Text || read.Record.ID != seed.Capture.ID {
		t.Fatal("legacy read positive control failed")
	}
	extra := invokeOldWikiFixture(t, helper, proof, "capture", positiveRoot, "", configName, "positive-capture")
	if extra.Refused || extra.Capture == nil || extra.Capture.ID == seed.Capture.ID || !extra.Capture.Eligible {
		t.Fatal("legacy capture positive control failed")
	}
	removed := invokeOldWikiFixture(t, helper, proof, "delete", positiveRoot, seed.Capture.ID, configName, "positive-delete")
	if removed.Refused || removed.Mutation == nil || removed.Mutation.Removed != 1 || !removed.Mutation.Complete {
		t.Fatal("legacy delete positive control failed")
	}
	positive := *e
	positive.Root = positiveRoot
	if _, err := positive.ReadCapture("case-a", seed.Capture.ID); !errors.Is(err, ErrCaptureUnavailable) {
		t.Fatalf("legacy delete did not remove real source visibility: %v", err)
	}
	if kept, err := positive.ReadCapture("case-a", extra.Capture.ID); err != nil || kept.Text != config.Text {
		t.Fatal("legacy mutation lost unrelated real source")
	}
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", ""); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"read", "capture", "delete"} {
		t.Run(operation, func(t *testing.T) {
			before := privateWikiTree(t, e.Root)
			result := invokeOldWikiFixture(t, helper, proof, operation, e.Root, capture.ID, configName, "protected-"+operation)
			if !result.Refused || result.Code != "protected_cohort" {
				t.Fatalf("old real API did not identify protected cohort: %+v", result)
			}
			if !reflect.DeepEqual(before, privateWikiTree(t, e.Root)) {
				t.Fatal("old process mutated source, graph, fence or modes")
			}
		})
	}
}
