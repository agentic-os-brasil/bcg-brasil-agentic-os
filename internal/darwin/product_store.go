package darwin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

type cacheRecovery struct {
	Kind                      string `json:"kind,omitempty"`
	CompletedOccurrenceDigest string `json:"completed_occurrence_digest,omitempty"`
	CompletedArtifactSHA256   string `json:"completed_artifact_sha256,omitempty"`

	JobID            string    `json:"job_id"`
	RequestedAt      time.Time `json:"requested_at"`
	ScheduledFor     time.Time `json:"scheduled_for,omitzero"`
	SchemaVersion    int       `json:"schema_version"`
	WorkspaceID      string    `json:"workspace_id"`
	ExternalSHA256   string    `json:"external_sha256"`
	OccurrenceDigest string    `json:"occurrence_digest"`
	Before           []byte    `json:"before"`
	After            []byte    `json:"after"`
	Phase            string    `json:"phase"`
}

func (j cacheRecovery) transitionSHA256() string {
	identity := []any{j.SchemaVersion, j.WorkspaceID, j.JobID, j.RequestedAt, j.OccurrenceDigest, j.ExternalSHA256, j.Before, j.After, j.Kind}
	// Existing completed R1 records keep their exact transition identity. New
	// records additionally bind the independently admitted scheduled instant.
	if !j.ScheduledFor.IsZero() {
		identity = append(identity, "scheduled_for", j.ScheduledFor.UTC())
	}
	return productHash(identity)
}

func originalOccurrenceDigest(job string, scheduled time.Time) string {
	trigger := maintenance.TriggerPresence
	if job == maintenance.DarwinMonthlyJobID {
		trigger = maintenance.TriggerMonthly
	}
	return (maintenance.Command{JobID: job, Trigger: trigger, ScheduledFor: scheduled}).OccurrenceDigest()
}

