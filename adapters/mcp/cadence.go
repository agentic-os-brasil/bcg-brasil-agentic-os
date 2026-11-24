package product

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func maintenanceQueries() []any {
	return []any{variant("maintenance.status", map[string]any{}), variant("maintenance.report", map[string]any{"job_id": map[string]any{"type": "string", "enum": []string{maintenance.DarwinDailyJobID, maintenance.DarwinWeeklyJobID, maintenance.DarwinMonthlyJobID, maintenance.UpdateCheckJobID, maintenance.UpdateAvailabilityJobID, maintenance.MemoryModelJobID, maintenance.MemoryDeepDreamJobID, maintenance.MemoryCaptureJobID, maintenance.SelfObservationCaptureJobID, maintenance.YodaSelfReviewWeeklyJobID, "memory-retention-check", "skills-index-refresh", "runtime-health-check", "capability-recheck", "runtime-drift-check", maintenance.WikiIncrementalJobID, maintenance.WikiReconcileJobID, maintenance.WikiIntegrityJobID}}}, "job_id")}
}

func maintenanceTransitions() []any {
	catalog, err := baseruntime.Maintenance()
	if err != nil {
		panic(err)
	}
	ids := make([]string, 0, len(catalog.Jobs))
	for _, job := range catalog.Jobs {
		ids = append(ids, job.ID)
	}
	var variants []any
	for _, kind := range []string{"qualify", "enroll", "pause", "resume", "revoke"} {
		fields := map[string]any{
			"job_id":            map[string]any{"type": "string", "enum": ids},
			"expected_revision": map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991},
		}
		required := []string{"job_id", "expected_revision"}
		if kind == "enroll" {
			fields["timezone"] = strSchema(96)
			required = append(required, "timezone")
		}
		variants = append(variants, variant("maintenance."+kind, fields, required...))
	}
	variants = append(variants, variant("maintenance.register", map[string]any{
		"job_id":            map[string]any{"type": "string", "enum": []string{maintenance.YodaSelfReviewWeeklyJobID, maintenance.MemoryModelJobID}},
		"expected_revision": map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991},
	}, "job_id", "expected_revision"))
	return variants
}

func maintenanceMutations() []any {
	return append(maintenanceTransitions(), variant("maintenance.run", map[string]any{}))
}

// MaintenanceCommandDiagnostic contains no source paths, requests or arbitrary errors.
type MaintenanceCommandDiagnostic struct {
	SchemaVersion int    `json:"schema_version"`
	Phase         string `json:"phase"`
	Code          string `json:"code"`
}

type MaintenanceCommandError struct {
	Diagnostic MaintenanceCommandDiagnostic
	cause      error
}

func (e *MaintenanceCommandError) Error() string {
	diagnostic := e.Diagnostic
	if !validMaintenanceDiagnostic(diagnostic) {
		diagnostic = MaintenanceCommandDiagnostic{SchemaVersion: 1, Phase: "unknown", Code: "maintenance_rejected"}
	}
	return "maestro: maintenance " + diagnostic.Code + " during " + diagnostic.Phase + "; completion was not confirmed; inspect maintenance-status before a fresh maintenance-review"
}
func (e *MaintenanceCommandError) Unwrap() error { return e.cause }

func maintenanceCommandPhase(phase string) string {
	switch phase {
	case "input", "admission", "review", "confirmation", "reservation", "selection", "execution", "verification", "publication", "output", "launch":
		return phase
	default:
		return "unknown"
	}
}

var ErrMaintenanceCommandOutputLimit = errors.New("maintenance command output limit")

// ErrWeeklyRegistrationUnavailable is the closed startup registration refusal.
var ErrWeeklyRegistrationUnavailable = errors.New("weekly_registration_unavailable")

func maintenanceErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrMaintenanceCommandOutputLimit):
		return "output_limit"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, ErrWeeklyRegistrationUnavailable):
		return "weekly_registration_unavailable"
	case errors.Is(err, ErrSourceRecoveryRequired):
		return "source_recovery_required"
	case errors.Is(err, processidentity.ErrChanged), errors.Is(err, errOwnerReviewRecordChanged):
		return "restart_required"
	case errors.Is(err, errConfirmationUnavailable):
		return "confirmation_unavailable"
	case errors.Is(err, errConfirmationDeclined):
		return "confirmation_declined"
	case errors.Is(err, maintenance.ErrProductConflict), errors.Is(err, maintenance.ErrCheckpointConflict), errors.Is(err, errOwnerReviewChanged):
		return "revision_conflict"
	case errors.Is(err, maintenance.ErrQualificationRequired):
		return "qualification_required"
	case errors.Is(err, scheduler.ErrCapabilityUnavailable):
		return "maintenance_unavailable"
	case errors.Is(err, scheduler.ErrLeaseBusy):
		return "maintenance_busy"
	default:
		return "maintenance_rejected"
	}
}

