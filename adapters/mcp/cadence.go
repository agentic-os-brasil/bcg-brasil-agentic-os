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
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func maintenanceQueries() []any {
	return []any{variant("maintenance.status", map[string]any{}), variant("maintenance.report", map[string]any{"job_id": map[string]any{"type": "string", "enum": []string{maintenance.MemoryDeepDreamJobID, "memory-retention-check", "skills-index-refresh", "runtime-health-check", "capability-recheck", "runtime-drift-check"}}}, "job_id")}
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
	return variants
}

func maintenanceMutations() []any {
	return append(maintenanceTransitions(), variant("maintenance.run", map[string]any{}))
}

func (b *Bound) maintenanceService() (*maintenance.Service, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	catalog, err := baseruntime.Maintenance()
	if err != nil {
		return nil, err
	}
	// This executable's foreground entry is local. ClientInfo is not evidence
	// that a native Code/Desktop adapter or background wake was qualified.
	return maintenance.NewService(maintenance.ServiceOptions{
		ProductRoot: b.options.ProductRoot, UserHome: b.options.UserHome,
		WorkspacePath: b.options.WorkspacePath, LookupEnv: b.options.LookupEnv,
		Catalog: catalog, Adapter: "local", Now: b.maintenanceClock, CLIVersion: b.options.CLIVersion,
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
	op, err := maintenanceOperation(in)
	if err != nil {
		return nil, maintenance.ProductReview{}, err
	}
	service, err := b.maintenanceService()
	if err != nil {
		return nil, maintenance.ProductReview{}, err
	}
	review, err := service.Review(op)
	return service, review, err
}

func (b *Bound) maintenanceReviewDigest(review maintenance.ProductReview) string {
	body, _ := json.Marshal([]any{b.home, b.workspaceID, review})
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func (b *Bound) MaintenanceReview(in MaintenanceMutation) (LocalMaintenanceReview, error) {
	_, review, err := b.maintenanceReview(in)
	if err != nil {
		return LocalMaintenanceReview{}, err
	}
	return LocalMaintenanceReview{Review: review, ReviewDigest: b.maintenanceReviewDigest(review)}, nil
}

func (b *Bound) ApplyMaintenanceReview(ctx context.Context, in MaintenanceMutation, digest string, confirmed bool) (any, error) {
	if !confirmed {
		return nil, errConfirmationDeclined
	}
	service, review, err := b.maintenanceReview(in)
	if err != nil {
		return nil, err
	}
	if digest != b.maintenanceReviewDigest(review) {
		return nil, maintenance.ErrProductConflict
	}
	job, err := service.Apply(ctx, review, func(context.Context, maintenance.ProductReview) error { return b.validate() })
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
	out := maintenanceJobMetadataView{JobID: job.JobID, Revision: job.Revision, State: job.State, Schedule: job.Schedule, Boundary: job.Boundary, Writes: job.Writes, QualificationState: job.QualificationState, Reason: job.Reason}
	if q := job.Qualification; q != nil {
		out.Qualification = &maintenanceQualificationMetadata{ID: q.ID, Adapter: q.Adapter, Platform: q.Platform, Implementation: q.Implementation, Configuration: q.Configuration, ArtifactRef: q.Artifact, ArtifactSHA256: q.ArtifactSHA256, ScheduledFor: q.ScheduledFor, RequestedAt: q.RequestedAt, VerifiedAt: q.VerifiedAt}
	}
	return out
}

func (b *Bound) MaintenanceStatus() (any, error) {
	service, err := b.maintenanceService()
	if err != nil {
		return nil, err
	}
	status, err := service.Status()
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
	service, err := b.maintenanceService()
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
	return map[string]any{
		"operation": "maintenance." + review.Operation.Kind, "job_id": review.Operation.JobID,
		"workspace_id": review.WorkspaceID, "expected_revision": review.Operation.ExpectedRevision,
		"schedule": review.Schedule, "writes": review.Writes,
		"qualification_id": review.QualificationID, "implementation": review.Implementation,
		"configuration": review.Configuration, "source_state": review.SourceState,
		"scope": "This exact job and workspace only. Qualification executes once; enrollment authorizes only this schedule and write set. Native scheduling remains separate.",
	}
}

func maintenanceResult(value any, err error) *mcp.CallToolResult {
	if err == nil {
		return result(value, nil)
	}
	var pending *confirmationPending
	if errors.As(err, &pending) {
		return pending.result
	}
	code := "maintenance_rejected"
	switch {
	case errors.Is(err, errConfirmationUnavailable):
		code = "confirmation_unavailable"
	case errors.Is(err, errConfirmationDeclined):
		code = "confirmation_declined"
	case errors.Is(err, maintenance.ErrProductConflict), errors.Is(err, maintenance.ErrCheckpointConflict), errors.Is(err, errOwnerReviewChanged):
		code = "revision_conflict"
	case errors.Is(err, maintenance.ErrQualificationRequired):
		code = "qualification_required"
	case errors.Is(err, scheduler.ErrCapabilityUnavailable):
		code = "maintenance_unavailable"
	}
	metadata := map[string]any{"code": code, "retryable": false}
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
		value, err := b.MaintenanceStatus()
		return maintenanceResult(value, err)
	}
	if in.Operation == "maintenance.report" {
		service, err := b.maintenanceService()
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
	service, review, err := b.maintenanceReview(mutation)
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
