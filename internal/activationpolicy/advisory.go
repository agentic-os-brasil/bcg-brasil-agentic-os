package activationpolicy

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type AdvisoryFact struct {
	Code           string         `json:"code"`
	Classification Classification `json:"classification"`
	ValueCode      string         `json:"value_code"`
}

type DeclassificationAttestation struct {
	ExporterID               string `json:"exporter_id"`
	NoClientIdentifiers      bool   `json:"no_client_identifiers"`
	NoStakeholderIdentifiers bool   `json:"no_stakeholder_identifiers"`
	NoRawExcerpts            bool   `json:"no_raw_excerpts"`
	NoScopedPointers         bool   `json:"no_scoped_pointers"`
}

type AdvisoryRequest struct {
	SchemaVersion  int                         `json:"schema_version"`
	RequestID      string                      `json:"request_id"`
	EpisodeSHA256  string                      `json:"episode_sha256"`
	PlanSHA256     string                      `json:"plan_sha256"`
	Expert         PAExpert                    `json:"expert"`
	QuestionCode   string                      `json:"question_code"`
	Classification Classification              `json:"classification"`
	Facts          []AdvisoryFact              `json:"facts"`
	OutputSections []string                    `json:"output_sections"`
	Attestation    DeclassificationAttestation `json:"attestation"`
}

type DeclassificationReceipt struct {
	SchemaVersion int    `json:"schema_version"`
	RequestID     string `json:"request_id"`
	RequestSHA256 string `json:"request_sha256"`
	PolicyVersion string `json:"policy_version"`
	ExporterID    string `json:"exporter_id"`
	ExpertID      string `json:"expert_id"`
	ExpertVersion string `json:"expert_version"`
	CanonSHA256   string `json:"canon_sha256"`
	Outcome       string `json:"outcome"`
	MayExport     bool   `json:"may_export"`
}

type AdvisoryResponse struct {
	SchemaVersion int      `json:"schema_version"`
	RequestSHA256 string   `json:"request_sha256"`
	ExpertID      string   `json:"expert_id"`
	ExpertVersion string   `json:"expert_version"`
	CanonSHA256   string   `json:"canon_sha256"`
	Findings      []string `json:"findings"`
	Assumptions   []string `json:"assumptions"`
	Challenges    []string `json:"challenges"`
	Cautions      []string `json:"application_cautions"`
}

type ReceiptKind string

const (
	OwnerReceipt     ReceiptKind = "owner"
	AdvisoryReceipt  ReceiptKind = "advisory"
	AssuranceReceipt ReceiptKind = "assurance"
)

type CompletionReceipt struct {
	SchemaVersion     int         `json:"schema_version"`
	EpisodeID         string      `json:"episode_id"`
	PlanSHA256        string      `json:"plan_sha256"`
	Kind              ReceiptKind `json:"kind"`
	ActorID           string      `json:"actor_id"`
	ExpertVersion     string      `json:"expert_version,omitempty"`
	CanonSHA256       string      `json:"canon_sha256,omitempty"`
	EvidenceAuthority string      `json:"evidence_authority"`
}

var (
	questionCode      = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
	advisoryRequestID = regexp.MustCompile(`^adv-[a-f0-9]{32}$`)
)

func Declassify(request AdvisoryRequest) (DeclassificationReceipt, error) {
	if request.SchemaVersion != 1 || !advisoryRequestID.MatchString(request.RequestID) ||
		!validSHA256(request.EpisodeSHA256) || !validSHA256(request.PlanSHA256) ||
		!validPAExpert(request.Expert) || !validAdvisoryCode(request.QuestionCode) ||
		len(request.QuestionCode) > 80 {
		return DeclassificationReceipt{}, errors.New("advisory request identity or expert binding is invalid")
	}
	if request.Classification != Public && request.Classification != Internal {
		return DeclassificationReceipt{}, errors.New("advisory request classification is not exportable")
	}
	if request.Attestation.ExporterID != "maestro" ||
		!request.Attestation.NoClientIdentifiers ||
		!request.Attestation.NoStakeholderIdentifiers ||
		!request.Attestation.NoRawExcerpts || !request.Attestation.NoScopedPointers {
		return DeclassificationReceipt{}, errors.New("advisory request lacks the complete declassification attestation")
	}
	if len(request.Facts) > 12 || len(request.OutputSections) == 0 || len(request.OutputSections) > 6 {
		return DeclassificationReceipt{}, errors.New("advisory request exceeds its bounded contract")
	}
	seenFacts := map[string]bool{}
	for _, fact := range request.Facts {
		if !validAdvisoryCode(fact.Code) || (fact.Classification != Public && fact.Classification != Internal) ||
			!validAdvisoryCode(fact.ValueCode) || len(fact.ValueCode) > 80 || seenFacts[fact.Code] {
			return DeclassificationReceipt{}, errors.New("advisory fact violates the declassified boundary")
		}
		seenFacts[fact.Code] = true
	}
	seen := map[string]bool{}
	for _, section := range request.OutputSections {
		if !allowedSection(section) || seen[section] {
			return DeclassificationReceipt{}, errors.New("advisory output section is unsupported or duplicated")
		}
		seen[section] = true
	}
	canonical := request
	canonical.Facts = append([]AdvisoryFact(nil), request.Facts...)
	sort.Slice(canonical.Facts, func(i, j int) bool {
		if canonical.Facts[i].Code == canonical.Facts[j].Code {
			return canonical.Facts[i].ValueCode < canonical.Facts[j].ValueCode
		}
		return canonical.Facts[i].Code < canonical.Facts[j].Code
	})
	canonical.OutputSections = append([]string(nil), request.OutputSections...)
	sort.Strings(canonical.OutputSections)
	body, err := json.Marshal(canonical)
	if err != nil {
		return DeclassificationReceipt{}, err
	}
	return DeclassificationReceipt{
		SchemaVersion: 1, RequestID: request.RequestID,
		RequestSHA256: SHA256Hex(body), PolicyVersion: PolicyVersion,
		ExporterID: request.Attestation.ExporterID, ExpertID: request.Expert.ID,
		ExpertVersion: request.Expert.Version, CanonSHA256: strings.ToLower(request.Expert.CanonSHA256),
		Outcome: "shadow_assessed_not_export_authorized", MayExport: false,
	}, nil
}

