package darwin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const productCacheProfile = "darwin-local-health-v1"
const productCacheLimit = 64 << 10

var ErrProductRecovery = maintenance.ErrDarwinRecoveryRequired

type productBinding struct{ scope maintenance.DarwinScope }
type localHealth struct {
	SchemaVersion     int                          `json:"schema_version"`
	Profile           string                       `json:"profile"`
	WorkspaceID       string                       `json:"workspace_id"`
	Implementation    string                       `json:"implementation"`
	Configuration     string                       `json:"configuration"`
	ExternalSHA256    string                       `json:"external_sha256"`
	NativeObservation string                       `json:"native_observation"`
	Coverage          []maintenance.SourceCoverage `json:"coverage"`
	Counts            map[string]int               `json:"counts"`
}

func productHash(value any) string { body, _ := json.Marshal(value); return productBytesHash(body) }
func productBytesHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func productDecode(body []byte, value any) error {
	if len(body) == 0 || len(body) > productCacheLimit {
		return errors.New("Darwin record size")
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("Darwin record trailing data")
	}
	return nil
}

// NewProductBinding admits only a scope constructed by maintenance. It is not
// a tool surface, health override, general path API or grant issuer.
func NewProductBinding(scope maintenance.DarwinScope) (maintenance.DarwinBinding, error) {
	if !filepath.IsAbs(scope.Root) || !idPattern.MatchString(scope.WorkspaceID) || !validEvolutionSHA(scope.Implementation) || !validEvolutionSHA(scope.CacheConfiguration) || scope.Observe == nil || scope.Configuration == nil {
		return nil, errors.New("invalid Darwin product scope")
	}
	return &productBinding{scope: scope}, nil
}
func (b *productBinding) cachePath() string {
	return filepath.Join(b.scope.Root, managedStateRelativePath)
}
func (b *productBinding) health() (localHealth, error) {
	in, err := b.scope.Observe()
	if err != nil {
		return localHealth{}, err
	}
	r := in.Runtime
	observed := 0
	coverage := []maintenance.SourceCoverage{}
	for _, o := range r.Observations {
		if o.State == "observed" && validEvolutionSHA(o.SHA256) {
			observed++
		}
		coverage = append(coverage, maintenance.SourceCoverage{Source: o.Source, State: o.State})
	}
	if observed < 2 || r.NativeObservation != "not_observed" || r.CapabilityManifest.Validate() != nil {
		return localHealth{}, errors.New("Darwin diagnosis lacks genuine observations")
	}
	coverage = append(coverage, maintenance.SourceCoverage{Source: "native", State: "not_observed"}, maintenance.SourceCoverage{Source: "state_documents", State: "unavailable"}, maintenance.SourceCoverage{Source: "modern_readiness", State: "unavailable"})
	counts := map[string]int{"observations": observed, "upstream_receipts": len(in.Scheduler), "capabilities_unavailable": len(r.Capabilities), "scheduler_failures": 0, "projection_conflicts": 0}
	if r.Projection != nil {
		counts["projection_conflicts"] = len(r.Projection.Conflicts)
	}
	for _, receipt := range in.Scheduler {
		if receipt.State == scheduler.Failed || receipt.State == scheduler.Unavailable {
			counts["scheduler_failures"]++
		}
	}
	// Only closed body-free observations enter the cache. Hash the complete
	// canonical runtime diagnosis for provenance; paths remain only in this frame.
	source := productHash([]any{productCacheProfile, b.scope.WorkspaceID, b.scope.Implementation, b.scope.CacheConfiguration, r, in.Scheduler})
	return localHealth{1, productCacheProfile, b.scope.WorkspaceID, b.scope.Implementation, b.scope.CacheConfiguration, source, "not_observed", coverage, counts}, nil
}
func (b *productBinding) readCache() (localHealth, []byte, error) {
	body, err := scheduler.ReadPrivateStateFile(b.cachePath(), productCacheLimit)
	var cache localHealth
	if err != nil {
		return cache, nil, err
	}
	if err = productDecode(body, &cache); err != nil {
		return cache, nil, err
	}
	if cache.SchemaVersion != 1 || cache.Profile != productCacheProfile || cache.WorkspaceID != b.scope.WorkspaceID || !validEvolutionSHA(cache.Implementation) || !validEvolutionSHA(cache.Configuration) || !validEvolutionSHA(cache.ExternalSHA256) || cache.NativeObservation != "not_observed" || cache.Counts["observations"] < 2 || len(cache.Coverage) > 32 {
		return cache, nil, errors.New("invalid Darwin health cache")
	}
	return cache, body, nil
}
func (b *productBinding) Inspect(job string) (maintenance.DarwinSource, error) {
	if !maintenance.DarwinJob(job) {
		return maintenance.DarwinSource{}, errors.New("unknown Darwin job")
	}
	health, err := b.health()
	if err != nil {
		return maintenance.DarwinSource{}, err
	}
	source := maintenance.DarwinSource{ExternalSHA256: health.ExternalSHA256, CacheState: "absent", Coverage: health.Coverage, Counts: health.Counts}
	cache, body, err := b.readCache()
	if errors.Is(err, os.ErrNotExist) {
		return source, nil
	}
	if err != nil {
		return source, err
	}
	source.Parent = maintenance.DarwinParent{Present: true, SHA256: productBytesHash(body)}
	source.CacheState = "stale"
	if reflect.DeepEqual(cache, health) {
		source.CacheState = "current"
	}
	return source, nil
}
func (b *productBinding) Handler(job string, source *maintenance.DarwinSource, attended bool) maintenance.Handler {
	return maintenance.HandlerFunc(func(ctx context.Context, command maintenance.Command, grant maintenance.ExecutionGrant) (maintenance.HandlerResult, error) {
		if runtime.GOOS == "windows" {
			return maintenance.HandlerResult{}, errors.New("Darwin private publication unavailable on Windows")
		}
		if source == nil || !maintenance.DarwinJob(job) || command.JobID != job || command.WorkspaceID != b.scope.WorkspaceID || job == maintenance.DarwinMonthlyJobID && !attended {
			return maintenance.HandlerResult{}, errors.New("Darwin scope/authority mismatch")
		}
		if err := maintenance.ValidateExecutionGrant(grant, command); err != nil {
			return maintenance.HandlerResult{}, err
		}
		var result maintenance.HandlerResult
		err := scheduler.WithPrivateLock(filepath.Join(b.scope.Root, "publication.guard"), func() error {
			guard := func(publish func() error) error {
				return maintenance.PublishExecutionGrant(ctx, grant, command, func() error {
					current, e := b.Inspect(job)
					if e != nil || !reflect.DeepEqual(current, *source) {
						return maintenance.ErrProductConflict
					}
					return publish()
				})
			}
			current, err := b.Inspect(job)
			if err != nil || !reflect.DeepEqual(current, *source) {
				return maintenance.ErrProductConflict
			}
			// Recover an immutable same-occurrence result before rebuilding a plan.
			if evidence, e := b.Evidence(job, command.OccurrenceDigest()); e == nil {
				if evidence.ExternalSHA256 != source.ExternalSHA256 {
					return maintenance.ErrProductConflict
				}
				evidence.ArtifactSHA256 = ""
				if err = b.publishEvidence(evidence, guard); err != nil {
					return err
				}
				if evidence.Recovery != nil {
					body, e := scheduler.ReadPrivateStateFile(filepath.Join(b.scope.Root, "managed-state", "recovery.json"), productCacheLimit)
					if e != nil {
						return e
					}
					var j cacheRecovery
					if productDecode(body, &j) != nil {
						return ErrProductRecovery
					}
					if err = b.closeRecovery(j, evidence, guard); err != nil {
						return err
					}
				}
				result = productResult(evidence)
				return nil
			} else if !errors.Is(e, os.ErrNotExist) {
				return e
			}
			before := source.Parent
			effect := "assessed"
			recovered, e := b.recoverPrepared(job, source, guard)
			if e != nil {
				return e
			}
			if recovered != nil {
				before = journalParent(recovered.Before)
				if recovered.Kind != "assessment" {
					effect = "refreshed"
					if !before.Present {
						effect = "initialized"
					}
				}
			}
			if job != maintenance.DarwinMonthlyJobID && source.CacheState != "current" {
				if source.CacheState == "absent" && !attended {
					return errors.New("Darwin cache initialization requires attended review")
				}
				if err = b.refresh(ctx, source, command, guard); err != nil {
					return err
				}
				effect = "refreshed"
				if !before.Present {
					effect = "initialized"
				}
			}
			proposalOccurrence, proposalTime := command.OccurrenceDigest(), command.ScheduledFor
			if recovered != nil {
				proposalOccurrence = recovered.OccurrenceDigest
				if job != maintenance.DarwinDailyJobID {
					proposalTime, e = b.originalProposalSchedule(*recovered)
					if e != nil {
						return e
					}
				}
			}
			packet := HealthPacket{SchemaVersion: 1, WindowID: "product-" + proposalOccurrence[:16], Runtime: "runtime-neutral", Mode: DeepReview}
			if source.Counts["capabilities_unavailable"] > 0 {
				packet.Observations = append(packet.Observations, Observation{Code: ObservationCapabilityUnavailable, Severity: SeverityMedium, Count: source.Counts["capabilities_unavailable"], State: "unavailable"})
			}
			if source.Counts["scheduler_failures"] > 0 {
				packet.Observations = append(packet.Observations, Observation{Code: ObservationSchedulerMissed, Severity: SeverityMedium, Count: source.Counts["scheduler_failures"], State: "warning"})
			}
			if source.Counts["projection_conflicts"] > 0 {
				packet.Observations = append(packet.Observations, Observation{Code: ObservationContractDrift, Severity: SeverityHigh, Count: source.Counts["projection_conflicts"], State: "failed"})
			}
			assessment, err := Plan(packet)
			if err != nil {
				return err
			}
			semantic := productHash([]any{job, source.ExternalSHA256, b.scope.Configuration(job)})
			previous, e := b.Report(job)
			if e == nil && previous.Evidence.SemanticSHA256 == semantic && effect == "assessed" {
				result = maintenance.HandlerResult{State: maintenance.ReceiptReviewedNoChange, ReasonCode: maintenance.ReasonReviewedNoChange}
				return nil
			}
			if e != nil && !errors.Is(e, os.ErrNotExist) {
				return e
			}
			evidence := maintenance.DarwinEvidence{SchemaVersion: 1, WorkspaceID: b.scope.WorkspaceID, JobID: job, OccurrenceDigest: command.OccurrenceDigest(), Implementation: b.scope.Implementation, Configuration: b.scope.Configuration(job), ExternalSHA256: source.ExternalSHA256, SemanticSHA256: semantic, Before: before, After: source.Parent, Effect: effect, Outcome: maintenance.ReceiptSucceeded, ObservedAt: command.RequestedAt, Coverage: source.Coverage, Counts: source.Counts}
			if recovered != nil {
				evidence.Recovery = &maintenance.DarwinRecoveryLineage{JobID: job, OriginalOccurrenceDigest: recovered.OccurrenceDigest, TransitionSHA256: recovered.transitionSHA256()}
			}
			if job != maintenance.DarwinDailyJobID {
				evidence.Outcome = maintenance.ReceiptReviewedNoChange
				if len(assessment.Proposals) > 0 {
					if recovered == nil {
						if err = b.prepareAssessment(command, *source, guard); err != nil {
							return err
						}
					}
					store := ProposalStore{Root: b.scope.Root}
					artifact := AssessmentProposalArtifact{SchemaVersion: 1, RecordType: "assessment", AgentID: AgentID, JobID: job, OccurrenceDigest: proposalOccurrence, ArtifactID: assessmentArtifactID(job, proposalOccurrence), WindowID: assessment.WindowID, ProposalDigest: proposalDigest(proposalOccurrence, assessment), Assessment: assessment, ScheduledFor: proposalTime.UTC(), RecordedAt: proposalTime.UTC()}
					artifact.Product = &ProductAssessmentProvenance{WorkspaceID: b.scope.WorkspaceID, Implementation: b.scope.Implementation, Configuration: b.scope.Configuration(job), ExternalSHA256: source.ExternalSHA256}
					if err = guard(func() error { return store.AppendProduct(artifact) }); err != nil {
						return err
					}
					evidence.Outcome = maintenance.ReceiptProposalEmitted
					evidence.ProposalArtifactID = artifact.ArtifactID
					evidence.ProposalDigest = artifact.ProposalDigest
					evidence.ProposalCount = len(assessment.Proposals)
				}
			}
			if err = b.publishEvidence(evidence, guard); err != nil {
				return err
			}
			if _, err = b.Evidence(job, command.OccurrenceDigest()); err != nil {
				return err
			}
			if recovered != nil {
				if err = b.closeRecovery(*recovered, evidence, guard); err != nil {
					return err
				}
			}
			result = productResult(evidence)
			return nil
		})
		return result, err
	})
}
func productResult(e maintenance.DarwinEvidence) maintenance.HandlerResult {
	reason := maintenance.ReasonCompleted
	if e.Outcome == maintenance.ReceiptProposalEmitted {
		reason = maintenance.ReasonProposalEmitted
	}
	if e.Outcome == maintenance.ReceiptReviewedNoChange {
		reason = maintenance.ReasonReviewedNoChange
	}
	return maintenance.HandlerResult{State: e.Outcome, ReasonCode: reason, ProposalArtifactID: e.ProposalArtifactID, ProposalDigest: e.ProposalDigest, ProposalCount: e.ProposalCount}
}