// A legacy partial without an explicit schedule can recover it only from its
// actual original immutable proposal. Request time is never schedule authority.
func (b *productBinding) originalProposalSchedule(j cacheRecovery) (time.Time, error) {
	if !j.ScheduledFor.IsZero() {
		return j.ScheduledFor, nil
	}
	path := filepath.Join(b.scope.Root, "proposals", assessmentArtifactID(j.JobID, j.OccurrenceDigest)+".json")
	body, err := scheduler.ReadPrivateStateFile(path, productCacheLimit)
	if err != nil {
		return time.Time{}, errors.Join(ErrProductRecovery, err)
	}
	var artifact AssessmentProposalArtifact
	if productDecode(body, &artifact) != nil || artifact.Validate() != nil || artifact.JobID != j.JobID || artifact.OccurrenceDigest != j.OccurrenceDigest || originalOccurrenceDigest(j.JobID, artifact.ScheduledFor) != j.OccurrenceDigest {
		return time.Time{}, ErrProductRecovery
	}
	p := artifact.Product
	if p == nil || p.WorkspaceID != j.WorkspaceID || p.Implementation != b.scope.Implementation || p.Configuration != b.scope.Configuration(j.JobID) || p.ExternalSHA256 != j.ExternalSHA256 {
		return time.Time{}, ErrProductRecovery
	}
	return artifact.ScheduledFor, nil
}
func journalParent(body []byte) maintenance.DarwinParent {
	p := maintenance.DarwinParent{Present: len(body) > 0}
	if p.Present {
		p.SHA256 = productBytesHash(body)
	}
	return p
}
func (b *productBinding) validRecovery(j cacheRecovery) bool {
	if !j.ScheduledFor.IsZero() && (j.ScheduledFor.After(j.RequestedAt) || originalOccurrenceDigest(j.JobID, j.ScheduledFor) != j.OccurrenceDigest) {
		return false
	}
	return j.SchemaVersion == 1 && j.WorkspaceID == b.scope.WorkspaceID && maintenance.DarwinJob(j.JobID) && !j.RequestedAt.IsZero() && validEvolutionSHA(j.OccurrenceDigest) && validEvolutionSHA(j.ExternalSHA256) && (j.Kind == "" || j.Kind == "cache" || j.Kind == "assessment") && (j.Phase == "prepared" || j.Phase == "committed" || j.Phase == "compensated") && (j.CompletedOccurrenceDigest == "") == (j.CompletedArtifactSHA256 == "") && (j.CompletedOccurrenceDigest == "" || validEvolutionSHA(j.CompletedOccurrenceDigest) && validEvolutionSHA(j.CompletedArtifactSHA256))
}
func (b *productBinding) activeEvidence(job string) (maintenance.DarwinEvidence, error) {
	body, err := scheduler.ReadPrivateStateFile(filepath.Join(b.evidenceRoot(job), "current.json"), 1024)
	if err != nil {
		return maintenance.DarwinEvidence{}, err
	}
	var pointer struct {
		Occurrence string `json:"occurrence"`
	}
	if productDecode(body, &pointer) != nil {
		return maintenance.DarwinEvidence{}, ErrProductRecovery
	}
	return b.Evidence(job, pointer.Occurrence)
}
func (b *productBinding) recoveryClosed(j cacheRecovery) (bool, error) {
	current, err := b.activeEvidence(j.JobID)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if j.CompletedOccurrenceDigest == "" {
		return current.OccurrenceDigest == j.OccurrenceDigest && current.ExternalSHA256 == j.ExternalSHA256 && current.After == journalParent(j.After), nil
	}
	if current.OccurrenceDigest != j.CompletedOccurrenceDigest || current.ArtifactSHA256 != j.CompletedArtifactSHA256 {
		return false, ErrProductRecovery
	}
	lineage := current.Recovery
	if lineage == nil || lineage.JobID != j.JobID || lineage.OriginalOccurrenceDigest != j.OccurrenceDigest || lineage.TransitionSHA256 != j.transitionSHA256() || current.ExternalSHA256 != j.ExternalSHA256 || current.Before != journalParent(j.Before) || current.After != journalParent(j.After) {
		return false, ErrProductRecovery
	}
	return true, nil
}
func (b *productBinding) recoverPrepared(job string, source *maintenance.DarwinSource, guard func(func() error) error) (*cacheRecovery, error) {
	body, err := scheduler.ReadPrivateStateFile(filepath.Join(b.scope.Root, "managed-state", "recovery.json"), productCacheLimit)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var j cacheRecovery
	if productDecode(body, &j) != nil || !b.validRecovery(j) {
		return nil, ErrProductRecovery
	}
	if j.Phase == "compensated" {
		return nil, nil
	}
	closed, err := b.recoveryClosed(j)
	if err != nil {
		return nil, err
	}
	if closed {
		return nil, nil
	}
	actual, err := scheduler.ReadPrivateStateFile(b.cachePath(), productCacheLimit)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if j.Kind != "assessment" && bytes.Equal(actual, j.Before) {
		return nil, nil
	}
	if j.JobID != job || source.ExternalSHA256 != j.ExternalSHA256 || !bytes.Equal(actual, j.After) {
		return nil, ErrProductRecovery
	}
	return &j, nil
}
func (b *productBinding) closeRecovery(j cacheRecovery, e maintenance.DarwinEvidence, guard func(func() error) error) error {
	if e.Recovery == nil || e.Recovery.TransitionSHA256 != j.transitionSHA256() {
		return ErrProductRecovery
	}
	verified, err := b.Evidence(e.JobID, e.OccurrenceDigest)
	if err != nil {
		return err
	}
	if verified.Before != journalParent(j.Before) || verified.After != journalParent(j.After) || verified.ExternalSHA256 != j.ExternalSHA256 {
		return ErrProductRecovery
	}
	active, err := b.activeEvidence(e.JobID)
	if err != nil || active.ArtifactSHA256 != verified.ArtifactSHA256 {
		return ErrProductRecovery
	}
	return scheduler.UpdatePrivateFileGuarded(filepath.Join(b.scope.Root, "managed-state", "recovery.json"), productCacheLimit, guard, func(body []byte) ([]byte, error) {
		var old cacheRecovery
		if productDecode(body, &old) != nil || old.transitionSHA256() != j.transitionSHA256() || old.CompletedOccurrenceDigest != "" && old.CompletedOccurrenceDigest != e.OccurrenceDigest {
			return nil, ErrProductRecovery
		}
		old.Phase = "committed"
		old.CompletedOccurrenceDigest = e.OccurrenceDigest
		old.CompletedArtifactSHA256 = verified.ArtifactSHA256
		return json.Marshal(old)
	})
}

