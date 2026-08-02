package activationpolicy

import (
	"reflect"
	"strings"
	"testing"
)

func TestAdvisoryExportRejectsScopedOrConfidentialContent(t *testing.T) {
	base := AdvisoryRequest{
		SchemaVersion: 1, RequestID: OpaqueAdvisoryRequestID("advisory-01"),
		EpisodeSHA256: digest("episode"), PlanSHA256: digest("plan"),
		Expert:       PAExpert{ID: "pa-expert-fpa-pricing", Kind: ExpertFPA, Version: "1.0.0", CanonSHA256: digest("canon"), Lifecycle: Published},
		QuestionCode: "pricing-strategy", Classification: Internal,
		Facts:          []AdvisoryFact{{Code: "fact-01", Classification: Internal, ValueCode: "market-prices-up"}},
		OutputSections: []string{"findings", "challenges"},
		Attestation: DeclassificationAttestation{
			ExporterID: "maestro", NoClientIdentifiers: true,
			NoStakeholderIdentifiers: true, NoRawExcerpts: true, NoScopedPointers: true,
		},
	}
	if _, err := Declassify(base); err != nil {
		t.Fatalf("valid advisory rejected: %v", err)
	}
	scoped := base
	scoped.Facts = []AdvisoryFact{{Code: "fact-01", Classification: Internal, ValueCode: "bcgos://workspace/client-a/raw"}}
	if _, err := Declassify(scoped); err == nil {
		t.Fatal("workspace pointer crossed PA expert boundary")
	}
	confidential := base
	confidential.Classification = Confidential
	if _, err := Declassify(confidential); err == nil {
		t.Fatal("confidential advisory crossed PA expert boundary")
	}
}

func TestCompletionRequiresExactReceipts(t *testing.T) {
	plan := RoutePlan{
		SchemaVersion: 1, EpisodeID: "episode-06", Route: D1Targeted,
		Owner: OwnerCase, PolicyVersion: PolicyVersion,
		InputSHA256: digest("input"),
		Experts:     []SelectedPAExpert{{ID: "pa-expert-fpa-pricing", Kind: ExpertFPA, Version: "1.0.0", CanonSHA256: digest("canon")}},
	}
	plan.PlanSHA256 = PlanDigest(plan)
	owner := CompletionReceipt{SchemaVersion: 1, EpisodeID: plan.EpisodeID, PlanSHA256: plan.PlanSHA256, Kind: OwnerReceipt, ActorID: string(plan.Owner), EvidenceAuthority: "unverified_breadcrumb"}
	if err := VerifyCompletion(plan, []CompletionReceipt{owner}); err == nil {
		t.Fatal("missing PA expert receipt completed D1 route")
	}
	expert := CompletionReceipt{SchemaVersion: 1, EpisodeID: plan.EpisodeID, PlanSHA256: plan.PlanSHA256, Kind: AdvisoryReceipt, ActorID: plan.Experts[0].ID, ExpertVersion: plan.Experts[0].Version, CanonSHA256: plan.Experts[0].CanonSHA256, EvidenceAuthority: "unverified_breadcrumb"}
	if err := VerifyCompletion(plan, []CompletionReceipt{owner, expert}); err != nil {
		t.Fatalf("exact receipt set rejected: %v", err)
	}
	if err := VerifyCompletion(plan, []CompletionReceipt{owner, expert, expert}); err == nil {
		t.Fatal("duplicate/extra receipt completed route")
	}
}

