package lifecycle

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

const (
	ClaudeMinimumVersion = "2.1.177"

	EvidenceConfigured      = "configured"
	EvidenceContractTested  = "contract-tested"
	EvidenceAdapterObserved = "adapter-observed"
	EvidenceNativeQualified = "native-qualified"
)

var versionPattern = regexp.MustCompile(`([0-9]+)\.([0-9]+)\.([0-9]+)`)

// QualificationInput is an in-memory attestation boundary. It is deliberately
// not a receipt schema: local configuration and adapter receipts cannot set
// NativeObservation or promote a manifest capability.
type QualificationInput struct {
	Runtime           string
	Event             string
	RuntimeVersion    string
	EvidenceClass     string
	NativeSurface     bool
	NativeObservation bool
}

type QualificationResult struct {
	State         string `json:"state"`
	EvidenceClass string `json:"evidence_class"`
	Blocker       string `json:"blocker,omitempty"`
}

// EvaluateNativeQualification returns the only state transition permitted by
// the lifecycle contract. It never mutates the runtime manifest. A separate,
// human-reviewed native-session record must supply NativeObservation=true.
func EvaluateNativeQualification(input QualificationInput) (QualificationResult, error) {
	if input.Runtime != "claude" && input.Runtime != "codex" {
		return QualificationResult{}, fmt.Errorf("unsupported lifecycle runtime %q", input.Runtime)
	}
	if !validEvents[input.Event] {
		return QualificationResult{}, fmt.Errorf("unsupported lifecycle event %q", input.Event)
	}
	if !validEvidenceClass(input.EvidenceClass) {
		return QualificationResult{}, fmt.Errorf("unsupported lifecycle evidence class %q", input.EvidenceClass)
	}
	result := QualificationResult{State: "unavailable", EvidenceClass: input.EvidenceClass}
	if input.Runtime == "claude" && !MeetsClaudeMinimum(input.RuntimeVersion) {
		result.State = "blocked"
		result.Blocker = "Claude runtime is below the required " + ClaudeMinimumVersion + " lifecycle-hook contract version"
		return result, nil
	}
	if !input.NativeSurface {
		result.State = "blocked"
		result.Blocker = "the runtime has no native surface for this lifecycle event"
		return result, nil
	}
	if input.EvidenceClass != EvidenceNativeQualified {
		result.Blocker = "native-session observation is required for capability qualification"
		return result, nil
	}
	if !input.NativeObservation {
		result.Blocker = "native-qualified evidence must include a reproducible native-session observation"
		return result, nil
	}
	result.State = "qualified"
	return result, nil
}

func validEvidenceClass(value string) bool {
	switch value {
	case EvidenceConfigured, EvidenceContractTested, EvidenceAdapterObserved, EvidenceNativeQualified:
		return true
	default:
		return false
	}
}

func MeetsClaudeMinimum(value string) bool {
	actual := versionPattern.FindStringSubmatch(value)
	if len(actual) != 4 {
		return false
	}
	for index, minimum := range []int{2, 1, 177} {
		parsed, err := strconv.Atoi(actual[index+1])
		if err != nil {
			return false
		}
		if parsed != minimum {
			return parsed > minimum
		}
	}
	return true
}

// ValidateEvidenceClass is used by conformance readers that need to reject
// invented promotion labels without evaluating a runtime claim.
func ValidateEvidenceClass(value string) error {
	if !validEvidenceClass(value) {
		return errors.New("unsupported lifecycle evidence class")
	}
	return nil
}