// Assessment-only work retains its original occurrence in this same owner,
// including when it changes no cache or the cache is absent.
func (b *productBinding) prepareAssessment(command maintenance.Command, source maintenance.DarwinSource, guard func(func() error) error) error {
	actual, err := scheduler.ReadPrivateStateFile(b.cachePath(), productCacheLimit)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	j := cacheRecovery{JobID: command.JobID, RequestedAt: command.RequestedAt, ScheduledFor: command.ScheduledFor.UTC(), SchemaVersion: 1, WorkspaceID: b.scope.WorkspaceID, ExternalSHA256: source.ExternalSHA256, OccurrenceDigest: command.OccurrenceDigest(), Before: actual, After: actual, Phase: "prepared", Kind: "assessment"}
	return scheduler.UpdatePrivateFileGuarded(filepath.Join(b.scope.Root, "managed-state", "recovery.json"), productCacheLimit, guard, func(body []byte) ([]byte, error) {
		if len(body) > 0 {
			var previous cacheRecovery
			if productDecode(body, &previous) != nil || !b.validRecovery(previous) {
				return nil, ErrProductRecovery
			}
			if previous.JobID == command.JobID && previous.OccurrenceDigest == command.OccurrenceDigest() {
				return nil, nil
			}
			if previous.Phase != "compensated" {
				closed, e := b.recoveryClosed(previous)
				if e != nil {
					return nil, e
				}
				if !closed {
					return nil, ErrProductRecovery
				}
			}
		}
		return json.Marshal(j)
	})
}

// First creation publishes exclusively. Existing bytes and failed readback are
// retained: this profile does not prove physical custody for replacement or
// compensation against an uncooperative writer.
func (invoker ManagedStateRepairInvoker) refreshProduct(previous, next []byte, guard func(func() error) error, validate func() error) error {
	if len(previous) != 0 {
		return maintenance.ErrDarwinRepairUnavailable
	}
	if guard == nil {
		return errors.New("Darwin publication guard required")
	}
	path := filepath.Join(invoker.Root, managedStateRelativePath)
	if err := guard(func() error { return scheduler.PublishNewPrivateFile(path, next) }); err != nil {
		return err
	}
	body, err := scheduler.ReadPrivateStateFile(path, productCacheLimit)
	if err == nil && !bytes.Equal(body, next) {
		err = errors.New("Darwin cache readback mismatch")
	}
	if err == nil && validate != nil {
		err = validate()
	}
	if err != nil {
		return errors.Join(ErrProductRecovery, err)
	}
	return nil
}