func TestAdvisoryDigestIsCanonicalAndBindsExactExpert(t *testing.T) {
	first := validAdvisoryRequest()
	first.Facts = append(first.Facts, AdvisoryFact{Code: "capacity-signal", Classification: Public, ValueCode: "stable"})
	first.OutputSections = []string{"challenges", "findings", "assumptions"}
	second := first
	second.Facts = []AdvisoryFact{first.Facts[1], first.Facts[0]}
	second.OutputSections = []string{"assumptions", "findings", "challenges"}
	left, err := Declassify(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := Declassify(second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("equivalent advisory order changed receipt: %#v != %#v", left, right)
	}
	response := AdvisoryResponse{
		SchemaVersion: 1, RequestSHA256: left.RequestSHA256,
		ExpertID: first.Expert.ID, ExpertVersion: first.Expert.Version,
		CanonSHA256: first.Expert.CanonSHA256, Findings: []string{"market signal is mixed"},
	}
	if err := ValidateResponse(response, first, left); err != nil {
		t.Fatalf("bounded shadow response rejected: %v", err)
	}
	forged := left
	forged.CanonSHA256 = strings.Repeat("b", 64)
	if err := ValidateResponse(response, first, forged); err == nil {
		t.Fatal("substituted expert receipt accepted")
	}
}

func TestAdvisoryBoundaryRejectsScopedAndDuplicatedMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AdvisoryRequest)
	}{
		{"semantic request id", func(value *AdvisoryRequest) { value.RequestID = "client-alpha" }},
		{"forged client code", func(value *AdvisoryRequest) { value.Facts[0].ValueCode = "client-alpha" }},
		{"workspace code", func(value *AdvisoryRequest) { value.Facts[0].Code = "workspace-fact" }},
		{"duplicate fact", func(value *AdvisoryRequest) { value.Facts = append(value.Facts, value.Facts[0]) }},
		{"wrong exporter", func(value *AdvisoryRequest) { value.Attestation.ExporterID = "case-agent-alpha" }},
		{"missing pointer attestation", func(value *AdvisoryRequest) { value.Attestation.NoScopedPointers = false }},
		{"forged canon", func(value *AdvisoryRequest) { value.Expert.CanonSHA256 = strings.Repeat("a", 63) + "!" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validAdvisoryRequest()
			test.mutate(&request)
			if _, err := Declassify(request); err == nil {
				t.Fatal("unsafe advisory metadata crossed the PA Expert boundary")
			}
		})
	}
}

func TestAdvisoryResponseRejectsScopedContentAndExportClaim(t *testing.T) {
	request := validAdvisoryRequest()
	receipt, err := Declassify(request)
	if err != nil {
		t.Fatal(err)
	}
	response := AdvisoryResponse{
		SchemaVersion: 1, RequestSHA256: receipt.RequestSHA256,
		ExpertID: request.Expert.ID, ExpertVersion: request.Expert.Version,
		CanonSHA256: request.Expert.CanonSHA256,
		Findings:    []string{"use bcgos://workspace/other/raw"},
	}
	if err := ValidateResponse(response, request, receipt); err == nil {
		t.Fatal("scoped content entered a PA Expert response")
	}
	response.Findings = []string{"bounded finding"}
	forged := receipt
	forged.Outcome, forged.MayExport = "export_authorized", true
	if err := ValidateResponse(response, request, forged); err == nil {
		t.Fatal("caller-forged export authority accepted")
	}
}

func validAdvisoryRequest() AdvisoryRequest {
	return AdvisoryRequest{
		SchemaVersion: 1, RequestID: OpaqueAdvisoryRequestID("advisory-test"),
		EpisodeSHA256: digest("episode"), PlanSHA256: digest("plan"),
		Expert: PAExpert{
			ID: "pa-expert-fpa-pricing", Kind: ExpertFPA,
			Version: "1.0.0", CanonSHA256: digest("canon"), Lifecycle: Published,
		},
		QuestionCode: "pricing-signal", Classification: Internal,
		Facts:          []AdvisoryFact{{Code: "market-signal", Classification: Internal, ValueCode: "demand-up"}},
		OutputSections: []string{"findings", "challenges"},
		Attestation: DeclassificationAttestation{
			ExporterID: "maestro", NoClientIdentifiers: true,
			NoStakeholderIdentifiers: true, NoRawExcerpts: true, NoScopedPointers: true,
		},
	}
}
