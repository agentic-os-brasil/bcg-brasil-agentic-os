package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const SelfObservationCaptureJobID = "self-observation-capture"

// Only this owner job shares its existing worker/control namespace across workspaces.
func (s *Service) selfObservationService() *Service {
	if s.ownerScoped {
		return s
	}
	owner := *s
	owner.ownerScoped = true
	owner.workspaceID, _ = ownerctx.SelfObservationOwnerScope(filepath.Join(s.home, "brain"))
	owner.binding = productDigest([]string{"owner-self-observation-v1", owner.workspaceID})
	return &owner
}

type selfObservationVersion struct {
	IntakeID      string `json:"intake_id"`
	ObservationID string `json:"observation_id"`
	Revision      string `json:"revision"`
}
type selfObservationCaptureReport struct {
	SchemaVersion  int                      `json:"schema_version"`
	OwnerScope     string                   `json:"owner_scope"`
	Implementation string                   `json:"implementation"`
	Configuration  string                   `json:"configuration"`
	Source         string                   `json:"source"`
	RecordedAt     time.Time                `json:"recorded_at"`
	Observations   []selfObservationVersion `json:"observations"`
}

func (s *Service) selfObservationRoot() string { return filepath.Join(s.home, "brain") }
func (s *Service) selfObservationReportRoot() string {
	return filepath.Join(s.receipts.Root, "workspaces", s.workspaceID, "self-observations")
}
func (s *Service) selfObservationSourceIdentity() (string, error) {
	source, err := ownerctx.InspectSelfObservationSource(s.selfObservationRoot(), s.now().UTC())
	return source.Fingerprint, err
}
func (s *Service) readSelfObservationReport(generation string) (selfObservationCaptureReport, string, error) {
	var report selfObservationCaptureReport
	if generation == "" {
		body, err := scheduler.ReadPrivateStateFile(filepath.Join(s.selfObservationReportRoot(), "current.json"), 1024)
		if err != nil {
			return report, "", err
		}
		var pointer struct {
			Generation string `json:"generation"`
		}
		if decodeProduct(body, &pointer) != nil {
			return report, "", errors.New("self observation report pointer is invalid")
		}
		generation = pointer.Generation
	}
	if !digestPattern.MatchString(generation) {
		return report, "", errors.New("self observation report identity is invalid")
	}
	path := filepath.Join(s.selfObservationReportRoot(), "versions", generation+".json")
	body, err := scheduler.ReadPrivateStateFile(path, 1<<20)
	if err != nil {
		return report, "", err
	}
	if decodeProduct(body, &report) != nil || productDigest(report) != generation || report.SchemaVersion != 1 || report.OwnerScope != s.workspaceID || !digestPattern.MatchString(report.Implementation) || !digestPattern.MatchString(report.Configuration) || !digestPattern.MatchString(report.Source) || report.RecordedAt.IsZero() || len(report.Observations) == 0 || len(report.Observations) > 1024 {
		return report, "", errors.New("self observation report is invalid")
	}
	seen := map[string]bool{}
	for _, version := range report.Observations {
		if seen[version.ObservationID] || !strings.HasPrefix(version.IntakeID, "self-intake-") || !digestPattern.MatchString(version.Revision) {
			return report, "", errors.New("self observation version is invalid")
		}
		if err := ownerctx.VerifySelfObservationVersion(s.selfObservationRoot(), version.IntakeID, version.ObservationID, version.Revision); err != nil {
			return report, "", err
		}
		seen[version.ObservationID] = true
	}
	return report, path, nil
}

type productSelfObservationHandler struct {
	service  *Service
	expected string
}

func (h productSelfObservationHandler) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if ValidateExecutionGrant(grant, command) != nil || !h.service.ownerScoped || command.JobID != SelfObservationCaptureJobID || command.WorkspaceID != h.service.workspaceID || command.Trigger != TriggerPresence {
		return HandlerResult{}, errors.New("self observation grant is invalid")
	}
	source, err := ownerctx.InspectSelfObservationSource(h.service.selfObservationRoot(), h.service.now().UTC())
	if err != nil {
		return HandlerResult{}, err
	}
	if source.OwnerScope != command.WorkspaceID || source.Fingerprint != h.expected {
		return HandlerResult{}, ErrProductConflict
	}
	guard := func(publish func() error) error {
		return publishWithGrant(ctx, grant, command, func() error {
			return ownerctx.WithSelfObservationSource(h.service.selfObservationRoot(), h.expected, func() error {
				if err := ctx.Err(); err != nil {
					return err
				}
				return publish()
			})
		})
	}
	report := selfObservationCaptureReport{SchemaVersion: 1, OwnerScope: command.WorkspaceID, Implementation: h.service.build, Configuration: h.service.configuration(SelfObservationCaptureJobID), Source: source.Fingerprint, RecordedAt: command.RequestedAt}
	for _, candidate := range source.Pending {
		if err := ctx.Err(); err != nil {
			return HandlerResult{}, err
		}
		// Capture owns its observation lock; grant/control precede that lock.
		captured, err := ownerctx.CaptureSelfObservation(ctx, h.service.selfObservationRoot(), candidate.ID, h.expected, func(publish func() error) error { return publishWithGrant(ctx, grant, command, publish) })
		if err != nil {
			return HandlerResult{}, err
		}
		if captured.Added {
			if err := ownerctx.VerifySelfObservationVersion(h.service.selfObservationRoot(), candidate.ID, captured.Observation.ID, captured.Observation.Revision); err != nil {
				return HandlerResult{}, err
			}
			report.Observations = append(report.Observations, selfObservationVersion{candidate.ID, captured.Observation.ID, captured.Observation.Revision})
		}
	}
	if len(report.Observations) == 0 {
		return HandlerResult{State: ReceiptReviewedNoChange, ReasonCode: ReasonReviewedNoChange}, nil
	}
	generation := productDigest(report)
	body, err := json.Marshal(report)
	if err != nil {
		return HandlerResult{}, err
	}
	path := filepath.Join(h.service.selfObservationReportRoot(), "versions", generation+".json")
	if err := guard(func() error {
		if err := scheduler.EnsurePrivateDirectory(filepath.Dir(path)); err != nil {
			return err
		}
		if err := scheduler.PublishNewPrivateFile(path, body); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return err
			}
			old, err := scheduler.ReadPrivateStateFile(path, 1<<20)
			if err != nil || string(old) != string(body) {
				return errors.New("self observation report conflict")
			}
		}
		return nil
	}); err != nil {
		return HandlerResult{}, err
	}
	pointer, _ := json.Marshal(struct {
		Generation string `json:"generation"`
	}{generation})
	if err := scheduler.UpdatePrivateFileGuarded(filepath.Join(h.service.selfObservationReportRoot(), "current.json"), 1024, guard, func([]byte) ([]byte, error) { return pointer, nil }); err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
}
