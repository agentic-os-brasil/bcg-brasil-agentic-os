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
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/setupauth"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

var cliCollectorPrivateKey = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x31}, ed25519.SeedSize))
var cliEnrollmentAuthorityPrivateKey = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x13}, ed25519.SeedSize))

func cliPriorWorkEnrollment(t *testing.T) string {
	t.Helper()
	publicAuthorityKey := cliEnrollmentAuthorityPrivateKey.Public().(ed25519.PublicKey)
	t.Setenv("BCGOS_PRIOR_WORK_AUTHORITY_KEY_ID", "admin-authority-v1")
	t.Setenv("BCGOS_PRIOR_WORK_AUTHORITY_PUBLIC_KEY", base64.StdEncoding.EncodeToString(publicAuthorityKey))
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
		RefreshHours:               24, StaleHours: 72, ScheduleTimezone: "America/Sao_Paulo",
		MaxItemBytes:     100_000_000,
		MaxSnapshotItems: 10_000, AllowedItemTypes: []string{"file", "folder"},
		AllowedOrigins: []string{"https://bcgbr.sharepoint.com"},
		Roots: []priorwork.RootRef{{
			SiteRef: "site-consulting", DriveRef: "drive-projects", FolderRef: "folder-enrolled",
		}},
	}
	signingBody, err := priorwork.EnrollmentAuthoritySigningBody(enrollment)
	if err != nil {
		t.Fatal(err)
	}
	enrollment.AuthorityKeyID = "admin-authority-v1"
	enrollment.AuthoritySignature = base64.StdEncoding.EncodeToString(ed25519.Sign(cliEnrollmentAuthorityPrivateKey, signingBody))
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

func TestGuidedSharePointSourceSelectionWorksBeforeEnrollmentTrustIsAvailable(t *testing.T) {
	dataRoot := t.TempDir()
	workspacePath := filepath.Join(t.TempDir(), "maestro-project")
	resolve := func() (string, error) { return dataRoot, nil }
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, resolve); code != ExitOK {
		t.Fatalf("init exit=%d output=%s", code, output.String())
	}
	inspection, err := workspace.Inspect(workspacePath, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := currentSetupIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (setupauth.Store{Root: dataRoot}).Authorize(setupauth.Request{WorkspaceID: inspection.WorkspaceID, WorkspacePath: workspacePath}, identity, true); err != nil {
		t.Fatal(err)
	}

	output.Reset()
	if code := runPriorWork(
		[]string{"source", "status", "--workspace", workspacePath},
		strings.NewReader(""), &output, &output, resolve,
	); code != ExitOK || !strings.Contains(output.String(), `"state": "selection_required"`) || strings.Contains(output.String(), "trust anchor") {
		t.Fatalf("initial source status exit=%d output=%s", code, output.String())
	}

	output.Reset()
	selection := `{"schema_version":1,"folder_urls":["https://bcg.sharepoint.com/sites/project/Shared%20Documents/Authorized-Folder"]}`
	if code := runPriorWork(
		[]string{"source", "select", "--workspace", workspacePath, "--stdin"},
		strings.NewReader(selection), &output, &output, resolve,
	); code != ExitOK || !strings.Contains(output.String(), `"state": "selected"`) || !strings.Contains(output.String(), `"authorization_state": "pending_signed_enrollment"`) || strings.Contains(output.String(), "Authorized-Folder") {
		t.Fatalf("source selection exit=%d output=%s", code, output.String())
	}
	selectedStatus, err := priorWorkSourceSelectionStore(dataRoot).Status(inspection.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	setupStatus, err := (setupauth.Store{Root: dataRoot}).Status(setupauth.Request{WorkspaceID: inspection.WorkspaceID, WorkspacePath: workspacePath, SourceFingerprint: selectedStatus.Fingerprint}, identity)
	if err != nil || setupStatus.State != setupauth.StateActive {
		t.Fatalf("selected source did not extend one-and-done grant: %#v err=%v", setupStatus, err)
	}

	output.Reset()
	if code := runSession([]string{"packet", workspacePath}, &output, &output, resolve); code != ExitOK || !strings.Contains(output.String(), `"sharepoint_source"`) || !strings.Contains(output.String(), `"state": "selected"`) || !strings.Contains(output.String(), `"folder_count": 1`) || strings.Contains(output.String(), "Authorized-Folder") {
		t.Fatalf("session source projection exit=%d output=%s", code, output.String())
	}

	output.Reset()
	if code := runPriorWork(
		[]string{"source", "defer", "--workspace", workspacePath},
		strings.NewReader(""), &output, &output, resolve,
	); code != ExitOK || !strings.Contains(output.String(), `"state": "deferred"`) {
		t.Fatalf("source defer exit=%d output=%s", code, output.String())
	}
}

func TestGuidedSharePointSourceSelectionRequiresStdinButNotConfirmation(t *testing.T) {
	dataRoot := t.TempDir()
	workspacePath := filepath.Join(t.TempDir(), "maestro-project")
	resolve := func() (string, error) { return dataRoot, nil }
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, resolve); code != ExitOK {
		t.Fatal(output.String())
	}
	selection := `{"schema_version":1,"folder_urls":["https://bcg.sharepoint.com/sites/project/Shared%20Documents/Authorized-Folder"]}`
	for _, test := range []struct {
		args  []string
		usage string
	}{
		{[]string{"source", "select", "--workspace", workspacePath, "--confirm"}, "--stdin"},
		{[]string{"source", "defer", "--workspace", workspacePath, "--stdin"}, "source defer"},
	} {
		output.Reset()
		if code := runPriorWork(test.args, strings.NewReader(selection), &output, &output, resolve); code != ExitUsage || !strings.Contains(output.String(), test.usage) {
			t.Fatalf("args=%v exit=%d output=%s", test.args, code, output.String())
		}
	}
}

