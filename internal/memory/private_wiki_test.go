//go:build !windows

package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func privateWikiEngine(t *testing.T) (*Engine, CaptureReceipt, time.Time) {
	t.Helper()
	if err := ownedIOAvailable(); err != nil {
		t.Skip(err)
	}
	e := testEngine(t)
	e.Root = filepath.Join(t.TempDir(), "brain", "memory")
	if err := os.Chmod(filepath.Dir(filepath.Dir(e.Root)), 0700); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	e.Now = func() time.Time { return day }
	if err := e.PrepareOwnedCaptureAuthority("case-a"); err != nil {
		t.Fatal(err)
	}
	e.Synthesizer = DeterministicL1Synthesizer{MaxRunes: 4096, MaxEntries: 64, MaxInputBytes: 1 << 20, MaxInputEntries: 256, Attestor: CaptureAttestor{Root: e.Root}}
	e.SynthesizerID = DeterministicL1SynthesizerID
	r, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "unique-wiki-launch-marker", day))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
	return e, r, day
}

func TestPrivateWikiNoninitialCursorRequiresAnExactGeneration(t *testing.T) {
	e, _, _ := privateWikiEngine(t)
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", ""); err != nil {
		t.Fatal(err)
	}
	if page, err := e.ReadPrivateWiki("case-a", "", "index.md", 1, 16); err == nil || page.Content != "" {
		t.Fatal("unbound noninitial cursor was accepted")
	}
}

func TestPrivateWikiScopesAndSourcePointersCannotCrossWorkspace(t *testing.T) {
	e, _, day := privateWikiEngine(t)
	a, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		t.Fatal(err)
	}
	index, err := e.ReadPrivateWiki("case-a", a.Generation, "index.md", 0, 65536)
	if err != nil {
		t.Fatal(err)
	}
	link := regexp.MustCompile(`\]\((concepts/[0-9a-f]{64}[.]md)\)`).FindStringSubmatch(index.Content)
	if len(link) != 2 {
		t.Fatal("source route absent")
	}
	concept, err := e.ReadPrivateWiki("case-a", a.Generation, link[1], 0, 65536)
	if err != nil {
		t.Fatal(err)
	}
	reference := regexp.MustCompile(`maestro://memory/[0-9a-f]{64}`).FindString(concept.Content)
	if err := e.PrepareOwnedCaptureAuthority("case-b"); err != nil {
		t.Fatal(err)
	}
	in, err := e.SealOwnedCapture("case-b", "", 0, "isolated-case-b-only-marker", day)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.CaptureOwned("case-b", in); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DreamDailyAttested(context.Background(), "case-b", day); err != nil {
		t.Fatal(err)
	}
	b, err := e.CompilePrivateWiki(context.Background(), "case-b", "")
	if err != nil {
		t.Fatal(err)
	}
	if page, err := e.ReadPrivateWiki("case-b", a.Generation, "index.md", 0, 65536); err == nil || page.Content != "" {
		t.Fatal("foreign generation accepted")
	}
	if page, err := e.ReadPrivateWikiSource("case-b", b.Generation, reference, 0, 65536); err == nil || page.Content != "" {
		t.Fatal("foreign source capability accepted")
	}
	before := privateWikiTree(t, e.Root)
	if page, err := e.ReadPrivateWiki("case-b", b.Generation, "../case-a/index.md", 0, 65536); err == nil || page.Content != "" {
		t.Fatal("traversal accepted")
	}
	if !reflect.DeepEqual(before, privateWikiTree(t, e.Root)) {
		t.Fatal("refused cross-scope read changed state")
	}
}

