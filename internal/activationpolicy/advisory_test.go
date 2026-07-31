package activationpolicy

import "testing"

func TestAdvisoryExportRejectsScopedOrConfidentialContent(t *testing.T) {
	base := AdvisoryRequest{
		SchemaVersion: 1, RequestID: "advisory-01",
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
