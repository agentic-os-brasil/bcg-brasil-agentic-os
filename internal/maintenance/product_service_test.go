package maintenance

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestProductCheckpointReadDoesNotInitialize(t *testing.T) {
	root := filepath.Join(t.TempDir(), "uninitialized")
	_, err := (ContinuityCheckpointStore{Root: root}).Load("workspace-a")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing checkpoint: %v", err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read initialized checkpoint storage: %v", err)
	}
}

func productFixture(t *testing.T) *Service {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("owned product admission unavailable")
	}
	base := t.TempDir()
	prod := filepath.Join(base, "product")
	owner := filepath.Join(base, "owner")
	ws := filepath.Join(base, "workspace")
	for _, p := range []string{prod, owner} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	lookup := func(string) (string, bool) { return "", false }
	h, err := datahome.InitializeFresh(datahome.Options{ProductRoot: prod, UserHome: owner, LookupEnv: lookup})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: ws, DataRoot: filepath.Join(h.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewService(ServiceOptions{ProductRoot: prod, UserHome: owner, WorkspacePath: ws, LookupEnv: lookup, Catalog: catalog, Adapter: "local"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	s.now = func() time.Time { return now }
	s.engine.Now = s.now
	return s
}
func productSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		body := []byte{}
		if !entry.IsDir() {
			body, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		}
		out[path] = fmt.Sprintf("%v:%x", info.Mode(), sha256.Sum256(body))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func approveProduct(context.Context, ProductReview) error { return nil }
func productApply(t *testing.T, s *Service, kind, id, zone string) ProductJobStatus {
	t.Helper()
	c, err := s.loadControl(id)
	if err != nil {
		t.Fatal(err)
	}
	review, err := s.Review(Operation{Kind: kind, JobID: id, ExpectedRevision: c.Revision, Timezone: zone})
	if err != nil {
		t.Fatalf("review %s: %v", kind, err)
	}
	out, err := s.Apply(context.Background(), review, approveProduct)
	if err != nil {
		t.Fatalf("apply %s: %v", kind, err)
	}
	return out
}
func productCapture(t *testing.T, s *Service, text string) {
	t.Helper()
	if err := s.engine.PrepareOwnedCaptureAuthority(s.workspaceID); err != nil {
		t.Fatal(err)
	}
	in, err := s.engine.SealOwnedCapture(s.workspaceID, "", 0, text, s.now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.engine.CaptureOwned(s.workspaceID, in); err != nil {
		t.Fatal(err)
	}
}

func TestProductServiceFreshReadAndDeclineAreReadOnly(t *testing.T) {
	s := productFixture(t)
	before := productSnapshot(t, s.home)
	status, err := s.Status()
	if err != nil || len(status.Jobs) != 18 {
		t.Fatalf("status %+v %v", status, err)
	}
	for _, j := range status.Jobs {
		if j.Revision != 0 || j.State != "inactive" {
			t.Fatalf("unexpected enrollment %+v", j)
		}
	}
	review, err := s.Review(Operation{Kind: "qualify", JobID: MemoryLightDreamJobID})
	if err != nil {
		t.Fatal(err)
	}
	denied := errors.New("declined")
	if _, err := s.Apply(context.Background(), review, func(context.Context, ProductReview) error { return denied }); !errors.Is(err, denied) {
		t.Fatalf("decline: %v", err)
	}
	if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
		t.Fatal("read or decline changed private home")
	}
	review.Writes = []string{"memory_rollups"}
	if _, err := s.Apply(context.Background(), review, approveProduct); !errors.Is(err, ErrProductConflict) {
		t.Fatalf("edited review accepted: %v", err)
	}
}

func TestProductServiceRealQualificationEnrollmentCheckpointReplay(t *testing.T) {
	s := productFixture(t)
	productCapture(t, s, "Review the evidence and preserve the accepted decision.")
	light := productApply(t, s, "qualify", MemoryLightDreamJobID, "")
	if light.State != "inactive" || light.QualificationState != "qualified" || light.Qualification == nil {
		t.Fatalf("qualification enrolled or lacked evidence: %+v", light)
	}
	if light.Qualification.Receipt.State != ReceiptSucceeded {
		t.Fatal("not a real success")
	}
	artifact, _, err := s.engine.ActiveArtifact(s.workspaceID, "L1/"+s.now().Format("2006-01-02"))
	if err != nil || len(artifact.Sources) != 1 {
		t.Fatalf("L1 readback: %+v %v", artifact, err)
	}
	enrolled := productApply(t, s, "enroll", MemoryLightDreamJobID, "America/Sao_Paulo")
	if enrolled.Revision <= light.Revision || enrolled.Schedule.IntervalHours != 3 || enrolled.Schedule.MaxCatchUp != 1 {
		t.Fatalf("schedule %+v", enrolled)
	}
	status, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, j := range status.Jobs {
		if j.State == "active" {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("active jobs %d", active)
	}
	checkpoint := productApply(t, s, "qualify", MemoryCheckpointJobID, "")
	if checkpoint.QualificationState != "qualified" || checkpoint.State != "inactive" {
		t.Fatalf("checkpoint %+v", checkpoint)
	}
	cp, err := (ContinuityCheckpointStore{Root: s.receipts.Root}).Load(s.workspaceID)
	if err != nil || cp.SourceCount < 1 {
		t.Fatalf("checkpoint readback: %+v %v", cp, err)
	}
	now := s.now().Add(4 * time.Hour)
	s.now = func() time.Time { return now }
	s.engine.Now = s.now
	productCapture(t, s, "A later owned capture must produce a new actual L1.")
	report, err := s.RunDue(context.Background())
	if err != nil || len(report.Due) != 1 || len(report.Receipts) != 1 || report.Receipts[0].State != ReceiptSucceeded {
		t.Fatalf("due: %+v %v", report, err)
	}
	before := productSnapshot(t, s.home)
	restarted, err := NewService(s.options)
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = s.now
	restarted.engine.Now = s.now
	report, err = restarted.RunDue(context.Background())
	if err != nil || len(report.Due) != 0 {
		t.Fatalf("restart duplicate: %+v %v", report, err)
	}
	if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
		t.Fatal("restart replay duplicated output")
	}
	current, err := restarted.jobStatus(MemoryLightDreamJobID)
	if err != nil || current.Schedule.Timezone != "America/Sao_Paulo" {
		t.Fatalf("timezone lost: %+v %v", current, err)
	}
}

func TestProductServiceNoOpCannotQualifyAndCheckpointNeedsSources(t *testing.T) {
	s := productFixture(t)
	for _, job := range []string{MemoryCheckpointJobID, MemoryLightDreamJobID} {
		review, err := s.Review(Operation{Kind: "qualify", JobID: job})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Apply(context.Background(), review, approveProduct); err == nil {
			t.Fatalf("empty %s qualified", job)
		}
		c, err := s.loadControl(job)
		if err != nil || c.Qualification != nil {
			t.Fatalf("fake qualification %+v %v", c, err)
		}
	}
	initial := s.now().Add(time.Second)
	s.now = func() time.Time { return initial }
	s.engine.Now = s.now
	productCapture(t, s, "An owned real source.")
	first := productApply(t, s, "qualify", MemoryLightDreamJobID, "")
	now := s.now().Add(time.Second)
	s.now = func() time.Time { return now }
	s.engine.Now = s.now
	r, err := s.Review(Operation{Kind: "qualify", JobID: MemoryLightDreamJobID, ExpectedRevision: first.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(context.Background(), r, approveProduct); !errors.Is(err, ErrQualificationRequired) {
		t.Fatalf("no-op qualification: %v", err)
	}
}

func TestProductServiceLifecycleCASNoBackfillAndQualificationBinding(t *testing.T) {
	s := productFixture(t)
	productCapture(t, s, "Source for lifecycle.")
	productApply(t, s, "qualify", MemoryLightDreamJobID, "")
	active := productApply(t, s, "enroll", MemoryLightDreamJobID, "UTC")
	stale, err := s.Review(Operation{Kind: "pause", JobID: MemoryLightDreamJobID, ExpectedRevision: active.Revision})
	if err != nil {
		t.Fatal(err)
	}
	paused := productApply(t, s, "pause", MemoryLightDreamJobID, "")
	if _, err := s.Apply(context.Background(), stale, approveProduct); !errors.Is(err, ErrProductConflict) {
		t.Fatalf("stale review %v", err)
	}
	now := s.now().Add(24 * time.Hour)
	s.now = func() time.Time { return now }
	s.engine.Now = s.now
	report, err := s.RunDue(context.Background())
	if err != nil || len(report.Due) != 0 {
		t.Fatalf("paused run %+v %v", report, err)
	}
	resumed := productApply(t, s, "resume", MemoryLightDreamJobID, "")
	if !resumed.Boundary.Equal(paused.Boundary) {
		t.Fatal("resume reset pending boundary")
	}
	productCapture(t, s, "Current-day source after delayed wake.")
	report, err = s.RunDue(context.Background())
	if err != nil || len(report.Due) != 1 || !report.Due[0].ScheduledFor.Equal(active.Boundary.Add(3*time.Hour)) {
		t.Fatalf("bounded catchup %+v %v", report, err)
	}
	revoked := productApply(t, s, "revoke", MemoryLightDreamJobID, "")
	if revoked.State != "revoked" {
		t.Fatal("missing denial tombstone")
	}
	reenrolled := productApply(t, s, "enroll", MemoryLightDreamJobID, "UTC")
	if !reenrolled.Boundary.Equal(now) {
		t.Fatal("reenrollment backfills old boundary")
	}
	report, err = s.RunDue(context.Background())
	if err != nil || len(report.Due) != 0 {
		t.Fatalf("reenrollment backfill %+v %v", report, err)
	}
	c, err := s.loadControl(MemoryLightDreamJobID)
	if err != nil {
		t.Fatal(err)
	}
	original := *c.Qualification
	for _, change := range []func(*ProductQualification){func(q *ProductQualification) { q.WorkspaceID = "other" }, func(q *ProductQualification) { q.Implementation = fmt.Sprintf("%064d", 1) }, func(q *ProductQualification) { q.Configuration = fmt.Sprintf("%064d", 2) }} {
		q := original
		change(&q)
		q.ID = ""
		q.ID = productDigest(q)
		if s.verifyQualification(&q) == nil {
			t.Fatal("foreign or changed qualification accepted")
		}
	}
	for i := range s.options.Catalog.Jobs {
		if s.options.Catalog.Jobs[i].ID == MemoryLightDreamJobID {
			originalReason := s.options.Catalog.Jobs[i].AvailabilityReason
			s.options.Catalog.Jobs[i].AvailabilityReason = "changed relevant installed configuration"
			if s.verifyQualification(&original) == nil {
				t.Fatal("changed actual catalog configuration accepted")
			}
			s.options.Catalog.Jobs[i].AvailabilityReason = originalReason
		}
	}
	other := productFixture(t)
	if other.verifyQualification(&original) == nil {
		t.Fatal("copied qualification accepted")
	}
	before := productSnapshot(t, s.home)
	s.options.WorkspacePath = other.options.WorkspacePath
	if _, err := s.Status(); err == nil {
		t.Fatal("registration change admitted")
	}
	if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
		t.Fatal("bad binding mutated home")
	}
}

type blockingProductSynthesis struct {
	inner            memory.Synthesizer
	entered, release chan struct{}
}

func (b blockingProductSynthesis) Synthesize(ctx context.Context, r memory.SynthesisRequest) (string, error) {
	close(b.entered)
	<-b.release
	return b.inner.Synthesize(ctx, r)
}

func TestProductServiceRevokeDuringSynthesisAcrossProcess(t *testing.T) {
	s := productFixture(t)
	productCapture(t, s, "Source before revoke.")
	entered, release := make(chan struct{}), make(chan struct{})
	s.engine.Synthesizer = blockingProductSynthesis{inner: s.engine.Synthesizer, entered: entered, release: release}
	review, err := s.Review(Operation{Kind: "qualify", JobID: MemoryLightDreamJobID})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.Apply(context.Background(), review, approveProduct); done <- err }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("synthesis never entered")
	}
	child := exec.Command(os.Args[0], "-test.run=^TestProductServiceRevokeHelper$")
	child.Env = append(os.Environ(), "MAESTRO_TEST_REVOKE=1", "MAESTRO_TEST_PRODUCT="+s.options.ProductRoot, "MAESTRO_TEST_OWNER="+s.options.UserHome, "MAESTRO_TEST_WORKSPACE="+s.options.WorkspacePath)
	if body, err := child.CombinedOutput(); err != nil {
		close(release)
		t.Fatalf("second process revoke: %v %s", err, body)
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("revoked attempt succeeded")
	}
	status, err := s.engine.Status(s.workspaceID)
	if err != nil || status.TransactionID != "" {
		t.Fatalf("revoked output visible: %+v %v", status, err)
	}
	receipts, err := s.receipts.Receipts(s.workspaceID, MemoryLightDreamJobID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range receipts {
		if r.State == ReceiptSucceeded {
			t.Fatal("success after denial")
		}
	}
	c, err := s.loadControl(MemoryLightDreamJobID)
	if err != nil || c.State != "revoked" || c.Qualification != nil {
		t.Fatalf("denial %+v %v", c, err)
	}
}
func TestProductServiceRevokeHelper(t *testing.T) {
	if os.Getenv("MAESTRO_TEST_REVOKE") != "1" {
		t.Skip("subprocess helper")
	}
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewService(ServiceOptions{ProductRoot: os.Getenv("MAESTRO_TEST_PRODUCT"), UserHome: os.Getenv("MAESTRO_TEST_OWNER"), WorkspacePath: os.Getenv("MAESTRO_TEST_WORKSPACE"), LookupEnv: func(string) (string, bool) { return "", false }, Catalog: catalog, Adapter: "local"})
	if err != nil {
		t.Fatal(err)
	}
	productApply(t, s, "revoke", MemoryLightDreamJobID, "")
}

func TestProductReviewStableAcrossRoundsAndRejectsChangedSource(t *testing.T) {
	s := productFixture(t)
	productCapture(t, s, "The exact initially reviewed source.")
	op := Operation{Kind: "qualify", JobID: MemoryLightDreamJobID}
	first, err := s.Review(op)
	if err != nil {
		t.Fatal(err)
	}
	now := s.now().Add(time.Second)
	s.now = func() time.Time { return now }
	s.engine.Now = s.now
	next, err := s.Review(op)
	if err != nil || first.seal != next.seal || productDigest(first) != productDigest(next) {
		t.Fatalf("review changed only with time: %v", err)
	}
	_, err = s.Apply(context.Background(), first, func(context.Context, ProductReview) error {
		productCapture(t, s, "A new source arrived during owner review.")
		return nil
	})
	if !errors.Is(err, ErrProductConflict) {
		t.Fatalf("stale source review accepted: %v", err)
	}
	c, err := s.loadControl(MemoryLightDreamJobID)
	if err != nil || c.Revision != 0 || c.Qualification != nil {
		t.Fatalf("stale review wrote control %+v %v", c, err)
	}
	if _, err := os.Lstat(filepath.Dir(s.controlPath(MemoryLightDreamJobID))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale review created control directory: %v", err)
	}
}

func TestProductCheckpointGuardPreservesLastKnownGood(t *testing.T) {
	s := productFixture(t)
	productCapture(t, s, "First actual checkpoint source.")
	productApply(t, s, "qualify", MemoryLightDreamJobID, "")
	productApply(t, s, "enroll", MemoryLightDreamJobID, "UTC")
	productApply(t, s, "qualify", MemoryCheckpointJobID, "")
	store := ContinuityCheckpointStore{Root: s.receipts.Root}
	before, err := store.Load(s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	now := s.now().Add(4 * time.Hour)
	s.now = func() time.Time { return now }
	s.engine.Now = s.now
	productCapture(t, s, "Second actual checkpoint source.")
	if _, err := s.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	receipts, err := s.scheduler.Receipts(s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	guarded := false
	store.PublicationGuard = func(func() error) error { guarded = true; return ErrProductConflict }
	if _, _, err := store.CommitSchedulerReceipts(s.workspaceID, receipts, now); !errors.Is(err, ErrProductConflict) {
		t.Fatalf("denied checkpoint published: %v", err)
	}
	after, err := store.Load(s.workspaceID)
	if err != nil || before != after || !guarded {
		t.Fatalf("last-known-good changed %+v %+v %v", before, after, err)
	}
}

func TestProductReplayReportsPartialReceiptEvidence(t *testing.T) {
	s := productFixture(t)
	productCapture(t, s, "Real source with interrupted receipt publication.")
	q := productApply(t, s, "qualify", MemoryLightDreamJobID, "").Qualification
	name := q.RequestedAt.UTC().Format("20060102T150405.000000000Z") + "-" + q.ScheduledFor.UTC().Format("20060102T150405.000000000Z") + ".json"
	path := filepath.Join(s.scheduler.Root, "workspaces", s.workspaceID, "receipts", MemoryLightDreamJobID, name)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	c, err := s.loadControl(MemoryLightDreamJobID)
	if err != nil {
		t.Fatal(err)
	}
	// Replay uses the existing real occurrence and receipt; missing scheduler
	// evidence must remain partial, even though a canonical artifact exists.
	p := &productExecution{service: s, jobID: MemoryLightDreamJobID, revision: c.Revision, oneShot: "replay-test"}
	c.OneShot = p.oneShot
	if err := s.withControl(c.JobID, func() error { return s.saveControl(c) }); err != nil {
		t.Fatal(err)
	}
	_, err = s.execute(context.Background(), p, scheduler.Occurrence{JobID: MemoryLightDreamJobID, ScheduledFor: q.ScheduledFor}, q.RequestedAt.Add(time.Second))
	if err == nil {
		t.Fatal("partial durable evidence became replay success")
	}
	if s.verifyQualification(q) == nil {
		t.Fatal("missing scheduler evidence retained qualification")
	}
}

func TestProductServiceRejectsControlSymlinkWithoutExternalWrite(t *testing.T) {
	s := productFixture(t)
	outside := t.TempDir()
	parent := filepath.Join(s.receipts.Root, "workspaces", s.workspaceID)
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(parent, "product-control")); err != nil {
		t.Fatal(err)
	}
	before := productSnapshot(t, outside)
	if _, err := s.Status(); err == nil {
		t.Fatal("status followed control symlink")
	}
	if _, err := s.Review(Operation{Kind: "qualify", JobID: MemoryLightDreamJobID}); err == nil {
		t.Fatal("review followed control symlink")
	}
	if !reflect.DeepEqual(before, productSnapshot(t, outside)) {
		t.Fatal("maintenance wrote outside admitted root")
	}
}

func TestProductServiceRefusesCrossJobQualification(t *testing.T) {
	s := productFixture(t)
	productCapture(t, s, "Real qualification for exactly one job.")
	light := productApply(t, s, "qualify", MemoryLightDreamJobID, "")
	c, err := s.loadControl(MemoryCheckpointJobID)
	if err != nil {
		t.Fatal(err)
	}
	c.Revision = 1
	c.Qualification = light.Qualification
	if err := s.withControl(c.JobID, func() error { return s.saveControl(c) }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Review(Operation{Kind: "enroll", JobID: MemoryCheckpointJobID, ExpectedRevision: 1, Timezone: "UTC"}); !errors.Is(err, ErrProductConflict) {
		t.Fatalf("cross-job enrollment accepted: %v", err)
	}
}

func TestProductCheckpointPreparedPublisherCannotReplaceNewerParent(t *testing.T) {
	s := productFixture(t)
	productCapture(t, s, "Baseline actual checkpoint source.")
	productApply(t, s, "qualify", MemoryLightDreamJobID, "")
	productApply(t, s, "enroll", MemoryLightDreamJobID, "UTC")
	productApply(t, s, "qualify", MemoryCheckpointJobID, "")
	store := ContinuityCheckpointStore{Root: s.receipts.Root}
	baseline, err := store.Load(s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	parentDigest, err := checkpointDigest(baseline)
	if err != nil {
		t.Fatal(err)
	}
	now := s.now().Add(4 * time.Hour)
	s.now = func() time.Time { return now }
	s.engine.Now = s.now
	productCapture(t, s, "Actual result A before checkpoint preparation.")
	if _, err := s.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	a, err := s.scheduler.Receipts(s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	prepared, release := make(chan struct{}), make(chan struct{})
	older := store
	older.FaultPoint = func(point string) error {
		if point == "after_version" {
			close(prepared)
			<-release
		}
		return nil
	}
	type result struct {
		checkpoint MemoryCheckpoint
		changed    bool
		err        error
	}
	done := make(chan result, 1)
	olderAt := now
	go func() {
		cp, changed, err := older.CommitSchedulerReceipts(s.workspaceID, a, olderAt)
		done <- result{cp, changed, err}
	}()
	select {
	case <-prepared:
	case <-time.After(10 * time.Second):
		t.Fatal("older checkpoint did not prepare")
	}
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	now = now.Add(4 * time.Hour)
	productCapture(t, s, "Actual result B after older checkpoint preparation.")
	if _, err := s.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	ab, err := s.scheduler.Receipts(s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	newer, changed, err := store.CommitSchedulerReceipts(s.workspaceID, ab, now)
	if err != nil || !changed {
		t.Fatalf("newer publication %+v %v %v", newer, changed, err)
	}
	close(release)
	released = true
	stale := <-done
	current, err := store.Load(s.workspaceID)
	if err != nil || current != newer {
		t.Fatalf("older prepared checkpoint replaced newer active pointer: current=%+v newer=%+v stale=%+v err=%v", current, newer, stale, err)
	}
	if !errors.Is(stale.err, ErrCheckpointConflict) || stale.changed {
		t.Fatalf("stale prepared candidate reported publication: %+v", stale)
	}
	if newer.Revision != baseline.Revision+1 || newer.PreviousCheckpointSHA256 != parentDigest || newer.SourceCount <= baseline.SourceCount {
		t.Fatalf("checkpoint ancestry or sources regressed: %+v", newer)
	}
	replay, changed, err := store.CommitSchedulerReceipts(s.workspaceID, ab, now.Add(time.Second))
	if err != nil || changed || replay != newer {
		t.Fatalf("same-source replay invented new checkpoint: %+v %v %v", replay, changed, err)
	}
}

// Stored authority must survive neither missing identities nor filesystem aliases.
func TestProductControlRejectsMissingStoredIdentity(t *testing.T) {
	s := productFixture(t)
	productCapture(t, s, "Canonical source for stored authority validation.")
	productApply(t, s, "qualify", MemoryLightDreamJobID, "")
	active := productApply(t, s, "enroll", MemoryLightDreamJobID, "UTC")
	path := s.controlPath(MemoryLightDreamJobID)
	valid, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Status(); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"schema_version", "workspace_id", "binding", "job_id"} {
		for _, null := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/null=%t", field, null), func(t *testing.T) {
				var stored map[string]json.RawMessage
				if err := json.Unmarshal(valid, &stored); err != nil {
					t.Fatal(err)
				}
				if null {
					stored[field] = json.RawMessage("null")
				} else {
					delete(stored, field)
				}
				damaged, err := json.Marshal(stored)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, damaged, 0600); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := os.WriteFile(path, valid, 0600); err != nil {
						t.Fatal(err)
					}
				}()
				assertProductControlRefused(t, s, active.Revision)
			})
		}
	}
	if status, err := s.jobStatus(MemoryLightDreamJobID); err != nil || status.State != "active" || status.QualificationState != "qualified" {
		t.Fatalf("valid stored authority lost: %+v %v", status, err)
	}
}

func TestProductControlRejectsHardLinkAuthority(t *testing.T) {
	s := productFixture(t)
	productCapture(t, s, "Canonical source for alias authority validation.")
	productApply(t, s, "qualify", MemoryLightDreamJobID, "")
	active := productApply(t, s, "enroll", MemoryLightDreamJobID, "UTC")
	path := s.controlPath(MemoryLightDreamJobID)
	aliasRoot := t.TempDir()
	alias := filepath.Join(aliasRoot, "aliased-control.json")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	before := productSnapshot(t, aliasRoot)
	assertProductControlRefused(t, s, active.Revision)
	if !reflect.DeepEqual(before, productSnapshot(t, aliasRoot)) {
		t.Fatal("alias bytes/mode/entries changed")
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if status, err := s.jobStatus(MemoryLightDreamJobID); err != nil || status.State != "active" || status.QualificationState != "qualified" {
		t.Fatalf("valid single-link control lost: %+v %v", status, err)
	}
}

func assertProductControlRefused(t *testing.T, s *Service, revision uint64) {
	t.Helper()
	before := productSnapshot(t, s.home)
	if _, err := s.Status(); err == nil {
		t.Error("damaged/aliased control admitted by status")
	}
	if _, err := s.Review(Operation{Kind: "pause", JobID: MemoryLightDreamJobID, ExpectedRevision: revision}); err == nil {
		t.Error("damaged/aliased control admitted by review")
	}
	if _, err := s.RunDue(context.Background()); err == nil {
		t.Error("damaged/aliased control admitted by due dispatch")
	}
	published := false
	p := &productExecution{service: s, jobID: MemoryLightDreamJobID, revision: revision}
	if err := p.guard(func() error { published = true; return nil }); err == nil || published {
		t.Error("damaged/aliased control admitted by publication guard")
	}
	if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
		t.Error("refusal changed bytes/mode/directory entries")
	}
}