func TestGuidedSharePointSourceSelectionMapsCommonLibraryViewURL(t *testing.T) {
	dataRoot := t.TempDir()
	workspacePath := filepath.Join(t.TempDir(), "maestro-project")
	resolve := func() (string, error) { return dataRoot, nil }
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, resolve); code != ExitOK {
		t.Fatal(output.String())
	}
	output.Reset()
	selection := `{"schema_version":1,"folder_urls":["https://bcgcloud.sharepoint.com/sites/xek407-rt/Shared%20Documents/Forms/AllItems.aspx"]}`
	if code := runPriorWork(
		[]string{"source", "select", "--workspace", workspacePath, "--stdin"},
		strings.NewReader(selection), &output, &output, resolve,
	); code != ExitOK || !strings.Contains(output.String(), `"state": "selected"`) {
		t.Fatalf("library-view selection exit=%d output=%s", code, output.String())
	}
}

func TestGuidedSharePointSelectionSurvivesSetupBindingMismatchAsRepairablePending(t *testing.T) {
	dataRoot := t.TempDir()
	workspacePath := filepath.Join(t.TempDir(), "maestro-project")
	resolve := func() (string, error) { return dataRoot, nil }
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, resolve); code != ExitOK {
		t.Fatal(output.String())
	}
	inspection, err := workspace.Inspect(workspacePath, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (setupauth.Store{Root: dataRoot}).Authorize(
		setupauth.Request{WorkspaceID: inspection.WorkspaceID, WorkspacePath: workspacePath},
		setupauth.DeriveIdentity("another-principal", "another-device"), true,
	); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	selection := `{"schema_version":1,"folder_urls":["https://bcg.sharepoint.com/sites/project/Shared%20Documents/Authorized-Folder"]}`
	if code := runPriorWork(
		[]string{"source", "select", "--workspace", workspacePath, "--stdin", "--confirm"},
		strings.NewReader(selection), &output, &output, resolve,
	); code != ExitOK || !strings.Contains(output.String(), `"state": "selected"`) || !strings.Contains(output.String(), `"authorization_state": "setup_binding_pending"`) || strings.Contains(output.String(), "Authorized-Folder") {
		t.Fatalf("selection should remain useful with repairable binding pending: exit=%d output=%s", code, output.String())
	}
	status, err := priorWorkSourceSelectionStore(dataRoot).Status(inspection.WorkspaceID)
	if err != nil || status.State != priorwork.SourceSelected || status.FolderCount != 1 {
		t.Fatalf("durable selection was lost: %#v err=%v", status, err)
	}
}

