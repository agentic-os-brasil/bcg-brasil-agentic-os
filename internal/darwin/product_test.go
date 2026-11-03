package darwin_test

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

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func productService(t *testing.T) (*maintenance.Service, string, *time.Time) {
	return productServiceWith(t, nil)
}
func productServiceWith(t *testing.T, decorate func(*maintenance.ServiceOptions)) (*maintenance.Service, string, *time.Time) {
	t.Helper()
	base := t.TempDir()
	prod, owner, ws := filepath.Join(base, "product"), filepath.Join(base, "owner"), filepath.Join(base, "workspace")
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
	if _, err = workspace.Initialize(workspace.Options{WorkspacePath: ws, DataRoot: filepath.Join(h.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	catalog, err := maintenance.LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	options := maintenance.ServiceOptions{ProductRoot: prod, UserHome: owner, WorkspacePath: ws, LookupEnv: lookup, Catalog: catalog, Adapter: "local", DarwinFactory: darwin.NewProductBinding, Now: func() time.Time { return now }}
	if decorate != nil {
		decorate(&options)
	}
	s, err := maintenance.NewService(options)
	if err != nil {
		t.Fatal(err)
	}
	return s, h.Home, &now
}

type gatedProduct struct {
	maintenance.DarwinBinding
	blocked, release chan struct{}
}

func (g gatedProduct) Handler(job string, source *maintenance.DarwinSource, attended bool) maintenance.Handler {
	original := g.DarwinBinding.Handler(job, source, attended)
	return maintenance.HandlerFunc(func(ctx context.Context, command maintenance.Command, grant maintenance.ExecutionGrant) (maintenance.HandlerResult, error) {
		close(g.blocked)
		<-g.release
		return original.ExecuteAuthorized(ctx, command, grant)
	})
}
func TestProductDarwinLiveControlSourceAndLeaseFences(t *testing.T) {
	for _, action := range []string{"revoke", "lease", "source", "installation"} {
		t.Run(action, func(t *testing.T) {
			blocked, release := make(chan struct{}), make(chan struct{})
			denied := false
			s, home, _ := productServiceWith(t, func(options *maintenance.ServiceOptions) {
				options.DarwinFactory = func(scope maintenance.DarwinScope) (maintenance.DarwinBinding, error) {
					b, e := darwin.NewProductBinding(scope)
					return gatedProduct{b, blocked, release}, e
				}
				options.InstalledGuard = func(publish func() error) error {
					if denied {
						return errors.New("installation replaced")
					}
					return publish()
				}
			})
			review, e := s.Review(maintenance.Operation{Kind: "qualify", JobID: maintenance.DarwinDailyJobID})
			if e != nil {
				t.Fatal(e)
			}
			done := make(chan error, 1)
			go func() {
				_, e := s.Apply(context.Background(), review, func(context.Context, maintenance.ProductReview) error { return nil })
				done <- e
			}()
			select {
			case <-blocked:
			case <-time.After(10 * time.Second):
				close(release)
				t.Fatal("worker did not hand off real grant")
			}
			switch action {
			case "revoke":
				mutate(t, s, "revoke", maintenance.DarwinDailyJobID)
			case "installation":
				denied = true
			case "source":
				qualify(t, s, "runtime-health-check")
			case "lease":
				store := scheduler.Store{Root: filepath.Join(home, "state", "scheduler")}
				entries, e := os.ReadDir(filepath.Join(store.Root, "workspaces"))
				if e != nil {
					t.Fatal(e)
				}
				root := filepath.Join(store.Root, "workspaces", entries[0].Name(), "leases", maintenance.DarwinDailyJobID)
				files, e := os.ReadDir(root)
				if e != nil {
					t.Fatal(e)
				}
				released := false
				for _, f := range files {
					if strings.HasSuffix(f.Name(), ".json") {
						body, e := os.ReadFile(filepath.Join(root, f.Name()))
						if e != nil {
							t.Fatal(e)
						}
						var lease scheduler.Lease
						if e = json.Unmarshal(body, &lease); e != nil {
							t.Fatal(e)
						}
						if e = store.ReleaseLease(lease); e != nil {
							t.Fatal(e)
						}
						released = true
					}
				}
				if !released {
					t.Fatal("actual lease missing")
				}
			}
			close(release)
			select {
			case e := <-done:
				if e == nil {
					t.Fatal("lost live fence published success")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("lock order deadlocked")
			}
			roots, e := os.ReadDir(filepath.Join(home, "state", "maintenance", "workspaces"))
			if e != nil {
				t.Fatal(e)
			}
			cache := filepath.Join(home, "state", "maintenance", "workspaces", roots[0].Name(), "darwin", "managed-state", "current.json")
			if _, e = os.Stat(cache); !os.IsNotExist(e) {
				t.Fatal("denied worker published active cache")
			}
		})
	}
}

func qualify(t *testing.T, s *maintenance.Service, id string) maintenance.ProductJobStatus {
	t.Helper()
	status, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	var revision uint64
	for _, j := range status.Jobs {
		if j.JobID == id {
			revision = j.Revision
		}
	}
	r, err := s.Review(maintenance.Operation{Kind: "qualify", JobID: id, ExpectedRevision: revision})
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.Apply(context.Background(), r, func(context.Context, maintenance.ProductReview) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestProductDarwinGenuineUpstreamRepairAndReplay(t *testing.T) {
	s, home, now := productService(t)
	const daily = "darwin-housekeeping-daily"
	if _, err := os.Stat(filepath.Join(home, "state", "maintenance", "workspaces")); !os.IsNotExist(err) {
		t.Fatalf("construction wrote workspace maintenance state: %v", err)
	}
	first := qualify(t, s, daily)
	if first.Qualification == nil {
		t.Fatal("real diagnosis initialization must qualify")
	}
	report, err := s.Report(daily)
	if err != nil || report.Finding != "current" {
		t.Fatalf("initialized readback: %+v %v", report, err)
	}
	*now = now.Add(time.Second)
	qualify(t, s, "runtime-health-check") // real owning handler and durable scheduler event
	report, err = s.Report(daily)
	if err != nil || report.Finding != "stale" {
		t.Fatalf("real upstream change must stale cache: %+v %v", report, err)
	}
	*now = now.Add(time.Second)
	repaired := qualify(t, s, daily)
	if repaired.Qualification == nil || repaired.Qualification.ID == first.Qualification.ID {
		t.Fatal("actual cache refresh must produce new verified evidence")
	}
	report, err = s.Report(daily)
	if err != nil || report.Finding != "current" {
		t.Fatalf("repaired readback: %+v %v", report, err)
	}
	*now = now.Add(time.Second)
	replay := qualify(t, s, daily)
	if replay.Qualification == nil || replay.Qualification.ID != repaired.Qualification.ID {
		t.Fatal("unchanged replay minted qualification")
	}
}

func revision(t *testing.T, s *maintenance.Service, id string) uint64 {
	t.Helper()
	status, e := s.Status()
	if e != nil {
		t.Fatal(e)
	}
	for _, j := range status.Jobs {
		if j.JobID == id {
			return j.Revision
		}
	}
	t.Fatal("job missing")
	return 0
}
func mutate(t *testing.T, s *maintenance.Service, kind, id string) maintenance.ProductJobStatus {
	t.Helper()
	op := maintenance.Operation{Kind: kind, JobID: id, ExpectedRevision: revision(t, s, id)}
	if kind == "enroll" {
		op.Timezone = "UTC"
	}
	r, e := s.Review(op)
	if e != nil {
		t.Fatal(e)
	}
	out, e := s.Apply(context.Background(), r, func(context.Context, maintenance.ProductReview) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestProductDarwinAssessmentsAndOptIn(t *testing.T) {
	s, _, now := productService(t)
	for _, id := range []string{maintenance.DarwinWeeklyJobID, maintenance.DarwinMonthlyJobID} {
		*now = now.Add(time.Second)
		first := qualify(t, s, id)
		if first.Qualification == nil || first.Qualification.Receipt.State != maintenance.ReceiptProposalEmitted {
			t.Fatalf("actual proposal outcome missing: %+v", first)
		}
		report, e := s.Report(id)
		if e != nil || report.Darwin == nil || report.Darwin.Evidence.ProposalCount < 1 || report.NativeObservation != "not_observed" {
			t.Fatalf("bounded proposal readback: %+v %v", report, e)
		}
		*now = now.Add(time.Second)
		replay := qualify(t, s, id)
		if replay.Qualification.ID != first.Qualification.ID {
			t.Fatal("assessment replay qualified occurrence churn")
		}
		if id == maintenance.DarwinMonthlyJobID {
			for _, kind := range []string{"enroll", "resume", "pause", "revoke"} {
				if _, e = s.Review(maintenance.Operation{Kind: kind, JobID: id, ExpectedRevision: replay.Revision, Timezone: "UTC"}); e == nil {
					t.Fatalf("monthly accepted %s", kind)
				}
			}
		}
	}
	active := mutate(t, s, "enroll", maintenance.DarwinWeeklyJobID)
	if active.State != "active" {
		t.Fatal("weekly enrollment missing")
	}
	mutate(t, s, "pause", maintenance.DarwinWeeklyJobID)
	*now = now.Add(8 * 24 * time.Hour)
	due, e := s.RunDue(context.Background())
	if e != nil || len(due.Due) != 0 {
		t.Fatalf("paused work dispatched: %+v %v", due, e)
	}
	mutate(t, s, "resume", maintenance.DarwinWeeklyJobID)
	due, e = s.RunDue(context.Background())
	if e != nil || len(due.Due) > 1 {
		t.Fatalf("catch-up not bounded: %+v %v", due, e)
	}
	mutate(t, s, "revoke", maintenance.DarwinWeeklyJobID)
}
func TestProductDarwinDeclineAndChangedSourceRefuse(t *testing.T) {
	s, home, now := productService(t)
	id := maintenance.DarwinDailyJobID
	review, e := s.Review(maintenance.Operation{Kind: "qualify", JobID: id})
	if e != nil {
		t.Fatal(e)
	}
	denied := errors.New("declined")
	if _, e = s.Apply(context.Background(), review, func(context.Context, maintenance.ProductReview) error { return denied }); !errors.Is(e, denied) {
		t.Fatal(e)
	}
	if revision(t, s, id) != 0 {
		t.Fatal("decline wrote control")
	}
	if _, e = os.Stat(filepath.Join(home, "state", "maintenance", "workspaces")); !os.IsNotExist(e) {
		t.Fatal("read/decline wrote state")
	}
	qualify(t, s, "runtime-health-check")
	*now = now.Add(time.Second)
	if _, e = s.Apply(context.Background(), review, func(context.Context, maintenance.ProductReview) error { return nil }); !errors.Is(e, maintenance.ErrProductConflict) {
		t.Fatalf("stale reviewed source accepted: %v", e)
	}
}

func TestProductDarwinInterruptedProposalRecovery(t *testing.T) {
	var root string
	fail := true
	s, _, _ := productServiceWith(t, func(o *maintenance.ServiceOptions) {
		o.DarwinFactory = func(scope maintenance.DarwinScope) (maintenance.DarwinBinding, error) {
			root = scope.Root
			return darwin.NewProductBinding(scope)
		}
		o.InstalledGuard = func(publish func() error) error {
			if fail {
				entries, _ := os.ReadDir(filepath.Join(root, "proposals"))
				if len(entries) > 0 {
					return errors.New("publication interrupted after immutable proposal")
				}
			}
			return publish()
		}
	})
	id := maintenance.DarwinWeeklyJobID
	review, e := s.Review(maintenance.Operation{Kind: "qualify", JobID: id})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(context.Background(), review, func(context.Context, maintenance.ProductReview) error { return nil }); e == nil {
		t.Fatal("interrupted publication claimed success")
	}
	entries, e := os.ReadDir(filepath.Join(root, "proposals"))
	if e != nil || len(entries) != 1 {
		t.Fatalf("actual immutable proposal missing: %v %v", entries, e)
	}
	first, e := os.ReadFile(filepath.Join(root, "proposals", entries[0].Name()))
	if e != nil {
		t.Fatal(e)
	}
	fail = false
	result := qualify(t, s, id)
	if result.Qualification == nil || result.Qualification.Receipt.State != maintenance.ReceiptProposalEmitted {
		t.Fatal("recovery failed to qualify actual stored proposal")
	}
	after, e := os.ReadDir(filepath.Join(root, "proposals"))
	if e != nil || len(after) != 1 {
		t.Fatal("recovery duplicated occurrence")
	}
	body, e := os.ReadFile(filepath.Join(root, "proposals", after[0].Name()))
	if e != nil || !bytes.Equal(body, first) {
		t.Fatal("recovery relabelled immutable proposal")
	}
}

func TestProductDarwinInterruptedCacheRecoversExactAppliedTransition(t *testing.T) {
	var root string
	fail := true
	s, _, _ := productServiceWith(t, func(o *maintenance.ServiceOptions) {
		o.DarwinFactory = func(scope maintenance.DarwinScope) (maintenance.DarwinBinding, error) {
			root = scope.Root
			return darwin.NewProductBinding(scope)
		}
		o.InstalledGuard = func(publish func() error) error {
			if fail {
				if _, e := os.Stat(filepath.Join(root, "managed-state", "current.json")); e == nil {
					return errors.New("interrupted after actual cache rename")
				}
			}
			return publish()
		}
	})
	review, e := s.Review(maintenance.Operation{Kind: "qualify", JobID: maintenance.DarwinDailyJobID})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(context.Background(), review, func(context.Context, maintenance.ProductReview) error { return nil }); !errors.Is(e, darwin.ErrProductRecovery) {
		t.Fatalf("partial cache effect not recovery-required: %v", e)
	}
	cache := filepath.Join(root, "managed-state", "current.json")
	partial, reportErr := s.Report(maintenance.DarwinDailyJobID)
	if reportErr != nil || partial.Finding != "recovery_required" || partial.Darwin.RecoveryState != "required" || partial.Darwin.Evidence.Outcome != maintenance.ReceiptRecoveryRequired {
		t.Fatalf("actual partial effect not reported: %+v %v", partial, reportErr)
	}
	before, e := os.ReadFile(cache)
	if e != nil {
		t.Fatal("actual interrupted effect absent", e)
	}
	fail = false
	q := qualify(t, s, maintenance.DarwinDailyJobID)
	if q.Qualification == nil {
		t.Fatal("real recovered transition did not qualify")
	}
	after, e := os.ReadFile(cache)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("recovery rewrote/relabelled actual cache")
	}
	report, e := s.Report(maintenance.DarwinDailyJobID)
	if e != nil || report.Darwin.Evidence.Effect != "initialized" || report.Darwin.Evidence.Before.Present {
		t.Fatalf("exact absent parent lost during recovery: %+v %v", report, e)
	}
}

func TestProductDarwinUnsafeCacheRefusesWithoutRepair(t *testing.T) {
	for _, kind := range []string{"corrupt", "hardlink", "mode"} {
		t.Run(kind, func(t *testing.T) {
			s, home, _ := productService(t)
			q := qualify(t, s, maintenance.DarwinDailyJobID)
			status, e := s.Status()
			if e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(home, "state", "maintenance", "workspaces", status.WorkspaceID, "darwin", "managed-state", "current.json")
			switch kind {
			case "corrupt":
				e = os.WriteFile(path, []byte("{broken"), 0600)
			case "hardlink":
				e = os.Link(path, path+".alias")
			case "mode":
				e = os.Chmod(path, 0644)
			}
			if e != nil {
				t.Fatal(e)
			}
			before, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			info, e := os.Stat(path)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.Review(maintenance.Operation{Kind: "qualify", JobID: maintenance.DarwinDailyJobID, ExpectedRevision: q.Revision}); e == nil {
				t.Fatal("unsafe cache accepted")
			}
			after, e := os.ReadFile(path)
			if e != nil || !bytes.Equal(before, after) {
				t.Fatal("refusal repaired unsafe bytes")
			}
			afterInfo, e := os.Stat(path)
			if e != nil || afterInfo.Mode() != info.Mode() {
				t.Fatal("refusal repaired permissions")
			}
		})
	}
}
