package maintenance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

var ErrProductConflict = errors.New("maintenance review or revision changed")
var ErrQualificationRequired = errors.New("current actual maintenance qualification required")

// ServiceOptions is a native constructor boundary, never a tool schema.
// Catalog must come from bundles/base/runtime.Maintenance (which imports this package).
type ServiceOptions struct {
	SourceAdmission                                updateservice.SyncedAdmission            `json:"-"`
	WeeklyNativeExecutor                           WeeklyNativeExecutor                     `json:"-"`
	WeeklyCandidate                                *WeeklyModelRegistration                 `json:"-"`
	DarwinFactory                                  func(DarwinScope) (DarwinBinding, error) `json:"-"`
	ProductRoot, UserHome, WorkspacePath           string
	LookupEnv                                      func(string) (string, bool)
	Catalog                                        Catalog
	CLIVersion                                     string
	AuthorityRegistryPath, AuthorityRegistrySHA256 string
	Adapter                                        string
	// Now is an optional trusted constructor clock, never tool/CLI input.
	Now func() time.Time `json:"-"`
	// InstalledGuard comes from the fixed product binding. It is not a grant
	// or a model input and cannot replace the live job/control/lease checks.
	InstalledGuard func(func() error) error `json:"-"`
	// EligibilityGuard is a trusted native veto; it cannot replace job authority.
	EligibilityGuard func() error `json:"-"`
}

type Service struct {
	updateScoped                                  bool
	ownerScoped                                   bool
	weeklyScoped                                  bool
	darwin                                        DarwinBinding
	options                                       ServiceOptions
	home, workspaceID, binding, executable, build string
	scheduler                                     scheduler.Store
	receipts                                      Store
	engine                                        *memory.Engine
	weekly                                        *memory.Engine
	now                                           func() time.Time
}

