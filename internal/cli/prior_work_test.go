package cli

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/priorwork"
)

var cliCollectorPrivateKey = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x31}, ed25519.SeedSize))

func cliPriorWorkEnrollment(t *testing.T) string {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	publicKey := cliCollectorPrivateKey.Public().(ed25519.PublicKey)
	actor, err := localPriorWorkActorRef()
	if err != nil {
		t.Fatal(err)
	}
	enrollment := priorwork.Enrollment{
		SchemaVersion: 1, TenantRef: "tenant-br", Purpose: "prior_work_retrieval",
		PolicyVersion: "spwk-v1", AuthorizedBy: actor,
		CollectorKeyID:     "claude-sharepoint-collector-v1",
		CollectorPublicKey: base64.StdEncoding.EncodeToString(publicKey),
		EnrolledAt:         now, AuthorizationExpiresAt: now.AddDate(1, 0, 0),
		ScopeExpansionConfirmAfter: now.AddDate(0, 6, 0),
		RefreshHours:               24, StaleHours: 72,
		ScheduleTimezone: "America/Sao_Paulo",
		MaxItemBytes:     100_000_000,
		MaxSnapshotItems: 10_000, AllowedItemTypes: []string{"file", "folder"},
		AllowedOrigins: []string{"https://bcgbr.sharepoint.com"},
		Roots: []priorwork.RootRef{{
			SiteRef: "site-consulting", DriveRef: "drive-projects", FolderRef: "folder-enrolled",
		}},
	}
	body, err := json.Marshal(enrollment)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestPriorWorkEnrollAndStatusUsePrivateOrganizationalStore(t *testing.T) {
	dataRoot := t.TempDir()
	resolve := func() (string, error) { return dataRoot, nil }
	var output bytes.Buffer
	if code := runPriorWork(
		[]string{"enroll", "--stdin", "--confirm"},
		strings.NewReader(cliPriorWorkEnrollment(t)), &output, &output, resolve,
	); code != ExitOK || !strings.Contains(output.String(), `"state": "enrolled"`) {
		t.Fatalf("enroll exit=%d output=%s", code, output.String())
	}
	output.Reset()
	if code := runPriorWork(
		[]string{"status"},
		strings.NewReader(""), &output, &output, resolve,
	); code != ExitOK || !strings.Contains(output.String(), `"state": "absent"`) ||
		!strings.Contains(output.String(), `"due": true`) {
		t.Fatalf("status exit=%d output=%s", code, output.String())
	}
}

func TestPriorWorkRequiresConfirmationExplicitIntentAndStdin(t *testing.T) {
	dataRoot := t.TempDir()
	resolve := func() (string, error) { return dataRoot, nil }
	var output bytes.Buffer
	if code := runPriorWork(
		[]string{"enroll", "--stdin"},
		strings.NewReader(cliPriorWorkEnrollment(t)), &output, &output, resolve,
	); code != ExitUsage || !strings.Contains(output.String(), "--confirm") {
		t.Fatalf("unconfirmed enroll exit=%d output=%s", code, output.String())
	}
	output.Reset()
	if code := runPriorWork(
		[]string{"find", "--stdin"},
		strings.NewReader("Suzano Plantio"), &output, &output, resolve,
	); code != ExitUsage || !strings.Contains(output.String(), "--explicit") {
		t.Fatalf("implicit find exit=%d output=%s", code, output.String())
	}
	output.Reset()
	if code := runPriorWork(
		[]string{"find", "--explicit"},
		strings.NewReader("Suzano Plantio"), &output, &output, resolve,
	); code != ExitUsage || !strings.Contains(output.String(), "--stdin") {
		t.Fatalf("argv query exit=%d output=%s", code, output.String())
	}
}

func TestPriorWorkCLIEndToEndSignedImportAndSuzanoFind(t *testing.T) {
	dataRoot := t.TempDir()
	resolve := func() (string, error) { return dataRoot, nil }
	enrollmentBody := cliPriorWorkEnrollment(t)
	var enrollment priorwork.Enrollment
	if err := json.Unmarshal([]byte(enrollmentBody), &enrollment); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := runPriorWork(
		[]string{"enroll", "--stdin", "--confirm"},
		strings.NewReader(enrollmentBody), &output, &output, resolve,
	); code != ExitOK {
		t.Fatalf("enroll exit=%d output=%s", code, output.String())
	}
	enrollmentPath := filepath.Join(dataRoot, "atlases", "organization", "sharepoint-work", "enrollment.json")
	info, err := os.Stat(enrollmentPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("enrollment permissions=%o", info.Mode().Perm())
	}

	root := enrollment.Roots[0]
	now := enrollment.EnrolledAt
	snapshot := priorwork.Snapshot{
		SchemaVersion: 1, Source: "sharepoint", AdapterRuntime: "claude",
		TenantRef: enrollment.TenantRef, Mode: "full", CollectionSequence: 1,
		GeneratedAt: now, Watermark: "watermark-1", Roots: enrollment.Roots,
		RootResults: []priorwork.RootResult{{Root: root, State: "complete"}},
		Items: []priorwork.Item{{
			ItemRef: "item-suzano-plantio", ParentRef: root.FolderRef, Root: root,
			Kind: "file", Name: "Suzano CEO - Plantio 2023.pptx",
			PathSegments: []string{"Clientes", "Suzano", "Plantio"},
			SourceURL:    "https://bcgbr.sharepoint.com/sites/consulting/Shared%20Documents/Suzano-Plantio-2023.pptx",
			CreatedAt:    now, ModifiedAt: now.Add(time.Hour), SizeBytes: 4_200_000,
			MediaType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
			ETag:      "etag-suzano-v1",
			Facets: priorwork.Facets{
				Clients: []string{"Suzano"}, Projects: []string{"Plantio estratégico"},
				Themes: []string{"Plantio"}, Years: []int{2023}, Audiences: []string{"CEO"},
				People: []string{"CEO da Suzano"}, Presenters: []string{"Daniel Scardini"},
			},
			SearchTerms: []string{"silvicultura", "deck executivo"},
			Sensitivity: "client_restricted", Status: "active",
		}},
		Tombstones: []priorwork.Tombstone{},
	}
	receipt, signingBody, err := priorwork.BuildUnsignedImportReceipt(
		snapshot, enrollment, "receipt-cli-e2e", "trigger-manual-cli", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(cliCollectorPrivateKey, signingBody))
	snapshotPath := filepath.Join(t.TempDir(), "snapshot.json")
	receiptPath := filepath.Join(t.TempDir(), "receipt.json")
	for path, value := range map[string]any{snapshotPath: snapshot, receiptPath: receipt} {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	output.Reset()
	if code := runPriorWork(
		[]string{"import", "--snapshot", snapshotPath, "--receipt", receiptPath},
		strings.NewReader(""), &output, &output, resolve,
	); code != ExitOK || !strings.Contains(output.String(), `"state": "published"`) {
		t.Fatalf("import exit=%d output=%s", code, output.String())
	}
	output.Reset()
	if code := runPriorWork(
		[]string{"find", "--explicit", "--stdin"},
		strings.NewReader("quero o deck que apresentei pro CEO da Suzano em 2023 sobre PLANTIO"),
		&output, &output, resolve,
	); code != ExitOK || !strings.Contains(output.String(), "Suzano CEO - Plantio 2023.pptx") ||
		!strings.Contains(output.String(), `"catalog_freshness": "fresh"`) {
		t.Fatalf("find exit=%d output=%s", code, output.String())
	}
	output.Reset()
	if code := runPriorWork(
		[]string{"status", "--purpose", "general_research"},
		strings.NewReader(""), &output, &output, resolve,
	); code != ExitFailure || !strings.Contains(output.String(), "not authorized") {
		t.Fatalf("wrong purpose exit=%d output=%s", code, output.String())
	}
}
