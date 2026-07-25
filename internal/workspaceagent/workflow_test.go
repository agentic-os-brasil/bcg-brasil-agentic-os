package workspaceagent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResearchWorkflowRequiresApprovalBeforeEvidence(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root, "ws-123"); err != nil {
		t.Fatal(err)
	}
	plan, err := CreateResearchPlan(root, ResearchPlan{
		WorkspaceID: "ws-123",
		ValidUntil:  time.Now().UTC().Add(time.Hour),
		MaxQueries:  1,
		Purpose:     "understand public market conditions",
		QueryThemes: []string{"public market size"},
		Sources:     []string{"ibge.gov.br", "bcb.gov.br"},
	})
	if err != nil || plan.PlanID == "" || plan.State != "proposed" {
		t.Fatalf("CreateResearchPlan() = %#v, %v", plan, err)
	}
	evidence := Evidence{
		WorkspaceID: "ws-123", PlanID: plan.PlanID,
		Query:     "public market size",
		SourceURL: "https://www.ibge.gov.br/example", RetrievedAt: time.Now().UTC(),
		ValidUntil: time.Now().UTC().Add(24 * time.Hour),
		Claim:      "Public market fact", EvidenceStrength: "primary", Classification: "public",
	}
	if !errors.Is(RecordEvidence(root, evidence), ErrResearchApprovalRequired) {
		t.Fatalf("RecordEvidence() before approval should fail closed")
	}
	plan, err = ApproveResearchPlan(root, "ws-123", plan.PlanID, Approval{
		ApprovedAt: time.Now().UTC(), ApprovedBy: "owner", DisclosureLevel: "public_only",
	})
	if err != nil || plan.State != "approved" {
		t.Fatalf("ApproveResearchPlan() = %#v, %v", plan, err)
	}
	if _, err := ConsumeResearchQuery(root, QueryExecution{WorkspaceID: "ws-123", PlanID: plan.PlanID, Query: "public market size"}); err != nil {
		t.Fatalf("ConsumeResearchQuery() error = %v", err)
	}
	if _, err := ConsumeResearchQuery(root, QueryExecution{WorkspaceID: "ws-123", PlanID: plan.PlanID, Query: "public market size"}); err == nil || !strings.Contains(err.Error(), "exhausted") {
		t.Fatalf("ConsumeResearchQuery() second execution error = %v, want exhausted budget", err)
	}
	if err := RecordEvidence(root, evidence); err != nil {
		t.Fatalf("RecordEvidence() error = %v", err)
	}
	evidence.SourceURL = "https://example.com/outside-allowlist"
	if err := RecordEvidence(root, evidence); err == nil {
		t.Fatal("RecordEvidence() accepted a source outside the approved allowlist")
	}
	entries, err := os.ReadDir(filepath.Join(root, "workspaces", "ws-123", "dossier", "evidence"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("evidence entries = %v, %v", entries, err)
	}
}

func TestEvidenceRequiresConsumedQueryAndCurrentValidity(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root, "ws-evidence"); err != nil {
		t.Fatal(err)
	}
	plan, err := CreateResearchPlan(root, ResearchPlan{
		WorkspaceID: "ws-evidence", ValidUntil: time.Now().UTC().Add(time.Hour), MaxQueries: 1,
		Purpose: "public context", QueryThemes: []string{"market size"}, Sources: []string{"ibge.gov.br"},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err = ApproveResearchPlan(root, "ws-evidence", plan.PlanID, Approval{ApprovedAt: time.Now().UTC(), ApprovedBy: "owner", DisclosureLevel: "public_only"})
	if err != nil {
		t.Fatal(err)
	}
	evidence := Evidence{
		WorkspaceID: "ws-evidence", PlanID: plan.PlanID, Query: "market size",
		SourceURL: "https://www.ibge.gov.br/example", RetrievedAt: time.Now().UTC().Add(-time.Hour),
		ValidUntil: time.Now().UTC().Add(time.Hour), Claim: "Public fact",
		EvidenceStrength: "primary", Classification: "public",
	}
	if err := RecordEvidence(root, evidence); err == nil || !strings.Contains(err.Error(), "not consumed") {
		t.Fatalf("RecordEvidence() error = %v, want unconsumed query rejection", err)
	}
	if _, err := ConsumeResearchQuery(root, QueryExecution{WorkspaceID: "ws-evidence", PlanID: plan.PlanID, Query: "market size"}); err != nil {
		t.Fatal(err)
	}
	evidence.ValidUntil = time.Now().UTC().Add(-time.Minute)
	if err := RecordEvidence(root, evidence); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("RecordEvidence() error = %v, want expired evidence rejection", err)
	}
}

