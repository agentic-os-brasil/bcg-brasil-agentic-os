package maintenance

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
)

const UpdateCheckJobID = "update-check"

type UpdatePendingReadback struct {
	State              string                       `json:"state"`
	PlanID             string                       `json:"plan_id,omitempty"`
	Source             *releasecontract.SourceTuple `json:"source,omitempty"`
	Sequence           uint64                       `json:"sequence,omitempty"`
	GenerationSHA256   string                       `json:"generation_sha256,omitempty"`
	CandidateRelease   string                       `json:"candidate_release,omitempty"`
	ManifestSHA256     string                       `json:"manifest_sha256,omitempty"`
	ClientInputsSHA256 string                       `json:"client_inputs_sha256,omitempty"`
	TargetOS           string                       `json:"target_os,omitempty"`
	TargetArch         string                       `json:"target_arch,omitempty"`
}

// UpdateDiagnosis reports the check's actual source observation and prepared
// candidate. It never means native health, activation or rollout acceptance.
type UpdateDiagnosis struct {
	Sequence           uint64 `json:"sequence"`
	GenerationSHA256   string `json:"generation_sha256"`
	ManifestSHA256     string `json:"manifest_sha256"`
	ClientInputsSHA256 string `json:"client_inputs_sha256"`
	CandidateRelease   string `json:"candidate_release"`
	PlanID             string `json:"plan_id,omitempty"`
	Outcome            string `json:"outcome"`
}

func (s *Service) updateOptions() (updateservice.SyncedOptions, error) {
	dataRoot := filepath.Join(s.options.UserHome, ".maestro-install")
	state, err := installtx.ReadSourceState(dataRoot, s.options.ProductRoot)
	if err != nil || state.Source == nil {
		return updateservice.SyncedOptions{}, scheduler.ErrCapabilityUnavailable
	}
	path := filepath.Join(s.options.ProductRoot, "trust", "release-authority-registry.json")
	if s.options.AuthorityRegistryPath != path || s.options.AuthorityRegistrySHA256 == "" {
		return updateservice.SyncedOptions{}, scheduler.ErrCapabilityUnavailable
	}
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(path, s.options.AuthorityRegistrySHA256, s.now)
	if err != nil {
		return updateservice.SyncedOptions{}, err
	}
	return updateservice.SyncedOptions{UserHome: s.options.UserHome, ManagedRoot: s.options.ProductRoot, DataRoot: dataRoot, SourceRoot: state.Source.Locator, ExpectedSource: state.Source.Tuple, Registry: registry, Clock: s.now}, nil
}

func (s *Service) updateReview() (updateservice.SyncedReview, error) {
	o, err := s.updateOptions()
	if err != nil {
		return updateservice.SyncedReview{}, err
	}
	return updateservice.ReviewSynced(o)
}

type productUpdateHandler struct {
	service   *Service
	expected  string
	execution *productExecution
}

func (h productUpdateHandler) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if err := ValidateExecutionGrant(grant, command); err != nil {
		return HandlerResult{}, err
	}
	if command.JobID != UpdateCheckJobID || command.WorkspaceID != h.service.workspaceID {
		return HandlerResult{}, ErrProductConflict
	}
	o, err := h.service.updateOptions()
	if err != nil {
		return HandlerResult{}, err
	}
	review, err := updateservice.ReviewSynced(o)
	if err != nil {
		return HandlerResult{}, err
	}
	if review.Digest != h.expected {
		return HandlerResult{}, ErrProductConflict
	}
	o.BeforeEffect = func(effect func() error) error { return publishWithGrant(ctx, grant, command, effect) }
	o.AfterObserve = func(observed installtx.State) error {
		current, e := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
		if e != nil || installtx.StateDigest(current) != installtx.StateDigest(observed) {
			return ErrProductConflict
		}
		after, e := updateservice.ReviewSynced(o)
		if e != nil || after.GenerationSHA256 != review.GenerationSHA256 {
			return ErrProductConflict
		}
		h.execution.source = after.Digest
		return nil
	}
	checked, checkErr := updateservice.CheckSynced(o, review.Digest)
	if checkErr != nil && !errors.Is(checkErr, updateservice.ErrCurrent) {
		return HandlerResult{}, checkErr
	}
	observation := UpdateDiagnosis{Sequence: checked.Generation.Generation.Sequence, GenerationSHA256: checked.Generation.SHA256, ManifestSHA256: checked.Generation.Generation.ManifestSHA256, ClientInputsSHA256: checked.Generation.Generation.ClientInputsSHA256, CandidateRelease: checked.Generation.Generation.Release, Outcome: "current_signed_release"}
	if checked.Plan.ID != "" {
		err = publishWithGrant(ctx, grant, command, func() error {
			pending, e := updateservice.StagePending(o.DataRoot, checked.Current, checked, h.service.now())
			if e == nil {
				observation.PlanID = pending.Plan.ID
			}
			return e
		})
		if err != nil {
			return HandlerResult{}, err
		}
		observation.Outcome = "candidate_staged_activation_unconfirmed"
	}
	report := DiagnosticReport{SchemaVersion: 1, WorkspaceID: command.WorkspaceID, JobID: UpdateCheckJobID, Implementation: h.service.build, Configuration: h.service.configuration(UpdateCheckJobID), ObservedAt: command.RequestedAt, Finding: "authenticated_update_check", Update: &observation}
	report.SourceSHA256 = report.sourceDigest()
	changed, err := (DiagnosticStore{Root: h.service.receipts.Root}).Publish(report, func(publish func() error) error { return publishWithGrant(ctx, grant, command, publish) })
	if err != nil {
		return HandlerResult{}, err
	}
	if !changed {
		return HandlerResult{State: ReceiptReviewedNoChange, ReasonCode: ReasonReviewedNoChange}, nil
	}
	return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
}
