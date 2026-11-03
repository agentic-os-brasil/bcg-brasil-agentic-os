package maintenance

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const DarwinDailyJobID = "darwin-housekeeping-daily"
const DarwinWeeklyJobID = "darwin-deep-weekly"
const DarwinMonthlyJobID = "darwin-structural-evolution-proposal"

var ErrDarwinRecoveryRequired = errors.New("Darwin exact cache recovery required")

func DarwinJob(id string) bool {
	return id == DarwinDailyJobID || id == DarwinWeeklyJobID || id == DarwinMonthlyJobID
}

// DarwinScope exists only at native composition; none of these fields is tool input.
type DarwinScope struct {
	Root, WorkspaceID, Implementation, CacheConfiguration string
	Configuration                                         func(string) string
	Observe                                               func() (DarwinInputs, error)
}
type DarwinInputs struct {
	Runtime   RuntimeDiagnosis
	Scheduler []scheduler.Receipt
}
type DarwinParent struct {
	Present bool   `json:"present"`
	SHA256  string `json:"sha256,omitempty"`
}
type DarwinSource struct {
	ExternalSHA256 string           `json:"external_sha256"`
	Parent         DarwinParent     `json:"parent"`
	CacheState     string           `json:"cache_state"`
	Coverage       []SourceCoverage `json:"coverage"`
	Counts         map[string]int   `json:"counts"`
}

// DarwinReviewProjection explains the exact source commitment to the owner.
// Service derives it from the same snapshot as SourceState and seals both.
type DarwinReviewProjection struct {
	CacheState     string       `json:"cache_state"`
	Parent         DarwinParent `json:"parent"`
	ExternalSHA256 string       `json:"external_sha256"`
	Effect         string       `json:"effect"`
}
type DarwinRecoveryLineage struct {
	JobID                    string `json:"job_id"`
	OriginalOccurrenceDigest string `json:"original_occurrence_digest"`
	TransitionSHA256         string `json:"transition_sha256"`
}
type DarwinEvidence struct {
	Recovery           *DarwinRecoveryLineage `json:"recovery,omitempty"`
	SchemaVersion      int                    `json:"schema_version"`
	WorkspaceID        string                 `json:"workspace_id"`
	JobID              string                 `json:"job_id"`
	OccurrenceDigest   string                 `json:"occurrence_digest"`
	Implementation     string                 `json:"implementation"`
	Configuration      string                 `json:"configuration"`
	ExternalSHA256     string                 `json:"external_sha256"`
	SemanticSHA256     string                 `json:"semantic_sha256"`
	Before             DarwinParent           `json:"before"`
	After              DarwinParent           `json:"after"`
	Effect             string                 `json:"effect"`
	Outcome            ReceiptState           `json:"outcome"`
	Artifact           string                 `json:"artifact"`
	ArtifactSHA256     string                 `json:"artifact_sha256"`
	ProposalArtifactID string                 `json:"proposal_artifact_id,omitempty"`
	ProposalDigest     string                 `json:"proposal_digest,omitempty"`
	ProposalCount      int                    `json:"proposal_count"`
	ObservedAt         time.Time              `json:"observed_at"`
	Coverage           []SourceCoverage       `json:"coverage"`
	Counts             map[string]int         `json:"counts"`
}
type DarwinReport struct {
	RecoveryState     string         `json:"recovery_state,omitempty"`
	Evidence          DarwinEvidence `json:"evidence"`
	CacheState        string         `json:"cache_state"`
	NativeObservation string         `json:"native_observation"`
}
type DarwinBinding interface {
	Inspect(string) (DarwinSource, error)
	Handler(string, *DarwinSource, bool) Handler
	Evidence(string, string) (DarwinEvidence, error)
	Report(string) (DarwinReport, error)
}