func TestEconomicSnapshotRequiresIndependentPublicAttestationAndClaimProvenance(t *testing.T) {
	root := t.TempDir()
	source := PublicSource{URL: "https://www.bcb.gov.br/example", RetrievedAt: time.Now().UTC()}
	claim := PublicClaim{Statement: "Public macroeconomic claim", Classification: "public", SourceURLs: []string{source.URL}}
	if _, err := SaveEconomicSnapshot(root, EconomicSnapshot{AsOf: time.Now().UTC(), Claims: []PublicClaim{claim}, Sources: []PublicSource{source}}); err == nil {
		t.Fatal("SaveEconomicSnapshot() accepted a snapshot without attestation")
	}
	workspaceDerived := EconomicSnapshot{
		AsOf: time.Now().UTC(), Claims: []PublicClaim{claim}, Sources: []PublicSource{source},
		Attestation: PublicAttestation{AttestedBy: "owner", AttestedAt: time.Now().UTC(), Origin: "workspace_synthesis", NoWorkspaceDerivation: false},
	}
	if _, err := SaveEconomicSnapshot(root, workspaceDerived); err == nil {
		t.Fatal("SaveEconomicSnapshot() accepted workspace-derived material")
	}
	snapshot, err := SaveEconomicSnapshot(root, EconomicSnapshot{
		AsOf: time.Now().UTC(), Claims: []PublicClaim{claim}, Sources: []PublicSource{source},
		Attestation: PublicAttestation{AttestedBy: "owner", AttestedAt: time.Now().UTC(), Origin: "independent_public_sources", NoWorkspaceDerivation: true},
	})
	if err != nil || snapshot.SnapshotID == "" {
		t.Fatalf("SaveEconomicSnapshot() = %#v, %v", snapshot, err)
	}
	if _, err := Initialize(root, "ws-a"); err != nil {
		t.Fatal(err)
	}
	if err := AttachEconomicSnapshot(root, "ws-a", snapshot.SnapshotID); err != nil {
		t.Fatalf("AttachEconomicSnapshot() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "workspaces", "ws-a", "dossier", "economic-snapshot.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "economic", "public", "snapshots", snapshot.SnapshotID+".json")); err != nil {
		t.Fatal(err)
	}
}

func TestSaveBriefVersionsInterviewContextOutsideCompactState(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root, "ws-brief"); err != nil {
		t.Fatal(err)
	}
	brief, err := SaveBrief(root, Brief{
		WorkspaceID: "ws-brief", ReviewedBy: "owner", Classification: "confidential",
		Mandate: "Support one client decision", Objectives: []string{"deliver recommendation"},
		Stakeholders: []string{"project sponsor"}, Constraints: []string{"four-week horizon"},
		Bullish: []Thesis{{Statement: "Upside hypothesis", Evidence: []string{"public signal"}, Assumptions: []string{"adoption grows"}, CounterEvidence: []string{"weak conversion"}, InvalidationSignals: []string{"demand declines"}}},
		Bearish: []Thesis{{Statement: "Downside hypothesis", Evidence: []string{"public risk"}, Assumptions: []string{"cost remains high"}, CounterEvidence: []string{"efficiency improves"}, InvalidationSignals: []string{"cost falls"}}},
	})
	if err != nil || brief.BriefID == "" {
		t.Fatalf("SaveBrief() = %#v, %v", brief, err)
	}
	if _, err := os.Stat(filepath.Join(root, "workspaces", "ws-brief", "dossier", "briefs", brief.BriefID+".json")); err != nil {
		t.Fatal(err)
	}
	stateBytes, err := os.ReadFile(filepath.Join(root, "workspaces", "ws-brief", "agent", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state OperationalState
	if err := json.Unmarshal(stateBytes, &state); err != nil {
		t.Fatal(err)
	}
	if state.CurrentBriefID != brief.BriefID || state.CurrentObjective != "deliver recommendation" || strings.Contains(string(stateBytes), "Upside hypothesis") {
		t.Fatalf("compact state does not contain only operational pointers: %s", stateBytes)
	}
}

func TestBriefRequiresBalancedTheses(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root, "ws-theses"); err != nil {
		t.Fatal(err)
	}
	_, err := SaveBrief(root, Brief{
		WorkspaceID: "ws-theses", ReviewedBy: "owner", Classification: "internal",
		Mandate: "Support a decision", Objectives: []string{"recommendation"},
		Bullish: []Thesis{{Statement: "Upside", Evidence: []string{"signal"}, Assumptions: []string{"growth"}, CounterEvidence: []string{"weakness"}, InvalidationSignals: []string{"decline"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "bullish and one bearish") {
		t.Fatalf("SaveBrief() error = %v, want balanced thesis requirement", err)
	}
}

func TestExpiredResearchPlanFailsClosed(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root, "ws-expired"); err != nil {
		t.Fatal(err)
	}
	_, err := CreateResearchPlan(root, ResearchPlan{
		WorkspaceID: "ws-expired",
		ValidUntil:  time.Now().UTC().Add(-time.Minute),
		MaxQueries:  1,
		Purpose:     "public market context",
		QueryThemes: []string{"market size"},
		Sources:     []string{"ibge.gov.br"},
	})
	if err == nil || !strings.Contains(err.Error(), "future") {
		t.Fatalf("CreateResearchPlan() error = %v, want expired plan rejection", err)
	}
}
