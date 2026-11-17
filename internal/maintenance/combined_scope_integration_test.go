package maintenance

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestIntegratedOwnerServicesRemainExclusiveAcrossScopeCopies(t *testing.T) {
	base := productFixture(t)
	scopes := []struct {
		name, job string
		make      func(*Service) *Service
	}{
		{"availability", UpdateAvailabilityJobID, (*Service).updateAvailabilityService},
		{"weekly", YodaSelfReviewWeeklyJobID, (*Service).weeklyModelService},
		{"self", SelfObservationCaptureJobID, (*Service).selfObservationService},
	}
	expected := map[string]*Service{}
	for i, scope := range scopes {
		service := scope.make(base)
		expected[scope.name] = service
		c, e := service.loadControl(scope.job)
		if e != nil {
			t.Fatal(e)
		}
		c.Revision = uint64(i + 1)
		if e = service.withControl(scope.job, func() error { return service.saveControl(c) }); e != nil {
			t.Fatal(e)
		}
	}
	if expected["weekly"].workspaceID != expected["self"].workspaceID || expected["weekly"].binding == expected["self"].binding {
		t.Fatal("shared owner identity collapsed distinct job authority")
	}
	if expected["availability"].binding == expected["weekly"].binding || expected["availability"].workspaceID == expected["weekly"].workspaceID {
		t.Fatal("installation update scope collapsed into owner memory scope")
	}
	poisoned := *base
	poisoned.updateScoped, poisoned.weeklyScoped, poisoned.ownerScoped = true, true, true
	origins := []struct {
		name    string
		service *Service
	}{{"base", base}, {"availability", expected["availability"]}, {"weekly", expected["weekly"]}, {"self", expected["self"]}, {"mixed-flags", &poisoned}}
	before := productSnapshot(t, base.home)
	for _, origin := range origins {
		for i, scope := range scopes {
			t.Run(origin.name+"-to-"+scope.name, func(t *testing.T) {
				service := scope.make(origin.service)
				want := expected[scope.name]
				if service.updateScoped != (scope.name == "availability") || service.weeklyScoped != (scope.name == "weekly") || service.ownerScoped != (scope.name == "self") {
					t.Fatal("scope copy retained an incompatible flag")
				}
				if service.workspaceID != want.workspaceID || service.binding != want.binding {
					t.Fatal("scope copy retained foreign namespace or authority")
				}
				status, e := service.Status()
				if e != nil {
					t.Fatal(e)
				}
				if len(status.Jobs) != 1 || status.Jobs[0].JobID != scope.job || status.Jobs[0].Scope != "owner" || status.Jobs[0].ScopeID != want.workspaceID || status.Jobs[0].Revision != uint64(i+1) {
					t.Fatal("scoped status traversed another owner's control", status)
				}
				report, e := service.RunDue(context.Background())
				if e != nil || report.State != "no_due_work" || len(report.Due) != 0 || len(report.Receipts) != 0 {
					t.Fatal("inactive scoped wake traversed another scope", report, e)
				}
				domain, e := service.Report(scope.job)
				if e != nil && !errors.Is(e, os.ErrNotExist) {
					t.Fatal(e)
				}
				if domain.JobID != scope.job || domain.WorkspaceID != want.workspaceID {
					t.Fatal("domain report used foreign namespace", domain)
				}
				if origin.name != "mixed-flags" {
					review, e := origin.service.Review(Operation{Kind: "revoke", JobID: scope.job, ExpectedRevision: uint64(i + 1)})
					if e != nil || review.WorkspaceID != want.workspaceID {
						t.Fatal("cross-scope operation did not route to its owning control", review, e)
					}
				}
			})
		}
	}
	after := productSnapshot(t, base.home)
	if len(before) != len(after) {
		t.Fatal("scoped reads initialized another namespace")
	}
	for path, hash := range before {
		if after[path] != hash {
			t.Fatal("scoped read changed owner state", path)
		}
	}
}