func TestPrivateWikiPreparedSiblingPublishersCannotRebaseParent(t *testing.T) {
	e, _, day := privateWikiEngine(t)
	parent, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "new committed sibling race source", day.AddDate(0, 0, 1))); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DreamDailyAttested(context.Background(), "case-a", day.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	first, second := *e, *e
	ready1, ready2, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	first.FaultPoint = func(point string) error {
		if point == "wiki_after_snapshot" {
			close(ready1)
			<-release
		}
		return nil
	}
	second.FaultPoint = func(point string) error {
		if point == "wiki_after_snapshot" {
			close(ready2)
			<-release
		}
		return nil
	}
	type outcome struct {
		r   PrivateWikiResult
		err error
	}
	out := make(chan outcome, 2)
	go func() {
		r, err := first.CompilePrivateWiki(context.Background(), "case-a", parent.Generation)
		out <- outcome{r, err}
	}()
	select {
	case <-ready1:
	case <-time.After(30 * time.Second):
		t.Fatal("first publisher did not prepare")
	}
	go func() {
		r, err := second.CompilePrivateWiki(context.Background(), "case-a", parent.Generation)
		out <- outcome{r, err}
	}()
	select {
	case <-ready2:
	case <-time.After(30 * time.Second):
		close(release)
		t.Fatal("second publisher did not prepare")
	}
	close(release)
	wins, losses := 0, 0
	winner := ""
	for i := 0; i < 2; i++ {
		select {
		case result := <-out:
			if result.err == nil && result.r.Changed {
				wins++
				winner = result.r.Generation
			} else if errors.Is(result.err, ErrPrivateWikiConflict) || errors.Is(result.err, ErrDreamInProgress) {
				losses++
			} else {
				t.Fatalf("unexpected sibling result %+v %v", result.r, result.err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("prepared publisher did not finish")
		}
	}
	if wins != 1 || losses != 1 {
		t.Fatalf("sibling publishers rebased: wins=%d losses=%d", wins, losses)
	}
	status, err := e.PrivateWikiStatus("case-a")
	if err != nil || status.State != "current" || status.Generation != winner || status.Concepts != 2 {
		t.Fatalf("winner graph %+v %v", status, err)
	}
}

func privateWikiTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		st, err := os.Lstat(p)
		if err != nil {
			return err
		}
		name, _ := filepath.Rel(root, p)
		value := fmt.Sprint(st.Mode())
		if st.Mode().IsRegular() {
			body, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			value += " " + fmt.Sprintf("%x", body)
		}
		result[name] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPrivateWikiNoopReadsAndCaptureRevisionDoNotChangeGraph(t *testing.T) {
	e, _, day := privateWikiEngine(t)
	r, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		t.Fatal(err)
	}
	before := privateWikiTree(t, e.Root)
	for i := 0; i < 2; i++ {
		s, err := e.PrivateWikiStatus("case-a")
		if err != nil || s.State != "current" || s.Generation != r.Generation || s.Concepts != 1 {
			t.Fatalf("status %+v %v", s, err)
		}
		if _, err := e.ReadPrivateWiki("case-a", r.Generation, "index.md", 0, 64); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before, privateWikiTree(t, e.Root)) {
		t.Fatal("query created/changed files, entries or modes")
	}
	if _, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "new raw capture without a committed rollup", day.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	s, err := e.PrivateWikiStatus("case-a")
	if err != nil || s.State != "current" || s.Generation != r.Generation {
		t.Fatalf("raw revision invalidated committed source: %+v %v", s, err)
	}
	before = privateWikiTree(t, e.privateWikiRoot("case-a"))
	again, err := e.CompilePrivateWiki(context.Background(), "case-a", r.Generation)
	if err != nil || again.Changed || again.Generation != r.Generation {
		t.Fatalf("no-op replay %+v %v", again, err)
	}
	if !reflect.DeepEqual(before, privateWikiTree(t, e.privateWikiRoot("case-a"))) {
		t.Fatal("no-op created a time-only generation")
	}
}

func TestPrivateWikiChangedCommitHidesAllOldPagesAndFencesParent(t *testing.T) {
	e, _, day := privateWikiEngine(t)
	r, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "second committed route", day.AddDate(0, 0, 1))); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DreamDailyAttested(context.Background(), "case-a", day.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"index.md", "backlinks.json", "log.md"} {
		if page, err := e.ReadPrivateWiki("case-a", r.Generation, ref, 0, 65536); err == nil || page.Content != "" {
			t.Fatal("changed committed source left an old page readable")
		}
	}
	status, err := e.PrivateWikiStatus("case-a")
	if err != nil || status.State != "stale" || status.Concepts != 0 {
		t.Fatalf("stale projection leaked old graph: %+v %v", status, err)
	}
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", ""); !errors.Is(err, ErrPrivateWikiConflict) {
		t.Fatalf("stale parent accepted: %v", err)
	}
	next, err := e.CompilePrivateWiki(context.Background(), "case-a", r.Generation)
	if err != nil || !next.Changed || next.Generation == r.Generation {
		t.Fatalf("replacement graph %+v %v", next, err)
	}
}

func TestPrivateWikiSourceChangeDuringCompileCannotPublish(t *testing.T) {
	e, _, day := privateWikiEngine(t)
	e.FaultPoint = func(point string) error {
		if point == "wiki_after_snapshot" {
			e.FaultPoint = nil
			if _, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "concurrent committed source", day.AddDate(0, 0, 1))); err != nil {
				return err
			}
			_, err := e.DreamDailyAttested(context.Background(), "case-a", day.AddDate(0, 0, 1))
			return err
		}
		return nil
	}
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", ""); !errors.Is(err, ErrPrivateWikiStale) {
		t.Fatalf("changed source published: %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.privateWikiRoot("case-a"), "current.json")); !os.IsNotExist(err) {
		t.Fatal("stale source acquired an active pointer")
	}
}