func (s *Service) newDarwinBinding() (DarwinBinding, error) {
	if s.options.DarwinFactory == nil {
		return nil, nil
	}
	ids := []string{DarwinDailyJobID, DarwinWeeklyJobID, DarwinMonthlyJobID}
	configs := []string{"darwin-local-health-v1"}
	for _, id := range ids {
		configs = append(configs, s.configuration(id))
	}
	return s.options.DarwinFactory(DarwinScope{Root: filepath.Join(s.receipts.Root, "workspaces", s.workspaceID, "darwin"), WorkspaceID: s.workspaceID, Implementation: s.build, CacheConfiguration: productDigest(configs), Configuration: s.configuration, Observe: s.darwinInputs})
}

// This closed collector deliberately never enumerates Darwin's own job trees.
// Enumerate with hard entry limits before opening any receipt body.
func (s *Service) darwinInputs() (DarwinInputs, error) {
	diagnosis, err := s.runtimeDiagnosis()
	if err != nil {
		return DarwinInputs{}, err
	}
	out := DarwinInputs{Runtime: diagnosis}
	type selected struct{ job, name string }
	var files []selected
	total := 0
	for _, j := range s.options.Catalog.Jobs {
		if DarwinJob(j.ID) {
			continue
		}
		dir, err := scheduler.OpenDirectory(filepath.Join(s.scheduler.Root, "workspaces", s.workspaceID, "receipts", j.ID))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return out, err
		}
		entries, readErr := dir.ReadDir(129)
		dir.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return out, readErr
		}
		total += len(entries)
		if len(entries) > 128 || total > 256 {
			return out, errors.New("Darwin scheduler observation bound exceeded")
		}
		for _, entry := range entries {
			if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
				return out, errors.New("unsafe scheduler observation entry")
			}
			files = append(files, selected{j.ID, entry.Name()})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].job != files[j].job {
			return files[i].job < files[j].job
		}
		return files[i].name < files[j].name
	})
	for _, f := range files {
		r, err := s.scheduler.ReadReceipt(s.workspaceID, f.job, f.name)
		if err != nil {
			return out, err
		}
		out.Scheduler = append(out.Scheduler, r)
	}
	return out, nil
}

func (s *Service) darwinSource(id string) (DarwinSource, error) {
	if s.darwin == nil || !DarwinJob(id) || runtime.GOOS == "windows" {
		return DarwinSource{}, scheduler.ErrCapabilityUnavailable
	}
	return s.darwin.Inspect(id)
}

