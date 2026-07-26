package activationpolicy

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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
}

type AdvisoryRequest struct {
	SchemaVersion  int                         `json:"schema_version"`
	RequestID      string                      `json:"request_id"`
	EpisodeSHA256  string                      `json:"episode_sha256"`
	PlanSHA256     string                      `json:"plan_sha256"`
	Expert         PXpert                      `json:"expert"`
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

var questionCode = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

func Declassify(request AdvisoryRequest) (DeclassificationReceipt, error) {
	if request.SchemaVersion != 1 || !validID(request.RequestID) ||
		!validSHA256(request.EpisodeSHA256) || !validSHA256(request.PlanSHA256) ||
		!validPXpert(request.Expert) || !questionCode.MatchString(request.QuestionCode) ||
		len(request.QuestionCode) > 80 {
		return DeclassificationReceipt{}, errors.New("advisory request identity or expert binding is invalid")
	}
	if request.Classification != Public && request.Classification != Internal {
		return DeclassificationReceipt{}, errors.New("advisory request classification is not exportable")
	}
	if !validID(request.Attestation.ExporterID) ||
		!request.Attestation.NoClientIdentifiers ||
		!request.Attestation.NoStakeholderIdentifiers ||
		!request.Attestation.NoRawExcerpts {
		return DeclassificationReceipt{}, errors.New("advisory request lacks the complete declassification attestation")
	}
	if len(request.Facts) > 12 || len(request.OutputSections) == 0 || len(request.OutputSections) > 6 {
		return DeclassificationReceipt{}, errors.New("advisory request exceeds its bounded contract")
	}
	for _, fact := range request.Facts {
		if !validID(fact.Code) || (fact.Classification != Public && fact.Classification != Internal) ||
			!validID(fact.ValueCode) || len(fact.ValueCode) > 80 {
			return DeclassificationReceipt{}, errors.New("advisory fact violates the declassified boundary")
		}
	}
	seen := map[string]bool{}
	for _, section := range request.OutputSections {
		if !allowedSection(section) || seen[section] {
			return DeclassificationReceipt{}, errors.New("advisory output section is unsupported or duplicated")
		}
		seen[section] = true
	}
	body, err := json.Marshal(request)
	if err != nil {
		return DeclassificationReceipt{}, err
	}
	return DeclassificationReceipt{
		SchemaVersion: 1, RequestID: request.RequestID,
		RequestSHA256: SHA256Hex(body), PolicyVersion: PolicyVersion,
		ExporterID: request.Attestation.ExporterID,
		Outcome:    "shadow_assessed_not_export_authorized", MayExport: false,
	}, nil
}

func ValidateResponse(response AdvisoryResponse, request AdvisoryRequest, receipt DeclassificationReceipt) error {
	if response.SchemaVersion != 1 || receipt.Outcome != "export_authorized" || !receipt.MayExport ||
		response.RequestSHA256 != receipt.RequestSHA256 ||
		response.ExpertID != request.Expert.ID ||
		response.ExpertVersion != request.Expert.Version ||
		response.CanonSHA256 != request.Expert.CanonSHA256 {
		return errors.New("advisory response does not match the declassified request and expert version")
	}
	total := len(response.Findings) + len(response.Assumptions) + len(response.Challenges) + len(response.Cautions)
	if total == 0 || total > 24 {
		return errors.New("advisory response exceeds its bounded contract")
	}
	for _, group := range [][]string{response.Findings, response.Assumptions, response.Challenges, response.Cautions} {
		for _, value := range group {
			if strings.TrimSpace(value) == "" || len([]byte(value)) > 500 || containsScopedPointer(value) {
				return errors.New("advisory response contains invalid or scoped content")
			}
		}
	}
	return nil
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
