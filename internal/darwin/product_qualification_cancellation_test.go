package darwin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
)

type canceledQualificationInspection struct {
	maintenance.DarwinBinding
	afterInspect func()
}

func (b canceledQualificationInspection) Inspect(job string) (maintenance.DarwinSource, error) {
	source, err := b.DarwinBinding.Inspect(job)
	if err == nil && job == maintenance.DarwinDailyJobID {
		b.afterInspect()
	}
	return source, err
}

func TestProductDarwinCancellationDuringQualificationInspectionDoesNotReserve(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	confirmed := false
	inspectionsAfterConfirmation := 0
	canceledDuringInspection := false
	s, _, _ := productServiceWith(t, func(options *maintenance.ServiceOptions) {
		options.DarwinFactory = func(scope maintenance.DarwinScope) (maintenance.DarwinBinding, error) {
			actual, err := darwin.NewProductBinding(scope)
			if err != nil {
				return nil, err
			}
			return canceledQualificationInspection{actual, func() {
				if confirmed {
					inspectionsAfterConfirmation++
					// The exact post-confirmation review validates and seals its
					// real source before qualification performs its own inspection.
					if inspectionsAfterConfirmation == 3 {
						canceledDuringInspection = true
						cancel()
					}
				}
			}}, nil
		}
	})
	review, err := s.Review(maintenance.Operation{Kind: "qualify", JobID: maintenance.DarwinDailyJobID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Apply(ctx, review, func(context.Context, maintenance.ProductReview) error { confirmed = true; return nil })
	if !canceledDuringInspection || !errors.Is(err, context.Canceled) {
		t.Fatalf("did not observe cancellation during the real qualification inspection: canceled=%t err=%v", canceledDuringInspection, err)
	}
	status, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range status.Jobs {
		if job.JobID == maintenance.DarwinDailyJobID {
			if job.Revision != 0 || job.State != "inactive" || job.Qualification != nil {
				t.Fatalf("canceled source inspection reserved control state: revision=%d state=%s qualified=%t", job.Revision, job.State, job.Qualification != nil)
			}
			return
		}
	}
	t.Fatal("Darwin job absent from canonical status")
}
