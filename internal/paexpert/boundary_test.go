package paexpert

import (
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
)

func TestDeclassifyBindsExactExpertAndReturnsBoundedReceipt(t *testing.T) {
	request := validRequest()
	packet, receipt, err := Declassify(request)
	if err != nil {
		t.Fatal(err)
	}
	if packet.PacketSHA256 == "" || receipt.PacketSHA256 != packet.PacketSHA256 ||
		receipt.ExpertID != request.Expert.ID || receipt.ExpertVersion != request.Expert.Version ||
		receipt.CanonSHA256 != request.Expert.CanonSHA256 || receipt.MayExport {
		t.Fatalf("unexpected PA Expert boundary receipt: %#v", receipt)
	}
	response := Response{
		SchemaVersion: 1, PacketSHA256: packet.PacketSHA256,
		ExpertID: request.Expert.ID, ExpertVersion: request.Expert.Version,
		CanonSHA256: request.Expert.CanonSHA256, Findings: []string{"market signal is mixed"},
	}
	if _, err := ValidateResponse(packet, response, receipt); err != nil {
		t.Fatalf("bounded response rejected: %v", err)
	}
}

func TestDeclassifyCanonicalizesFactAndSectionOrder(t *testing.T) {
	first := validRequest()
	first.Facts = append(first.Facts, Fact{Code: "capacity-signal", Classification: "public", ValueCode: "stable"})
	first.OutputSections = []string{"challenges", "findings", "assumptions"}
	second := first
	second.Facts = []Fact{first.Facts[1], first.Facts[0]}
	second.OutputSections = []string{"assumptions", "findings", "challenges"}
	left, _, err := Declassify(first)
	if err != nil {
		t.Fatal(err)
	}
	right, _, err := Declassify(second)
	if err != nil {
		t.Fatal(err)
	}
	if left.PacketSHA256 != right.PacketSHA256 {
		t.Fatalf("packet digest changed with equivalent ordering: %s != %s", left.PacketSHA256, right.PacketSHA256)
	}
}

func TestDeclassifyRejectsForgedScopeIdentifiersAndRawPointers(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Request)
	}{
		{"forged client code", func(value *Request) { value.Facts[0].ValueCode = "client-alpha" }},
		{"workspace code", func(value *Request) { value.Facts[0].Code = "workspace-fact" }},
		{"raw pointer", func(value *Request) { value.Facts[0].ValueCode = "bcgos-workspace-alpha" }},
		{"missing scoped-pointer attestation", func(value *Request) { value.Attestation.NoScopedPointers = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validRequest()
			test.mutate(&request)
			if _, _, err := Declassify(request); err == nil {
				t.Fatal("forged or unclassified content crossed the PA Expert boundary")
			}
		})
	}
}

func TestDeclassifyRejectsCrossScopePathAndForgedCanon(t *testing.T) {
	request := validRequest()
	request.SourceSHA256 = strings.Repeat("g", 64)
	if _, _, err := Declassify(request); err == nil {
		t.Fatal("invalid source digest was accepted")
	}
	request = validRequest()
	request.Expert.CanonSHA256 = strings.Repeat("a", 63) + "!"
	if _, _, err := Declassify(request); err == nil {
		t.Fatal("forged canon digest was accepted")
	}
}

func TestResponseRejectsCrossScopeContentAndReceiptSubstitution(t *testing.T) {
	packet, receipt, err := Declassify(validRequest())
	if err != nil {
		t.Fatal(err)
	}
	response := Response{
		SchemaVersion: 1, PacketSHA256: packet.PacketSHA256,
		ExpertID: packet.Expert.ID, ExpertVersion: packet.Expert.Version,
		CanonSHA256: packet.Expert.CanonSHA256,
		Findings:    []string{"use bcgos://workspace/other/raw"},
	}
	if _, err := ValidateResponse(packet, response, receipt); err == nil {
		t.Fatal("raw scoped pointer entered PA Expert response")
	}
	response.Findings = []string{"bounded finding"}
	forged := receipt
	forged.CanonSHA256 = strings.Repeat("b", 64)
	if _, err := ValidateResponse(packet, response, forged); err == nil {
		t.Fatal("receipt substitution was accepted")
	}
}

func validRequest() Request {
	return Request{
		SchemaVersion: 1, PacketID: "packet-01",
		SourceSHA256: digest("case-source"), PlanSHA256: digest("plan"),
		Expert: activationpolicy.PAExpert{
			ID: "pa-expert-fpa-pricing", Kind: activationpolicy.ExpertFPA,
			Version: "1.0.0", CanonSHA256: digest("canon"), Lifecycle: activationpolicy.Published,
		},
		QuestionCode: "pricing-signal", Classification: "internal",
		Facts:          []Fact{{Code: "market-signal", Classification: "internal", ValueCode: "demand-up"}},
		OutputSections: []string{"findings", "challenges"},
		Attestation: Attestation{
			ExporterID: "maestro", NoClientIdentifiers: true,
			NoStakeholderIdentifiers: true, NoRawExcerpts: true, NoScopedPointers: true,
		},
	}
}

func digest(value string) string {
	return activationpolicy.SHA256Hex([]byte(value))
}
