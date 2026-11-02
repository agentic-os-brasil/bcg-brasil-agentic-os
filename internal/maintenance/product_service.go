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
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

var ErrProductConflict = errors.New("maintenance review or revision changed")
var ErrQualificationRequired = errors.New("current actual maintenance qualification required")

// ServiceOptions is a native constructor boundary, never a tool schema.
// Catalog must come from bundles/base/runtime.Maintenance (which imports this package).
type ServiceOptions struct {
	ProductRoot, UserHome, WorkspacePath string
	LookupEnv                            func(string) (string, bool)
	Catalog                              Catalog
	Adapter                              string
	// Now is an optional trusted constructor clock, never tool/CLI input.
	Now func() time.Time `json:"-"`
}

type Service struct {
	options                                       ServiceOptions
	home, workspaceID, binding, executable, build string
	scheduler                                     scheduler.Store
	receipts                                      Store
	engine                                        *memory.Engine
	now                                           func() time.Time
}

type Operation struct {
	Kind             string `json:"kind"`
	JobID            string `json:"job_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Timezone         string `json:"timezone,omitempty"`
}

type ProductSchedule struct {
	Timezone      string            `json:"timezone"`
	Cadence       scheduler.Cadence `json:"cadence"`
	IntervalHours int               `json:"interval_hours"`
	MaxCatchUp    int               `json:"max_catch_up"`
}

// ProductReview contains metadata only. Its private binding rejects edited or
// serialized reviews; trusted confirmation is still required for every Apply.
type ProductReview struct {
	Operation       Operation       `json:"operation"`
	WorkspaceID     string          `json:"workspace_id"`
	Schedule        ProductSchedule `json:"schedule"`
	Writes          []string        `json:"writes"`
	QualificationID string          `json:"qualification_id,omitempty"`
	Implementation  string          `json:"implementation"`
	Configuration   string          `json:"configuration"`
	SourceState     string          `json:"source_state,omitempty"`
	seal            string
}

type ProductQualification struct {
	ID             string    `json:"id"`
	WorkspaceID    string    `json:"workspace_id"`
	JobID          string    `json:"job_id"`
	Binding        string    `json:"binding"`
	Implementation string    `json:"implementation"`
	Configuration  string    `json:"configuration"`
	Adapter        string    `json:"adapter"`
	Platform       string    `json:"platform"`
	Artifact       string    `json:"artifact"`
	ArtifactSHA256 string    `json:"artifact_sha256"`
	Receipt        Receipt   `json:"receipt"`
	ScheduledFor   time.Time `json:"scheduled_for"`
	RequestedAt    time.Time `json:"requested_at"`
	VerifiedAt     time.Time `json:"verified_at"`
}

type ProductJobStatus struct {
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
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	f, err := scheduler.OpenRegularFile(executable)
	if err != nil {
		return err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return errors.New("running executable unavailable")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return err
	}
	after, err := os.Stat(executable)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || before.ModTime() != after.ModTime() {
		return errors.New("running executable changed")
	}
	build := hex.EncodeToString(hash.Sum(nil))
	binding := productDigest([]string{h.Home, path, w.WorkspaceID})
	if s.binding != "" && (s.binding != binding || s.build != build || s.executable != executable) {
		return ErrProductConflict
	}
	if s.binding != "" {
		return nil
	}
	s.home, s.workspaceID, s.binding, s.executable, s.build = h.Home, w.WorkspaceID, binding, executable, build
	return nil
}

func productDigest(value any) string {
	body, _ := json.Marshal(value)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func productJob(jobID string) bool {
	return jobID == MemoryLightDreamJobID || jobID == MemoryCheckpointJobID
}
func (s *Service) configuration(jobID string) string {
	job, _ := findCatalogJob(s.options.Catalog, jobID)
	policy, _ := basememory.Policy()
	cfg, _ := basememory.Runtime()
	return productDigest([]any{"product-maintenance-v1", job, policy, cfg, memory.DeterministicL1SynthesizerID})
}
func (s *Service) controlPath(jobID string) string {
	return filepath.Join(s.receipts.Root, "workspaces", s.workspaceID, "product-control", jobID, "current.json")
}
func (s *Service) loadControl(jobID string) (productControl, error) {
	c := productControl{SchemaVersion: 1, WorkspaceID: s.workspaceID, Binding: s.binding, JobID: jobID, State: "inactive"}
	body, err := scheduler.ReadPrivateFile(s.controlPath(jobID), 64<<10)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
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
	if c.Qualification != nil && (c.Qualification.JobID != c.JobID || c.Qualification.WorkspaceID != c.WorkspaceID || c.Qualification.Binding != c.Binding) {
		return c, ErrProductConflict
	}
	for _, write := range c.Writes {
		if !validWrites[write] {
			return c, ErrProductConflict
		}
	}
	if c.State != "inactive" && c.State != "active" && c.State != "paused" && c.State != "revoked" {
		return c, ErrProductConflict
	}
	if c.State == "active" || c.State == "paused" {
		if len(c.Writes) == 0 || c.Boundary.IsZero() || c.Schedule.Cadence != scheduler.Interval || c.Schedule.IntervalHours != 3 || c.Schedule.MaxCatchUp != 1 {
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
		c, err := s.loadControl(job.ID)
		if err != nil {
			return report, err
		}
		item := ProductJobStatus{JobID: job.ID, Revision: c.Revision, State: c.State, Schedule: c.Schedule, Boundary: c.Boundary, Writes: append([]string(nil), job.Writes...), Qualification: c.Qualification, QualificationState: "unqualified"}
		if len(c.Writes) > 0 {
			item.Writes = append([]string(nil), c.Writes...)
		}
		if !productJob(job.ID) {
			item.Reason = "production_binding_unavailable"
		} else if c.Qualification != nil {
			if err := s.verifyQualification(c.Qualification); err == nil {
				item.QualificationState = "qualified"
			} else {
				item.QualificationState = "stale"
				item.Reason = "qualification_requires_refresh"
			}
		}
		report.Jobs = append(report.Jobs, item)
	}
	return report, nil
}

func (s *Service) Review(op Operation) (ProductReview, error) {
	if err := s.validate(); err != nil {
		return ProductReview{}, err
	}
	if !productJob(op.JobID) {
		return ProductReview{}, scheduler.ErrCapabilityUnavailable
	}
	if op.Kind != "qualify" && op.Kind != "enroll" && op.Kind != "pause" && op.Kind != "resume" && op.Kind != "revoke" {
		return ProductReview{}, errors.New("unknown maintenance operation")
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
		schedule = ProductSchedule{Timezone: op.Timezone, Cadence: scheduler.Interval, IntervalHours: 3, MaxCatchUp: 1}
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
	if c.Qualification != nil {
		r.QualificationID = c.Qualification.ID
	}
	if op.Kind == "qualify" || op.Kind == "enroll" {
		if op.JobID == MemoryLightDreamJobID {
			r.SourceState, err = s.engine.DailyInputIdentity(s.workspaceID, s.now())
		} else {
			var sources []scheduler.Receipt
			sources, err = s.scheduler.Receipts(s.workspaceID)
			r.SourceState = productDigest(sources)
		}
		if err != nil {
			return ProductReview{}, err
		}
	}
	r.seal = productDigest([]any{s.binding, r})
	return r, nil
}

// Apply invokes the trusted host's exact review before any write, then repeats
// read-only review and CAS. Returning an error from confirm leaves no state.
func (s *Service) Apply(ctx context.Context, r ProductReview, confirm func(context.Context, ProductReview) error) (ProductJobStatus, error) {
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
	service  *Service
	jobID    string
	revision uint64
	oneShot  string
	source   string
}

func (p *productExecution) guard(publish func() error) error {
	return p.service.withControl(p.jobID, func() error {
		c, err := p.service.loadControl(p.jobID)
		if err != nil {
			return err
		}
		if c.Revision != p.revision || p.oneShot != "" && c.OneShot != p.oneShot || p.oneShot == "" && c.State != "active" {
			return ErrProductConflict
		}
		return publish()
	})
}
func (s *Service) worker(p *productExecution) Worker {
	return Worker{Catalog: s.options.Catalog, Scheduler: s.scheduler, Receipts: s.receipts, Jobs: []scheduler.Job{{ID: p.jobID, Cadence: scheduler.Interval, IntervalHours: 3, MaxCatchUp: 1}}, Handlers: map[string]any{
		MemoryLightDreamJobID: MemoryLightDreamHandler{Engine: s.engine, ExpectedSourceFingerprint: p.source}, MemoryCheckpointJobID: MemoryCheckpointHandler{Scheduler: s.scheduler, Store: ContinuityCheckpointStore{Root: s.receipts.Root}, ExpectedSourceFingerprint: p.source},
	}, Deadline: 2 * time.Minute, Now: s.now, product: p}
}
func (s *Service) execute(ctx context.Context, p *productExecution, occ scheduler.Occurrence, now time.Time) (Receipt, error) {
	if err := p.guard(func() error { return nil }); err != nil {
		return Receipt{}, err
	}
	authority := ExecutionAuthority{catalog: s.options.Catalog, occurrences: map[string]OccurrenceAuthorization{}, activated: map[string]bool{p.jobID: true}, attended: p.oneShot != "", preauthorized: p.oneShot == "", ready: true, productBound: true}
	if err := authority.addOccurrence(OccurrenceAuthorization{WorkspaceID: s.workspaceID, JobID: p.jobID, Trigger: TriggerPresence, ScheduledFor: occ.ScheduledFor}); err != nil {
		return Receipt{}, err
	}
	request := WakeRequest{WorkspaceID: s.workspaceID, Trigger: TriggerPresence, Now: now, OwnerID: fmt.Sprintf("product-%d", os.Getpid()), Attended: p.oneShot != "", Preauthorized: p.oneShot == "", IdleState: IdleConfirmed}
	receipt, err := s.worker(p).runOccurrence(ctx, request, now, occ, authority)
	if err != nil {
		return receipt, err
	}
	if receipt.State == ReceiptSucceeded || receipt.State == ReceiptReviewedNoChange {
		// A maintenance receipt alone cannot conceal a failed scheduler publication.
		receipts, readErr := s.scheduler.Receipts(s.workspaceID)
		if readErr != nil {
			return receipt, readErr
		}
		for _, durable := range receipts {
			if durable.JobID == p.jobID && durable.State == scheduler.Succeeded && durable.ScheduledFor.Equal(occ.ScheduledFor) && durable.AttemptedAt.Equal(receipt.RecordedAt) {
				return receipt, nil
			}
		}
		return receipt, errors.New("maintenance domain/receipt publication is partial")
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
		due, err := scheduler.PlanDue([]scheduler.Job{{ID: job.ID, Cadence: c.Schedule.Cadence, IntervalHours: c.Schedule.IntervalHours, MaxCatchUp: c.Schedule.MaxCatchUp}}, c.Boundary, filtered, now.In(location))
		if err != nil {
			return report, err
		}
		for _, occ := range due {
			report.Due = append(report.Due, occ)
			receipt, err := s.execute(ctx, &productExecution{service: s, jobID: job.ID, revision: c.Revision}, occ, now)
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
	now := s.now().UTC()
	before, _, beforeErr := s.domainEvidence(p.jobID, now)
	if beforeErr != nil && !errors.Is(beforeErr, os.ErrNotExist) {
		return ProductJobStatus{}, beforeErr
	}
	occ := scheduler.Occurrence{JobID: p.jobID, ScheduledFor: now}
	receipt, err := s.execute(ctx, p, occ, now)
	if err != nil {
		return ProductJobStatus{}, err
	}
	if receipt.State != ReceiptSucceeded {
		return ProductJobStatus{}, ErrQualificationRequired
	}
	after, path, err := s.domainEvidence(p.jobID, now)
	if err != nil {
		return ProductJobStatus{}, err
	}
	if before == after || after == "" {
		return ProductJobStatus{}, ErrQualificationRequired
	}
	q := ProductQualification{WorkspaceID: s.workspaceID, JobID: p.jobID, Binding: s.binding, Implementation: s.build, Configuration: s.configuration(p.jobID), Adapter: s.options.Adapter, Platform: runtime.GOOS + "/" + runtime.GOARCH, Artifact: path, ArtifactSHA256: after, Receipt: receipt, ScheduledFor: occ.ScheduledFor, RequestedAt: now, VerifiedAt: s.now().UTC()}
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
	default:
		return "", "", scheduler.ErrCapabilityUnavailable
	}
	body, err := scheduler.ReadPrivateFile(path, 2<<20)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), ref, nil
}

func (s *Service) verifyQualification(q *ProductQualification) error {
	if q == nil {
		return ErrQualificationRequired
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
	if strings.Contains(q.Artifact, "\\") || filepath.IsAbs(q.Artifact) || filepath.ToSlash(filepath.Clean(q.Artifact)) != q.Artifact || strings.Contains(q.Artifact, "..") {
		return ErrQualificationRequired
	}
	var path string
	prefix := "memory/workspaces/" + s.workspaceID + "/versions/"
	if q.JobID == MemoryLightDreamJobID {
		if !strings.HasPrefix(q.Artifact, prefix) {
			return ErrQualificationRequired
		}
		path = filepath.Join(s.engine.Root, strings.TrimPrefix(q.Artifact, "memory/"))
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
		if decodeProduct(body, &artifact) != nil || artifact.WorkspaceID != s.workspaceID || artifact.Layer != "L1" || artifact.Period != q.RequestedAt.UTC().Format("2006-01-02") || len(artifact.Sources) == 0 || artifact.Content == "" || artifact.SynthesizerID != memory.DeterministicL1SynthesizerID {
			return ErrQualificationRequired
		}
	} else {
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
