package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimeprojection"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestProductRemainingJobsProduceDomainEvidence(t *testing.T) {
	for _, id := range []string{MemoryDeepDreamJobID, "memory-retention-check", "runtime-health-check", "capability-recheck", "runtime-drift-check"} {
		t.Run(id, func(t *testing.T) {
			s := productFixture(t)
			productCapture(t, s, "Keep the reviewed ownership boundary and source evidence.")
			productApply(t, s, "qualify", MemoryLightDreamJobID, "")
			result := productApply(t, s, "qualify", id, "")
			if result.Qualification == nil || result.QualificationState != "qualified" || result.State != "inactive" {
				t.Fatalf("missing actual qualification: %+v", result)
			}
			if err := s.verifyQualification(result.Qualification); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProductRemainingReviewIgnoresClockOnlyChange(t *testing.T) {
	s := productFixture(t)
	r, err := s.Review(Operation{Kind: "qualify", JobID: "runtime-health-check"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Apply(context.Background(), r, func(context.Context, ProductReview) error {
		later := s.now().Add(time.Second)
		s.now = func() time.Time { return later }
		return nil
	})
	if err != nil {
		t.Fatalf("clock-only confirmation retry changed source identity: %v", err)
	}
}

func TestProductIndexUsesVerifiedProjectionAndAtomicPair(t *testing.T) {
	s := productFixture(t)
	if _, err := runtimeprojection.Install("codex", s.options.WorkspacePath); err != nil {
		t.Fatal(err)
	}
	qualified := productApply(t, s, "qualify", "skills-index-refresh", "")
	index, err := skillsindex.LoadRuntimeIndex(s.indexRoot(), "")
	if err != nil {
		t.Fatal(err)
	}
	catalog, _, installed, err := runtimeprojection.RoutingInputs("codex", s.options.WorkspacePath)
	if err != nil || len(index.Metadata.Source.Catalog.Skills) != len(catalog.Skills) || len(index.Metadata.Source.Pointers) != len(installed) {
		t.Fatalf("verified installed subset absent: %v", err)
	}
	// Inspect the machine view selected by the durable publication pointer, not
	// merely the canonical fields retained in metadata.
	pointerBody, err := os.ReadFile(filepath.Join(s.indexRoot(), "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pointer struct {
		Generation string `json:"generation"`
	}
	if err := json.Unmarshal(pointerBody, &pointer); err != nil {
		t.Fatal(err)
	}
	if pointer.Generation != index.Generation {
		t.Fatal("readback did not select the published generation")
	}
	catalogBody, err := os.ReadFile(filepath.Join(s.indexRoot(), "generations", pointer.Generation, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		ID              string `json:"id"`
		DefaultPrompt   string `json:"default_prompt"`
		UserInvokedOnly bool   `json:"user_invoked_only"`
		Pointer         string `json:"pointer"`
	}
	if err := json.Unmarshal(catalogBody, &entries); err != nil {
		t.Fatal(err)
	}
	restricted := 0
	for _, skill := range catalog.Skills {
		if !skill.UserInvokedOnly {
			continue
		}
		restricted++
		if skill.DefaultPrompt == "" {
			t.Fatalf("installed restricted skill %s has no canonical default prompt", skill.ID)
		}
		found := false
		for _, entry := range entries {
			if entry.ID == skill.ID {
				found = true
				if entry.DefaultPrompt != skill.DefaultPrompt || !entry.UserInvokedOnly {
					t.Errorf("persisted catalog lost canonical prompt/invocation policy for %s: prompt=%q user_invoked_only=%t", skill.ID, entry.DefaultPrompt, entry.UserInvokedOnly)
				}
				for _, installedSkill := range installed {
					if installedSkill.ID == skill.ID && entry.Pointer != installedSkill.Pointer {
						t.Errorf("persisted catalog changed installed pointer for %s", skill.ID)
					}
				}
			}
		}
		if !found {
			t.Errorf("persisted catalog omitted installed restricted skill %s", skill.ID)
		}
	}
	if restricted == 0 {
		t.Fatal("fixture must contain a genuine installed user-invoked-only skill")
	}
	if err := s.verifyQualification(qualified.Qualification); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(s.options.WorkspacePath, installed[0].Pointer)
	if err := os.WriteFile(skill, []byte("tampered installed method"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Review(Operation{Kind: "qualify", JobID: "skills-index-refresh", ExpectedRevision: qualified.Revision}); err == nil {
		t.Fatal("tampered projection accepted for index")
	}
	drift := productApply(t, s, "qualify", "runtime-drift-check", "")
	report, _, err := (DiagnosticStore{Root: s.receipts.Root}).Load(s.workspaceID, "runtime-drift-check", "")
	if err != nil || report.Finding != "drift_detected" || len(report.Runtime.Projection.Conflicts) == 0 {
		t.Fatalf("real drift disappeared: %+v %v", report, err)
	}
	if drift.QualificationState != "qualified" {
		t.Fatal("diagnostic execution was confused with healthy status")
	}
}
func TestProductDiagnosticsNoOpAndChangedSourceRefused(t *testing.T) {
	s := productFixture(t)
	qualified := productApply(t, s, "qualify", "runtime-health-check", "")
	report, _, err := (DiagnosticStore{Root: s.receipts.Root}).Load(s.workspaceID, "runtime-health-check", "")
	if err != nil || report.Finding != "unavailable" || report.Runtime.NativeObservation != "not_observed" {
		t.Fatalf("manufactured health: %+v %v", report, err)
	}
	review, err := s.Review(Operation{Kind: "qualify", JobID: "runtime-health-check", ExpectedRevision: qualified.Revision})
	if err != nil {
		t.Fatal(err)
	}
	later := s.now().Add(time.Second)
	s.now = func() time.Time { return later }
	if _, err = s.Apply(context.Background(), review, approveProduct); !errors.Is(err, ErrQualificationRequired) {
		t.Fatalf("timestamp-only report qualified: %v", err)
	}
	s = productFixture(t)
	review, err = s.Review(Operation{Kind: "qualify", JobID: "capability-recheck"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Apply(context.Background(), review, func(context.Context, ProductReview) error {
		_, err := runtimeprojection.Install("codex", s.options.WorkspacePath)
		return err
	})
	if !errors.Is(err, ErrProductConflict) {
		t.Fatalf("changed projection did not require review: %v", err)
	}
	c, err := s.loadControl("capability-recheck")
	if err != nil || c.Qualification != nil || c.Revision != 0 {
		t.Fatalf("stale review wrote control: %+v %v", c, err)
	}
}
func TestProductRetentionEmptyAndDiagnosticTamperingRefused(t *testing.T) {
	s := productFixture(t)
	review, err := s.Review(Operation{Kind: "qualify", JobID: "memory-retention-check"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(context.Background(), review, approveProduct); err == nil {
		t.Fatal("empty retention inspection qualified")
	}
	q := productApply(t, s, "qualify", "capability-recheck", "")
	report, path, err := (DiagnosticStore{Root: s.receipts.Root}).Load(s.workspaceID, "capability-recheck", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range report.Runtime.Capabilities {
		if c.NativeQualified || c.AdapterObserved || c.State != "unavailable" {
			t.Fatal("native capability promoted")
		}
	}
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if s.verifyQualification(q.Qualification) == nil {
		t.Fatal("tampered diagnostic artifact retained qualification")
	}
}
func TestProductDeepPublishesRealLayersWithoutStandingLifetimeApproval(t *testing.T) {
	s := productFixture(t)
	productCapture(t, s, "First real weekly source.")
	productApply(t, s, "qualify", MemoryLightDreamJobID, "")
	q := productApply(t, s, "qualify", MemoryDeepDreamJobID, "")
	if q.Qualification == nil {
		t.Fatal("missing weekly commit")
	}
	if _, _, err := s.engine.ActiveArtifact(s.workspaceID, "L2/"+weeklyPeriod(s.now())); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.engine.ActiveArtifact(s.workspaceID, "L3/current"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.engine.ActiveArtifact(s.workspaceID, "lifetime/current"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unreviewed lifetime: %v", err)
	}
	next := s.now().AddDate(0, 0, 7)
	s.now = func() time.Time { return next }
	s.engine.Now = s.now
	productCapture(t, s, "Second distinct week with actual L3 ancestry.")
	light, err := s.loadControl(MemoryLightDreamJobID)
	if err != nil {
		t.Fatal(err)
	}
	review, err := s.Review(Operation{Kind: "qualify", JobID: MemoryLightDreamJobID, ExpectedRevision: light.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(context.Background(), review, approveProduct); err != nil {
		t.Fatal(err)
	}
	productApply(t, s, "qualify", MemoryDeepDreamJobID, "")
	l2, _, err := s.engine.ActiveArtifact(s.workspaceID, "L2/"+weeklyPeriod(s.now()))
	if err != nil || l2.Promotion == nil || l2.Promotion.Eligible || !strings.Contains(l2.Promotion.Reason, "confirmation unavailable") {
		t.Fatalf("two-L3 candidate escaped exact review: %+v %v", l2.Promotion, err)
	}
	if _, _, err := s.engine.ActiveArtifact(s.workspaceID, "lifetime/current"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("standing lifetime approval: %v", err)
	}
}

func TestProductRemainingEnrollmentDueAndReplay(t *testing.T) {
	for _, job := range []string{MemoryDeepDreamJobID, "memory-retention-check", "skills-index-refresh", "runtime-health-check", "capability-recheck", "runtime-drift-check"} {
		t.Run(job, func(t *testing.T) {
			s := productFixture(t)
			productCapture(t, s, "The first approved source remains preserved.")
			productApply(t, s, "qualify", MemoryLightDreamJobID, "")
			if job == "skills-index-refresh" {
				if _, err := runtimeprojection.Install("codex", s.options.WorkspacePath); err != nil {
					t.Fatal(err)
				}
			}
			productApply(t, s, "qualify", job, "")
			active := productApply(t, s, "enroll", job, "UTC")
			if active.Schedule.MaxCatchUp != 1 {
				t.Fatal("unbounded catch-up")
			}
			if job == MemoryDeepDreamJobID && active.Schedule.Cadence != scheduler.Weekly {
				t.Fatal("deep job did not retain weekly cadence")
			}
			days := 1
			if job == MemoryDeepDreamJobID {
				days = 8
			}
			later := s.now().AddDate(0, 0, days)
			s.now = func() time.Time { return later }
			s.engine.Now = s.now
			productCapture(t, s, "A later real source updates the eligible snapshot.")
			productApply(t, s, "qualify", MemoryLightDreamJobID, "")
			if job == "skills-index-refresh" {
				if _, err := runtimeprojection.Uninstall("codex", s.options.WorkspacePath); err != nil {
					t.Fatal(err)
				}
				if _, err := runtimeprojection.Install("claude", s.options.WorkspacePath); err != nil {
					t.Fatal(err)
				}
			} else if diagnosticJob(job) && job != "memory-retention-check" {
				if _, err := runtimeprojection.Install("codex", s.options.WorkspacePath); err != nil {
					t.Fatal(err)
				}
			}
			out, err := s.RunDue(context.Background())
			if err != nil || len(out.Due) != 1 || len(out.Receipts) != 1 || out.Receipts[0].State != ReceiptSucceeded {
				t.Fatalf("actual due execution: %+v %v", out, err)
			}
			before := productSnapshot(t, s.home)
			out, err = s.RunDue(context.Background())
			if err != nil || len(out.Due) != 0 {
				t.Fatalf("replay duplicated work: %+v %v", out, err)
			}
			if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
				t.Fatal("replay changed durable output")
			}
		})
	}
}
func TestProductDeepRevocationBlocksCommit(t *testing.T) {
	s := productFixture(t)
	productCapture(t, s, "A real source whose pending weekly output can be revoked.")
	productApply(t, s, "qualify", MemoryLightDreamJobID, "")
	blocked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	s.weekly.FaultPoint = func(point string) error {
		if point == "after_stage_manifest" {
			close(blocked)
			<-release
		}
		return nil
	}
	review, err := s.Review(Operation{Kind: "qualify", JobID: MemoryDeepDreamJobID})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _, err := s.Apply(context.Background(), review, approveProduct); done <- err }()
	select {
	case <-blocked:
	case <-time.After(30 * time.Second):
		t.Fatal("weekly publication did not block")
	}
	productApply(t, s, "revoke", MemoryDeepDreamJobID, "")
	close(release)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("revoked weekly attempt succeeded")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("revocation lock order deadlocked")
	}
	if _, _, err := s.engine.ActiveArtifact(s.workspaceID, "L3/current"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("revoked commit became visible: %v", err)
	}
	receipts, err := s.receipts.Receipts(s.workspaceID, MemoryDeepDreamJobID)
	if err != nil {
		t.Fatal(err)
	}
	for _, receipt := range receipts {
		if receipt.State == ReceiptSucceeded {
			t.Fatal("revoked weekly receipt succeeded")
		}
	}
}
func TestProductDiagnosticAndIndexPublicationGuardPreservesLastGood(t *testing.T) {
	s := productFixture(t)
	productApply(t, s, "qualify", "runtime-health-check", "")
	store := DiagnosticStore{Root: s.receipts.Root}
	before, path, err := store.Load(s.workspaceID, "runtime-health-check", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeprojection.Install("codex", s.options.WorkspacePath); err != nil {
		t.Fatal(err)
	}
	next, err := s.diagnosticSource("runtime-health-check")
	if err != nil {
		t.Fatal(err)
	}
	next.ObservedAt = s.now().Add(time.Second)
	denied := errors.New("revoked live publication grant")
	changed, err := store.Publish(next, func(func() error) error { return denied })
	if !errors.Is(err, denied) || changed {
		t.Fatal("denied report became active")
	}
	after, afterPath, err := store.Load(s.workspaceID, "runtime-health-check", "")
	if err != nil || path != afterPath || before.SourceSHA256 != after.SourceSHA256 {
		t.Fatal("denied report replaced last good")
	}
	source, err := s.indexSource()
	if err != nil {
		t.Fatal(err)
	}
	_, changed, err = skillsindex.PublishRuntimeIndex(s.indexRoot(), source, func(func() error) error { return denied })
	if !errors.Is(err, denied) || changed {
		t.Fatal("denied index became active")
	}
	if _, err := skillsindex.LoadRuntimeIndex(s.indexRoot(), ""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial pair was published: %v", err)
	}
}

func TestProductRuntimeMalformedSelectionAndEmptyWeeklyRefused(t *testing.T) {
	for _, job := range []string{"runtime-health-check", "capability-recheck", "runtime-drift-check", "skills-index-refresh"} {
		t.Run(job, func(t *testing.T) {
			s := productFixture(t)
			path := filepath.Join(s.options.WorkspacePath, runtimeprojection.ManifestRelativePath)
			if err := os.WriteFile(path, []byte(`{"runtime":"invented-runtime"}`), 0600); err != nil {
				t.Fatal(err)
			}
			before := productSnapshot(t, s.home)
			if _, err := s.Review(Operation{Kind: "qualify", JobID: job}); err == nil {
				t.Fatal("unverified runtime selection accepted")
			}
			if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
				t.Fatal("refused source changed private state")
			}
		})
	}
	s := productFixture(t)
	review, err := s.Review(Operation{Kind: "qualify", JobID: MemoryDeepDreamJobID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(context.Background(), review, approveProduct); err == nil {
		t.Fatal("source-free weekly job qualified")
	}
}

func TestProductProvenanceHistoricalReadbackAndRecovery(t *testing.T) {
	for _, job := range []string{"skills-index-refresh", "memory-retention-check"} {
		t.Run(job, func(t *testing.T) {
			s := productFixture(t)
			productCapture(t, s, "Unchanged canonical input for provenance recovery.")
			if _, err := runtimeprojection.Install("codex", s.options.WorkspacePath); err != nil {
				t.Fatal(err)
			}
			qualified := productApply(t, s, "qualify", job, "")
			var legacyPath, domainRoot string
			if job == "skills-index-refresh" {
				index, err := skillsindex.LoadRuntimeIndex(s.indexRoot(), "")
				if err != nil {
					t.Fatal(err)
				}
				legacy := index.Metadata
				legacy.Source.Implementation, legacy.Source.Configuration = "", ""
				body, err := json.Marshal(legacy)
				if err != nil {
					t.Fatal(err)
				}
				generation := rawDigest(body)
				dir := filepath.Join(s.indexRoot(), "generations", generation)
				if err := scheduler.EnsurePrivateDirectory(dir); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"catalog.json", "INDEX.md"} {
					pair, err := os.ReadFile(filepath.Join(filepath.Dir(index.MetadataPath), name))
					if err != nil {
						t.Fatal(err)
					}
					if err := scheduler.WritePrivateFileAtomic(filepath.Join(dir, name), pair); err != nil {
						t.Fatal(err)
					}
				}
				legacyPath = filepath.Join(dir, "metadata.json")
				if err := scheduler.WritePrivateFileAtomic(legacyPath, body); err != nil {
					t.Fatal(err)
				}
				pointer, _ := json.Marshal(map[string]string{"generation": generation})
				if err := scheduler.WritePrivateFileAtomic(filepath.Join(s.indexRoot(), "current.json"), pointer); err != nil {
					t.Fatal(err)
				}
				read, err := skillsindex.LoadRuntimeIndex(s.indexRoot(), "")
				if err != nil || read.Metadata.Source.Implementation != "" || read.Metadata.Source.Configuration != "" {
					t.Fatalf("historical provenance synthesized or intact readback failed: %+v %v", read, err)
				}
				called := false
				if _, _, err := skillsindex.PublishRuntimeIndex(s.indexRoot(), read.Metadata.Source, func(func() error) error { called = true; return nil }); err == nil || called {
					t.Fatal("new publication admitted missing provenance")
				}
				domainRoot = s.indexRoot()
			} else {
				store := DiagnosticStore{Root: s.receipts.Root}
				legacy, _, err := store.Load(s.workspaceID, job, "")
				if err != nil {
					t.Fatal(err)
				}
				legacy.Implementation, legacy.Configuration = "", ""
				legacy.SourceSHA256 = productDigest(legacy.Retention)
				body, err := json.Marshal(legacy)
				if err != nil {
					t.Fatal(err)
				}
				generation := rawDigest(body)
				domainRoot = store.root(s.workspaceID, job)
				legacyPath = filepath.Join(domainRoot, "versions", generation+".json")
				if err := scheduler.WritePrivateFileAtomic(legacyPath, body); err != nil {
					t.Fatal(err)
				}
				pointer, _ := json.Marshal(map[string]string{"generation": generation})
				if err := scheduler.WritePrivateFileAtomic(filepath.Join(domainRoot, "current.json"), pointer); err != nil {
					t.Fatal(err)
				}
				read, _, err := store.Load(s.workspaceID, job, "")
				if err != nil || read.Implementation != "" || read.Configuration != "" {
					t.Fatalf("historical provenance synthesized or intact readback failed: %+v %v", read, err)
				}
				called := false
				if _, err := store.Publish(read, func(func() error) error { called = true; return nil }); err == nil || called {
					t.Fatal("new publication admitted missing provenance")
				}
			}
			before, ref, err := s.domainEvidence(job, s.now())
			if err != nil || before == "" {
				t.Fatalf("intact historical pre-run evidence blocked: %v", err)
			}
			// Even an otherwise current qualification cannot present a provenance-free
			// historical artifact as current execution evidence.
			oldQ := *qualified.Qualification
			oldQ.Artifact, oldQ.ArtifactSHA256 = ref, before
			oldQ.ID = ""
			oldQ.ID = productDigest(oldQ)
			if err := s.verifyQualification(&oldQ); err == nil {
				t.Fatal("historical artifact qualified as current execution")
			}
			review, err := s.Review(Operation{Kind: "qualify", JobID: job, ExpectedRevision: qualified.Revision})
			if err != nil {
				t.Fatal(err)
			}
			q, err := s.Apply(context.Background(), review, func(context.Context, ProductReview) error {
				later := s.now().Add(time.Second)
				s.now = func() time.Time { return later }
				return nil
			})
			if err != nil || q.QualificationState != "qualified" {
				t.Fatalf("historical recovery/clock-only review: %+v %v", q, err)
			}
			if err := s.verifyQualification(q.Qualification); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(legacyPath); err != nil {
				t.Fatal("old immutable evidence removed", err)
			}
			// A real configuration change is also provenance, not a nonce or clock.
			s.options.CLIVersion = "changed-trusted-configuration"
			// A distinct attempt needs a distinct scheduler occurrence; configuration,
			// not this clock advance, is the changed execution evidence.
			configuredAt := s.now().Add(time.Second)
			s.now = func() time.Time { return configuredAt }
			status, err := s.jobStatus(job)
			if err != nil || status.QualificationState != "stale" {
				t.Fatalf("changed configuration did not stale qualification: %+v %v", status, err)
			}
			q = productApply(t, s, "qualify", job, "")
			review, err = s.Review(Operation{Kind: "qualify", JobID: job, ExpectedRevision: q.Revision})
			if err != nil {
				t.Fatal(err)
			}
			beforeNoOp := productSnapshot(t, domainRoot)
			later := s.now().Add(time.Second)
			s.now = func() time.Time { return later }
			if _, err := s.Apply(context.Background(), review, approveProduct); !errors.Is(err, ErrQualificationRequired) {
				t.Fatalf("same-input clock-only retry qualified: %v", err)
			}
			if !reflect.DeepEqual(beforeNoOp, productSnapshot(t, domainRoot)) {
				t.Fatal("no-op changed durable domain output")
			}
			_, currentRef, err := s.domainEvidence(job, s.now())
			if err != nil {
				t.Fatal(err)
			}
			currentPath := filepath.Join(s.home, currentRef)
			if job != "skills-index-refresh" {
				currentPath = filepath.Join(s.receipts.Root, strings.TrimPrefix(currentRef, "maintenance/"))
			}
			if err := os.WriteFile(currentPath, []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
			tampered := productSnapshot(t, domainRoot)
			control, err := s.loadControl(job)
			if err != nil {
				t.Fatal(err)
			}
			review, err = s.Review(Operation{Kind: "qualify", JobID: job, ExpectedRevision: control.Revision})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Apply(context.Background(), review, approveProduct); err == nil {
				t.Fatal("tampered previous evidence silently replaced")
			}
			if !reflect.DeepEqual(tampered, productSnapshot(t, domainRoot)) {
				t.Fatal("failed recovery repaired tampered evidence")
			}
		})
	}
}
