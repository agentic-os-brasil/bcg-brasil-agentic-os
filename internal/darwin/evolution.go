package darwin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// EvolutionSchemaVersion is independent from the operational health receipt
// schema so structural proposals can evolve without changing remediation
// receipts.
const EvolutionSchemaVersion = 1

type EvolutionCadence string

const (
	EvolutionWeekly  EvolutionCadence = "weekly"
	EvolutionMonthly EvolutionCadence = "monthly"
)

type EvolutionTarget string

const (
	EvolutionAgent    EvolutionTarget = "agent"
	EvolutionPAExpert EvolutionTarget = "pa_expert"
	EvolutionSkill    EvolutionTarget = "skill"
	EvolutionPolicy   EvolutionTarget = "policy"
)

// StructuralProposal is Darwin's proposal-only seam for selecting or
// evolving agents, PA Experts, skills and policies. It is never executable
// routing state: approval and live activation are separate contracts.
type StructuralProposal struct {
	SchemaVersion  int              `json:"schema_version"`
	ProposalID     string           `json:"proposal_id"`
	PolicyVersion  string           `json:"policy_version"`
	Cadence        EvolutionCadence `json:"cadence"`
	Target         EvolutionTarget  `json:"target"`
	CurrentDigest  string           `json:"current_digest"`
	ProposedDigest string           `json:"proposed_digest"`
	EvidenceWindow string           `json:"evidence_window"`
	ApprovalState  string           `json:"approval_state"`
	EvaluatedBy    string           `json:"evaluated_by,omitempty"`
	MayMutateRoute bool             `json:"may_mutate_route"`
}

// IndependentApproval is evidence about a proposal, not an activation
// command. Darwin cannot issue its own approval and no method in this package
// applies an approved proposal to live routing.
type IndependentApproval struct {
	SchemaVersion  int    `json:"schema_version"`
	ProposalID     string `json:"proposal_id"`
	ProposalDigest string `json:"proposal_digest"`
	ApproverID     string `json:"approver_id"`
	Decision       string `json:"decision"`
}

var evolutionDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

// independentApproverIDs is the explicit authority registry for structural
// evolution. It intentionally names the registered Walter reviewer rather
// than accepting arbitrary non-Darwin strings as an approval authority.
var independentApproverIDs = map[string]struct{}{
	"walter": {},
}

func (proposal StructuralProposal) Validate() error {
	if proposal.SchemaVersion != EvolutionSchemaVersion || !idPattern.MatchString(proposal.ProposalID) ||
		proposal.PolicyVersion == "" || !idPattern.MatchString(proposal.EvidenceWindow) ||
		!evolutionDigest.MatchString(proposal.CurrentDigest) || !evolutionDigest.MatchString(proposal.ProposedDigest) ||
		(proposal.Cadence != EvolutionWeekly && proposal.Cadence != EvolutionMonthly) ||
		(proposal.Target != EvolutionAgent && proposal.Target != EvolutionPAExpert && proposal.Target != EvolutionSkill && proposal.Target != EvolutionPolicy) {
		return errors.New("Darwin structural proposal is malformed")
	}
	if proposal.ApprovalState != "proposal_only" || proposal.EvaluatedBy != "" || proposal.MayMutateRoute {
		return errors.New("Darwin structural proposal must remain unevaluated and proposal-only")
	}
	return nil
}

func (proposal StructuralProposal) Digest() string {
	body, err := json.Marshal(proposal)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func (approval IndependentApproval) Validate(proposal StructuralProposal) error {
	if err := proposal.Validate(); err != nil {
		return err
	}
	if approval.SchemaVersion != EvolutionSchemaVersion || approval.ProposalID != proposal.ProposalID ||
		approval.ProposalDigest != proposal.Digest() || approval.ApproverID == "" || approval.ApproverID == AgentID ||
		!isIndependentApprover(approval.ApproverID) ||
		(approval.Decision != "approved" && approval.Decision != "rejected") {
		return fmt.Errorf("independent approval is invalid or self-issued")
	}
	return nil
}

func isIndependentApprover(id string) bool {
	_, ok := independentApproverIDs[id]
	return ok
}