func TestGuidedSharePointSelectionDoesNotHideCorruptSetupGrant(t *testing.T) {
	dataRoot := t.TempDir()
	workspacePath := filepath.Join(t.TempDir(), "maestro-project")
	resolve := func() (string, error) { return dataRoot, nil }
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, resolve); code != ExitOK {
		t.Fatal(output.String())
	}
	inspection, err := workspace.Inspect(workspacePath, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := currentSetupIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (setupauth.Store{Root: dataRoot}).Authorize(
		setupauth.Request{WorkspaceID: inspection.WorkspaceID, WorkspacePath: workspacePath}, identity, true,
	); err != nil {
		t.Fatal(err)
	}
	grantPath := filepath.Join(dataRoot, "setup-authorizations", inspection.WorkspaceID+".json")
	if err := os.WriteFile(grantPath, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	selection := `{"schema_version":1,"folder_urls":["https://bcg.sharepoint.com/sites/project/Shared%20Documents/Authorized-Folder"]}`
	code := runPriorWork(
		[]string{"source", "select", "--workspace", workspacePath, "--stdin", "--confirm"},
		strings.NewReader(selection), &output, &output, resolve,
	)
	if code != ExitFailure || !strings.Contains(output.String(), "setup binding failed") || !strings.Contains(output.String(), "invalid identity or scope fields") {
		t.Fatalf("corrupt grant was hidden as pending: exit=%d output=%s", code, output.String())
	}
	status, err := priorWorkSourceSelectionStore(dataRoot).Status(inspection.WorkspaceID)
	if err != nil || status.State != priorwork.SourceSelected {
		t.Fatalf("durable source selection was lost after grant corruption: %#v err=%v", status, err)
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
		snapshot, enrollment, "receipt-cli-e2e", "manual-cli-e2e", now,
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

func TestPriorWorkSyncDueCodexIsUnavailableAndRemainsDue(t *testing.T) {
	dataRoot := t.TempDir()
	resolve := func() (string, error) { return dataRoot, nil }
	var enrollment priorwork.Enrollment
	if err := json.Unmarshal([]byte(cliPriorWorkEnrollment(t)), &enrollment); err != nil {
		t.Fatal(err)
	}
	enrollment.EnrolledAt = time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	enrollment.ScopeExpansionConfirmAfter = enrollment.EnrolledAt.AddDate(0, 6, 0)
	enrollment.AuthorizationExpiresAt = enrollment.EnrolledAt.AddDate(2, 0, 0)
	signingBody, err := priorwork.EnrollmentAuthoritySigningBody(enrollment)
	if err != nil {
		t.Fatal(err)
	}
	enrollment.AuthoritySignature = base64.StdEncoding.EncodeToString(ed25519.Sign(cliEnrollmentAuthorityPrivateKey, signingBody))
	body, err := json.Marshal(enrollment)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := runPriorWork(
		[]string{"enroll", "--stdin", "--confirm"},
		bytes.NewReader(body), &output, &output, resolve,
	); code != ExitOK {
		t.Fatalf("enroll exit=%d output=%s", code, output.String())
	}
	for attempt := 0; attempt < 2; attempt++ {
		output.Reset()
		code := runPriorWork(
			[]string{"sync-due", "--runtime", "codex"},
			strings.NewReader(""), &output, &output, resolve,
		)
		if code != ExitUnavailable || !strings.Contains(output.String(), `"due": true`) ||
			!strings.Contains(output.String(), `"state": "unavailable"`) ||
			!strings.Contains(output.String(), "corporate_policy") {
			t.Fatalf("attempt=%d exit=%d output=%s", attempt, code, output.String())
		}
	}
}
