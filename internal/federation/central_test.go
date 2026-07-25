package federation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCentralInboxAcceptsOnlyTrustedTypedBatchesAndDeduplicates(t *testing.T) {
	batch := validBatch()
	inbox := CentralInbox{Root: t.TempDir(), AllowedInstallations: map[string]bool{batch.InstallationID: true}}
	accepted, err := inbox.Accept(batch)
	if err != nil || !accepted {
		t.Fatalf("first accept = %v, %v", accepted, err)
	}
	accepted, err = inbox.Accept(batch)
	if err != nil || accepted {
		t.Fatalf("duplicate accept = %v, %v", accepted, err)
	}
	denied := batch
	denied.InstallationID = "fedcba9876543210"
	if _, err := inbox.Accept(denied); err == nil {
		t.Fatal("untrusted installation was accepted")
	}
	digest, err := inbox.Digest(batch.Period)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(digest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), batch.InstallationID) || digest.BatchCount != 1 {
		t.Fatalf("digest exposed installation identity or count: %s", encoded)
	}
}

func TestCentralBridgeCompletesThePilotHTTPSBatchPath(t *testing.T) {
	batch := validBatch()
	inbox := CentralInbox{Root: t.TempDir(), AllowedInstallations: map[string]bool{batch.InstallationID: true}}
	server := httptest.NewTLSServer(CentralBridge{Inbox: inbox})
	defer server.Close()
	bridge, err := NewHTTPBridge(server.URL+"/federation/v1/batches", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.Submit(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	digest, err := inbox.Digest(batch.Period)
	if err != nil || digest.BatchCount != 1 {
		t.Fatalf("digest = %#v, err = %v", digest, err)
	}
}

func TestCentralDarwinCuratorCreatesHumanOnlyProposalAfterCohortThreshold(t *testing.T) {
	batch := validBatch()
	second := validBatch()
	second.InstallationID = "fedcba9876543210"
	inbox := CentralInbox{Root: t.TempDir(), AllowedInstallations: map[string]bool{batch.InstallationID: true, second.InstallationID: true}}
	if _, err := inbox.Accept(batch); err != nil {
		t.Fatal(err)
	}
	if _, err := inbox.Accept(second); err != nil {
		t.Fatal(err)
	}
	digest, err := inbox.Digest(batch.Period)
	if err != nil {
		t.Fatal(err)
	}
	proposals, err := (CentralDarwinCurator{}).Curate(digest)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) != 1 || !proposals[0].RequiresHumanAcceptance || proposals[0].Status != ProposalDraft {
		t.Fatalf("proposals = %#v", proposals)
	}
	issue := proposals[0].Issue()
	if !strings.Contains(issue.Body, "Human maintainer") || !strings.Contains(issue.Title, "pilot") {
		t.Fatalf("issue = %#v", issue)
	}
}

func TestCentralDarwinRoutesRepeatedFailureAsActionableIncident(t *testing.T) {
	batch := validBatch()
	batch.Signals[0].Kind = SignalFailure
	batch.Signals[0].Outcome = OutcomeFailed
	second := batch
	second.InstallationID = "fedcba9876543210"
	inbox := CentralInbox{Root: t.TempDir(), AllowedInstallations: map[string]bool{batch.InstallationID: true, second.InstallationID: true}}
	if _, err := inbox.Accept(batch); err != nil {
		t.Fatal(err)
	}
	if _, err := inbox.Accept(second); err != nil {
		t.Fatal(err)
	}
	digest, err := inbox.Digest(batch.Period)
	if err != nil {
		t.Fatal(err)
	}
	proposals, err := (CentralDarwinCurator{}).Curate(digest)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) != 1 || proposals[0].Kind != ProposalIncident || proposals[0].Issue().Labels[1] != "pilot-incident" {
		t.Fatalf("proposals = %#v", proposals)
	}
}

func TestGitHubAppIssuePublisherUsesOnlyCentralInstallationToken(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.URL.Path != "/repos/agentic-os-brasil/bcg-brasil-agentic-os/issues" {
			t.Fatalf("path = %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer central-installation-token" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		writer.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	publisher, err := NewGitHubAppIssuePublisher(server.URL, "agentic-os-brasil", "bcg-brasil-agentic-os", staticInstallationToken("central-installation-token"), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	proposal := AdvancementProposal{SchemaVersion: SchemaVersion, ID: strings.Repeat("a", 64), Period: "2026-W30", Kind: ProposalGuidance, Template: TemplateFrictionGuidance, Evidence: EvidenceTwoToThree, Status: ProposalDraft, RequiresHumanAcceptance: true}
	if err := publisher.Publish(context.Background(), proposal.Issue()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestPublishedAdvancementProposalSchemaIsRecognized(t *testing.T) {
	if err := ValidateAdvancementProposalSchemaFile("../../schemas/advancement-proposal.schema.json"); err != nil {
		t.Fatal(err)
	}
}

type staticInstallationToken string

func (token staticInstallationToken) InstallationToken(context.Context) (string, error) {
	return string(token), nil
}
