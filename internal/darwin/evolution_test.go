package darwin

import "testing"

func TestStructuralProposalIsVersionedProposalOnly(t *testing.T) {
	proposal := StructuralProposal{
		SchemaVersion: EvolutionSchemaVersion, ProposalID: "evolution-1", PolicyVersion: "pae-v1-experimental",
		Cadence: EvolutionWeekly, Target: EvolutionPolicy,
		CurrentDigest:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ProposedDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		EvidenceWindow: "window-1", ApprovalState: "proposal_only",
	}
	if err := proposal.Validate(); err != nil {
		t.Fatal(err)
	}
	proposal.MayMutateRoute = true
	if err := proposal.Validate(); err == nil {
		t.Fatal("structural proposal gained live routing authority")
	}
}

func TestStructuralApprovalCannotBeIssuedByDarwin(t *testing.T) {
	proposal := StructuralProposal{
		SchemaVersion: EvolutionSchemaVersion, ProposalID: "evolution-2", PolicyVersion: "pae-v1-experimental",
		Cadence: EvolutionMonthly, Target: EvolutionAgent,
		CurrentDigest:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ProposedDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		EvidenceWindow: "window-2", ApprovalState: "proposal_only",
	}
	approval := IndependentApproval{
		SchemaVersion: EvolutionSchemaVersion, ProposalID: proposal.ProposalID,
		ProposalDigest: proposal.Digest(),
		ApproverID:     AgentID, Decision: "approved",
	}
	if err := approval.Validate(proposal); err == nil {
		t.Fatal("Darwin self-approval was accepted")
	}
	approval.ApproverID = "walter"
	if err := approval.Validate(proposal); err != nil {
		t.Fatal(err)
	}
	approval.ProposalDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := approval.Validate(proposal); err == nil {
		t.Fatal("approval for a different proposal digest was accepted")
	}
}
