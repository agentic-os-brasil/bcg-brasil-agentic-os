package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

// WeeklyModelRegistration is an explicit local declaration, not a native
// qualification, activation, credential source or model-selected executable.
type WeeklyModelRegistration struct {
	Configuration    WeeklyConfiguration        `json:"configuration,omitempty"`
	SchemaVersion    int                        `json:"schema_version"`
	WorkingDirectory string                     `json:"working_directory,omitempty"`
	Environment      []string                   `json:"environment,omitempty"`
	Descriptor       WeeklyModelDescriptor      `json:"descriptor"`
	Policy           ownerctx.WeeklySelfOptions `json:"policy"`
}

func (r WeeklyModelRegistration) Validate() error {
	if err := ValidateWeeklyModelEnvironment(r.Environment); err != nil {
		return err
	}
	if r.SchemaVersion != 1 {
		return errors.New("weekly registration schema is invalid")
	}
	if body, err := json.Marshal(r); err != nil || len(body) > 32768 {
		return errors.New("weekly registration exceeds fixed control metadata bound")
	}
	if r.Descriptor.RuntimeIdentity != nil {
		if !filepath.IsAbs(r.WorkingDirectory) || filepath.Clean(r.WorkingDirectory) != r.WorkingDirectory {
			return errors.New("weekly native working directory must be explicit absolute path")
		}
		if err := r.Configuration.Validate(); err != nil {
			return err
		}
		if r.Configuration.Digest("settings") != r.Descriptor.ConfigurationSHA256 || r.Configuration.Digest("managed_policy") != r.Descriptor.ManagedPolicySHA256 {
			return errors.New("weekly configuration digests differ from explicit inventory")
		}
	}
	if err := r.Descriptor.Validate(); err != nil {
		return err
	}
	if err := ownerctx.ValidateWeeklySelfPolicy(r.Policy); err != nil {
		return err
	}
	if r.Policy.Limits.MaxInputBytes > r.Descriptor.Limits.MaxInputBytes {
		return errors.New("weekly source policy exceeds registered input bound")
	}
	return nil
}
func (r WeeklyModelRegistration) Digest() string { return productDigest(r) }

type weeklyModelControl struct {
	Registration             WeeklyModelRegistration `json:"registration"`
	RegistrationReviewSHA256 string                  `json:"registration_review_sha256"`
}

func (s *Service) weeklyModelService() *Service {
	if s.weeklyScoped {
		return s
	}
	owner := *s
	owner.weeklyScoped = true
	owner.ownerScoped = false
	owner.workspaceID, _ = ownerctx.SelfObservationOwnerScope(filepath.Join(s.home, "brain"))
	owner.binding = productDigest([]string{"owner-weekly-model-v1", owner.workspaceID})
	return &owner
}
func (s *Service) reviewWeeklyRegistration(op Operation) (ProductReview, error) {
	var empty ProductReview
	if op.JobID != YodaSelfReviewWeeklyJobID || op.Kind != "register" || op.Timezone != "" || s.options.WeeklyCandidate == nil {
		return empty, errors.New("explicit weekly native registration declaration is unavailable")
	}
	if err := s.validate(); err != nil {
		return empty, err
	}
	owned, err := cloneWeeklyRegistration(s.options.WeeklyCandidate)
	if err != nil {
		return empty, err
	}
	candidate := *owned
	c, err := s.loadControl(op.JobID)
	if err != nil {
		return empty, err
	}
	if c.Revision != op.ExpectedRevision {
		return empty, ErrProductConflict
	}
	job, _ := findCatalogJob(s.options.Catalog, op.JobID)
	r := ProductReview{Scope: "owner", Operation: op, WorkspaceID: s.workspaceID, Writes: append([]string(nil), job.Writes...), Implementation: s.build, Configuration: s.configuration(op.JobID), SourceState: candidate.Digest(), WeeklyRegistration: &candidate}
	r.seal = productDigest([]any{s.binding, r})
	return r, nil
}
func (s *Service) applyWeeklyRegistration(ctx context.Context, r ProductReview, confirm func(context.Context, ProductReview) error) (ProductJobStatus, error) {
	if confirm == nil {
		return ProductJobStatus{}, errors.New("trusted exact weekly registration review required")
	}
	exact, err := s.reviewWeeklyRegistration(r.Operation)
	if err != nil {
		return ProductJobStatus{}, err
	}
	if r.seal == "" || r.seal != exact.seal || productDigest(r) != productDigest(exact) {
		return ProductJobStatus{}, ErrProductConflict
	}
	if err := confirm(ctx, r); err != nil {
		return ProductJobStatus{}, err
	}
	if err := ctx.Err(); err != nil {
		return ProductJobStatus{}, err
	}
	exact, err = s.reviewWeeklyRegistration(r.Operation)
	if err != nil || r.seal != exact.seal || productDigest(r) != productDigest(exact) {
		return ProductJobStatus{}, ErrProductConflict
	}
	err = s.withControl(r.Operation.JobID, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		c, err := s.loadControl(r.Operation.JobID)
		if err != nil {
			return err
		}
		if c.Revision != r.Operation.ExpectedRevision {
			return ErrProductConflict
		}
		c.Revision++
		c.State = "inactive"
		c.OneShot = ""
		c.Qualification = nil
		c.Weekly = &weeklyModelControl{Registration: *exact.WeeklyRegistration, RegistrationReviewSHA256: r.seal}
		return s.saveControl(c)
	})
	if err != nil {
		return ProductJobStatus{}, err
	}
	c, err := s.loadControl(r.Operation.JobID)
	if err != nil {
		return ProductJobStatus{}, err
	}
	return ProductJobStatus{Scope: "owner", ScopeID: s.workspaceID, JobID: c.JobID, Revision: c.Revision, State: c.State, QualificationState: "unqualified", Reason: "registration_accepted_native_qualification_required"}, nil
}
func cloneWeeklyRegistration(candidate *WeeklyModelRegistration) (*WeeklyModelRegistration, error) {
	if candidate == nil {
		return nil, nil
	}
	if err := candidate.Validate(); err != nil {
		return nil, err
	}
	body, err := json.Marshal(candidate)
	if err != nil {
		return nil, err
	}
	var owned WeeklyModelRegistration
	if err := json.Unmarshal(body, &owned); err != nil {
		return nil, err
	}
	return &owned, nil
}

// weeklyExecutionPlan is private execution state selected only by the product
// owner after its current control, registration and qualification checks.
type weeklyExecutionPlan struct {
	committed                         *ownerctx.WeeklySelfWorking
	mode                              WeeklyModelMode
	registration                      WeeklyModelRegistration
	input                             ownerctx.WeeklySelfInput
	qualification, activation, intent string
}

func (p *weeklyExecutionPlan) binding(owner string) weeklyModelBinding {
	b := weeklyModelBinding{Mode: p.mode, OwnerScope: owner, DescriptorSHA256: p.registration.Descriptor.Digest(), InputSHA256: p.input.InputSHA256, PolicySHA256: p.input.PolicySHA256}
	if p.mode == WeeklyNativeProbe {
		b.TerminalOperation = "qualification_evidence_only"
		b.ProbeIntentSHA256 = p.intent
	} else {
		b.TerminalOperation = "working_projection_only"
		b.QualificationSHA256 = p.qualification
		b.ActivationSHA256 = p.activation
	}
	return b
}
