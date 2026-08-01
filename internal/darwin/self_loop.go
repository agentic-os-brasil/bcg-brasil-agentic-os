package darwin

import (
	"errors"
	"regexp"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userintent"
)

var selfDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// SelfMaintenanceReceipt is Darwin's metadata-only view of the owner-context
// self loop. It reports health and re-evaluation proposals; it cannot promote,
// rewrite or delete canonical self facets.
type SelfMaintenanceReceipt struct {
	SchemaVersion          int       `json:"schema_version"`
	AgentID                string    `json:"agent_id"`
	SnapshotVersion        int       `json:"snapshot_version"`
	SnapshotSHA256         string    `json:"snapshot_sha256"`
	ObservationCount       int       `json:"observation_count"`
	DuplicateCount         int       `json:"duplicate_count"`
	ContradictionCount     int       `json:"contradiction_count"`
	RecheckDue             int       `json:"recheck_due"`
	DecayCandidates        int       `json:"decay_candidates"`
	OwnerConfirmedSignals  int       `json:"owner_confirmed_signals"`
	ReevaluationProposals  int       `json:"reevaluation_proposals"`
	CanonicalMutations     int       `json:"canonical_mutations"`
	ProposalEvidenceSHA256 []string  `json:"proposal_evidence_sha256,omitempty"`
	RecordedAt             time.Time `json:"recorded_at"`
}

func (receipt SelfMaintenanceReceipt) Validate() error {
	if receipt.SchemaVersion != SchemaVersion || receipt.AgentID != AgentID || receipt.SnapshotVersion < 1 || !selfDigestPattern.MatchString(receipt.SnapshotSHA256) || receipt.RecordedAt.IsZero() || receipt.CanonicalMutations != 0 {
		return errors.New("invalid Darwin self maintenance receipt")
	}
	for _, value := range []int{receipt.ObservationCount, receipt.DuplicateCount, receipt.ContradictionCount, receipt.RecheckDue, receipt.DecayCandidates, receipt.OwnerConfirmedSignals, receipt.ReevaluationProposals, receipt.CanonicalMutations} {
		if value < 0 {
			return errors.New("Darwin self maintenance receipt contains a negative count")
		}
	}
	for _, digest := range receipt.ProposalEvidenceSHA256 {
		if !selfDigestPattern.MatchString(digest) {
			return errors.New("Darwin self maintenance proposal digest is invalid")
		}
	}
	return nil
}

func MaintainSelfLoop(snapshot userintent.UserSelfSnapshot, log userintent.AbsorptionLog, now time.Time) (SelfMaintenanceReceipt, error) {
	report, err := log.Analyze(snapshot, now)
	if err != nil {
		return SelfMaintenanceReceipt{}, err
	}
	receipt := SelfMaintenanceReceipt{
		SchemaVersion: report.SchemaVersion, AgentID: AgentID,
		SnapshotVersion: report.SnapshotVersion, SnapshotSHA256: report.SnapshotSHA256,
		ObservationCount: report.ObservationCount, DuplicateCount: report.DuplicateCount,
		ContradictionCount: report.ContradictionCount, RecheckDue: report.RecheckDue,
		DecayCandidates: report.DecayCandidates, OwnerConfirmedSignals: report.OwnerConfirmedSignals,
		ReevaluationProposals: report.ReevaluationProposals, CanonicalMutations: report.CanonicalMutationsByDarwin,
		RecordedAt: now.UTC(),
	}
	for _, proposal := range report.ProposalReceipts {
		if proposal.MayPromoteCanonical || !proposal.OwnerActionRequired || proposal.Status != "proposal_only" || proposal.BaseSnapshotSHA256 != snapshot.Digest {
			return SelfMaintenanceReceipt{}, errors.New("Darwin self proposal exceeded metadata-only authority")
		}
		receipt.ProposalEvidenceSHA256 = append(receipt.ProposalEvidenceSHA256, proposal.EvidenceSHA256)
	}
	return receipt, receipt.Validate()
}
