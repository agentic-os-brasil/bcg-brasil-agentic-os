package darwin

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestEvolutionEpisodePinsPolicyAndApprovedPortfolio(t *testing.T) {
	episode := testEvolutionEpisode()
	if err := episode.Validate(); err != nil {
		t.Fatal(err)
	}
	episode.Policy.PolicyVersion = "pae-v2"
	if err := episode.Validate(); err != nil {
		t.Fatal(err)
	}
	episode.Portfolio.ApprovedBy = AgentID
	if err := episode.Validate(); err == nil {
		t.Fatal("Darwin self-approved portfolio was accepted")
	}
	episode = testEvolutionEpisode()
	episode.Portfolio.SnapshotSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := episode.Validate(); err == nil {
		t.Fatal("tampered portfolio digest was accepted")
	}
}

func TestEvolutionStoreRecoversAndReplaysAcrossSessions(t *testing.T) {
	root := t.TempDir()
	first := EvolutionStore{Root: root}
	if err := first.AppendEpisode(testEvolutionEpisode()); err != nil {
		t.Fatal(err)
	}
	interrupted := EpisodeEvent{EventID: "event-interrupted", EpisodeID: "episode-1", Revision: 1, State: EventInterrupted, RecordedAt: testTime}
	if err := first.AppendEpisodeEvent(interrupted); err != nil {
		t.Fatal(err)
	}
	window := testEvidenceWindow()
	if err := first.AppendWindow(window); err != nil {
		t.Fatal(err)
	}
	proposal := testEvolutionProposal(window)
	if err := first.AppendProposal(proposal); err != nil {
		t.Fatal(err)
	}

	second := EvolutionStore{Root: root}
	recovered, events, err := second.RecoverEpisode("episode-1")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State != EpisodeInterrupted || recovered.Revision != 1 || len(events) != 1 {
		t.Fatalf("recovered = %#v, events = %#v", recovered, events)
	}
	if err := second.AppendEpisodeEvent(interrupted); err != nil {
		t.Fatalf("same event replay was not idempotent: %v", err)
	}
	conflict := interrupted
	conflict.State = EventClosed
	if err := second.AppendEpisodeEvent(conflict); !errors.Is(err, ErrEvolutionReplayConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}
	if err := second.AppendEpisodeEvent(EpisodeEvent{EventID: "event-resumed", EpisodeID: "episode-1", Revision: 2, State: EventResumed, RecordedAt: testTime.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	decision := EvolutionDecisionReceipt{ReceiptID: "decision-1", ProposalID: proposal.Proposal.ProposalID, ProposalSHA256: proposal.ProposalSHA256, Decision: "approved", ApproverID: "walter", RecordedAt: testTime}
	if err := second.AppendDecision(decision); err != nil {
		t.Fatal(err)
	}
	windows, err := second.RecoverWindow("window-1")
	if err != nil || len(windows) != 1 {
		t.Fatalf("windows = %#v, err = %v", windows, err)
	}
	if err := second.AppendWindow(window); err != nil {
		t.Fatalf("same window replay was not idempotent: %v", err)
	}
	window.Observations[0].DurationSeconds++
	window.WindowSHA256 = ""
	window.WindowSHA256 = digestJSON(window)
	if err := second.AppendWindow(window); !errors.Is(err, ErrEvolutionReplayConflict) {
		t.Fatalf("conflicting window replay error = %v", err)
	}
}

func TestEvolutionRecoveryIgnoresIncompleteTemporaryFiles(t *testing.T) {
	root := t.TempDir()
	store := EvolutionStore{Root: root}
	if err := store.AppendEpisode(testEvolutionEpisode()); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEpisodeEvent(EpisodeEvent{EventID: "event-interrupted", EpisodeID: "episode-1", Revision: 1, State: EventInterrupted, RecordedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	temporary := filepath.Join(root, "evolution", "episodes", "episode-1", "events", ".evolution-crash.tmp")
	if err := os.WriteFile(temporary, []byte(`{"partial":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, events, err := store.RecoverEpisode("episode-1"); err != nil || len(events) != 1 {
		t.Fatalf("recovery = %v, events = %#v", err, events)
	}
}

func TestEvolutionHealthReceiptsRemainSeparate(t *testing.T) {
	root := t.TempDir()
	health := Store{Root: root}
	if err := health.Append(Receipt{SchemaVersion: SchemaVersion, AgentID: AgentID, DisplayName: DisplayName, Emoji: Emoji, WindowID: "health-1", Mode: Interactive, Outcome: OutcomeNoAction, RecordedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	evolution := EvolutionStore{Root: root}
	if err := evolution.AppendEpisode(testEvolutionEpisode()); err != nil {
		t.Fatal(err)
	}
	receipts, err := health.Receipts()
	if err != nil || len(receipts) != 1 || receipts[0].WindowID != "health-1" {
		t.Fatalf("health receipts = %#v, err = %v", receipts, err)
	}
	if windows, err := evolution.RecoverWindow("health-1"); err != nil || len(windows) != 0 {
		t.Fatalf("evolution saw health records: %#v, err = %v", windows, err)
	}
}

func TestNativeEvolutionPersistenceIsExplicitlyUnavailable(t *testing.T) {
	report := (EvolutionStore{Root: t.TempDir()}).Capability()
	if report.LocalState != "available" || report.NativeState != "unavailable" || report.NativeReason == "" {
		t.Fatalf("capability report = %#v", report)
	}
	if !errors.Is(NativeEvolutionPersistence(), ErrNativeEvolutionPersistence) {
		t.Fatal("native persistence did not fail closed")
	}
}

func TestEvolutionSchemaRejectsContextBearingFields(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "schemas", "darwin-evolution.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("darwin-evolution.schema.json", document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("darwin-evolution.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	valid := decodeEvolutionJSON(t, `{"record_type":"decision_receipt","schema_version":1,"receipt_id":"receipt-1","proposal_id":"proposal-1","proposal_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","decision":"approved","approver_id":"walter","recorded_at":"2026-07-30T12:00:00Z","receipt_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`)
	if err := schema.Validate(valid); err != nil {
		t.Fatal(err)
	}
	proposalBody, err := json.Marshal(testEvolutionProposal(testEvidenceWindow()))
	if err != nil {
		t.Fatal(err)
	}
	var proposalValue any
	if err := json.Unmarshal(proposalBody, &proposalValue); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(proposalValue); err != nil {
		t.Fatalf("valid proposal rejected by schema: %v", err)
	}
	invalid := decodeEvolutionJSON(t, `{"record_type":"decision_receipt","schema_version":1,"receipt_id":"receipt-1","proposal_id":"proposal-1","proposal_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","decision":"approved","approver_id":"walter","recorded_at":"2026-07-30T12:00:00Z","receipt_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","prompt":"secret"}`)
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("schema accepted context-bearing prompt")
	}
}

var testTime = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

func testEvolutionPolicy() PolicyPin {
	return PolicyPin{PolicyID: "activation-policy", PolicyVersion: "pae-v1", PlanSHA256: digestJSON("plan")}
}

func testPortfolio() PortfolioSnapshot {
	portfolio := PortfolioSnapshot{
		SchemaVersion:     EvolutionPersistenceSchemaVersion,
		Authority:         "pa-expert-registry-v2",
		ApprovedBy:        "managed_registry",
		ApprovalRefSHA256: digestJSON("registry"),
		Experts:           []PortfolioExpert{{ID: "pa-expert-fpa-pricing", Kind: "FPA", Version: "1.0.0", CanonSHA256: digestJSON("canon")}},
	}
	portfolio.SnapshotSHA256 = digestJSON(portfolio)
	return portfolio
}

func testEvolutionEpisode() EvolutionEpisode {
	return EvolutionEpisode{
		RecordType: "episode", SchemaVersion: EvolutionPersistenceSchemaVersion,
		EpisodeID: "episode-1", WindowID: "window-1", Policy: testEvolutionPolicy(), Portfolio: testPortfolio(),
		State: EpisodeOpen, Revision: 0, CreatedAt: testTime, UpdatedAt: testTime,
	}
}

func testEvidenceWindow() EvidenceWindow {
	window := EvidenceWindow{
		RecordType: "evidence_window", SchemaVersion: EvolutionPersistenceSchemaVersion,
		WindowID: "window-1", Version: 1, Policy: testEvolutionPolicy(), Portfolio: testPortfolio(),
		Observations: []EpisodeObservation{{SchemaVersion: EvolutionPersistenceSchemaVersion, EpisodeID: "episode-1", PlanSHA256: digestJSON("plan"), Route: "D0_DIRECT", Outcome: "completed", DurationSeconds: 12}},
		RecordedAt:   testTime,
	}
	window.WindowSHA256 = digestJSON(window)
	return window
}

func testEvolutionProposal(window EvidenceWindow) EvolutionProposalArtifact {
	proposal := EvolutionProposalArtifact{
		RecordType: "proposal", SchemaVersion: EvolutionPersistenceSchemaVersion,
		Proposal: StructuralProposal{
			SchemaVersion: EvolutionSchemaVersion, ProposalID: "proposal-1", PolicyVersion: "pae-v1",
			Cadence: EvolutionWeekly, Target: EvolutionPolicy,
			CurrentDigest:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ProposedDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			EvidenceWindow: window.WindowID, ApprovalState: "proposal_only",
		},
		Policy: testEvolutionPolicy(), Portfolio: testPortfolio(), EvidenceWindowID: window.WindowID,
		EvidenceWindowSHA256: window.WindowSHA256, CreatedAt: testTime,
	}
	proposal.ProposalSHA256 = digestJSON(proposal)
	return proposal
}

func decodeEvolutionJSON(t *testing.T, raw string) any {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}
