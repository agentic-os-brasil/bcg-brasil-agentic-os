package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
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
	helper := os.Getenv("MAESTRO_PRIVATE_WIKI_OLD_WRITER")
	if helper == "" {
		t.Skip("actual old-cohort helper not selected")
	}
	if !filepath.IsAbs(helper) {
		t.Fatal("old helper path must be absolute")
	}
	e, capture, _ := privateWikiEngine(t)
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", ""); err != nil {
		t.Fatal(err)
	}
	cfg := struct {
		Policy    Policy                   `json:"policy"`
		Budgets   map[string]int           `json:"budgets"`
		Freshness map[string]time.Duration `json:"freshness"`
		Clock     time.Time                `json:"clock"`
	}{e.Policy, e.Budgets, e.Freshness, e.Now()}
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "historical-config.json")
	if err := os.WriteFile(config, body, 0600); err != nil {
		t.Fatal(err)
	}
	type oldCall struct {
		API           string `json:"api"`
		Outcome       string `json:"outcome"`
		Error         string `json:"error,omitempty"`
		ReturnedEmpty bool   `json:"returned_empty"`
	}
	type oldResult struct {
		SchemaVersion      int       `json:"schema_version"`
		ArtifactKind       string    `json:"artifact_kind"`
		LibraryRevision    string    `json:"library_revision"`
		Platform           string    `json:"platform"`
		Operation          string    `json:"operation"`
		PositiveControl    bool      `json:"positive_control"`
		OwnedSupported     bool      `json:"owned_supported"`
		LegacyContextFound bool      `json:"legacy_context_found"`
		Calls              []oldCall `json:"calls"`
	}
	invoke := func(args ...string) oldResult {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, helper, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("historical-library harness failed: %v %s", err, output)
		}
		var receipt oldResult
		if err := decodeMemoryJSON(output, &receipt); err != nil || receipt.SchemaVersion != 2 ||
			receipt.ArtifactKind != "new_harness_unmodified_historical_library" ||
			receipt.LibraryRevision != "f93e9ce21edcd19246e7e5985d97489297dae18b" ||
			receipt.Platform != runtime.GOOS || receipt.Operation != args[0] {
			t.Fatalf("unbound or invalid historical receipt: %v %s", err, output)
		}
		t.Logf("historical operation=%s library=%s platform=%s calls=%+v", receipt.Operation, receipt.LibraryRevision, receipt.Platform, receipt.Calls)
		return receipt
	}
	snapshot := func() map[string]string {
		t.Helper()
		state := map[string]string{}
		err := filepath.WalkDir(e.Root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			value := info.Mode().String() + " " + statusSecuritySnapshot(t, path)
			// Historical activation locks change directory mtimes transiently; final entries and security must remain identical.
			if !info.IsDir() {
				value += fmt.Sprintf(" %d %s", info.Size(), info.ModTime())
			}
			if info.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				value += " " + target
			} else if info.Mode().IsRegular() {
				body, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				value += fmt.Sprintf(" %x", sha256.Sum256(body))
			}
			state[path] = value
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	program, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("actual historical-library harness sha256=%x", sha256.Sum256(program))
	beforePositive := snapshot()
	fresh := t.TempDir()
	positive := invoke("positive", fresh, config)
	if !positive.PositiveControl || !positive.LegacyContextFound || positive.OwnedSupported != (runtime.GOOS != "windows") || len(positive.Calls) != 13 {
		t.Fatalf("historical API positive control missing: %+v", positive)
	}
	wantPositive := []string{"memory.Bootstrap(legacy)", "Engine.Capture(legacy_journal)", "CaptureAttestor.Seal",
		"Engine.Capture(attested_journal)", "Engine.DreamDailyAttested", "Engine.Status", "Engine.AssembleContext",
		"memory.Bootstrap(owned)", "Engine.PrepareOwnedCaptureAuthority", "Engine.SealOwnedCapture",
		"Engine.CaptureOwned", "Engine.ReadCapture", "Engine.DeleteCapture"}
	for i, call := range positive.Calls {
		want := "success"
		if runtime.GOOS == "windows" && i >= 8 {
			want = "confinement_unavailable"
		}
		if call.API != wantPositive[i] || call.Outcome != want || (want == "success" && call.Error != "") ||
			(want != "success" && (call.Error != ErrConfinementUnavailable.Error() || !call.ReturnedEmpty)) {
			t.Fatalf("positive library call not genuine: %+v", call)
		}
	}
	legacy := &Engine{Root: filepath.Join(fresh, "legacy", "brain", "memory"), Policy: e.Policy,
		Budgets: e.Budgets, Freshness: e.Freshness, Now: e.Now}
	status, err := legacy.Status("case-a")
	if err != nil || status.State != "ready" || status.CaptureFiles < 2 {
		t.Fatalf("current reader cannot see actual old positive source: %+v %v", status, err)
	}
	bundle, err := legacy.AssembleContext("case-a")
	found := false
	for _, section := range bundle.Sections {
		found = found || strings.Contains(section.Content, "MAESTRO_HISTORICAL_LIBRARY_POSITIVE")
	}
	if err != nil || !found {
		t.Fatalf("genuine historical positive did not return source context: %v", err)
	}
	if !reflect.DeepEqual(beforePositive, snapshot()) {
		t.Fatal("separate historical positive changed the protected root")
	}
	operations := []struct {
		name string
		api  string
	}{
		{"legacy_capture", "Engine.Capture"}, {"legacy_status", "Engine.Status"},
		{"legacy_context", "Engine.AssembleContext"}, {"owned_read", "Engine.ReadCapture"},
		{"owned_capture", "Engine.CaptureOwned"}, {"owned_delete", "Engine.DeleteCapture"},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			before := snapshot()
			receipt := invoke(operation.name, e.Root, config, capture.ID, strconv.FormatUint(capture.Revision, 10))
			expectedCalls := 1
			if operation.name == "owned_capture" {
				expectedCalls = 2
			}
			if receipt.PositiveControl || len(receipt.Calls) != expectedCalls {
				t.Fatalf("actual historical operation is missing: %+v", receipt)
			}
			want := "visibility_requirement_refused"
			wantError := "invalid memory visibility checkpoint requirement"
			if runtime.GOOS == "windows" && strings.HasPrefix(operation.name, "owned_") {
				want, wantError = "confinement_unavailable", ErrConfinementUnavailable.Error()
			}
			if expectedCalls == 2 {
				seal := receipt.Calls[0]
				if seal.API != "Engine.SealOwnedCapture" ||
					(runtime.GOOS == "windows" && (seal.Outcome != "confinement_unavailable" || seal.Error != wantError || !seal.ReturnedEmpty)) ||
					(runtime.GOOS != "windows" && (seal.Outcome != "success" || seal.Error != "" || seal.ReturnedEmpty)) {
					t.Fatalf("capture input preparation was misrepresented: %+v", seal)
				}
			}
			actual := receipt.Calls[len(receipt.Calls)-1]
			if actual.API != operation.api || actual.Outcome != want || actual.Error != wantError || !actual.ReturnedEmpty {
				t.Fatalf("actual historical call did not refuse truthfully: %+v", actual)
			}
			if !reflect.DeepEqual(before, snapshot()) {
				t.Fatal("historical process changed final source, graph, fence, file metadata or native security")
			}
		})
	}
}