func (b *productBinding) refresh(ctx context.Context, source *maintenance.DarwinSource, command maintenance.Command, guard func(func() error) error) error {
	health, err := b.health()
	if err != nil || health.ExternalSHA256 != source.ExternalSHA256 {
		return maintenance.ErrProductConflict
	}
	var previous []byte
	_, previous, err = b.readCache()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if (len(previous) > 0) != source.Parent.Present || len(previous) > 0 && productBytesHash(previous) != source.Parent.SHA256 {
		return maintenance.ErrProductConflict
	}
	desired, err := json.Marshal(health)
	if err != nil {
		return err
	}
	journal := cacheRecovery{Kind: "cache", JobID: command.JobID, RequestedAt: command.RequestedAt, ScheduledFor: command.ScheduledFor.UTC(), SchemaVersion: 1, WorkspaceID: b.scope.WorkspaceID, ExternalSHA256: source.ExternalSHA256, OccurrenceDigest: command.OccurrenceDigest(), Before: previous, After: desired, Phase: "prepared"}
	journalPath := filepath.Join(b.scope.Root, "managed-state", "recovery.json")
	if err = scheduler.UpdatePrivateFileGuarded(journalPath, productCacheLimit, guard, func(body []byte) ([]byte, error) {
		if len(body) > 0 {
			var old cacheRecovery
			if productDecode(body, &old) != nil || old.SchemaVersion != 1 || old.WorkspaceID != b.scope.WorkspaceID {
				return nil, ErrProductRecovery
			}
			if old.Phase == "prepared" && !bytes.Equal(old.Before, previous) && !bytes.Equal(old.After, previous) {
				return nil, ErrProductRecovery
			}
		}
		return json.Marshal(journal)
	}); err != nil {
		return err
	}
	before := *source
	// After the rename, the same execution may publish only its exact own cache
	// transition. Compensation checks under that updated live parent identity.
	liveGuard := func(publish func() error) error {
		return guard(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			err := publish()
			if err == nil {
				actual, e := scheduler.ReadPrivateStateFile(b.cachePath(), productCacheLimit)
				if errors.Is(e, os.ErrNotExist) && previous == nil {
					*source = before
				} else if e == nil && bytes.Equal(actual, previous) {
					*source = before
				} else if e == nil && bytes.Equal(actual, desired) {
					source.Parent = maintenance.DarwinParent{Present: true, SHA256: productBytesHash(desired)}
					source.CacheState = "current"
				} else {
					return ErrProductRecovery
				}
			}
			return err
		})
	}
	err = (ManagedStateRepairInvoker{Root: b.scope.Root}).refreshProduct(previous, desired, liveGuard, func() error {
		current, e := b.health()
		if e != nil {
			return e
		}
		if !reflect.DeepEqual(current, health) {
			return maintenance.ErrProductConflict
		}
		return nil
	})
	if err != nil {
		actual, e := scheduler.ReadPrivateStateFile(b.cachePath(), productCacheLimit)
		if errors.Is(e, os.ErrNotExist) && previous == nil || e == nil && bytes.Equal(actual, previous) {
			*source = before
			journal.Phase = "compensated"
		} else {
			return errors.Join(ErrProductRecovery, err)
		}
	} else {
		journal.Phase = "committed"
	}
	if writeErr := scheduler.UpdatePrivateFileGuarded(journalPath, productCacheLimit, guard, func(body []byte) ([]byte, error) {
		var actual cacheRecovery
		if productDecode(body, &actual) != nil || actual.OccurrenceDigest != journal.OccurrenceDigest {
			return nil, ErrProductRecovery
		}
		return json.Marshal(journal)
	}); writeErr != nil {
		return errors.Join(ErrProductRecovery, err, writeErr)
	}
	return err
}

