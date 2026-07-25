package federation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// QualitativePerception is the closed vocabulary through which Local Darwin
// can represent qualitative pilot feedback. It intentionally has no prose,
// free-form rating, person, client or workspace field.
type QualitativePerception string

const (
	PerceptionNavigationFriction QualitativePerception = "navigation_friction"
	PerceptionWorkflowRepetition QualitativePerception = "workflow_repetition"
	PerceptionRecoveryConfidence QualitativePerception = "recovery_confidence"
	PerceptionBoundaryConfidence QualitativePerception = "boundary_confidence"
)

type QualitativeFinding struct {
	Perception QualitativePerception
	Stage      WorkflowStage
	Evidence   EvidenceBucket
	Confidence Confidence
	Outcome    Outcome
}

// SkillRecipe is a managed structural template, not a user-authored skill
// body. The deterministic mapping allows Local Darwin to signal a potentially
// reusable pattern without exporting instructions from a workspace.
type SkillRecipe string

const (
	RecipeHandoffGuard      SkillRecipe = "handoff_guard"
	RecipeRecoveryGuide     SkillRecipe = "recovery_guide"
	RecipeBoundaryChecklist SkillRecipe = "boundary_checklist"
	RecipeIntakeGuide       SkillRecipe = "intake_guide"
)

type RecipeFinding struct {
	Recipe   SkillRecipe
	Evidence EvidenceBucket
}

// LocalDarwinPacket never leaves the device. WorkspaceID and PrivateContext
// scope the local evaluation but are not read by the structural compiler.
type LocalDarwinPacket struct {
	InstallationID string
	Period         string
	ProductVersion string
	Runtime        Runtime
	WorkspaceID    string
	PrivateContext string
	Findings       []QualitativeFinding
	Recipes        []RecipeFinding
}

// FederateLocal turns a local Darwin packet into the closed Batch contract.
// It makes no network call, reads no file and cannot put either private field
// into the outbound representation.
func FederateLocal(packet LocalDarwinPacket) (Batch, error) {
	if err := packet.Validate(); err != nil {
		return Batch{}, err
	}
	signals := make([]Signal, 0, len(packet.Findings))
	for _, finding := range packet.Findings {
		signal, err := finding.Signal()
		if err != nil {
			return Batch{}, err
		}
		signals = append(signals, signal)
	}
	candidates := make([]SkillCandidate, 0, len(packet.Recipes))
	for _, finding := range packet.Recipes {
		candidate, err := finding.Candidate()
		if err != nil {
			return Batch{}, err
		}
		candidates = append(candidates, candidate)
	}
	return CompileWorkspace(WorkspaceObservation{
		InstallationID: packet.InstallationID,
		Period:         packet.Period,
		ProductVersion: packet.ProductVersion,
		Runtime:        packet.Runtime,
		WorkspaceID:    packet.WorkspaceID,
		PrivateText:    packet.PrivateContext,
		Signals:        signals,
		Candidates:     candidates,
	})
}

func (packet LocalDarwinPacket) Validate() error {
	if !installationIDPattern.MatchString(packet.InstallationID) || !periodPattern.MatchString(packet.Period) || !versionPattern.MatchString(packet.ProductVersion) || (packet.Runtime != RuntimeClaude && packet.Runtime != RuntimeCodex) {
		return errors.New("invalid local Darwin packet header")
	}
	if len(packet.Findings)+len(packet.Recipes) == 0 || len(packet.Findings) > 8 || len(packet.Recipes) > 8 {
		return errors.New("local Darwin packet exceeds its closed finding vocabulary")
	}
	for _, finding := range packet.Findings {
		if _, err := finding.Signal(); err != nil {
			return err
		}
	}
	for _, recipe := range packet.Recipes {
		if _, err := recipe.Candidate(); err != nil {
			return err
		}
	}
	return nil
}

func (finding QualitativeFinding) Signal() (Signal, error) {
	capability, ok := perceptionCapability(finding.Perception)
	if !ok {
		return Signal{}, fmt.Errorf("unapproved local Darwin perception %q", finding.Perception)
	}
	signal := Signal{Kind: perceptionKind(finding.Perception), Capability: capability, Stage: finding.Stage, Evidence: finding.Evidence, Confidence: finding.Confidence, Outcome: finding.Outcome}
	if err := signal.Validate(); err != nil {
		return Signal{}, err
	}
	return signal, nil
}

func (finding RecipeFinding) Candidate() (SkillCandidate, error) {
	candidate, ok := recipeCandidate(finding.Recipe, finding.Evidence)
	if !ok {
		return SkillCandidate{}, fmt.Errorf("unapproved local Darwin recipe %q", finding.Recipe)
	}
	candidate.Fingerprint = structuralFingerprint(finding.Recipe, candidate)
	if err := candidate.Validate(); err != nil {
		return SkillCandidate{}, err
	}
	return candidate, nil
}

func perceptionCapability(value QualitativePerception) (CapabilityID, bool) {
	switch value {
	case PerceptionNavigationFriction:
		return CapabilityInteractionProfile, true
	case PerceptionWorkflowRepetition:
		return CapabilityWorkspaceAgentSetup, true
	case PerceptionRecoveryConfidence:
		return CapabilityDreamMemory, true
	case PerceptionBoundaryConfidence:
		return CapabilityIngestContent, true
	default:
		return "", false
	}
}

func perceptionKind(value QualitativePerception) SignalKind {
	if value == PerceptionWorkflowRepetition {
		return SignalWorkflowPattern
	}
	return SignalFriction
}

func recipeCandidate(recipe SkillRecipe, evidence EvidenceBucket) (SkillCandidate, bool) {
	switch recipe {
	case RecipeHandoffGuard:
		return SkillCandidate{Class: CandidateQualityGuard, Trigger: TriggerQualityRisk, Dependencies: []CapabilityID{CapabilityWorkspaceAgentSetup}, Evidence: evidence, SafetyFlags: []SafetyFlag{SafetyRequiresReview, SafetyRuntimeParity}}, true
	case RecipeRecoveryGuide:
		return SkillCandidate{Class: CandidateContextGuidance, Trigger: TriggerFrictionReduction, Dependencies: []CapabilityID{CapabilityDreamMemory}, Evidence: evidence, SafetyFlags: []SafetyFlag{SafetyRequiresReview, SafetyRuntimeParity}}, true
	case RecipeBoundaryChecklist:
		return SkillCandidate{Class: CandidateDataBoundary, Trigger: TriggerQualityRisk, Dependencies: []CapabilityID{CapabilityIngestContent}, Evidence: evidence, SafetyFlags: []SafetyFlag{SafetyDataBoundary, SafetyRequiresReview}}, true
	case RecipeIntakeGuide:
		return SkillCandidate{Class: CandidateWorkflowAutomation, Trigger: TriggerCapabilityGap, Dependencies: []CapabilityID{CapabilityWorkspaceAgentSetup}, Evidence: evidence, SafetyFlags: []SafetyFlag{SafetyRequiresReview, SafetyRuntimeParity}}, true
	default:
		return SkillCandidate{}, false
	}
}

func structuralFingerprint(recipe SkillRecipe, candidate SkillCandidate) string {
	canonical := strings.Join([]string{
		string(recipe), string(candidate.Class), string(candidate.Trigger), string(candidate.Evidence),
		strings.Join(capabilityStrings(candidate.Dependencies), ","), strings.Join(safetyStrings(candidate.SafetyFlags), ","),
	}, "|")
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:])
}

func capabilityStrings(values []CapabilityID) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}

func safetyStrings(values []SafetyFlag) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}
