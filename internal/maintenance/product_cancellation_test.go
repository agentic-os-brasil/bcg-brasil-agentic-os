package maintenance

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestProductServiceCanceledQualificationDoesNotAskConfirmation(t *testing.T) {
	s := productFixture(t)
	review, err := s.Review(Operation{Kind: "qualify", JobID: MemoryLightDreamJobID})
	if err != nil {
		t.Fatal(err)
	}
	before := productSnapshot(t, s.home)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	asked := false
	_, err = s.Apply(ctx, review, func(context.Context, ProductReview) error {
		asked = true
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled qualification: %v", err)
	}
	if asked {
		t.Error("canceled operation asked for owner confirmation")
	}
	if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
		t.Error("canceled operation changed canonical state")
	}
}

func TestProductServiceCancellationDuringReReviewKeepsCause(t *testing.T) {
	s := productFixture(t)
	review, err := s.Review(Operation{Kind: "qualify", JobID: MemoryLightDreamJobID})
	if err != nil {
		t.Fatal(err)
	}
	before := productSnapshot(t, s.home)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	confirmed := false
	ctx = WithOperationObserver(ctx, func(phase string) {
		if confirmed && phase == "review" {
			cancel()
		}
	})
	_, err = s.Apply(ctx, review, func(context.Context, ProductReview) error {
		confirmed = true
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("re-review cancellation lost its cause: %v", err)
	}
	if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
		t.Fatal("canceled re-review reserved or changed canonical state")
	}
}

func TestProductServiceCancellationAfterReservationStaysUnqualified(t *testing.T) {
	s := productFixture(t)
	review, err := s.Review(Operation{Kind: "qualify", JobID: MemoryLightDreamJobID})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = WithOperationObserver(ctx, func(phase string) {
		if phase == "selection" {
			cancel()
		}
	})
	if _, err := s.Apply(ctx, review, approveProduct); !errors.Is(err, context.Canceled) {
		t.Fatalf("post-reservation cancellation: %v", err)
	}
	status, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, job := range status.Jobs {
		if job.JobID == MemoryLightDreamJobID {
			found = true
			if job.State != "inactive" || job.Revision != 1 || job.Qualification != nil || job.QualificationState != "unqualified" {
				t.Fatalf("canceled reservation changed eligibility: %+v", job)
			}
		}
	}
	if !found {
		t.Fatal("light job absent from canonical status")
	}
}
