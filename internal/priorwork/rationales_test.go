package priorwork

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMaterializeRationalesPrioritizesRecentItemsAndKeepsProvenance(t *testing.T) {
	enrollment := testEnrollment()
	first := suzanoDeck()
	first.SourceURL = "https://bcgbr.sharepoint.com/sites/consulting/Shared%20Documents/Projects/older.pptx"
	first.Name = "Older rationale.pptx"
	first.ModifiedAt = testTime.Add(time.Hour)
	second := suzanoDeck()
	second.ItemRef = "item-newer"
	second.SourceURL = "https://bcgbr.sharepoint.com/sites/consulting/Shared%20Documents/Projects/newer.pptx"
	second.Name = "Newer rationale.pptx"
	second.ModifiedAt = testTime.Add(3 * time.Hour)
	snapshot := Snapshot{
		SchemaVersion: 1, Source: "sharepoint", AdapterRuntime: "claude", TenantRef: enrollment.TenantRef,
		Mode: "full", CollectionSequence: 1, GeneratedAt: testTime, Watermark: "wm-1",
		Roots: []RootRef{testRoot()}, RootResults: []RootResult{{Root: testRoot(), State: "complete"}},
		Items: []Item{first, second}, Tombstones: []Tombstone{},
	}
	receipt, body, err := BuildUnsignedImportReceipt(snapshot, enrollment, "receipt-rationales", "trigger-rationales", testTime)
	if err != nil {
		t.Fatal(err)
	}
	receipt.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(testCollectorPrivateKey, body))
	batch := RationaleBatch{
		SchemaVersion: 1, WorkspaceID: strings.Repeat("a", 32), SourceSelectionFingerprint: strings.Repeat("b", 64),
		Snapshot: snapshot, Receipt: receipt,
		Rationales: []Rationale{
			{ItemRef: first.ItemRef, Root: first.Root, SourceURL: first.SourceURL, Name: first.Name, ModifiedAt: first.ModifiedAt, ContentDigest: strings.Repeat("1", 64), Text: "Older derived rationale."},
			{ItemRef: second.ItemRef, Root: second.Root, SourceURL: second.SourceURL, Name: second.Name, ModifiedAt: second.ModifiedAt, ContentDigest: strings.Repeat("2", 64), Text: "Newer derived rationale."},
		},
	}
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".bcgos"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".bcgos", "workspace.json"), []byte(`{"schema_version":1,"workspace_id":"`+strings.Repeat("a", 32)+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := MaterializeRationales(workspace, batch, []string{"https://bcgbr.sharepoint.com/sites/consulting/Shared%20Documents/Projects"}, enrollment)
	if err != nil {
		t.Fatal(err)
	}
	if report.RationaleCount != 2 || report.Items[0] != "01-"+rationaleID(second.ItemRef)+".md" {
		t.Fatalf("report = %#v", report)
	}
	index, err := os.ReadFile(filepath.Join(workspace, "brain/knowledge/sharepoint-rationales/index.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	if strings.Index(text, "Newer rationale") > strings.Index(text, "Older rationale") || !strings.Contains(text, "SharePoint continua sendo a autoridade") {
		t.Fatalf("index = %s", text)
	}
	file, err := os.ReadFile(filepath.Join(workspace, "brain/knowledge/sharepoint-rationales", report.Items[0]))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(file, []byte(second.SourceURL)) || bytes.Contains(file, []byte("raw document body")) {
		t.Fatalf("rationale provenance/body = %s", file)
	}
	if _, err := os.Stat(filepath.Join(workspace, "brain/knowledge/sharepoint-rationales/README.md")); !os.IsNotExist(err) {
		t.Fatalf("expected materializer to write only its governed files, got README err=%v", err)
	}
}

func TestRationaleBatchRejectsSourceOutsideSelectedFolder(t *testing.T) {
	batch := RationaleBatch{SchemaVersion: 1, WorkspaceID: strings.Repeat("a", 32), SourceSelectionFingerprint: strings.Repeat("b", 64)}
	batch.Snapshot = Snapshot{SchemaVersion: 1, Source: "sharepoint", AdapterRuntime: "claude", TenantRef: "tenant-br", Mode: "full", CollectionSequence: 1, GeneratedAt: testTime, Watermark: "wm-1", Roots: []RootRef{testRoot()}, RootResults: []RootResult{{Root: testRoot(), State: "complete"}}, Items: []Item{suzanoDeck()}, Tombstones: []Tombstone{}}
	batch.Receipt = ImportReceipt{SchemaVersion: 1, ReceiptID: "receipt-rationales", EvidenceClass: "adapter_command", Capability: "sharepoint_work_collection", ProducerRuntime: "claude", Outcome: "succeeded", EmittedAt: testTime, TenantRef: "tenant-br", Roots: []RootRef{testRoot()}, CollectionSequence: 1, Watermark: "wm-1", SnapshotDigest: strings.Repeat("c", 64), PolicyVersion: "spwk-v1", EnrollmentFingerprint: strings.Repeat("d", 64), KeyID: "collector-key", TriggerRef: "trigger-rationales", Signature: strings.Repeat("e", 128)}
	batch.Rationales = []Rationale{{ItemRef: suzanoDeck().ItemRef, Root: suzanoDeck().Root, SourceURL: suzanoDeck().SourceURL, Name: suzanoDeck().Name, ModifiedAt: suzanoDeck().ModifiedAt, ContentDigest: strings.Repeat("1", 64), Text: "Derived."}}
	if err := validateRationaleFolders(batch.Rationales, []string{"https://bcgbr.sharepoint.com/sites/consulting/Shared%20Documents/Other"}); err == nil {
		t.Fatal("rationale outside selected folder was accepted")
	}
}
