package agentdispatch

import (
	"errors"
	"fmt"
	"sort"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
)

// DonePolicy is a closed completion predicate. The producing agent cannot
// replace it with prose in a return message.
type DonePolicy string

const (
	DoneAuthenticatedReturn DonePolicy = "authenticated_return"
	DoneTypedWalterVerdict  DonePolicy = "typed_walter_verdict"
)

const maxDoneEvidence = 12

// DoneContract is carried and signed in every schema-v2 work packet. It is
// intentionally structural: the packet remains the authority, while the
// returned body only supplies evidence pointers that satisfy this contract.
type DoneContract struct {
	SchemaVersion        int        `json:"schema_version"`
	Policy               DonePolicy `json:"policy"`
	RequiredEvidenceRefs []string   `json:"required_evidence_refs,omitempty"`
	MinimumEvidenceRefs  int        `json:"minimum_evidence_refs"`
}

func defaultDoneContract(targetID string) DoneContract {
	policy := DoneAuthenticatedReturn
	if targetID == "walter" {
		policy = DoneTypedWalterVerdict
	}
	return DoneContract{SchemaVersion: 1, Policy: policy}
}

func normalizeDoneContract(contract DoneContract, targetID, scopeKind, scopeID string) (DoneContract, error) {
	if contract.SchemaVersion == 0 && contract.Policy == "" && len(contract.RequiredEvidenceRefs) == 0 && contract.MinimumEvidenceRefs == 0 {
		contract = defaultDoneContract(targetID)
	}
	if err := validateDoneContract(contract, targetID, scopeKind, scopeID); err != nil {
		return DoneContract{}, err
	}
	refs := append([]string(nil), contract.RequiredEvidenceRefs...)
	sort.Strings(refs)
	contract.RequiredEvidenceRefs = refs
	return contract, nil
}

func validateDoneContract(contract DoneContract, targetID, scopeKind, scopeID string) error {
	if contract.SchemaVersion != 1 || contract.MinimumEvidenceRefs < 0 || contract.MinimumEvidenceRefs > maxDoneEvidence {
		return errors.New("done contract schema or evidence bound is invalid")
	}
	if targetID == "walter" {
		if contract.Policy != DoneTypedWalterVerdict {
			return errors.New("Walter requires the typed verdict done policy")
		}
	} else if contract.Policy != DoneAuthenticatedReturn {
		return errors.New("producer requires the authenticated return done policy")
	}
	if len(contract.RequiredEvidenceRefs) > maxDoneEvidence || contract.MinimumEvidenceRefs < len(contract.RequiredEvidenceRefs) {
		return errors.New("done contract evidence requirements exceed the bounded contract")
	}
	seen := make(map[string]bool, len(contract.RequiredEvidenceRefs))
	for _, ref := range contract.RequiredEvidenceRefs {
		normalized, valid := agentorchestration.NormalizeResource(ref)
		if !valid || normalized != ref || !agentorchestration.ResourceWithinScope(ref, scopeKind, scopeID) || seen[ref] {
			return errors.New("done contract contains an invalid, duplicate or cross-scope evidence pointer")
		}
		seen[ref] = true
	}
	return nil
}

func doneContractSatisfied(contract DoneContract, body ReturnBody, targetID string) error {
	if contract.SchemaVersion == 0 {
		contract = defaultDoneContract(targetID)
	}
	if contract.SchemaVersion != 1 || contract.Policy != DoneAuthenticatedReturn {
		return errors.New("return does not carry the authenticated return done policy")
	}
	if targetID == "walter" {
		return errors.New("Walter completion must use its typed verdict path")
	}
	if len(body.EvidenceRefs) < contract.MinimumEvidenceRefs {
		return fmt.Errorf("done contract requires at least %d evidence references", contract.MinimumEvidenceRefs)
	}
	seen := make(map[string]bool, len(body.EvidenceRefs))
	for _, required := range contract.RequiredEvidenceRefs {
		seen[required] = false
	}
	for _, ref := range body.EvidenceRefs {
		if _, required := seen[ref]; required {
			seen[ref] = true
		}
	}
	for ref, present := range seen {
		if !present {
			return fmt.Errorf("done contract evidence reference %q was not returned", ref)
		}
	}
	return nil
}