func (b *productBinding) evidenceRoot(job string) string {
	return filepath.Join(b.scope.Root, "reports", job)
}
func (b *productBinding) publishEvidence(e maintenance.DarwinEvidence, guard func(func() error) error) error {
	root := b.evidenceRoot(e.JobID)
	if err := scheduler.EnsurePrivateDirectory(filepath.Join(root, "versions")); err != nil {
		return err
	}
	e.Artifact = "darwin/reports/" + e.JobID + "/versions/" + e.OccurrenceDigest + ".json"
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if len(body) > productCacheLimit {
		return errors.New("Darwin evidence bound")
	}
	path := filepath.Join(root, "versions", e.OccurrenceDigest+".json")
	if err = guard(func() error { return scheduler.PublishNewPrivateFile(path, body) }); err != nil {
		existing, readErr := scheduler.ReadPrivateStateFile(path, productCacheLimit)
		if !errors.Is(err, os.ErrExist) || readErr != nil || !bytes.Equal(body, existing) {
			return err
		}
	}
	pointer, _ := json.Marshal(struct {
		Occurrence string `json:"occurrence"`
	}{e.OccurrenceDigest})
	return scheduler.UpdatePrivateFileGuarded(filepath.Join(root, "current.json"), 1024, guard, func([]byte) ([]byte, error) { return pointer, nil })
}
func (b *productBinding) Evidence(job, occurrence string) (maintenance.DarwinEvidence, error) {
	var e maintenance.DarwinEvidence
	if !maintenance.DarwinJob(job) || !validEvolutionSHA(occurrence) {
		return e, errors.New("invalid Darwin evidence selector")
	}
	path := filepath.Join(b.evidenceRoot(job), "versions", occurrence+".json")
	body, err := scheduler.ReadPrivateStateFile(path, productCacheLimit)
	if err != nil {
		return e, err
	}
	if err = productDecode(body, &e); err != nil {
		return e, err
	}
	if e.SchemaVersion != 1 || e.WorkspaceID != b.scope.WorkspaceID || e.JobID != job || e.OccurrenceDigest != occurrence || !validEvolutionSHA(e.Implementation) || !validEvolutionSHA(e.Configuration) || !validEvolutionSHA(e.ExternalSHA256) || !validEvolutionSHA(e.SemanticSHA256) || e.Counts["observations"] < 2 || e.ObservedAt.IsZero() || e.Artifact != "darwin/reports/"+job+"/versions/"+occurrence+".json" || e.ArtifactSHA256 != "" {
		return e, errors.New("invalid Darwin evidence binding")
	}
	if e.Effect != "initialized" && e.Effect != "refreshed" && e.Effect != "assessed" {
		return e, errors.New("invalid Darwin effect")
	}
	if r := e.Recovery; r != nil {
		if r.JobID != job || !validEvolutionSHA(r.OriginalOccurrenceDigest) || !validEvolutionSHA(r.TransitionSHA256) {
			return e, errors.New("invalid recovered transition lineage")
		}
	}
	if e.Outcome != maintenance.ReceiptSucceeded && e.Outcome != maintenance.ReceiptProposalEmitted && e.Outcome != maintenance.ReceiptReviewedNoChange {
		return e, errors.New("invalid Darwin outcome")
	}
	if e.Outcome == maintenance.ReceiptProposalEmitted {
		receipt := maintenance.Receipt{State: e.Outcome, JobID: job, OccurrenceDigest: occurrence, ProposalArtifactID: e.ProposalArtifactID, ProposalDigest: e.ProposalDigest, ProposalCount: e.ProposalCount}
		if err = (ProposalStore{Root: b.scope.Root}).ValidateProductReceipt(receipt, e); err != nil {
			return e, err
		}
	} else if e.ProposalCount != 0 || e.ProposalDigest != "" || e.ProposalArtifactID != "" {
		return e, errors.New("unexpected Darwin proposal binding")
	}
	e.ArtifactSHA256 = productBytesHash(body)
	return e, nil
}
func (b *productBinding) Report(job string) (maintenance.DarwinReport, error) {
	var out maintenance.DarwinReport
	if !maintenance.DarwinJob(job) {
		return out, errors.New("unknown Darwin report")
	}
	// A committed cache rename is an actual effect even if a later live fence
	// denies its report/terminal. Read the exact retained transition; never
	// manufacture a successful receipt or silently restore/relabel those bytes.
	if report, ok, err := b.pendingEffectReport(job); err != nil {
		return out, err
	} else if ok {
		return report, nil
	}
	body, err := scheduler.ReadPrivateStateFile(filepath.Join(b.evidenceRoot(job), "current.json"), 1024)
	if err != nil {
		return out, err
	}
	var pointer struct {
		Occurrence string `json:"occurrence"`
	}
	if err = productDecode(body, &pointer); err != nil {
		return out, err
	}
	e, err := b.Evidence(job, pointer.Occurrence)
	if err != nil {
		return out, err
	}
	source, err := b.Inspect(job)
	if err != nil {
		return out, err
	}
	return maintenance.DarwinReport{Evidence: e, CacheState: source.CacheState, NativeObservation: "not_observed"}, nil
}