type Operation struct {
	Kind             string `json:"kind"`
	JobID            string `json:"job_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Timezone         string `json:"timezone,omitempty"`
}

type ProductSchedule struct {
	EventOnly     bool              `json:"event_only,omitempty"`
	Timezone      string            `json:"timezone"`
	Cadence       scheduler.Cadence `json:"cadence"`
	IntervalHours int               `json:"interval_hours"`
	Weekday       time.Weekday      `json:"weekday,omitempty"`
	LocalHour     int               `json:"local_hour,omitempty"`
	LocalMinute   int               `json:"local_minute,omitempty"`
	MaxCatchUp    int               `json:"max_catch_up"`
}

// ProductReview contains metadata only. Its private binding rejects edited or
// serialized reviews; trusted confirmation is still required for every Apply.
type ProductReview struct {
	WeeklyRegistration *WeeklyModelRegistration    `json:"weekly_registration,omitempty"`
	Scope              string                      `json:"scope,omitempty"`
	DarwinReview       *DarwinReviewProjection     `json:"darwin_review,omitempty"`
	Operation          Operation                   `json:"operation"`
	WorkspaceID        string                      `json:"workspace_id"`
	Schedule           ProductSchedule             `json:"schedule"`
	Writes             []string                    `json:"writes"`
	QualificationID    string                      `json:"qualification_id,omitempty"`
	Implementation     string                      `json:"implementation"`
	Configuration      string                      `json:"configuration"`
	SourceState        string                      `json:"source_state,omitempty"`
	SourceReview       *updateservice.SyncedReview `json:"source_review,omitempty"`
	seal               string
}

type ProductQualification struct {
	EventID        string             `json:"event_id,omitempty"`
	ID             string             `json:"id"`
	WorkspaceID    string             `json:"workspace_id"`
	JobID          string             `json:"job_id"`
	Binding        string             `json:"binding"`
	Implementation string             `json:"implementation"`
	Configuration  string             `json:"configuration"`
	Adapter        string             `json:"adapter"`
	Platform       string             `json:"platform"`
	Artifact       string             `json:"artifact"`
	ArtifactSHA256 string             `json:"artifact_sha256"`
	Receipt        Receipt            `json:"receipt"`
	ScheduledFor   time.Time          `json:"scheduled_for"`
	RequestedAt    time.Time          `json:"requested_at"`
	VerifiedAt     time.Time          `json:"verified_at"`
	DailyInput     *memory.DailyInput `json:"daily_input,omitempty"`
}

type ProductJobStatus struct {
	Scope              string                `json:"scope,omitempty"`
	ScopeID            string                `json:"scope_id,omitempty"`
	Diagnosis          *DarwinSource         `json:"diagnosis,omitempty"`
	JobID              string                `json:"job_id"`
	Revision           uint64                `json:"revision"`
	State              string                `json:"state"`
	Schedule           ProductSchedule       `json:"schedule"`
	Boundary           time.Time             `json:"no_backfill_before,omitempty"`
	Writes             []string              `json:"writes"`
	Qualification      *ProductQualification `json:"qualification,omitempty"`
	QualificationState string                `json:"qualification_state"`
	Reason             string                `json:"reason,omitempty"`
}

type ProductStatus struct {
	WorkspaceID string             `json:"workspace_id"`
	Jobs        []ProductJobStatus `json:"jobs"`
}

type productControl struct {
	Weekly        *weeklyModelControl   `json:"weekly_model,omitempty"`
	Writes        []string              `json:"writes,omitempty"`
	SchemaVersion int                   `json:"schema_version"`
	WorkspaceID   string                `json:"workspace_id"`
	Binding       string                `json:"binding"`
	JobID         string                `json:"job_id"`
	Revision      uint64                `json:"revision"`
	State         string                `json:"state"`
	Schedule      ProductSchedule       `json:"schedule"`
	Boundary      time.Time             `json:"boundary"`
	Qualification *ProductQualification `json:"qualification,omitempty"`
	OneShot       string                `json:"one_shot,omitempty"`
}

// NewProductMemoryEngine shares the embedded configuration and attestor with
// product memory operations. Construction never prepares capture authority.
func NewProductMemoryEngine(home string) (*memory.Engine, basememory.RuntimeConfig, error) {
	policy, err := basememory.Policy()
	if err != nil {
		return nil, basememory.RuntimeConfig{}, err
	}
	cfg, err := basememory.Runtime()
	if err != nil {
		return nil, cfg, err
	}
	root := filepath.Join(home, "brain", "memory")
	budgets := cfg.ContextBudgets()
	budgets["L1"] = cfg.L1MaxRunes
	daily := memory.DeterministicL1Synthesizer{MaxRunes: cfg.L1MaxRunes, MaxEntries: cfg.L1MaxEntries, MaxInputBytes: cfg.L1MaxInputBytes, MaxInputEntries: cfg.L1MaxInputEntries, Attestor: memory.CaptureAttestor{Root: root}}
	return &memory.Engine{Root: root, Policy: policy, Budgets: budgets, MaxSourceBytes: cfg.L1MaxInputBytes, Synthesizer: daily, SynthesizerID: memory.DeterministicL1SynthesizerID, Now: time.Now}, cfg, nil
}

func NewService(options ServiceOptions) (*Service, error) {
	if options.Adapter != "code" && options.Adapter != "desktop" && options.Adapter != "local" {
		return nil, errors.New("maintenance adapter must be code, desktop or local")
	}
	if err := options.Catalog.Validate(); err != nil {
		return nil, err
	}
	// Own the catalog bytes rather than retaining a caller-mutable slice.
	body, err := json.Marshal(options.Catalog)
	if err != nil {
		return nil, err
	}
	var ownedCatalog Catalog
	if err := json.Unmarshal(body, &ownedCatalog); err != nil {
		return nil, err
	}
	options.Catalog = ownedCatalog
	options.WeeklyCandidate, err = cloneWeeklyRegistration(options.WeeklyCandidate)
	if err != nil {
		return nil, err
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	s := &Service{options: options, now: now}
	if err := s.validate(); err != nil {
		return nil, err
	}
	s.scheduler = scheduler.Store{Root: filepath.Join(s.home, "state", "scheduler")}
	s.receipts = Store{Root: filepath.Join(s.home, "state", "maintenance")}
	s.engine, _, err = NewProductMemoryEngine(s.home)
	if err == nil {
		s.engine.Now = now
	}
	if err != nil {
		return nil, err
	}
	s.weekly, err = NewProductWeeklyMemoryEngine(s.home)
	if err == nil {
		s.weekly.Now = now
	}
	if err == nil {
		s.darwin, err = s.newDarwinBinding()
	}
	return s, err
}

func (s *Service) validate() error {
	h, err := datahome.Resolve(datahome.Options{ProductRoot: s.options.ProductRoot, UserHome: s.options.UserHome, LookupEnv: s.options.LookupEnv})
	if err != nil || !h.Writable {
		return errors.New("maintenance home unavailable")
	}
	path := s.options.WorkspacePath
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
		return errors.New("maintenance workspace unavailable")
	}
	for _, root := range []string{h.Home, s.options.ProductRoot} {
		for _, pair := range [][2]string{{root, path}, {path, root}} {
			rel, err := filepath.Rel(strings.ToLower(pair[0]), strings.ToLower(pair[1]))
			if err != nil || rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return errors.New("maintenance binding overlaps")
			}
		}
	}
	stateRoot := filepath.Join(h.Home, "state")
	if err := workspace.PreflightInitialize(workspace.Options{WorkspacePath: path, DataRoot: stateRoot}); err != nil {
		return err
	}
	w, err := workspace.Inspect(path, stateRoot)
	if err != nil || w.MetadataStatus != "valid" || w.State != "ready" {
		return errors.New("maintenance workspace unavailable")
	}
	reference, err := workspace.ResolveReference(w.WorkspaceID, stateRoot)
	if err != nil || reference != path {
		return errors.New("maintenance workspace registration changed")
	}
	identity, err := processidentity.Current()
	if err != nil {
		return err
	}
	executable, build := identity.Path, identity.SHA256
	binding := productDigest([]string{h.Home, path, w.WorkspaceID})
	scopeID := w.WorkspaceID
	if s.ownerScoped || s.weeklyScoped {
		scopeID, err = ownerctx.SelfObservationOwnerScope(filepath.Join(h.Home, "brain"))
		if err != nil {
			return err
		}
		binding = productDigest([]string{"owner-self-observation-v1", scopeID})
		if s.weeklyScoped {
			binding = productDigest([]string{"owner-weekly-model-v1", scopeID})
		}
	}
	if s.updateScoped {
		scopeID, binding = updateAvailabilityScope(h.Home, s.options.ProductRoot)
	}
	if s.binding != "" && (s.binding != binding || s.build != build || s.executable != executable) {
		return ErrProductConflict
	}
	if s.binding != "" {
		return nil
	}
	s.home, s.workspaceID, s.binding, s.executable, s.build = h.Home, scopeID, binding, executable, build
	return nil
}

func productDigest(value any) string {
	body, _ := json.Marshal(value)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func productJob(jobID string) bool {
	return jobID == YodaSelfReviewWeeklyJobID || jobID == SelfObservationCaptureJobID || DarwinJob(jobID) || jobID == MemoryCaptureJobID || jobID == MemoryLightDreamJobID || jobID == MemoryCheckpointJobID || jobID == MemoryDeepDreamJobID || jobID == "skills-index-refresh" || diagnosticJob(jobID)
}
func (s *Service) configuration(jobID string) string {
	job, _ := findCatalogJob(s.options.Catalog, jobID)
	policy, _ := basememory.Policy()
	cfg, _ := basememory.Runtime()
	return productDigest([]any{"product-maintenance-v1", job, policy, cfg, memory.DeterministicL1SynthesizerID, memory.DeterministicWeeklySynthesizerID, s.options.CLIVersion})
}
func (s *Service) controlPath(jobID string) string {
	return filepath.Join(s.receipts.Root, "workspaces", s.workspaceID, "product-control", jobID, "current.json")
}
func (s *Service) loadControl(jobID string) (productControl, error) {
	var c productControl
	body, err := scheduler.ReadPrivateStateFile(s.controlPath(jobID), 64<<10)
	if errors.Is(err, os.ErrNotExist) {
		return productControl{SchemaVersion: 1, WorkspaceID: s.workspaceID, Binding: s.binding, JobID: jobID, State: "inactive"}, nil
	}
	if err != nil {
		return c, err
	}
	if err := decodeProduct(body, &c); err != nil {
		return c, err
	}
	if c.SchemaVersion != 1 || c.WorkspaceID != s.workspaceID || c.Binding != s.binding || c.JobID != jobID || c.Revision == 0 {
		return c, ErrProductConflict
	}
	if c.Weekly != nil && (jobID != YodaSelfReviewWeeklyJobID || c.Weekly.Registration.Validate() != nil || !digestPattern.MatchString(c.Weekly.RegistrationReviewSHA256)) {
		return c, ErrProductConflict
	}
	if c.Qualification != nil && (c.Qualification.JobID != c.JobID || c.Qualification.WorkspaceID != c.WorkspaceID || c.Qualification.Binding != c.Binding) {
		return c, ErrProductConflict
	}
	for _, write := range c.Writes {
		if !validWrite(write) {
			return c, ErrProductConflict
		}
	}
	if c.State != "inactive" && c.State != "active" && c.State != "paused" && c.State != "revoked" {
		return c, ErrProductConflict
	}
	if c.State == "active" || c.State == "paused" {
		expectedSchedule := scheduleFor(c.JobID)
		expectedSchedule.Timezone = c.Schedule.Timezone
		if len(c.Writes) == 0 || c.Boundary.IsZero() || c.Schedule != expectedSchedule {
			return c, ErrProductConflict
		}
		if _, err := time.LoadLocation(c.Schedule.Timezone); err != nil {
			return c, err
		}
	}
	return c, nil
}
func decodeProduct(body []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("invalid trailing maintenance state")
	}
	return nil
}
func (s *Service) saveControl(c productControl) error {
	body, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return scheduler.WritePrivateFileAtomic(s.controlPath(c.JobID), body)
}
func (s *Service) withControl(jobID string, fn func() error) error {
	return scheduler.WithPrivateLock(filepath.Join(filepath.Dir(s.controlPath(jobID)), "control.guard"), fn)
}

func (s *Service) Status() (ProductStatus, error) {
	if err := s.validate(); err != nil {
		return ProductStatus{}, err
	}
	report := ProductStatus{WorkspaceID: s.workspaceID}
	for _, job := range s.options.Catalog.Jobs {
		if s.updateScoped && job.ID != UpdateAvailabilityJobID || s.weeklyScoped && job.ID != YodaSelfReviewWeeklyJobID || s.ownerScoped && job.ID != SelfObservationCaptureJobID {
			continue
		}
		service := s
		if job.ID == YodaSelfReviewWeeklyJobID {
			service = s.weeklyModelService()
		}
		if job.ID == SelfObservationCaptureJobID {
			service = s.selfObservationService()
		}
		if job.ID == UpdateAvailabilityJobID {
			service = s.updateAvailabilityService()
		}
		c, err := service.loadControl(job.ID)
		if err != nil {
			return report, err
		}
		item := ProductJobStatus{JobID: job.ID, Revision: c.Revision, State: c.State, Schedule: c.Schedule, Boundary: c.Boundary, Writes: append([]string(nil), job.Writes...), Qualification: c.Qualification, QualificationState: "unqualified"}
		if len(c.Writes) > 0 {
			item.Writes = append([]string(nil), c.Writes...)
		}
		if !productJob(job.ID) || DarwinJob(job.ID) && s.darwin == nil || job.ID == YodaSelfReviewWeeklyJobID && (service.options.WeeklyNativeExecutor == nil || c.Weekly == nil) {
			item.Reason = "production_binding_unavailable"
		} else if c.Qualification != nil {
			if err := service.verifyQualification(c.Qualification); err == nil {
				item.QualificationState = "qualified"
			} else {
				item.QualificationState = "stale"
				item.Reason = "qualification_requires_refresh"
			}
		}
		if job.ID == SelfObservationCaptureJobID || job.ID == UpdateAvailabilityJobID || job.ID == YodaSelfReviewWeeklyJobID {
			item.Scope = "owner"
			item.ScopeID = service.workspaceID
		}
		if DarwinJob(job.ID) {
			source := DarwinSource{CacheState: "unavailable"}
			if s.darwin != nil {
				var diagnosisErr error
				source, diagnosisErr = s.darwinSource(job.ID)
				if diagnosisErr != nil {
					if c.Qualification != nil {
						item.QualificationState = "stale"
					}
					item.Reason = "diagnosis_" + source.CacheState
				}
				if job.ID == DarwinDailyJobID && source.CacheState == "stale" {
					item.Reason = "manual_refresh_required"
				}
			}
			item.Diagnosis = &source
		}
		report.Jobs = append(report.Jobs, item)
	}
	return report, nil
}

func (s *Service) Review(op Operation) (ProductReview, error) {
	if op.JobID == UpdateAvailabilityJobID && !s.updateScoped {
		return s.updateAvailabilityService().Review(op)
	}
	if op.JobID == YodaSelfReviewWeeklyJobID && op.Kind == "register" {
		return s.weeklyModelService().reviewWeeklyRegistration(op)
	}
	if op.JobID == YodaSelfReviewWeeklyJobID && !s.weeklyScoped {
		return s.weeklyModelService().Review(op)
	}
	if op.JobID == SelfObservationCaptureJobID && !s.ownerScoped {
		return s.selfObservationService().Review(op)
	}
	if err := s.validate(); err != nil {
		return ProductReview{}, err
	}
	if !productJob(op.JobID) {
		return ProductReview{}, scheduler.ErrCapabilityUnavailable
	}
	if DarwinJob(op.JobID) {
		// Denial transitions remain usable when the optional cache is damaged.
		if op.Kind == "qualify" || op.Kind == "enroll" || op.Kind == "resume" {
			source, err := s.darwinSource(op.JobID)
			if err != nil {
				return ProductReview{}, err
			}
			if op.JobID == DarwinDailyJobID && source.CacheState == "stale" && op.Kind != "qualify" {
				return ProductReview{}, ErrDarwinRepairUnavailable
			}
		}
		if op.JobID == DarwinMonthlyJobID && op.Kind != "qualify" {
			return ProductReview{}, errors.New("Darwin monthly proposal is attended-only")
		}
	}
	if op.Kind != "qualify" && op.Kind != "enroll" && op.Kind != "pause" && op.Kind != "resume" && op.Kind != "revoke" {
		return ProductReview{}, errors.New("unknown maintenance operation")
	}
	if op.JobID == UpdateCheckJobID && op.Kind != "qualify" {
		return ProductReview{}, errors.New("update-check is attended-only; recurring enrollment and activation are unavailable")
	}
	c, err := s.loadControl(op.JobID)
	if err != nil {
		return ProductReview{}, err
	}
	if c.Revision != op.ExpectedRevision {
		return ProductReview{}, ErrProductConflict
	}
	schedule := c.Schedule
	if op.Kind == "enroll" {
		if c.State == "active" || c.State == "paused" {
			return ProductReview{}, ErrProductConflict
		}
		if _, err := time.LoadLocation(op.Timezone); err != nil || op.Timezone == "" {
			return ProductReview{}, errors.New("exact enrollment timezone required")
		}
		schedule = scheduleFor(op.JobID)
		schedule.Timezone = op.Timezone
	} else if op.Timezone != "" {
		return ProductReview{}, errors.New("timezone belongs only to enrollment review")
	}
	if op.Kind == "pause" && c.State != "active" || op.Kind == "resume" && c.State != "paused" {
		return ProductReview{}, ErrProductConflict
	}
	if op.Kind == "enroll" || op.Kind == "resume" {
		if err := s.verifyQualification(c.Qualification); err != nil {
			return ProductReview{}, err
		}
	}
	job, _ := findCatalogJob(s.options.Catalog, op.JobID)
	r := ProductReview{Operation: op, WorkspaceID: s.workspaceID, Schedule: schedule, Writes: append([]string(nil), job.Writes...), Implementation: s.build, Configuration: s.configuration(op.JobID)}
	if op.JobID == SelfObservationCaptureJobID || op.JobID == YodaSelfReviewWeeklyJobID {
		r.Scope = "owner"
	}
	if op.JobID == YodaSelfReviewWeeklyJobID && c.Weekly != nil {
		r.WeeklyRegistration, err = cloneWeeklyRegistration(&c.Weekly.Registration)
		if err != nil {
			return ProductReview{}, err
		}
	}

	if c.Qualification != nil {
		r.QualificationID = c.Qualification.ID
	}
	if op.Kind == "qualify" || op.Kind == "enroll" {
		if DarwinJob(op.JobID) {
			source, e := s.darwinSource(op.JobID)
			if e != nil {
				return ProductReview{}, e
			}
			effect := "assess"
			if op.Kind == "qualify" && op.JobID != DarwinMonthlyJobID {
				if source.CacheState == "absent" {
					effect = "initialize"
				} else if source.CacheState == "stale" && op.JobID == DarwinDailyJobID {
					effect = "manual_refresh_required"
				}
			}
			r.SourceState = productDigest(source)
			r.DarwinReview = &DarwinReviewProjection{CacheState: source.CacheState, Parent: source.Parent, ExternalSHA256: source.ExternalSHA256, Effect: effect}
		} else {
			r.SourceState, err = s.sourceState(op.JobID)
		}
		if err != nil {
			return ProductReview{}, err
		}
	}
	if op.JobID == UpdateCheckJobID {
		review, e := s.updateReview()
		if e != nil {
			return ProductReview{}, e
		}
		r.SourceReview = &review
	}
	r.seal = productDigest([]any{s.binding, r})
	return r, nil
}

// Apply invokes the trusted host's exact review before any write, then repeats
// read-only review and CAS. Returning an error from confirm leaves no state.
func (s *Service) Apply(ctx context.Context, r ProductReview, confirm func(context.Context, ProductReview) error) (ProductJobStatus, error) {
	if r.Operation.JobID == UpdateAvailabilityJobID && !s.updateScoped {
		return s.updateAvailabilityService().Apply(ctx, r, confirm)
	}
	if r.Operation.JobID == YodaSelfReviewWeeklyJobID && r.Operation.Kind == "register" {
		return s.weeklyModelService().applyWeeklyRegistration(ctx, r, confirm)
	}
	if r.Operation.JobID == YodaSelfReviewWeeklyJobID && !s.weeklyScoped {
		return s.weeklyModelService().Apply(ctx, r, confirm)
	}
	if r.Operation.JobID == SelfObservationCaptureJobID && !s.ownerScoped {
		return s.selfObservationService().Apply(ctx, r, confirm)
	}
	if confirm == nil {
		return ProductJobStatus{}, errors.New("trusted maintenance review required")
	}
	exact, err := s.Review(r.Operation)
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
	exact, err = s.Review(r.Operation)
	if err != nil || r.seal != exact.seal || productDigest(r) != productDigest(exact) {
		return ProductJobStatus{}, ErrProductConflict
	}
	if r.Operation.Kind == "qualify" {
		if DarwinJob(r.Operation.JobID) {
			return s.qualifyDarwin(ctx, r)
		}
		return s.qualify(ctx, r)
	}
	err = s.withControl(r.Operation.JobID, func() error {
		c, err := s.loadControl(r.Operation.JobID)
		if err != nil {
			return err
		}
		if c.Revision != r.Operation.ExpectedRevision {
			return ErrProductConflict
		}
		c.Revision++
		c.OneShot = ""
		switch r.Operation.Kind {
		case "enroll":
			c.State = "active"
			c.Schedule = r.Schedule
			c.Writes = append([]string(nil), r.Writes...)
			c.Boundary = s.now().UTC()
		case "pause":
			c.State = "paused"
		case "resume":
			c.State = "active"
		case "revoke":
			c.State = "revoked"
		}
		return s.saveControl(c)
	})
	if err != nil {
		return ProductJobStatus{}, err
	}
	return s.jobStatus(r.Operation.JobID)
}
func (s *Service) jobStatus(id string) (ProductJobStatus, error) {
	status, err := s.Status()
	if err != nil {
		return ProductJobStatus{}, err
	}
	for _, job := range status.Jobs {
		if job.JobID == id {
			return job, nil
		}
	}
	return ProductJobStatus{}, scheduler.ErrCapabilityUnavailable
}

// Existing memory activation lock → control guard → occurrence lease guard →
// visible publication. Control transitions never acquire memory or lease locks.
type productExecution struct {
	weeklyPlan   *weeklyExecutionPlan
	ctx          context.Context
	darwinSource *DarwinSource
	service      *Service
	jobID        string
	revision     uint64
	oneShot      string
	source       string
	dailyInput   *memory.DailyInput
}

func (p *productExecution) selectDailyInput() error {
	if p.jobID != MemoryLightDreamJobID {
		return nil
	}
	inputs, err := p.service.engine.DailyInputCohort(p.service.workspaceID)
	if err != nil {
		return err
	}
	if productDigest(inputs) != p.source {
		return ErrProductConflict
	}
	p.dailyInput, err = p.service.engine.PendingDailyInput(p.service.workspaceID, inputs)
	return err
}

func (p *productExecution) evidenceAt(at time.Time) time.Time {
	if p.dailyInput != nil {
		day, _ := time.Parse("2006-01-02", p.dailyInput.Period)
		return day
	}
	return at
}

func (p *productExecution) guard(publish func() error) error {
	return p.guardWithControl(p.ctx, publish, func(action func() error) error {
		return p.service.withControl(p.jobID, action)
	})
}

func (p *productExecution) guardWithin(ctx, acquisitionCtx context.Context, publish func() error) error {
	return p.guardWithControl(ctx, publish, func(action func() error) error {
		return scheduler.WithPrivateLockContext(acquisitionCtx, filepath.Join(filepath.Dir(p.service.controlPath(p.jobID)), "control.guard"), action)
	})
}

func (p *productExecution) guardWithControl(ctx context.Context, publish func() error, withControl func(func() error) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.ctx != nil && p.ctx.Err() != nil {
		return p.ctx.Err()
	}
	if p.service.options.EligibilityGuard != nil {
		if err := p.service.options.EligibilityGuard(); err != nil {
			return err
		}
	}
	// Refuse before touching control state, then recheck after acquiring its lock.
	if _, err := processidentity.Current(); err != nil {
		return err
	}
	return withControl(func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if p.ctx != nil && p.ctx.Err() != nil {
			return p.ctx.Err()
		}
		if _, err := processidentity.Current(); err != nil {
			return err
		}
		c, err := p.service.loadControl(p.jobID)
		if err != nil {
			return err
		}
		if c.Revision != p.revision || p.oneShot != "" && c.OneShot != p.oneShot || p.oneShot == "" && c.State != "active" {
			return ErrProductConflict
		}
		if p.weeklyPlan != nil {
			if p.weeklyPlan.committed != nil {
				if err := p.validateWeeklyCommitted(); err != nil {
					return err
				}
			} else {
				if err := ownerctx.ValidateWeeklySelfInputCurrent(ctx, filepath.Join(p.service.home, "brain"), p.weeklyPlan.input, p.service.now().UTC()); err != nil {
					return err
				}
			}
			if c.Weekly == nil || c.Weekly.Registration.Digest() != p.weeklyPlan.registration.Digest() {
				return ErrProductConflict
			}
			if p.weeklyPlan.mode == WeeklyWorking {
				if c.Qualification == nil || c.Qualification.ID != p.weeklyPlan.qualification || p.service.verifyQualification(c.Qualification) != nil {
					return ErrQualificationRequired
				}
			}
		}
		if p.darwinSource != nil {
			current, err := p.service.darwinSource(p.jobID)
			if err != nil || productDigest(current) != productDigest(*p.darwinSource) {
				return ErrProductConflict
			}
		}
		if (p.jobID == UpdateAvailabilityJobID || p.jobID == SelfObservationCaptureJobID || wikiJob(p.jobID) || p.jobID == UpdateCheckJobID || p.jobID == MemoryCaptureJobID || p.jobID == MemoryLightDreamJobID) && p.source != "" {
			current, err := p.service.sourceState(p.jobID)
			if err != nil || current != p.source {
				return ErrProductConflict
			}
		}
		// The update owner acquires installation serialization itself. Holding
		// that lock here would recursively lock ObserveSource/activation. The
		// exact current source snapshot above and canonical CAS fence this job.
		guardedPublish := func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			// Installation serialization can wait after the control-fence check.
			if p.ctx != nil && p.ctx.Err() != nil {
				return p.ctx.Err()
			}
			return publish()
		}
		if p.jobID != UpdateCheckJobID && p.service.options.InstalledGuard != nil {
			return p.service.options.InstalledGuard(guardedPublish)
		}
		return guardedPublish()
	})
}
func (s *Service) worker(p *productExecution) Worker {
	schedule := scheduleFor(p.jobID)
	weekly := *s.weekly
	weekly.Now = s.now
	handlers := map[string]any{
		SelfObservationCaptureJobID: productSelfObservationHandler{service: s, expected: p.source},
		MemoryCaptureJobID:          productCaptureHandler{service: s, expected: p.source},
		MemoryLightDreamJobID:       MemoryLightDreamHandler{Engine: s.engine, ProductSelection: p.source != "", SelectedInput: p.dailyInput},
		MemoryCheckpointJobID:       MemoryCheckpointHandler{Scheduler: s.scheduler, Store: ContinuityCheckpointStore{Root: s.receipts.Root}, ExpectedSourceFingerprint: p.source},
		MemoryDeepDreamJobID:        MemoryDeepDreamHandler{Engine: &weekly, ExpectedSourceFingerprint: p.source},
		"skills-index-refresh":      productIndexHandler{service: s, expected: p.source},
	}
	for _, job := range []string{"memory-retention-check", "runtime-health-check", "capability-recheck", "runtime-drift-check"} {
		handlers[job] = productDiagnosticHandler{service: s, expected: p.source, job: job}
	}
	for _, job := range []string{WikiIncrementalJobID, WikiReconcileJobID, WikiIntegrityJobID} {
		handlers[job] = productWikiHandler{service: s, expected: p.source, job: job}
	}
	if p.weeklyPlan != nil {
		handlers[YodaSelfReviewWeeklyJobID] = productWeeklyModelHandler{service: s, plan: p.weeklyPlan}
	}
	handlers[UpdateCheckJobID] = productUpdateHandler{service: s, expected: p.source, execution: p}
	handlers[UpdateAvailabilityJobID] = productAvailabilityHandler{service: s, expected: p.source, qualification: p.oneShot != ""}
	if DarwinJob(p.jobID) && s.darwin != nil && p.darwinSource != nil {
		handlers[p.jobID] = s.darwin.Handler(p.jobID, p.darwinSource, p.oneShot != "")
	}
	return Worker{Catalog: s.options.Catalog, Scheduler: s.scheduler, Receipts: s.receipts, Jobs: []scheduler.Job{{ID: p.jobID, Cadence: schedule.Cadence, Weekday: schedule.Weekday, LocalHour: schedule.LocalHour, LocalMinute: schedule.LocalMinute, IntervalHours: schedule.IntervalHours, MaxCatchUp: schedule.MaxCatchUp}}, Handlers: handlers, Deadline: 2 * time.Minute, Now: s.now, product: p}
}

func (s *Service) execute(ctx context.Context, p *productExecution, occ scheduler.Occurrence, now time.Time) (Receipt, error) {
	p.ctx = ctx
	if p.jobID == YodaSelfReviewWeeklyJobID {
		if err := p.selectWeekly(ctx, occ, now); err != nil {
			return Receipt{}, err
		}
	}
	if err := p.guard(func() error { return nil }); err != nil {
		return Receipt{}, err
	}
	if p.weeklyPlan != nil && p.weeklyPlan.committed != nil {
		return s.reconcileWeeklyCommitted(ctx, p, occ, now)
	}
	authority := ExecutionAuthority{catalog: s.options.Catalog, occurrences: map[string]OccurrenceAuthorization{}, activated: map[string]bool{p.jobID: true}, attended: p.oneShot != "", preauthorized: p.oneShot == "", ready: true, productBound: true}
	trigger := TriggerPresence
	if p.jobID == DarwinMonthlyJobID {
		trigger = TriggerMonthly
	}
	if p.jobID == YodaSelfReviewWeeklyJobID {
		trigger = TriggerWeekly
	}
	if occ.EventID != "" {
		trigger = TriggerEvent
	}
	if err := authority.addOccurrence(OccurrenceAuthorization{WorkspaceID: s.workspaceID, JobID: p.jobID, Trigger: trigger, EventID: occ.EventID, ScheduledFor: occ.ScheduledFor}); err != nil {
		return Receipt{}, err
	}
	request := WakeRequest{WorkspaceID: s.workspaceID, Trigger: trigger, EventID: occ.EventID, Now: now, OwnerID: fmt.Sprintf("product-%d", os.Getpid()), Attended: p.oneShot != "", Preauthorized: p.oneShot == "", IdleState: IdleConfirmed}
	receipt, err := s.worker(p).runOccurrence(ctx, request, now, occ, authority)
	if err != nil {
		return rejectedTerminalReceipt(receipt), err
	}
	if receipt.State == ReceiptSucceeded || receipt.State == ReceiptReviewedNoChange || receipt.State == ReceiptProposalEmitted {
		if occ.EventID != "" {
			if err := s.receipts.VerifyEventReceipt(receipt); err != nil {
				return rejectedTerminalReceipt(receipt), err
			}
			return receipt, nil
		}
		// A maintenance receipt alone cannot conceal a failed scheduler publication.
		receipts, readErr := s.scheduler.Receipts(s.workspaceID)
		if readErr != nil {
			return rejectedTerminalReceipt(receipt), readErr
		}
		for _, durable := range receipts {
			if durable.JobID == p.jobID && durable.State == scheduler.Succeeded && durable.ScheduledFor.Equal(occ.ScheduledFor) && durable.AttemptedAt.Equal(receipt.RecordedAt) {
				return receipt, nil
			}
		}
		return rejectedTerminalReceipt(receipt), errors.New("maintenance domain/receipt publication is partial")
	}
	return receipt, nil
}

func (s *Service) RunDue(ctx context.Context) (WakeReport, error) {
	if err := s.validate(); err != nil {
		return WakeReport{}, err
	}
	report := WakeReport{SchemaVersion: 1, WorkspaceID: s.workspaceID, State: "no_due_work"}
	now := s.now().UTC()

	for _, job := range s.options.Catalog.Jobs {
		if s.updateScoped && job.ID != UpdateAvailabilityJobID || s.weeklyScoped && job.ID != YodaSelfReviewWeeklyJobID || s.ownerScoped && job.ID != SelfObservationCaptureJobID {
			continue
		}
		if job.ID == UpdateAvailabilityJobID && !s.updateScoped {
			// The serve-owned adapter runs this read-only-source job separately
			// while active; its ordinary background jobs retain native idle gating.
			if s.options.EligibilityGuard != nil {
				continue
			}
			ownerReport, err := s.updateAvailabilityService().RunDue(ctx)
			report.Due = append(report.Due, ownerReport.Due...)
			report.Receipts = append(report.Receipts, ownerReport.Receipts...)
			if ownerReport.State != "no_due_work" {
				report.State = ownerReport.State
			}
			if err != nil {
				return report, err
			}
			continue
		}
		if job.ID == YodaSelfReviewWeeklyJobID && !s.weeklyScoped {
			ownerReport, err := s.weeklyModelService().RunDue(ctx)
			report.Due = append(report.Due, ownerReport.Due...)
			report.Receipts = append(report.Receipts, ownerReport.Receipts...)
			if ownerReport.State != "no_due_work" {
				report.State = ownerReport.State
			}
			if err != nil {
				return report, err
			}
			continue
		}
		if job.ID == SelfObservationCaptureJobID && !s.ownerScoped {
			ownerReport, err := s.selfObservationService().RunDue(ctx)
			report.Due = append(report.Due, ownerReport.Due...)
			report.Receipts = append(report.Receipts, ownerReport.Receipts...)
			if ownerReport.State != "no_due_work" {
				report.State = ownerReport.State
			}
			if err != nil {
				return report, err
			}
			continue
		}
		if !productJob(job.ID) {
			continue
		}
		c, err := s.loadControl(job.ID)
		if err != nil {
			return report, err
		}
		if c.State != "active" {
			continue
		}
		if err := s.verifyQualification(c.Qualification); err != nil {
			return report, err
		}
		if c.Schedule.EventOnly {
			snapshot, err := s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
			if err != nil {
				return report, err
			}
			if snapshot.EventID == "" || snapshot.EventAt.Before(c.Boundary) {
				continue
			}
			closed, err := s.receipts.EventClosed(s.workspaceID, job.ID, snapshot.EventID)
			if err != nil {
				report.State = "completed_with_failures"
				return report, err
			}
			if closed {
				continue
			}
			occ := scheduler.Occurrence{JobID: job.ID, EventID: snapshot.EventID, ScheduledFor: snapshot.EventAt}
			report.Due = append(report.Due, occ)
			receipt, err := s.execute(ctx, &productExecution{service: s, jobID: job.ID, revision: c.Revision, source: snapshot.SourceIdentity}, occ, now)
			report.Receipts = append(report.Receipts, receipt)
			report.State = "processed"
			if err != nil {
				report.State = "completed_with_failures"
				return report, err
			}
			continue
		}
		receipts, err := s.scheduler.Receipts(s.workspaceID)
		if err != nil {
			return report, err
		}
		filtered := receipts[:0]
		for _, r := range receipts {
			if r.JobID == job.ID && !r.ScheduledFor.Before(c.Boundary) && !r.AttemptedAt.Before(c.Boundary) {
				filtered = append(filtered, r)
			}
		}
		location, err := time.LoadLocation(c.Schedule.Timezone)
		if err != nil {
			return report, err
		}
		boundary := c.Boundary
		if job.ID == UpdateAvailabilityJobID {
			// Availability describes the source now; missed calendar days do not
			// require historical observations on subsequent reconnects.
			local := now.In(location)
			today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).Add(-time.Nanosecond)
			if today.After(boundary) {
				boundary = today
			}
		}
		due, err := scheduler.PlanDue([]scheduler.Job{{ID: job.ID, Cadence: c.Schedule.Cadence, Weekday: c.Schedule.Weekday, LocalHour: c.Schedule.LocalHour, LocalMinute: c.Schedule.LocalMinute, IntervalHours: c.Schedule.IntervalHours, MaxCatchUp: c.Schedule.MaxCatchUp}}, boundary, filtered, now.In(location))
		if err != nil {
			return report, err
		}
		for _, occ := range due {
			report.Due = append(report.Due, occ)
			p := &productExecution{service: s, jobID: job.ID, revision: c.Revision}
			if DarwinJob(job.ID) {
				source, e := s.darwinSource(job.ID)
				if e != nil {
					return report, e
				}
				p.darwinSource = &source
			}
			if job.ID == UpdateAvailabilityJobID || job.ID == SelfObservationCaptureJobID || wikiJob(job.ID) || job.ID == MemoryCaptureJobID || job.ID == MemoryLightDreamJobID {
				p.source, err = s.sourceState(job.ID)
				if err != nil {
					return report, err
				}
			}
			if err := p.selectDailyInput(); err != nil {
				return report, err
			}
			receipt, err := s.execute(ctx, p, occ, now)
			report.Receipts = append(report.Receipts, receipt)
			report.State = "processed"
			if err != nil {
				report.State = "completed_with_failures"
				return report, err
			}
		}
	}
	return report, nil
}

func (s *Service) qualify(ctx context.Context, r ProductReview) (ProductJobStatus, error) {
	token, err := attemptID()
	if err != nil {
		return ProductJobStatus{}, err
	}
	var revision uint64
	err = s.withControl(r.Operation.JobID, func() error {
		c, err := s.loadControl(r.Operation.JobID)
		if err != nil {
			return err
		}
		if c.Revision != r.Operation.ExpectedRevision {
			return ErrProductConflict
		}
		c.Revision++
		c.OneShot = token
		revision = c.Revision
		return s.saveControl(c)
	})
	if err != nil {
		return ProductJobStatus{}, err
	}
	p := &productExecution{service: s, jobID: r.Operation.JobID, revision: revision, oneShot: token, source: r.SourceState}
	if err := p.selectDailyInput(); err != nil {
		return ProductJobStatus{}, err
	}
	now := s.now().UTC()
	before, _, beforeErr := s.domainEvidence(p.jobID, p.evidenceAt(now))
	if beforeErr != nil && !errors.Is(beforeErr, os.ErrNotExist) {
		return ProductJobStatus{}, beforeErr
	}
	occ := scheduler.Occurrence{JobID: p.jobID, ScheduledFor: now}
	if p.jobID == WikiIncrementalJobID {
		snapshot, inspectErr := s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
		if inspectErr != nil || snapshot.EventID == "" {
			return ProductJobStatus{}, ErrQualificationRequired
		}
		occ.EventID, occ.ScheduledFor = snapshot.EventID, snapshot.EventAt
	}
	receipt, err := s.execute(ctx, p, occ, now)
	if err != nil {
		return ProductJobStatus{}, err
	}
	if p.jobID == UpdateCheckJobID && receipt.State == ReceiptReviewedNoChange {
		// A genuine unchanged check may refresh custody and its exact pending
		// plan, but cannot mint qualification from timestamp-only ID churn.
		err := p.guard(func() error {
			c, e := s.loadControl(p.jobID)
			if e != nil {
				return e
			}
			c.Revision++
			c.OneShot = ""
			return s.saveControl(c)
		})
		if err != nil {
			return ProductJobStatus{}, err
		}
		status, err := s.jobStatus(p.jobID)
		status.Reason = "reviewed_no_change"
		return status, err
	}
	if receipt.State != ReceiptSucceeded {
		return ProductJobStatus{}, ErrQualificationRequired
	}
	after, path, err := s.domainEvidence(p.jobID, p.evidenceAt(now))
	if err != nil {
		return ProductJobStatus{}, err
	}
	if before == after || after == "" {
		return ProductJobStatus{}, ErrQualificationRequired
	}
	q := ProductQualification{EventID: occ.EventID, WorkspaceID: s.workspaceID, JobID: p.jobID, Binding: s.binding, Implementation: s.build, Configuration: s.configuration(p.jobID), Adapter: s.options.Adapter, Platform: runtime.GOOS + "/" + runtime.GOARCH, Artifact: path, ArtifactSHA256: after, Receipt: receipt, ScheduledFor: occ.ScheduledFor, RequestedAt: now, VerifiedAt: s.now().UTC()}
	if p.dailyInput != nil {
		inputs, err := s.engine.DailyInputCohort(s.workspaceID)
		if err != nil || productDigest(inputs) != p.source {
			return ProductJobStatus{}, ErrProductConflict
		}
		found := false
		for _, input := range inputs {
			if input == *p.dailyInput {
				found = true
				break
			}
		}
		if !found {
			return ProductJobStatus{}, ErrQualificationRequired
		}
		selected := *p.dailyInput
		q.DailyInput = &selected
	}
	q.ID = productDigest(q)
	if err := s.verifyQualification(&q); err != nil {
		return ProductJobStatus{}, err
	}
	err = p.guard(func() error {
		c, err := s.loadControl(p.jobID)
		if err != nil {
			return err
		}
		c.Revision++
		c.OneShot = ""
		c.Qualification = &q
		return s.saveControl(c)
	})
	if err != nil {
		return ProductJobStatus{}, err
	}
	return s.jobStatus(p.jobID)
}

func (s *Service) domainEvidence(jobID string, at time.Time) (string, string, error) {
	var path, ref string
	switch jobID {
	case YodaSelfReviewWeeklyJobID:
		return s.weeklyDomainEvidence()
	case SelfObservationCaptureJobID:
		_, p, err := s.readSelfObservationReport("")
		if err != nil {
			return "", "", err
		}
		path = p
		relative, _ := filepath.Rel(s.receipts.Root, path)
		ref = "maintenance/" + filepath.ToSlash(relative)
	case MemoryCaptureJobID:
		_, p, err := s.readCaptureReport("")
		if err != nil {
			return "", "", err
		}
		path = p
		relative, _ := filepath.Rel(s.receipts.Root, path)
		ref = "maintenance/" + filepath.ToSlash(relative)
	case MemoryLightDreamJobID:
		artifact, p, err := s.engine.ActiveArtifact(s.workspaceID, "L1/"+at.UTC().Format("2006-01-02"))
		if err != nil {
			return "", "", err
		}
		if len(artifact.Sources) == 0 || artifact.Content == "" || artifact.SynthesizerID != memory.DeterministicL1SynthesizerID {
			return "", "", ErrQualificationRequired
		}
		relative, err := filepath.Rel(s.engine.Root, p)
		if err != nil {
			return "", "", err
		}
		ref = "memory/" + filepath.ToSlash(relative)
		path = p
	case MemoryCheckpointJobID:
		store := ContinuityCheckpointStore{Root: s.receipts.Root}
		cp, err := store.Load(s.workspaceID)
		if err != nil {
			return "", "", err
		}
		if cp.SourceCount < 1 {
			return "", "", ErrQualificationRequired
		}
		root := filepath.Join(s.receipts.Root, "workspaces", s.workspaceID, "memory-checkpoints")
		var pointer checkpointPointer
		if err := readCheckpointJSON(filepath.Join(root, "current.json"), &pointer); err != nil {
			return "", "", err
		}
		path = filepath.Join(root, "versions", pointer.Artifact)
		relative, _ := filepath.Rel(s.receipts.Root, path)
		ref = "maintenance/" + filepath.ToSlash(relative)
	case MemoryDeepDreamJobID:
		var err error
		path, err = s.weekly.WeeklyCommitEvidence(s.workspaceID, weeklyPeriod(at), "")
		if err != nil {
			return "", "", err
		}
		relative, _ := filepath.Rel(s.engine.Root, path)
		ref = "memory/" + filepath.ToSlash(relative)
	case "skills-index-refresh":
		index, err := skillsindex.LoadRuntimeIndex(s.indexRoot(), "")
		if err != nil {
			return "", "", err
		}
		path = index.MetadataPath
		relative, _ := filepath.Rel(s.home, path)
		ref = filepath.ToSlash(relative)
	default:
		if !diagnosticJob(jobID) {
			return "", "", scheduler.ErrCapabilityUnavailable
		}
		_, p, err := (DiagnosticStore{Root: s.receipts.Root}).Load(s.workspaceID, jobID, "")
		if err != nil {
			return "", "", err
		}
		path = p
		relative, _ := filepath.Rel(s.receipts.Root, path)
		ref = "maintenance/" + filepath.ToSlash(relative)
	}
	body, err := scheduler.ReadPrivateFile(path, 2<<20)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), ref, nil
}

func (s *Service) verifyQualification(q *ProductQualification) error {
	if q != nil && DarwinJob(q.JobID) {
		return s.verifyDarwinQualification(q)
	}
	if q == nil {
		return ErrQualificationRequired
	}
	if q.DailyInput != nil {
		day, err := time.Parse("2006-01-02", q.DailyInput.Period)
		if q.JobID != MemoryLightDreamJobID || err != nil || day.Format("2006-01-02") != q.DailyInput.Period || !digestPattern.MatchString(q.DailyInput.Fingerprint) {
			return ErrQualificationRequired
		}
	}
	unsigned := *q
	unsigned.ID = ""
	if q.ID != productDigest(unsigned) || q.WorkspaceID != s.workspaceID || q.Binding != s.binding || !productJob(q.JobID) || q.Implementation != s.build || q.Configuration != s.configuration(q.JobID) || q.Adapter != s.options.Adapter || q.Platform != runtime.GOOS+"/"+runtime.GOARCH || q.VerifiedAt.IsZero() || q.RequestedAt.IsZero() || q.ScheduledFor.IsZero() || q.VerifiedAt.Before(q.RequestedAt) {
		return ErrQualificationRequired
	}
	if err := q.Receipt.Validate(); err != nil {
		return ErrQualificationRequired
	}
	if q.Receipt.JobID != q.JobID || q.Receipt.WorkspaceID != s.workspaceID || q.Receipt.State != ReceiptSucceeded || !q.Receipt.RecordedAt.Equal(q.RequestedAt) {
		return ErrQualificationRequired
	}
	if (q.JobID == WikiIncrementalJobID) != (q.EventID != "") || q.Receipt.EventID != q.EventID {
		return ErrQualificationRequired
	}
	if strings.Contains(q.Artifact, "\\") || filepath.IsAbs(q.Artifact) || filepath.ToSlash(filepath.Clean(q.Artifact)) != q.Artifact || strings.Contains(q.Artifact, "..") {
		return ErrQualificationRequired
	}
	var path string
	prefix := "memory/workspaces/" + s.workspaceID + "/versions/"
	if q.JobID == YodaSelfReviewWeeklyJobID {
		var err error
		path, err = s.verifyWeeklyQualification(q)
		if err != nil {
			return err
		}
	} else if q.JobID == SelfObservationCaptureJobID {
		prefix = "maintenance/workspaces/" + s.workspaceID + "/self-observations/versions/"
		if !s.ownerScoped || !strings.HasPrefix(q.Artifact, prefix) || !strings.HasSuffix(q.Artifact, ".json") {
			return ErrQualificationRequired
		}
		generation := strings.TrimSuffix(strings.TrimPrefix(q.Artifact, prefix), ".json")
		report, p, err := s.readSelfObservationReport(generation)
		if err != nil || report.Implementation != s.build || report.Configuration != s.configuration(q.JobID) || !report.RecordedAt.Equal(q.RequestedAt) {
			return ErrQualificationRequired
		}
		path = p
	} else if q.JobID == MemoryCaptureJobID {
		prefix = "maintenance/workspaces/" + s.workspaceID + "/capture-results/versions/"
		if !strings.HasPrefix(q.Artifact, prefix) || !strings.HasSuffix(q.Artifact, ".json") {
			return ErrQualificationRequired
		}
		generation := strings.TrimSuffix(strings.TrimPrefix(q.Artifact, prefix), ".json")
		report, p, err := s.readCaptureReport(generation)
		if err != nil || report.Implementation != s.build || report.Configuration != s.configuration(q.JobID) || !report.RecordedAt.Equal(q.RequestedAt) {
			return ErrQualificationRequired
		}
		path = p
	} else if q.JobID == MemoryLightDreamJobID {
		if !strings.HasPrefix(q.Artifact, prefix) {
			return ErrQualificationRequired
		}
		path = filepath.Join(s.engine.Root, strings.TrimPrefix(q.Artifact, "memory/"))
	} else if q.JobID == MemoryDeepDreamJobID {
		prefix = "memory/workspaces/" + s.workspaceID + "/commits/"
		if !strings.HasPrefix(q.Artifact, prefix) {
			return ErrQualificationRequired
		}
		var err error
		path, err = s.weekly.WeeklyCommitEvidence(s.workspaceID, weeklyPeriod(q.RequestedAt), strings.TrimPrefix(q.Artifact, prefix))
		if err != nil {
			return ErrQualificationRequired
		}
	} else if q.JobID == "skills-index-refresh" {
		prefix = "brain/runtime/workspaces/" + s.workspaceID + "/skills-index/generations/"
		if !strings.HasPrefix(q.Artifact, prefix) || !strings.HasSuffix(q.Artifact, "/metadata.json") {
			return ErrQualificationRequired
		}
		generation := strings.TrimSuffix(strings.TrimPrefix(q.Artifact, prefix), "/metadata.json")
		index, err := skillsindex.LoadRuntimeIndex(s.indexRoot(), generation)
		if err != nil || index.Metadata.Source.WorkspaceID != s.workspaceID || index.Metadata.Source.Implementation != s.build || index.Metadata.Source.Configuration != s.configuration(q.JobID) {
			return ErrQualificationRequired
		}
		path = index.MetadataPath
	} else if diagnosticJob(q.JobID) {
		prefix = "maintenance/workspaces/" + s.workspaceID + "/diagnostics/" + q.JobID + "/versions/"
		if !strings.HasPrefix(q.Artifact, prefix) || !strings.HasSuffix(q.Artifact, ".json") {
			return ErrQualificationRequired
		}
		generation := strings.TrimSuffix(strings.TrimPrefix(q.Artifact, prefix), ".json")
		report, p, err := (DiagnosticStore{Root: s.receipts.Root}).Load(s.workspaceID, q.JobID, generation)
		if err != nil || !report.ObservedAt.Equal(q.RequestedAt) || report.Implementation != s.build || report.Configuration != s.configuration(q.JobID) {
			return ErrQualificationRequired
		}
		if q.JobID == UpdateAvailabilityJobID && (report.Availability == nil || !report.Availability.MetadataReady()) {
			return ErrQualificationRequired
		}
		path = p
	} else {
		prefix = "maintenance/workspaces/" + s.workspaceID + "/memory-checkpoints/versions/"
		if !strings.HasPrefix(q.Artifact, prefix) {
			return ErrQualificationRequired
		}
		path = filepath.Join(s.receipts.Root, strings.TrimPrefix(q.Artifact, "maintenance/"))
	}
	body, err := scheduler.ReadPrivateFile(path, 2<<20)
	if err != nil {
		return ErrQualificationRequired
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != q.ArtifactSHA256 {
		return ErrQualificationRequired
	}
	if q.JobID == MemoryLightDreamJobID {
		var artifact memory.Artifact
		if decodeProduct(body, &artifact) != nil || artifact.WorkspaceID != s.workspaceID || artifact.Layer != "L1" || len(artifact.Sources) == 0 || artifact.Content == "" || artifact.SynthesizerID != memory.DeterministicL1SynthesizerID {
			return ErrQualificationRequired
		}
		if q.DailyInput != nil {
			if artifact.Period != q.DailyInput.Period || artifact.SourceFingerprint != q.DailyInput.Fingerprint {
				return ErrQualificationRequired
			}
		} else if artifact.Period != q.RequestedAt.UTC().Format("2006-01-02") {
			return ErrQualificationRequired
		}
	} else if q.JobID == MemoryCheckpointJobID {
		var cp MemoryCheckpoint
		if decodeProduct(body, &cp) != nil || validateMemoryCheckpoint(cp) != nil || cp.WorkspaceID != s.workspaceID {
			return ErrQualificationRequired
		}
	}
	receiptPath := filepath.Join(s.receipts.Root, "workspaces", s.workspaceID, "receipts", q.JobID, q.Receipt.CommandID+"--"+q.Receipt.AttemptID+".json")
	body, err = scheduler.ReadPrivateFile(receiptPath, maximumReceiptBytes)
	if err != nil {
		return ErrQualificationRequired
	}
	var actual Receipt
	if decodeProduct(body, &actual) != nil || actual != q.Receipt {
		return ErrQualificationRequired
	}
	if q.EventID != "" {
		return s.receipts.VerifyEventReceipt(q.Receipt)
	}
	name := q.RequestedAt.UTC().Format("20060102T150405.000000000Z") + "-" + q.ScheduledFor.UTC().Format("20060102T150405.000000000Z") + ".json"
	body, err = scheduler.ReadPrivateFile(filepath.Join(s.scheduler.Root, "workspaces", s.workspaceID, "receipts", q.JobID, name), 64<<10)
	if err != nil {
		return ErrQualificationRequired
	}
	var sr scheduler.Receipt
	if decodeProduct(body, &sr) != nil || sr.WorkspaceID != s.workspaceID || sr.JobID != q.JobID || sr.State != scheduler.Succeeded || !sr.ScheduledFor.Equal(q.ScheduledFor) || !sr.AttemptedAt.Equal(q.RequestedAt) {
		return ErrQualificationRequired
	}
	return nil
}

// NewProductWeeklyMemoryEngine preserves exact lifetime review in the existing
// attended memory.consolidate path. Job enrollment grants only L2/L3 work.
func NewProductWeeklyMemoryEngine(home string) (*memory.Engine, error) {
	engine, cfg, err := NewProductMemoryEngine(home)
	if err != nil {
		return nil, err
	}
	daily := engine.Synthesizer.(memory.DeterministicL1Synthesizer)
	engine.Synthesizer = memory.DeterministicWeeklySynthesizer{Daily: daily, MaxRunes: engine.Budgets, MaxBytes: cfg.ContextBudgets(), MaxInputBytes: cfg.L1MaxInputBytes}
	engine.SynthesizerID = memory.DeterministicWeeklySynthesizerID
	engine.EligibilityPolicyID = "deterministic-two-l3-generations+exact-owner-review-v1"
	engine.Eligibility = memory.EligibilityFunc(func(ctx context.Context, candidate memory.Artifact) (bool, string, error) {
		eligible, reason, err := (memory.DeterministicLifetimeEligibility{MinL3Generations: 2}).Evaluate(ctx, candidate)
		if err != nil || !eligible {
			return false, reason, err
		}
		return false, "trusted exact-candidate confirmation unavailable; L2/L3 retained without lifetime promotion", nil
	})
	return engine, nil
}
func scheduleFor(job string) ProductSchedule {
	if job == YodaSelfReviewWeeklyJobID || job == DarwinWeeklyJobID {
		return ProductSchedule{Cadence: scheduler.Weekly, Weekday: time.Sunday, MaxCatchUp: 1}
	}
	if job == WikiIncrementalJobID {
		return ProductSchedule{EventOnly: true, MaxCatchUp: 1}
	}
	if job == MemoryDeepDreamJobID || job == WikiReconcileJobID {
		return ProductSchedule{Cadence: scheduler.Weekly, Weekday: time.Sunday, MaxCatchUp: 1}
	}
	if job == MemoryLightDreamJobID || job == MemoryCheckpointJobID {
		return ProductSchedule{Cadence: scheduler.Interval, IntervalHours: 3, MaxCatchUp: 1}
	}
	return ProductSchedule{Cadence: scheduler.Daily, MaxCatchUp: 1}
}
func (s *Service) sourceState(job string) (string, error) {
	if job == UpdateAvailabilityJobID {
		return s.updateAvailabilitySource()
	}
	if DarwinJob(job) {
		source, err := s.darwinSource(job)
		return productDigest(source), err
	}
	if job == UpdateCheckJobID {
		review, err := s.updateReview()
		return review.Digest, err
	}
	if wikiJob(job) {
		return s.wikiSourceIdentity(job)
	}
	switch job {
	case YodaSelfReviewWeeklyJobID:
		c, e := s.loadControl(job)
		if e != nil {
			return "", e
		}
		if c.Weekly == nil {
			return "", ErrQualificationRequired
		}
		return c.Weekly.Registration.Digest(), nil
	case SelfObservationCaptureJobID:
		return s.selfObservationSourceIdentity()
	case MemoryCaptureJobID:
		return s.captureSourceIdentity()
	case MemoryLightDreamJobID:
		inputs, err := s.engine.DailyInputCohort(s.workspaceID)
		return productDigest(inputs), err
	case MemoryDeepDreamJobID:
		return s.weekly.WeeklyInputIdentity(s.workspaceID, s.now())
	case MemoryCheckpointJobID:
		sources, err := s.scheduler.Receipts(s.workspaceID)
		return productDigest(sources), err
	case "skills-index-refresh":
		source, err := s.indexSource()
		return productDigest(source), err
	default:
		source, err := s.diagnosticSource(job)
		return source.SourceSHA256, err
	}
}
func weeklyPeriod(at time.Time) string {
	year, week := at.UTC().ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}