func TestPrivateWikiCheckpointDowngradeWithGraphRefusesReadback(t *testing.T) {
	e, _, _ := privateWikiEngine(t)
	r, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		t.Fatal(err)
	}
	root := e.workspaceRoot("case-a")
	p := filepath.Join(root, ".visibility", "checkpoint.json")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if json.Unmarshal(b, &state) != nil {
		t.Fatal("checkpoint decode")
	}
	delete(state, "private_wiki")
	b, _ = json.Marshal(state)
	if os.WriteFile(p, b, 0600) != nil {
		t.Fatal("checkpoint fixture write")
	}
	if os.WriteFile(filepath.Join(root, ".visibility-checkpoint-required"), []byte(`{"schema_version":1,"workspace_id":"case-a"}`), 0600) != nil {
		t.Fatal("marker fixture write")
	}
	before := privateWikiTree(t, e.Root)
	if page, err := e.ReadPrivateWiki("case-a", r.Generation, "index.md", 0, 65536); err == nil || page.Content != "" {
		t.Fatal("legacy cohort served a protected graph")
	}
	if !reflect.DeepEqual(before, privateWikiTree(t, e.Root)) {
		t.Fatal("read repaired a downgraded checkpoint")
	}
}

func TestPrivateWikiCancellationAtPointerCannotActivateGraph(t *testing.T) {
	e, _, _ := privateWikiEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.FaultPoint = func(point string) error {
		if point == "wiki_before_pointer" {
			cancel()
		}
		return nil
	}
	if _, err := e.CompilePrivateWiki(ctx, "case-a", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled compile succeeded: %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.privateWikiRoot("case-a"), "current.json")); !os.IsNotExist(err) {
		t.Fatal("cancelled compile published a pointer")
	}
}

func TestPrivateWikiUnknownGraphDirectoryRefusesWithoutRepair(t *testing.T) {
	e, _, _ := privateWikiEngine(t)
	r, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(e.privateWikiRoot("case-a"), "versions", r.Generation, "unlisted-directory"), 0700); err != nil {
		t.Fatal(err)
	}
	before := privateWikiTree(t, e.Root)
	if page, err := e.ReadPrivateWiki("case-a", r.Generation, "index.md", 0, 65536); err == nil || page.Content != "" {
		t.Fatal("unlisted graph structure admitted")
	}
	if !reflect.DeepEqual(before, privateWikiTree(t, e.Root)) {
		t.Fatal("read repaired an unknown graph directory")
	}
}

func TestPrivateWikiCommittedRollupHasReadableConceptIndexAndBacklinks(t *testing.T) {
	e, _, _ := privateWikiEngine(t)
	r, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		t.Fatalf("canonical committed rollup has no private navigation: %v", err)
	}
	if !r.Changed || len(r.Generation) != 64 || r.SourceWatermark == "" {
		t.Fatalf("missing actual generation: %+v", r)
	}
	index, err := e.ReadPrivateWiki("case-a", r.Generation, "index.md", 0, 65536)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(index.Content, "Memory navigation") || !strings.Contains(index.Content, "2026-07-20") || !strings.Contains(index.Content, "okf_version: \"0.2\"") {
		t.Fatalf("unexpected index: %s", index.Content)
	}
	links := regexp.MustCompile(`\]\((concepts/[0-9a-f]{64}[.]md)\)`).FindAllStringSubmatch(index.Content, -1)
	if len(links) != 1 {
		t.Fatalf("expected exactly one known daily concept: %s", index.Content)
	}
	concept, err := e.ReadPrivateWiki("case-a", r.Generation, links[0][1], 0, 65536)
	if err != nil {
		t.Fatal(err)
	}
	for _, literal := range []string{"unique-wiki-launch-marker", "private-memory-local-v1", "local-home-workspace", "maestro://memory/", "../index.md", "../layers/l1/index.md"} {
		if !strings.Contains(concept.Content, literal) {
			t.Fatalf("concept lacks independent expected %q", literal)
		}
	}
	backlinks, err := e.ReadPrivateWiki("case-a", r.Generation, "backlinks.json", 0, 65536)
	if err != nil {
		t.Fatal(err)
	}
	var graph map[string][]string
	if err := json.Unmarshal([]byte(backlinks.Content), &graph); err != nil {
		t.Fatal(err)
	}
	if strings.Join(graph[links[0][1]], ",") != "index.md,layers/l1/index.md" {
		t.Fatalf("actual incoming links = %v", graph[links[0][1]])
	}
}

