// Package federation owns the closed, runtime-neutral outbound contract for
// Maestro's Federated Improvement Loop. It intentionally cannot represent raw
// workspace content, paths, identifiers or free-text analysis.
package federation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
)

const SchemaVersion = 1

var (
	installationIDPattern = regexp.MustCompile(`^[a-f0-9]{16,64}$`)
	periodPattern         = regexp.MustCompile(`^[0-9]{4}-W(0[1-9]|[1-4][0-9]|5[0-3])$`)
	versionPattern        = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)
	fingerprintPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type Runtime string

const (
	RuntimeClaude Runtime = "claude"
	RuntimeCodex  Runtime = "codex"
)

type SignalKind string

const (
	SignalAdoption        SignalKind = "adoption"
	SignalFailure         SignalKind = "failure"
	SignalFriction        SignalKind = "friction"
	SignalWorkflowPattern SignalKind = "workflow_pattern"
)

type CapabilityID string

const (
	CapabilityWorkspaceAgentSetup CapabilityID = "workspace-agent-setup"
	CapabilityDreamMemory         CapabilityID = "dream-memory"
	CapabilityIngestContent       CapabilityID = "ingest-content"
	CapabilityInteractionProfile  CapabilityID = "interaction-profile"
)

type WorkflowStage string

const (
	StageFirstUse    WorkflowStage = "first_use"
	StageExecution   WorkflowStage = "execution"
	StageHandoff     WorkflowStage = "handoff"
	StageRecovery    WorkflowStage = "recovery"
	StageMaintenance WorkflowStage = "maintenance"
)

type EvidenceBucket string

const (
	EvidenceOnce        EvidenceBucket = "once"
	EvidenceTwoToThree  EvidenceBucket = "two_to_three"
	EvidenceFourToSeven EvidenceBucket = "four_to_seven"
	EvidenceEightPlus   EvidenceBucket = "eight_plus"
)

type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

type Outcome string

const (
	OutcomeImproved Outcome = "improved"
	OutcomeNeutral  Outcome = "neutral"
	OutcomeBlocked  Outcome = "blocked"
	OutcomeFailed   Outcome = "failed"
)

type CandidateClass string

const (
	CandidateWorkflowAutomation CandidateClass = "workflow_automation"
	CandidateQualityGuard       CandidateClass = "quality_guard"
	CandidateContextGuidance    CandidateClass = "context_guidance"
	CandidateDataBoundary       CandidateClass = "data_boundary"
)

type CandidateTrigger string

const (
	TriggerWorkflowRepetition CandidateTrigger = "workflow_repetition"
	TriggerCapabilityGap      CandidateTrigger = "capability_gap"
	TriggerFrictionReduction  CandidateTrigger = "friction_reduction"
	TriggerQualityRisk        CandidateTrigger = "quality_risk"
)

type SafetyFlag string

const (
	SafetyRequiresReview SafetyFlag = "requires_review"
	SafetyDataBoundary   SafetyFlag = "data_boundary"
	SafetyRuntimeParity  SafetyFlag = "runtime_parity"
)

// Batch is the complete automatic egress vocabulary. Its fields deliberately
// exclude maps, arbitrary attributes, free-text bodies and workspace identity.
type Batch struct {
	SchemaVersion        int              `json:"schema_version"`
	InstallationID       string           `json:"installation_id"`
	Period               string           `json:"period"`
	ProductVersion       string           `json:"product_version"`
	Runtime              Runtime          `json:"runtime"`
	Signals              []Signal         `json:"signals"`
	Candidates           []SkillCandidate `json:"candidates"`
	PortableSkillContent string           `json:"portable_skill_content,omitempty"`
}

type Signal struct {
	Kind       SignalKind     `json:"kind"`
	Capability CapabilityID   `json:"capability"`
	Stage      WorkflowStage  `json:"stage"`
	Evidence   EvidenceBucket `json:"evidence"`
	Confidence Confidence     `json:"confidence"`
	Outcome    Outcome        `json:"outcome"`
}

type SkillCandidate struct {
	Class        CandidateClass   `json:"class"`
	Trigger      CandidateTrigger `json:"trigger"`
	Dependencies []CapabilityID   `json:"dependencies"`
	Evidence     EvidenceBucket   `json:"evidence"`
	SafetyFlags  []SafetyFlag     `json:"safety_flags"`
	Fingerprint  string           `json:"fingerprint"`
}

// WorkspaceObservation exists only inside the local compiler. WorkspaceID and
// PrivateText are intentionally not part of Batch and never reach a transport.
type WorkspaceObservation struct {
	InstallationID string
	Period         string
	ProductVersion string
	Runtime        Runtime
	WorkspaceID    string
	PrivateText    string
	// PortableSkillContent is a deliberate local-only denial guard. Complete
	// skills travel only through the separate born-portable collector contract,
	// never through a workspace observation or a typed Batch.
	PortableSkillContent string
	Signals              []Signal
	Candidates           []SkillCandidate
}

// CompileWorkspace constructs a structural export. It cannot carry the
// observation's workspace identity or private text into the result.
func CompileWorkspace(observation WorkspaceObservation) (Batch, error) {
	if observation.PortableSkillContent != "" {
		return Batch{}, errors.New("workspace observation cannot carry portable skill content")
	}
	batch := Batch{
		SchemaVersion:  SchemaVersion,
		InstallationID: observation.InstallationID,
		Period:         observation.Period,
		ProductVersion: observation.ProductVersion,
		Runtime:        observation.Runtime,
		Signals:        append([]Signal(nil), observation.Signals...),
		Candidates:     cloneCandidates(observation.Candidates),
	}
	if err := batch.Validate(); err != nil {
		return Batch{}, err
	}
	return batch, nil
}

