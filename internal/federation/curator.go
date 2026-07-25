package federation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ProposalKind string

const (
	ProposalGuidance    ProposalKind = "guidance"
	ProposalSharedSkill ProposalKind = "shared_skill"
	ProposalIncident    ProposalKind = "incident"
)

type ProposalTemplate string

const (
	TemplateAdoptionReview   ProposalTemplate = "adoption_review"
	TemplateFrictionGuidance ProposalTemplate = "friction_guidance"
	TemplateWorkflowSkill    ProposalTemplate = "workflow_skill"
	TemplateFailureIncident  ProposalTemplate = "failure_incident"
)

type ProposalStatus string

const ProposalDraft ProposalStatus = "draft"

// AdvancementProposal is the central Darwin output. It is intentionally a
// proposal and cannot execute code, publish a package, merge a pull request or
// release Maestro. The GitHub issue is a human review artifact, not approval.
type AdvancementProposal struct {
	SchemaVersion           int              `json:"schema_version"`
	ID                      string           `json:"id"`
	Period                  string           `json:"period"`
	Kind                    ProposalKind     `json:"kind"`
	Template                ProposalTemplate `json:"template"`
	Evidence                EvidenceBucket   `json:"evidence"`
	Status                  ProposalStatus   `json:"status"`
	RequiresHumanAcceptance bool             `json:"requires_human_acceptance"`
}

type ProposalIssue struct {
	Title  string
	Body   string
	Labels []string
}

type CentralDarwinCurator struct{}

func (CentralDarwinCurator) Curate(digest CentralDigest) ([]AdvancementProposal, error) {
	if err := validateCentralDigest(digest); err != nil {
		return nil, err
	}
	proposals := make([]AdvancementProposal, 0)
	for _, tally := range digest.Signals {
		if tally.Count < minimumCohortEvidence {
			continue
		}
		kind, template := proposalForSignal(tally.Signal)
		proposals = append(proposals, newProposal(digest.Period, kind, template, evidenceForCount(tally.Count)))
	}
	for _, tally := range digest.Candidates {
		if tally.Count < minimumCohortEvidence {
			continue
		}
		proposals = append(proposals, newProposal(digest.Period, ProposalSharedSkill, TemplateWorkflowSkill, evidenceForCount(tally.Count)))
	}
	sort.Slice(proposals, func(left, right int) bool { return proposals[left].ID < proposals[right].ID })
	return deduplicateProposals(proposals), nil
}

func (proposal AdvancementProposal) Issue() ProposalIssue {
	title, label := issuePresentation(proposal.Kind, proposal.Template)
	return ProposalIssue{
		Title:  "[Maestro pilot] " + title,
		Labels: []string{"maestro-federation", label},
		Body: strings.Join([]string{
			"## Federated pilot advancement proposal",
			"",
			"This proposal was produced from aggregated, typed cohort evidence. It contains no workspace content, installation identity, prompts or customer material.",
			"",
			"- Period: `" + proposal.Period + "`",
			"- Evidence: `" + string(proposal.Evidence) + "`",
			"- Template: `" + string(proposal.Template) + "`",
			"",
			"A Human maintainer must accept this proposal before any implementation, pull request or release. This issue authorizes no autonomous source change.",
		}, "\n"),
	}
}

func newProposal(period string, kind ProposalKind, template ProposalTemplate, evidence EvidenceBucket) AdvancementProposal {
	canonical := strings.Join([]string{period, string(kind), string(template), string(evidence)}, "|")
	digest := sha256.Sum256([]byte(canonical))
	return AdvancementProposal{SchemaVersion: SchemaVersion, ID: hex.EncodeToString(digest[:]), Period: period, Kind: kind, Template: template, Evidence: evidence, Status: ProposalDraft, RequiresHumanAcceptance: true}
}

func proposalForSignal(signal Signal) (ProposalKind, ProposalTemplate) {
	if signal.Kind == SignalFailure || signal.Outcome == OutcomeFailed {
		return ProposalIncident, TemplateFailureIncident
	}
	if signal.Kind == SignalWorkflowPattern {
		return ProposalSharedSkill, TemplateWorkflowSkill
	}
	if signal.Kind == SignalFriction || signal.Outcome == OutcomeBlocked {
		return ProposalGuidance, TemplateFrictionGuidance
	}
	return ProposalGuidance, TemplateAdoptionReview
}

func issuePresentation(kind ProposalKind, template ProposalTemplate) (string, string) {
	if kind == ProposalIncident || template == TemplateFailureIncident {
		return "actionable incident", "pilot-incident"
	}
	if kind == ProposalSharedSkill {
		return "shared skill candidate", "pilot-proposal"
	}
	return "guidance candidate", "pilot-proposal"
}

func validateCentralDigest(digest CentralDigest) error {
	if digest.SchemaVersion != SchemaVersion || !periodPattern.MatchString(digest.Period) || digest.BatchCount < 0 {
		return errors.New("invalid central federation digest")
	}
	for _, tally := range digest.Signals {
		if tally.Count <= 0 || tally.Signal.Validate() != nil {
			return errors.New("invalid central federation signal tally")
		}
	}
	for _, tally := range digest.Candidates {
		if tally.Count <= 0 || tally.Candidate.Validate() != nil {
			return errors.New("invalid central federation candidate tally")
		}
	}
	return nil
}

func evidenceForCount(count int) EvidenceBucket {
	switch {
	case count <= 1:
		return EvidenceOnce
	case count <= 3:
		return EvidenceTwoToThree
	case count <= 7:
		return EvidenceFourToSeven
	default:
		return EvidenceEightPlus
	}
}

func deduplicateProposals(values []AdvancementProposal) []AdvancementProposal {
	result := make([]AdvancementProposal, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if seen[value.ID] {
			continue
		}
		seen[value.ID] = true
		result = append(result, value)
	}
	return result
}

func (proposal AdvancementProposal) Validate() error {
	if proposal.SchemaVersion != SchemaVersion || !fingerprintPattern.MatchString(proposal.ID) || !periodPattern.MatchString(proposal.Period) || !validEvidence(proposal.Evidence) || proposal.Status != ProposalDraft || !proposal.RequiresHumanAcceptance {
		return errors.New("invalid advancement proposal")
	}
	if proposal.Kind != ProposalGuidance && proposal.Kind != ProposalSharedSkill && proposal.Kind != ProposalIncident {
		return fmt.Errorf("invalid advancement proposal kind %q", proposal.Kind)
	}
	if proposal.Template != TemplateAdoptionReview && proposal.Template != TemplateFrictionGuidance && proposal.Template != TemplateWorkflowSkill && proposal.Template != TemplateFailureIncident {
		return fmt.Errorf("invalid advancement proposal template %q", proposal.Template)
	}
	return nil
}

// ValidateAdvancementProposalSchemaFile makes the central-curation output
// contract visible to release tooling without adding schema machinery at
// runtime.
func ValidateAdvancementProposalSchemaFile(path string) error {
	var schema map[string]any
	if err := readStrictJSON(path, &schema); err != nil {
		return err
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		return errors.New("advancement proposal schema must use JSON Schema draft 2020-12")
	}
	if schema["$id"] != "urn:bcg-brasil-agentic-os:schema:advancement-proposal:v1" {
		return errors.New("advancement proposal schema has an unexpected identifier")
	}
	return nil
}