func ValidateResponse(response AdvisoryResponse, request AdvisoryRequest, receipt DeclassificationReceipt) error {
	expectedReceipt, err := Declassify(request)
	if err != nil {
		return err
	}
	if response.SchemaVersion != 1 || receipt != expectedReceipt ||
		receipt.Outcome != "shadow_assessed_not_export_authorized" || receipt.MayExport ||
		response.RequestSHA256 != receipt.RequestSHA256 ||
		response.ExpertID != request.Expert.ID ||
		response.ExpertVersion != request.Expert.Version ||
		strings.ToLower(response.CanonSHA256) != strings.ToLower(request.Expert.CanonSHA256) {
		return errors.New("advisory response does not match the declassified request and expert version")
	}
	total := len(response.Findings) + len(response.Assumptions) + len(response.Challenges) + len(response.Cautions)
	if total == 0 || total > 24 {
		return errors.New("advisory response exceeds its bounded contract")
	}
	for _, group := range [][]string{response.Findings, response.Assumptions, response.Challenges, response.Cautions} {
		for _, value := range group {
			if strings.TrimSpace(value) == "" || len([]byte(value)) > 500 ||
				strings.ContainsAny(value, "\r\n") || containsScopedPointer(value) {
				return errors.New("advisory response contains invalid or scoped content")
			}
		}
	}
	return nil
}

// OpaqueAdvisoryRequestID converts a local correlation label into the only
// request identity allowed to cross the PA Expert boundary.
func OpaqueAdvisoryRequestID(localID string) string {
	if strings.TrimSpace(localID) == "" {
		return ""
	}
	return "adv-" + SHA256Hex([]byte(localID))[:32]
}

func VerifyCompletion(plan RoutePlan, receipts []CompletionReceipt) error {
	if plan.SchemaVersion != 1 || plan.PolicyVersion != PolicyVersion ||
		plan.Route == Blocked || plan.PlanSHA256 == "" || PlanDigest(plan) != plan.PlanSHA256 {
		return errors.New("route plan is invalid or blocked")
	}
	expected := map[string]CompletionReceipt{}
	ownerKey := string(OwnerReceipt) + ":" + string(plan.Owner)
	expected[ownerKey] = CompletionReceipt{Kind: OwnerReceipt, ActorID: string(plan.Owner)}
	for _, expert := range plan.Experts {
		key := string(AdvisoryReceipt) + ":" + expert.ID
		expected[key] = CompletionReceipt{
			Kind: AdvisoryReceipt, ActorID: expert.ID,
			ExpertVersion: expert.Version, CanonSHA256: expert.CanonSHA256,
		}
	}
	if plan.RequiresAssurance {
		expected[string(AssuranceReceipt)+":"+plan.AssuranceAgentID] = CompletionReceipt{Kind: AssuranceReceipt, ActorID: plan.AssuranceAgentID}
	}
	if len(receipts) != len(expected) {
		return fmt.Errorf("route requires exactly %d completion receipts", len(expected))
	}
	seen := map[string]bool{}
	for _, receipt := range receipts {
		if receipt.SchemaVersion != 1 || receipt.EpisodeID != plan.EpisodeID ||
			receipt.PlanSHA256 != plan.PlanSHA256 ||
			receipt.EvidenceAuthority != "unverified_breadcrumb" ||
			(!validID(receipt.ActorID) && receipt.ActorID != string(OwnerAccount) && receipt.ActorID != string(OwnerCase)) {
			return errors.New("completion receipt is stale or malformed")
		}
		key := string(receipt.Kind) + ":" + receipt.ActorID
		wanted, ok := expected[key]
		if !ok || seen[key] ||
			receipt.ExpertVersion != wanted.ExpertVersion ||
			receipt.CanonSHA256 != wanted.CanonSHA256 {
			return errors.New("completion receipt is unexpected, duplicated or version-mismatched")
		}
		seen[key] = true
	}
	return nil
}

func containsScopedPointer(value string) bool {
	lower := strings.ToLower(value)
	for _, forbidden := range []string{
		"bcgos://workspace/", "bcgos://account/", "bcgos://case/",
		"workspace://", "account://", "case://", "file://",
		"client id", "account id", "workspace id", "case id", "stakeholder id",
	} {
		if strings.Contains(lower, forbidden) {
			return true
		}
	}
	return false
}

func allowedSection(value string) bool {
	switch value {
	case "findings", "assumptions", "challenges", "application_cautions":
		return true
	default:
		return false
	}
}

func validAdvisoryCode(value string) bool {
	if !questionCode.MatchString(value) || len(value) > 80 {
		return false
	}
	lower := strings.ToLower(value)
	for _, forbidden := range []string{"client", "account", "case", "workspace", "stakeholder", "person", "raw", "file"} {
		if lower == forbidden || strings.HasPrefix(lower, forbidden+"-") || strings.Contains(lower, "-"+forbidden+"-") {
			return false
		}
	}
	return true
}
