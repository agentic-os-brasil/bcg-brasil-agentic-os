package maintenance

import (
	"context"
	"errors"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
)

const UpdateAvailabilityJobID = "update-availability"

func updateAvailabilityScope(home, managed string) (string, string) {
	scope := "owner-update-" + productDigest([]string{home, managed})[:40]
	return scope, productDigest([]string{"owner-update-availability-v1", scope})
}

func (s *Service) updateAvailabilityService() *Service {
	if s.updateScoped {
		return s
	}
	owner := *s
	owner.updateScoped, owner.ownerScoped = true, false
	owner.workspaceID, owner.binding = updateAvailabilityScope(s.home, s.options.ProductRoot)
	return &owner
}

// RunUpdateAvailability runs only this enrolled metadata job. It can observe
// availability while the user is active; every other job retains its idle gate.
func (s *Service) RunUpdateAvailability(ctx context.Context) (WakeReport, error) {
	owner := *s.updateAvailabilityService()
	owner.options.EligibilityGuard = nil
	return owner.RunDue(ctx)
}

func (s *Service) updateAvailabilitySource() (string, error) {
	o, err := s.updateOptions()
	if err != nil {
		return "", err
	}
	state, err := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
	if err != nil || state.Source == nil {
		return "", errors.New("enrolled update source is unavailable")
	}
	return installtx.StateDigest(state), nil
}

type productAvailabilityHandler struct {
	service       *Service
	expected      string
	qualification bool
}

func (h productAvailabilityHandler) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if err := ValidateExecutionGrant(grant, command); err != nil {
		return HandlerResult{}, err
	}
	if command.JobID != UpdateAvailabilityJobID || command.WorkspaceID != h.service.workspaceID || !h.service.updateScoped {
		return HandlerResult{}, ErrProductConflict
	}
	o, err := h.service.updateOptions()
	if err != nil {
		return HandlerResult{}, err
	}
	observation, err := updateservice.ObserveSyncedAvailability(o)
	if err != nil {
		return HandlerResult{}, err
	}
	if observation.SourceStateSHA256 != h.expected {
		return HandlerResult{}, ErrProductConflict
	}
	if h.qualification && !observation.MetadataReady() {
		return HandlerResult{}, ErrQualificationRequired
	}
	report := DiagnosticReport{SchemaVersion: 1, WorkspaceID: command.WorkspaceID, JobID: command.JobID,
		Implementation: h.service.build, Configuration: h.service.configuration(command.JobID),
		ObservedAt: command.RequestedAt, Finding: "update_availability_observed", Availability: &observation}
	report.SourceSHA256 = report.sourceDigest()
	_, err = (DiagnosticStore{Root: h.service.receipts.Root}).Publish(report, func(publish func() error) error {
		return publishWithGrant(ctx, grant, command, func() error {
			current, err := h.service.updateAvailabilitySource()
			if err != nil || current != h.expected {
				return ErrProductConflict
			}
			return publish()
		})
	})
	if err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
}
