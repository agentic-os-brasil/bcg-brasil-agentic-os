package ownerctx

import (
	"context"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"time"
)

// WeeklySelfExecution is an original terminal intent committed atomically with
// the working effect. It records existing authority bindings; it grants none.
type WeeklySelfExecution struct {
	RegistrationSHA256   string    `json:"registration_sha256"`
	QualificationSHA256  string    `json:"qualification_sha256"`
	ActivationSHA256     string    `json:"activation_sha256"`
	ImplementationSHA256 string    `json:"implementation_sha256"`
	OwnerScope           string    `json:"owner_scope"`
	CommandID            string    `json:"command_id"`
	AttemptID            string    `json:"attempt_id"`
	OccurrenceSHA256     string    `json:"occurrence_sha256"`
	RequestedAt          time.Time `json:"requested_at"`
	Deadline             time.Time `json:"deadline"`
}

func (e WeeklySelfExecution) Validate() error {
	for _, value := range []string{e.RegistrationSHA256, e.QualificationSHA256, e.ActivationSHA256, e.ImplementationSHA256, e.OccurrenceSHA256} {
		if !validDigest(value) {
			return errors.New("weekly execution digest invalid")
		}
	}
	if !observationIdentifier.MatchString(e.OwnerScope) || !observationIdentifier.MatchString(e.CommandID) || len(e.CommandID) > 64 || len(e.AttemptID) != 32 || !validDigest(e.AttemptID+e.AttemptID) || e.RequestedAt.IsZero() || e.Deadline.IsZero() || !e.Deadline.After(e.RequestedAt) {
		return errors.New("weekly execution terminal intent invalid")
	}
	return nil
}

func (e WeeklySelfExecution) validateSource(options WeeklySelfOptions) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if e.RequestedAt.Before(options.ScheduledFor) || e.OccurrenceSHA256 != digest(scheduler.ScheduledOccurrenceKey("yoda-self-review-weekly", options.ScheduledFor)) || options.OccurrenceID != digestJSON([]any{e.OwnerScope, "yoda-self-review-weekly", options.ScheduledFor}) {
		return ErrWeeklySelfChanged
	}
	return nil
}
func PublishWeeklySelfWithExecution(ctx context.Context, root, inputSHA256 string, candidate WeeklySelfCandidate, expectedRevision uint64, execution WeeklySelfExecution, commit func(func() error) error, now time.Time) (WeeklySelfReceipt, error) {
	if err := execution.Validate(); err != nil {
		return WeeklySelfReceipt{}, err
	}
	return publishWeeklySelf(ctx, root, inputSHA256, candidate, expectedRevision, commit, now, &execution)
}

// WithCurrentWeeklySelfWorking holds source and current-effect serialization
// through metadata reconciliation; expiry or invalidation cannot race success.
func WithCurrentWeeklySelfWorking(ctx context.Context, root string, expected WeeklySelfWorking, now time.Time, action func() error) error {
	if ctx == nil || action == nil {
		return errors.New("weekly working guard requires context and action")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := readWeeklySelf(root, expected.Source.Options, now, func(current WeeklySelfWorking) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if digestJSON(current) != digestJSON(expected) {
			return ErrWeeklySelfChanged
		}
		return action()
	})
	return err
}