func (s *Service) qualifyDarwin(ctx context.Context, r ProductReview) (ProductJobStatus, error) {
	source, err := s.darwinSource(r.Operation.JobID)
	if err != nil || productDigest(source) != r.SourceState {
		return ProductJobStatus{}, ErrProductConflict
	}
	token, err := attemptID()
	if err != nil {
		return ProductJobStatus{}, err
	}
	var revision uint64
	err = s.withControl(r.Operation.JobID, func() error {
		c, e := s.loadControl(r.Operation.JobID)
		if e != nil {
			return e
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
	p := &productExecution{service: s, jobID: r.Operation.JobID, revision: revision, oneShot: token, darwinSource: &source}
	now := s.now().UTC()
	occ := scheduler.Occurrence{JobID: p.jobID, ScheduledFor: now}
	receipt, err := s.execute(ctx, p, occ, now)
	if err != nil {
		if report, e := s.darwin.Report(p.jobID); e == nil && report.RecoveryState != "" {
			return ProductJobStatus{}, errors.Join(ErrDarwinRecoveryRequired, err)
		}
		return ProductJobStatus{}, err
	}
	evidence, evidenceErr := s.darwin.Evidence(p.jobID, receipt.OccurrenceDigest)
	if receipt.State == ReceiptReviewedNoChange && errors.Is(evidenceErr, os.ErrNotExist) {
		err = p.guard(func() error {
			c, e := s.loadControl(p.jobID)
			if e != nil {
				return e
			}
			c.OneShot = ""
			c.Revision++
			return s.saveControl(c)
		})
		if err != nil {
			return ProductJobStatus{}, err
		}
		out, e := s.jobStatus(p.jobID)
		out.Reason = "reviewed_no_change"
		return out, e
	}
	if receipt.State != ReceiptSucceeded && receipt.State != ReceiptProposalEmitted && receipt.State != ReceiptReviewedNoChange {
		return ProductJobStatus{}, ErrQualificationRequired
	}
	if evidenceErr != nil {
		return ProductJobStatus{}, evidenceErr
	}
	q := ProductQualification{WorkspaceID: s.workspaceID, JobID: p.jobID, Binding: s.binding, Implementation: s.build, Configuration: s.configuration(p.jobID), Adapter: s.options.Adapter, Platform: runtime.GOOS + "/" + runtime.GOARCH, Artifact: evidence.Artifact, ArtifactSHA256: evidence.ArtifactSHA256, Receipt: receipt, ScheduledFor: now, RequestedAt: now, VerifiedAt: s.now().UTC()}
	q.ID = productDigest(q)
	if err = s.verifyQualification(&q); err != nil {
		return ProductJobStatus{}, err
	}
	err = p.guard(func() error {
		c, e := s.loadControl(p.jobID)
		if e != nil {
			return e
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

func (s *Service) verifyDarwinQualification(q *ProductQualification) error {
	if s.darwin == nil {
		return ErrQualificationRequired
	}
	unsigned := *q
	unsigned.ID = ""
	if q.ID != productDigest(unsigned) || q.WorkspaceID != s.workspaceID || q.Binding != s.binding || q.Implementation != s.build || q.Configuration != s.configuration(q.JobID) || q.Adapter != s.options.Adapter || q.Platform != runtime.GOOS+"/"+runtime.GOARCH || q.VerifiedAt.IsZero() || q.VerifiedAt.Before(q.RequestedAt) {
		return ErrQualificationRequired
	}
	if q.Receipt.Validate() != nil || q.Receipt.JobID != q.JobID || q.Receipt.WorkspaceID != s.workspaceID || q.Receipt.State != ReceiptSucceeded && q.Receipt.State != ReceiptProposalEmitted && q.Receipt.State != ReceiptReviewedNoChange || !q.Receipt.RecordedAt.Equal(q.RequestedAt) {
		return ErrQualificationRequired
	}
	evidence, err := s.darwin.Evidence(q.JobID, q.Receipt.OccurrenceDigest)
	if err != nil || evidence.Artifact != q.Artifact || evidence.ArtifactSHA256 != q.ArtifactSHA256 || evidence.Implementation != q.Implementation || evidence.Configuration != q.Configuration || evidence.Outcome != q.Receipt.State || !evidence.ObservedAt.Equal(q.RequestedAt) || evidence.ProposalArtifactID != q.Receipt.ProposalArtifactID || evidence.ProposalDigest != q.Receipt.ProposalDigest || evidence.ProposalCount != q.Receipt.ProposalCount {
		return ErrQualificationRequired
	}
	receiptPath := filepath.Join(s.receipts.Root, "workspaces", s.workspaceID, "receipts", q.JobID, q.Receipt.CommandID+"--"+q.Receipt.AttemptID+".json")
	body, err := scheduler.ReadPrivateStateFile(receiptPath, maximumReceiptBytes)
	var actual Receipt
	if err != nil || decodeProduct(body, &actual) != nil || actual != q.Receipt {
		return ErrQualificationRequired
	}
	name := q.RequestedAt.UTC().Format("20060102T150405.000000000Z") + "-" + q.ScheduledFor.UTC().Format("20060102T150405.000000000Z") + ".json"
	sr, err := s.scheduler.ReadReceipt(s.workspaceID, q.JobID, name)
	if err != nil || sr.State != scheduler.Succeeded {
		return ErrQualificationRequired
	}
	return nil
}