// MaintenanceCommandFailure preserves only a closed stage/category when displayed.
func MaintenanceCommandFailure(err error, phase string) *MaintenanceCommandError {
	var prior *MaintenanceCommandError
	if errors.As(err, &prior) && prior != nil && validMaintenanceDiagnostic(prior.Diagnostic) && prior.Diagnostic.Code != "in_progress" {
		return prior
	}
	return &MaintenanceCommandError{Diagnostic: MaintenanceCommandDiagnostic{SchemaVersion: 1, Phase: maintenanceCommandPhase(phase), Code: maintenanceErrorCode(err)}, cause: err}
}

// MaintenanceCommandProgress is fixed metadata; it grants no effects or authority.
func MaintenanceCommandProgress(phase string) MaintenanceCommandDiagnostic {
	return MaintenanceCommandDiagnostic{SchemaVersion: 1, Phase: maintenanceCommandPhase(phase), Code: "in_progress"}
}

func validMaintenanceDiagnostic(d MaintenanceCommandDiagnostic) bool {
	if d.SchemaVersion != 1 || d.Phase != maintenanceCommandPhase(d.Phase) {
		return false
	}
	switch d.Code {
	case "in_progress", "interrupted", "output_limit", "deadline_exceeded", "canceled", "weekly_registration_unavailable", "source_recovery_required", "restart_required", "confirmation_unavailable", "confirmation_declined", "revision_conflict", "qualification_required", "maintenance_unavailable", "maintenance_busy", "maintenance_rejected":
		return true
	}
	return false
}

// MaintenanceChildFailure reads only bounded closed diagnostics. An interrupted
// process has unknown completion, even if it emitted an earlier success-like line.
func MaintenanceChildFailure(stderr []byte, cause error, interrupted bool) *MaintenanceCommandError {
	last := MaintenanceCommandDiagnostic{SchemaVersion: 1, Phase: "launch", Code: "interrupted"}
	if len(stderr) <= 4096 {
		for _, line := range bytes.Split(stderr, []byte{'\n'}) {
			if len(line) == 0 || len(line) > 512 {
				continue
			}
			var d MaintenanceCommandDiagnostic
			decoder := json.NewDecoder(bytes.NewReader(line))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&d) == nil && decoder.Decode(new(any)) == io.EOF && validMaintenanceDiagnostic(d) {
				last = d
			}
		}
	}
	if interrupted || last.Code == "in_progress" {
		last.Code = "interrupted"
	}
	return &MaintenanceCommandError{Diagnostic: last, cause: cause}
}

// MaintenanceCommandTimeout retains the original ninety-second process supervision budget.
const MaintenanceCommandTimeout = 90 * time.Second