func cloneCandidates(values []SkillCandidate) []SkillCandidate {
	result := make([]SkillCandidate, len(values))
	for index, candidate := range values {
		result[index] = candidate
		result[index].Dependencies = append([]CapabilityID(nil), candidate.Dependencies...)
		result[index].SafetyFlags = append([]SafetyFlag(nil), candidate.SafetyFlags...)
	}
	return result
}

func Parse(reader io.Reader) (Batch, error) {
	payload, err := io.ReadAll(reader)
	if err != nil {
		return Batch{}, err
	}
	if err := rejectDuplicateKeys(payload); err != nil {
		return Batch{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var batch Batch
	if err := decoder.Decode(&batch); err != nil {
		return Batch{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Batch{}, errors.New("federation batch contains multiple JSON values")
		}
		return Batch{}, err
	}
	if err := batch.Validate(); err != nil {
		return Batch{}, err
	}
	return batch, nil
}

func rejectDuplicateKeys(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := walkJSON(decoder); err != nil {
		return err
	}
	return nil
}

func walkJSON(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch delimiter := token.(type) {
	case json.Delim:
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("federation object key is not a string")
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate federation object key %q", key)
				}
				seen[key] = struct{}{}
				if err := walkJSON(decoder); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walkJSON(decoder); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		}
	}
	return nil
}

func (batch Batch) Validate() error {
	if batch.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported federation schema version %d", batch.SchemaVersion)
	}
	if !installationIDPattern.MatchString(batch.InstallationID) || !periodPattern.MatchString(batch.Period) || !versionPattern.MatchString(batch.ProductVersion) {
		return errors.New("federation batch has an invalid bounded header")
	}
	if batch.Runtime != RuntimeClaude && batch.Runtime != RuntimeCodex {
		return fmt.Errorf("unsupported federation runtime %q", batch.Runtime)
	}
	if len(batch.Signals)+len(batch.Candidates) == 0 || len(batch.Signals) > 8 || len(batch.Candidates) > 8 {
		return errors.New("federation batch exceeds its bounded artifact vocabulary")
	}
	if batch.PortableSkillContent != "" {
		return errors.New("portable skill content requires the dedicated portable collector contract")
	}
	for _, signal := range batch.Signals {
		if err := signal.Validate(); err != nil {
			return err
		}
	}
	for _, candidate := range batch.Candidates {
		if err := candidate.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (signal Signal) Validate() error {
	if !validSignalKind(signal.Kind) || !validCapability(signal.Capability) || !validStage(signal.Stage) || !validEvidence(signal.Evidence) || !validConfidence(signal.Confidence) || !validOutcome(signal.Outcome) {
		return errors.New("federation batch contains an unapproved signal value")
	}
	return nil
}

func (candidate SkillCandidate) Validate() error {
	if !validCandidateClass(candidate.Class) || !validTrigger(candidate.Trigger) || !validEvidence(candidate.Evidence) || !fingerprintPattern.MatchString(candidate.Fingerprint) || len(candidate.Dependencies) == 0 || len(candidate.Dependencies) > 6 || len(candidate.SafetyFlags) == 0 || len(candidate.SafetyFlags) > 3 {
		return errors.New("federation batch contains an invalid structural skill candidate")
	}
	seenDependencies := map[CapabilityID]bool{}
	for _, dependency := range candidate.Dependencies {
		if !validCapability(dependency) || seenDependencies[dependency] {
			return errors.New("federation batch contains an invalid structural skill dependency")
		}
		seenDependencies[dependency] = true
	}
	seenFlags := map[SafetyFlag]bool{}
	for _, flag := range candidate.SafetyFlags {
		if !validSafetyFlag(flag) || seenFlags[flag] {
			return errors.New("federation batch contains an invalid structural skill safety flag")
		}
		seenFlags[flag] = true
	}
	return nil
}

func validSignalKind(value SignalKind) bool {
	return value == SignalAdoption || value == SignalFailure || value == SignalFriction || value == SignalWorkflowPattern
}

func validCapability(value CapabilityID) bool {
	return value == CapabilityWorkspaceAgentSetup || value == CapabilityDreamMemory || value == CapabilityIngestContent || value == CapabilityInteractionProfile
}

func validStage(value WorkflowStage) bool {
	return value == StageFirstUse || value == StageExecution || value == StageHandoff || value == StageRecovery || value == StageMaintenance
}

func validEvidence(value EvidenceBucket) bool {
	return value == EvidenceOnce || value == EvidenceTwoToThree || value == EvidenceFourToSeven || value == EvidenceEightPlus
}

func validConfidence(value Confidence) bool {
	return value == ConfidenceLow || value == ConfidenceMedium || value == ConfidenceHigh
}

func validOutcome(value Outcome) bool {
	return value == OutcomeImproved || value == OutcomeNeutral || value == OutcomeBlocked || value == OutcomeFailed
}

func validCandidateClass(value CandidateClass) bool {
	return value == CandidateWorkflowAutomation || value == CandidateQualityGuard || value == CandidateContextGuidance || value == CandidateDataBoundary
}

func validTrigger(value CandidateTrigger) bool {
	return value == TriggerWorkflowRepetition || value == TriggerCapabilityGap || value == TriggerFrictionReduction || value == TriggerQualityRisk
}

func validSafetyFlag(value SafetyFlag) bool {
	return value == SafetyRequiresReview || value == SafetyDataBoundary || value == SafetyRuntimeParity
}