func TestPrivateWikiProtectedCheckpointRejectsLegacyReader(t *testing.T) {
	e, _, _ := privateWikiEngine(t)
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", ""); err != nil {
		t.Fatal(err)
	}
	marker, err := os.ReadFile(filepath.Join(e.Root, "workspaces", "case-a", ".visibility-checkpoint-required"))
	if err != nil {
		t.Fatal(err)
	}
	var requirement struct {
		SchemaVersion int    `json:"schema_version"`
		WorkspaceID   string `json:"workspace_id"`
	}
	if err := json.Unmarshal(marker, &requirement); err != nil {
		t.Fatal(err)
	}
	if requirement.SchemaVersion == 1 {
		t.Fatal("protected graph remains writable by the old strict checkpoint cohort")
	}
	body, err := os.ReadFile(filepath.Join(e.Root, "workspaces", "case-a", ".visibility", "checkpoint.json"))
	if err != nil {
		t.Fatal(err)
	}
	var old struct {
		SchemaVersion int               `json:"schema_version"`
		WorkspaceID   string            `json:"workspace_id"`
		Revision      uint64            `json:"revision"`
		Generation    uint64            `json:"generation"`
		LegacyThrough uint64            `json:"legacy_through,omitempty"`
		Denied        map[string]uint64 `json:"denied"`
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(&old); err == nil {
		t.Fatal("old checkpoint decoder accepts protected writer metadata")
	}
}

func TestPrivateWikiDenialRetiresEveryDerivedBody(t *testing.T) {
	e, capture, _ := privateWikiEngine(t)
	r, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		t.Fatal(err)
	}
	removed, err := e.DeleteCapture("case-a", capture.ID, capture.Revision)
	if err != nil || !removed.Complete {
		t.Fatalf("deletion not complete: %+v %v", removed, err)
	}
	if page, err := e.ReadPrivateWiki("case-a", r.Generation, "index.md", 0, 65536); err == nil || page.Content != "" {
		t.Fatal("deleted source graph remains readable")
	}
	if _, err := os.Lstat(filepath.Join(e.Root, "workspaces", "case-a", ".private-wiki")); !os.IsNotExist(err) {
		t.Fatal("denial left derived history, staging or pointer bodies on disk")
	}
}

func TestPrivateWikiPartialProtectedMarkerRefusesOwningWrites(t *testing.T) {
	for _, operation := range []string{"compile", "legacy-capture", "delete", "retention"} {
		t.Run(operation, func(t *testing.T) {
			for _, damage := range []string{"missing", "downgraded"} {
				t.Run(damage, func(t *testing.T) {
					e, capture, day := privateWikiEngine(t)
					legacy := Capture{WorkspaceID: "case-a", RecordedAt: day, Kind: "legacy", Text: "existing journal source", Sanitized: true}
					// Keep the existing journal directory/file present so a refused
					// legacy capture must preserve the entire owning tree.
					if _, err := e.Capture(legacy); err != nil {
						t.Fatal(err)
					}
					published, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
					if err != nil {
						t.Fatal(err)
					}
					revision, err := e.VisibilityRevision("case-a")
					if err != nil {
						t.Fatal(err)
					}
					marker := filepath.Join(e.workspaceRoot("case-a"), ".visibility-checkpoint-required")
					if damage == "missing" {
						err = os.Remove(marker)
					} else {
						err = os.WriteFile(marker, []byte("{\"schema_version\":1,\"workspace_id\":\"case-a\"}"), 0600)
					}
					if err != nil {
						t.Fatal(err)
					}
					before := privateWikiTree(t, e.Root)
					switch operation {
					case "compile":
						result, err := e.CompilePrivateWiki(context.Background(), "case-a", published.Generation)
						if !errors.Is(err, ErrPrivateWikiCohort) {
							t.Errorf("partial protected authority admitted compilation: result=%+v err=%v", result, err)
						}
					case "legacy-capture":
						legacy.Text = "must refuse new journal source"
						path, err := e.Capture(legacy)
						if !errors.Is(err, ErrPrivateWikiCohort) || path != "" {
							t.Errorf("partial protected authority admitted legacy capture: path=%q err=%v", path, err)
						}
					case "delete":
						result, err := e.DeleteCapture("case-a", capture.ID, capture.Revision)
						if !errors.Is(err, ErrPrivateWikiCohort) || result.Complete {
							t.Errorf("partial protected authority admitted deletion: result=%+v err=%v", result, err)
						}
					case "retention":
						result, err := e.ApplyRetention("case-a", day.Add(time.Hour), revision)
						if !errors.Is(err, ErrPrivateWikiCohort) || result.Complete {
							t.Errorf("partial protected authority admitted retention: result=%+v err=%v", result, err)
						}
					}
					if !reflect.DeepEqual(before, privateWikiTree(t, e.Root)) {
						t.Error("refused write changed marker, checkpoint, source, graph or modes")
					}
				})
			}
		})
	}
}