// MaintenanceCommandContext can only shorten the local operation's deadline.
// The native child receives a deadline, never a longer duration or new authority.
func MaintenanceCommandContext(parent context.Context, propagated string) (context.Context, context.CancelFunc, error) {
	if parent == nil {
		return nil, nil, errors.New("maintenance context required")
	}
	deadline := time.Now().Add(MaintenanceCommandTimeout)
	if propagated != "" {
		supplied, err := time.Parse(time.RFC3339Nano, propagated)
		if err != nil {
			return nil, nil, errors.New("invalid maintenance deadline")
		}
		if supplied.Before(deadline) {
			deadline = supplied
		}
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	if err := ctx.Err(); err != nil {
		cancel()
		return nil, nil, err
	}
	return ctx, cancel, nil
}

func (b *Bound) maintenanceService() (*maintenance.Service, error) {
	return b.maintenanceServiceContext(context.Background())
}

func (b *Bound) maintenanceServiceContext(ctx context.Context) (*maintenance.Service, error) {
	return b.newMaintenanceService(ctx, nil, b.guardPublication, b.guardPublicationContext)
}

func (b *Bound) maintenanceServiceWithEligibility(eligibility func() error) (*maintenance.Service, error) {
	return b.maintenanceServiceWithGuards(eligibility, b.guardPublication)
}

func (b *Bound) maintenanceServiceWithGuards(eligibility func() error, installedGuard func(func() error) error) (*maintenance.Service, error) {
	return b.newMaintenanceService(context.Background(), eligibility, installedGuard, nil)
}

func (b *Bound) newMaintenanceService(ctx context.Context, eligibility func() error, installedGuard func(func() error) error, installedGuardContext func(context.Context, func() error) error) (*maintenance.Service, error) {
	if err := b.validateContext(ctx); err != nil {
		return nil, err
	}
	catalog, err := baseruntime.Maintenance()
	if err != nil {
		return nil, err
	}
	// Foreground and serve-owned work share local authority. ClientInfo cannot
	// qualify a native Code/Desktop adapter or an operating-system wake.
	return maintenance.NewService(maintenance.ServiceOptions{
		Installed:       maintenance.InstalledSelection{ManifestSHA256: b.options.InstalledManifestSHA256, ClientInputsSHA256: b.options.InstalledClientInputsSHA256, CarrierID: b.options.ManagementCarrierID, InstallerVersion: b.options.ManagementInstallerVersion},
		SourceAdmission: b.experimentalSourceAdmission(), DiagnosticContext: ctx,
		EligibilityGuard: eligibility,
		WeeklyCandidate:  b.options.WeeklyCandidate, WeeklyNativeExecutor: b.options.WeeklyNativeExecutor, MemoryNativeExecutor: b.options.MemoryNativeExecutor,
		ProductRoot: b.options.ProductRoot, UserHome: b.options.UserHome,
		WorkspacePath: b.options.WorkspacePath, LookupEnv: b.options.LookupEnv,
		DarwinFactory: darwin.NewProductBinding, Catalog: catalog, Adapter: "local", Now: b.maintenanceClock, CLIVersion: b.options.CLIVersion, InstalledGuard: installedGuard, InstalledGuardContext: installedGuardContext, AuthorityRegistryPath: b.options.AuthorityRegistryPath, AuthorityRegistrySHA256: b.options.AuthorityRegistrySHA256,
	})
}

// MaintenanceMutation is only an intent. Neither model arguments nor local
// stdin can provide a service review, executable identity or qualification.
type MaintenanceMutation struct {
	Operation        string `json:"operation"`
	JobID            string `json:"job_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Timezone         string `json:"timezone,omitempty"`
}

func DecodeMaintenanceMutation(body []byte) (MaintenanceMutation, error) {
	if len(body) > 16<<10 || !utf8.Valid(body) {
		return MaintenanceMutation{}, errors.New("maintenance_invalid_input")
	}
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return MaintenanceMutation{}, err
	}
	bodySchema, err := json.Marshal(map[string]any{"type": "object", "oneOf": maintenanceTransitions()})
	if err != nil {
		return MaintenanceMutation{}, err
	}
	var definition jsonschema.Schema
	if err := json.Unmarshal(bodySchema, &definition); err != nil {
		return MaintenanceMutation{}, err
	}
	resolved, err := definition.Resolve(nil)
	if err != nil {
		return MaintenanceMutation{}, err
	}
	if err := resolved.Validate(raw); err != nil {
		return MaintenanceMutation{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var in MaintenanceMutation
	if err := decoder.Decode(&in); err != nil {
		return in, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return in, errors.New("maintenance_trailing_input")
	}
	return in, nil
}

func maintenanceOperation(in MaintenanceMutation) (maintenance.Operation, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return maintenance.Operation{}, err
	}
	if _, err := DecodeMaintenanceMutation(body); err != nil {
		return maintenance.Operation{}, err
	}
	return maintenance.Operation{Kind: strings.TrimPrefix(in.Operation, "maintenance."), JobID: in.JobID, ExpectedRevision: in.ExpectedRevision, Timezone: in.Timezone}, nil
}

// LocalMaintenanceReview is metadata for the attended stdin fallback. Apply
// reconstructs its private service review from the current admitted binding.
type LocalMaintenanceReview struct {
	Review       maintenance.ProductReview `json:"review"`
	ReviewDigest string                    `json:"review_digest"`
}

func (b *Bound) maintenanceReview(in MaintenanceMutation) (*maintenance.Service, maintenance.ProductReview, error) {
	return b.maintenanceReviewContext(context.Background(), in)
}

func (b *Bound) maintenanceReviewContext(ctx context.Context, in MaintenanceMutation) (*maintenance.Service, maintenance.ProductReview, error) {
	if err := ctx.Err(); err != nil {
		return nil, maintenance.ProductReview{}, err
	}
	op, err := maintenanceOperation(in)
	if err != nil {
		return nil, maintenance.ProductReview{}, err
	}
	service, err := b.maintenanceServiceContext(ctx)
	if err != nil {
		return nil, maintenance.ProductReview{}, err
	}
	review, err := service.ReviewContext(ctx, op)
	return service, review, err
}

func (b *Bound) maintenanceReviewDigest(review maintenance.ProductReview) string {
	body, _ := json.Marshal([]any{b.home, b.workspaceID, review})
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func (b *Bound) MaintenanceReview(in MaintenanceMutation) (LocalMaintenanceReview, error) {
	return b.MaintenanceReviewContext(context.Background(), in)
}

func (b *Bound) MaintenanceReviewContext(ctx context.Context, in MaintenanceMutation) (LocalMaintenanceReview, error) {
	_, review, err := b.maintenanceReviewContext(ctx, in)
	if err != nil {
		return LocalMaintenanceReview{}, err
	}
	return LocalMaintenanceReview{Review: review, ReviewDigest: b.maintenanceReviewDigest(review)}, nil
}

func (b *Bound) ApplyMaintenanceReview(ctx context.Context, in MaintenanceMutation, digest string, confirmed bool) (any, error) {
	if !confirmed {
		return nil, errConfirmationDeclined
	}
	service, review, err := b.maintenanceReviewContext(ctx, in)
	if err != nil {
		return nil, err
	}
	if digest != b.maintenanceReviewDigest(review) {
		return nil, maintenance.ErrProductConflict
	}
	job, err := service.Apply(ctx, review, func(ctx context.Context, _ maintenance.ProductReview) error { return b.validateContext(ctx) })
	if err != nil {
		return nil, err
	}
	return maintenanceJobMetadata(job), nil
}

type maintenanceQualificationMetadata struct {
	ID             string    `json:"id"`
	Adapter        string    `json:"adapter"`
	Platform       string    `json:"platform"`
	Implementation string    `json:"implementation"`
	Configuration  string    `json:"configuration"`
	ArtifactRef    string    `json:"artifact_ref"`
	ArtifactSHA256 string    `json:"artifact_sha256"`
	ScheduledFor   time.Time `json:"scheduled_for"`
	RequestedAt    time.Time `json:"requested_at"`
	VerifiedAt     time.Time `json:"verified_at"`
}

type maintenanceJobMetadataView struct {
	Scope              string                            `json:"scope,omitempty"`
	ScopeID            string                            `json:"scope_id,omitempty"`
	Diagnosis          *maintenance.DarwinSource         `json:"diagnosis,omitempty"`
	JobID              string                            `json:"job_id"`
	Revision           uint64                            `json:"revision"`
	State              string                            `json:"state"`
	Schedule           maintenance.ProductSchedule       `json:"schedule"`
	Boundary           time.Time                         `json:"no_backfill_before,omitempty"`
	Writes             []string                          `json:"writes"`
	QualificationState string                            `json:"qualification_state"`
	Qualification      *maintenanceQualificationMetadata `json:"qualification,omitempty"`
	Reason             string                            `json:"reason,omitempty"`
}

func maintenanceJobMetadata(job maintenance.ProductJobStatus) maintenanceJobMetadataView {
	out := maintenanceJobMetadataView{Scope: job.Scope, ScopeID: job.ScopeID, Diagnosis: job.Diagnosis, JobID: job.JobID, Revision: job.Revision, State: job.State, Schedule: job.Schedule, Boundary: job.Boundary, Writes: job.Writes, QualificationState: job.QualificationState, Reason: job.Reason}
	if q := job.Qualification; q != nil {
		out.Qualification = &maintenanceQualificationMetadata{ID: q.ID, Adapter: q.Adapter, Platform: q.Platform, Implementation: q.Implementation, Configuration: q.Configuration, ArtifactRef: q.Artifact, ArtifactSHA256: q.ArtifactSHA256, ScheduledFor: q.ScheduledFor, RequestedAt: q.RequestedAt, VerifiedAt: q.VerifiedAt}
	}
	return out
}

func (b *Bound) MaintenanceStatus() (any, error) {
	return b.MaintenanceStatusContext(context.Background())
}

func (b *Bound) MaintenanceStatusContext(ctx context.Context) (any, error) {
	service, err := b.maintenanceServiceContext(ctx)
	if err != nil {
		return nil, err
	}
	status, err := service.StatusContext(ctx)
	if err != nil {
		return nil, err
	}
	jobs := make([]maintenanceJobMetadataView, 0, len(status.Jobs))
	for _, job := range status.Jobs {
		jobs = append(jobs, maintenanceJobMetadata(job))
	}
	return map[string]any{"workspace_id": status.WorkspaceID, "jobs": jobs}, nil
}

func (b *Bound) RunMaintenance(ctx context.Context) (any, error) {
	service, err := b.maintenanceServiceContext(ctx)
	if err != nil {
		return nil, err
	}
	report, err := service.RunDue(ctx)
	// Project only bounded domain metadata, never receipt proofs or errors.
	receipts := make([]map[string]any, 0, len(report.Receipts))
	for _, receipt := range report.Receipts {
		receipts = append(receipts, map[string]any{"job_id": receipt.JobID, "state": receipt.State, "recorded_at": receipt.RecordedAt, "reason_code": receipt.ReasonCode})
	}
	return map[string]any{"workspace_id": report.WorkspaceID, "state": report.State, "due": report.Due, "receipts": receipts}, err
}

func maintenancePresentation(review maintenance.ProductReview) any {
	presentation := map[string]any{
		"operation": "maintenance." + review.Operation.Kind, "job_id": review.Operation.JobID,
		"workspace_id": review.WorkspaceID, "expected_revision": review.Operation.ExpectedRevision,
		"schedule": review.Schedule, "writes": review.Writes,
		"qualification_id": review.QualificationID, "implementation": review.Implementation,
		"configuration": review.Configuration, "source_state": review.SourceState,
		"scope": "This exact job and workspace only. Qualification executes once; enrollment authorizes only this schedule and write set. Operating-system wake installation remains separate.",
	}
	if review.Operation.JobID == maintenance.SelfObservationCaptureJobID {
		presentation["scope"] = "This exact owner and self-observation job across admitted workspaces, with one shared owner control and occurrence. Consume only exact owner-reviewed professional declarations as versioned proposal input; never capture conversations, infer native interaction episodes, corroborate or change active SELF. Declaration intake is separate from this job's qualification and enrollment. Qualification must commit and read back a new canonical observation; replay or no-op cannot qualify. Rejection, redaction, pause and revocation remain authoritative."
	}
	if review.Operation.JobID == maintenance.YodaSelfReviewWeeklyJobID {
		delete(presentation, "workspace_id")
		presentation["scope_kind"] = "owner"
		presentation["owner_scope_id"] = review.WorkspaceID
		presentation["scope"] = "This exact owner and weekly Yoda job across admitted workspaces. Registration alone leaves the job inactive and unqualified. Qualification observes one bounded tool-free native attempt; enrollment separately authorizes this schedule. Publish only the validated silent working projection. Canonical SELF remains unchanged; optional ordinary advisory context additionally requires current accepted session policy, execution and terminal evidence."
		if review.WeeklyRegistration != nil {
			presentation["weekly_registration"] = review.WeeklyRegistration
		}
	}
	if review.Operation.JobID == maintenance.MemoryModelJobID {
		presentation["scope_kind"] = "workspace"
		presentation["scope"] = "This exact workspace and memory-weekly job only. Registration leaves it inactive and unqualified. Qualification is one bounded tool-free native evidence-only attempt with no memory publication; enrollment separately permits canonical L2/L3 and a private pending lifetime candidate through the original atomic engine. Lifetime still requires independent deterministic continuity and exact foreground review. Sources/correction/deletion, immutable occurrence, pause/revocation and provider/model bounds remain enforced. Explicitly pause the other scheduled weekly engine before enrolling or resuming this one; no fallback or automatic engine swap."
		if review.WeeklyRegistration != nil {
			presentation["weekly_registration"] = review.WeeklyRegistration
		}
	}
	if review.Operation.JobID == maintenance.MemoryCaptureJobID {
		presentation["scope"] = "Admit only body-free, workspace-bound adapter observations and witnessed canonical completed-record metadata through this exact capture job. Adapter observations are observed_only; committed records do not attest an original host or native invocation. No prompts, transcripts, tool bodies, objectives, summaries or document paths enter memory. Qualification runs once; enrollment separately authorizes this cadence. Source state, producer version and permanent correction/deletion denials are revalidated before each publication."
	}
	if review.Operation.JobID == maintenance.MemoryLightDreamJobID {
		presentation["scope"] = "Generate deterministic L1 from the oldest pending eligible ingestion day in this workspace's bounded canonical memory cohort, at most one day per occurrence. Historical input days remain pending across client downtime; source fingerprints and current correction/deletion denials are revalidated before publication. The artifact records actual synthesis time. Qualification executes once and binds that selected period and fingerprint; enrollment separately authorizes this cadence. No capture, raw history ingestion, lifetime promotion or native invocation proof is granted."
	}
	if review.Operation.JobID == maintenance.UpdateCheckJobID {
		presentation["signed_source_review"] = review.SourceReview
		presentation["scope"] = "One attended exact signed-source observation, private candidate staging and bounded diagnosis. No recurring enrollment, release activation, native health or rollout authority. Authenticated higher recalls persist even when expired or payload-incomplete."
	}
	if review.Operation.JobID == maintenance.UpdateAvailabilityJobID {
		presentation["scope"] = "Observe only signed metadata from this installation's already enrolled source, once per calendar day on first active use across admitted workspaces. Record bounded local diagnostics and scheduler receipts in the existing private owner state. Source custody, payload downloads/staging, code activation, client replacement and brain changes are excluded. Qualification requires a genuine authenticated metadata read; enrollment is separate and never installs an operating-system service."
	}
	if review.DarwinReview != nil {
		presentation["darwin_review"] = review.DarwinReview
		presentation["scope"] = "Review this exact cache state, parent and external source. Qualification performs only the shown initialization, exclusive first-cache creation or bounded assessment. Stale cache replacement requires a manual step. Daily/weekly enrollment is separate; monthly proposals never enroll or apply structural changes. Native health remains unobserved."
	}
	return presentation
}

func maintenanceResult(value any, err error) *mcp.CallToolResult {
	if err == nil {
		return result(value, nil)
	}
	var pending *confirmationPending
	if errors.As(err, &pending) {
		return pending.result
	}
	code := maintenanceErrorCode(err)
	metadata := map[string]any{"code": code, "retryable": false}
	if errors.Is(err, errOwnerReviewRecordChanged) {
		metadata["next_action"] = ownerReviewReconnectAction
	}
	if value != nil {
		metadata["result"] = value
	}
	if code == "confirmation_unavailable" {
		metadata["next_action"] = "Use local maintenance-review with JSON on stdin, then maintenance-apply with the same input, exact --review-digest and explicit --confirm."
	}
	out := result(metadata, nil)
	out.IsError = true
	return out
}

func (b *Bound) handleMaintenance(ctx context.Context, req *mcp.CallToolRequest, in input) *mcp.CallToolResult {
	if in.Operation == "maintenance.status" {
		value, err := b.MaintenanceStatusContext(ctx)
		return maintenanceResult(value, err)
	}
	if in.Operation == "maintenance.report" {
		service, err := b.maintenanceServiceContext(ctx)
		if err != nil {
			return maintenanceResult(nil, err)
		}
		value, err := service.Report(in.JobID)
		return maintenanceResult(value, err)
	}
	if in.Operation == "maintenance.run" {
		value, err := b.RunMaintenance(ctx)
		return maintenanceResult(value, err)
	}
	mutation := MaintenanceMutation{Operation: in.Operation, JobID: in.JobID, ExpectedRevision: uint64(in.ExpectedRevision), Timezone: in.Timezone}
	service, review, err := b.maintenanceReviewContext(ctx, mutation)
	if err != nil {
		return maintenanceResult(nil, err)
	}
	job, err := service.Apply(ctx, review, func(ctx context.Context, exact maintenance.ProductReview) error {
		return b.confirm(ctx, req, "Authorize this exact maintenance operation?", exact, maintenancePresentation(exact))
	})
	if err != nil {
		return maintenanceResult(nil, err)
	}
	return maintenanceResult(maintenanceJobMetadata(job), nil)
}