func (b *productBinding) pendingEffectReport(job string) (maintenance.DarwinReport, bool, error) {
	var out maintenance.DarwinReport
	body, err := scheduler.ReadPrivateStateFile(filepath.Join(b.scope.Root, "managed-state", "recovery.json"), productCacheLimit)
	if errors.Is(err, os.ErrNotExist) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	var j cacheRecovery
	if productDecode(body, &j) != nil || !b.validRecovery(j) {
		return out, false, ErrProductRecovery
	}
	if j.JobID != job || j.Phase == "compensated" {
		return out, false, nil
	}
	closed, err := b.recoveryClosed(j)
	if err != nil {
		return out, false, err
	}
	if closed {
		return out, false, nil
	}
	actual, err := scheduler.ReadPrivateStateFile(b.cachePath(), productCacheLimit)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return out, false, err
	}
	if j.Kind != "assessment" && bytes.Equal(actual, j.Before) {
		return out, false, nil
	}
	if j.Kind == "assessment" {
		path := filepath.Join(b.scope.Root, "proposals", assessmentArtifactID(job, j.OccurrenceDigest)+".json")
		if _, e := scheduler.ReadPrivateStateFile(path, productCacheLimit); errors.Is(e, os.ErrNotExist) {
			if _, e = b.Evidence(job, j.OccurrenceDigest); errors.Is(e, os.ErrNotExist) {
				return out, false, nil
			} else if e != nil {
				return out, false, e
			}
		} else if e != nil {
			return out, false, e
		}
	}
	state := "required"
	if !bytes.Equal(actual, j.After) {
		state = "parent_changed"
	}
	out = maintenance.DarwinReport{RecoveryState: state, CacheState: "recovery_required", NativeObservation: "not_observed", Evidence: maintenance.DarwinEvidence{SchemaVersion: 1, JobID: job, WorkspaceID: b.scope.WorkspaceID, OccurrenceDigest: j.OccurrenceDigest, ExternalSHA256: j.ExternalSHA256, Before: journalParent(j.Before), After: journalParent(actual), Outcome: maintenance.ReceiptRecoveryRequired, ObservedAt: j.RequestedAt}}
	return out, true, nil
}

// Product paths use the same ProposalStore artifact/validation owner with
// held private readers and exclusive atomic publication, not the old Lstat IO.
func (store ProposalStore) AppendProduct(artifact AssessmentProposalArtifact) error {
	if artifact.Product == nil {
		return errors.New("product assessment provenance required")
	}
	if err := artifact.Validate(); err != nil {
		return err
	}
	root := filepath.Join(store.Root, "proposals")
	if err := scheduler.EnsurePrivateDirectory(root); err != nil {
		return err
	}
	body, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	path := filepath.Join(root, artifact.ArtifactID+".json")
	err = scheduler.PublishNewPrivateFile(path, body)
	if errors.Is(err, os.ErrExist) {
		actual, e := scheduler.ReadPrivateStateFile(path, productCacheLimit)
		if e == nil && bytes.Equal(actual, body) {
			return nil
		}
		return ErrProposalReplayConflict
	}
	return err
}
func (store ProposalStore) ValidateProductReceipt(receipt maintenance.Receipt, evidence maintenance.DarwinEvidence) error {
	if !validEvolutionSHA(receipt.ProposalArtifactID) {
		return errors.New("invalid product proposal ID")
	}
	body, err := scheduler.ReadPrivateStateFile(filepath.Join(store.Root, "proposals", receipt.ProposalArtifactID+".json"), productCacheLimit)
	if err != nil {
		return err
	}
	var artifact AssessmentProposalArtifact
	if err = productDecode(body, &artifact); err != nil {
		return err
	}
	origin := receipt.OccurrenceDigest
	if evidence.Recovery != nil {
		if evidence.Recovery.JobID != receipt.JobID || !validEvolutionSHA(evidence.Recovery.TransitionSHA256) {
			return ErrProductRecovery
		}
		origin = evidence.Recovery.OriginalOccurrenceDigest
	}
	if artifact.Validate() != nil || artifact.OccurrenceDigest != origin || artifact.JobID != receipt.JobID || artifact.ProposalDigest != receipt.ProposalDigest || len(artifact.Assessment.Proposals) != receipt.ProposalCount {
		return errors.New("product proposal readback mismatch")
	}
	p := artifact.Product
	if p == nil || p.WorkspaceID != evidence.WorkspaceID || p.Implementation != evidence.Implementation || p.Configuration != evidence.Configuration || p.ExternalSHA256 != evidence.ExternalSHA256 {
		return errors.New("product proposal provenance mismatch")
	}
	return nil
}
