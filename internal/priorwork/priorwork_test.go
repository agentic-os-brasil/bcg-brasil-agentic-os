package priorwork

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var testTime = time.Date(2023, 8, 15, 12, 0, 0, 0, time.UTC)
var testCollectorPrivateKey = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x42}, ed25519.SeedSize))
var testCollectorPublicKey = testCollectorPrivateKey.Public().(ed25519.PublicKey)
var testReceiptCounter atomic.Uint64

func testRoot() RootRef {
	return RootRef{SiteRef: "site-consulting", DriveRef: "drive-projects", FolderRef: "folder-enrolled"}
}

func testEnrollment() Enrollment {
	return Enrollment{
		SchemaVersion:              1,
		TenantRef:                  "tenant-br",
		Purpose:                    "prior_work_retrieval",
		PolicyVersion:              "spwk-v1",
		AuthorizedBy:               "actor-partner",
		CollectorKeyID:             "claude-sharepoint-collector-v1",
		CollectorPublicKey:         base64.StdEncoding.EncodeToString(testCollectorPublicKey),
		EnrolledAt:                 testTime,
		AuthorizationExpiresAt:     testTime.AddDate(1, 0, 0),
		ScopeExpansionConfirmAfter: testTime.AddDate(0, 6, 0),
		RefreshHours:               24,
		StaleHours:                 72,
		ScheduleTimezone:           "America/Sao_Paulo",
		MaxItemBytes:               100_000_000,
		MaxSnapshotItems:           10_000,
		AllowedItemTypes:           []string{"file", "folder"},
		AllowedOrigins:             []string{"https://bcgbr.sharepoint.com"},
		Roots:                      []RootRef{testRoot()},
	}
}

func suzanoDeck() Item {
	return Item{
		ItemRef:      "item-suzano-plantio",
		ParentRef:    "folder-enrolled",
		Root:         testRoot(),
		Kind:         "file",
		Name:         "Suzano CEO - Plantio 2023.pptx",
		PathSegments: []string{"Clientes", "Suzano", "Plantio"},
		SourceURL:    "https://bcgbr.sharepoint.com/sites/consulting/Shared%20Documents/Suzano-Plantio-2023.pptx",
		CreatedAt:    testTime,
		ModifiedAt:   testTime.Add(2 * time.Hour),
		SizeBytes:    4_200_000,
		MediaType:    "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		ETag:         "etag-suzano-v1",
		Facets: Facets{
			Clients:    []string{"Suzano"},
			Projects:   []string{"Plantio estratégico"},
			Themes:     []string{"Plantio"},
			Years:      []int{2023},
			Audiences:  []string{"CEO"},
			People:     []string{"CEO da Suzano"},
			Presenters: []string{"Daniel Scardini"},
		},
		SearchTerms: []string{"silvicultura", "deck executivo"},
		Sensitivity: "client_restricted",
		Status:      "active",
	}
}

func testSnapshot(mode, previous, watermark string, items []Item, tombstones []Tombstone) Snapshot {
	if items == nil {
		items = []Item{}
	}
	if tombstones == nil {
		tombstones = []Tombstone{}
	}
	return Snapshot{
		SchemaVersion:      1,
		Source:             "sharepoint",
		AdapterRuntime:     "claude",
		TenantRef:          "tenant-br",
		Mode:               mode,
		CollectionSequence: 1,
		GeneratedAt:        testTime,
		PreviousWatermark:  previous,
		Watermark:          watermark,
		Roots:              []RootRef{testRoot()},
		RootResults:        []RootResult{{Root: testRoot(), State: "complete"}},
		Items:              items,
		Tombstones:         tombstones,
	}
}

func testReceipt(snapshot Snapshot, enrollment Enrollment) ImportReceipt {
	receiptID := fmt.Sprintf("receipt-test-%d", testReceiptCounter.Add(1))
	receipt, err := newImportReceipt(snapshot, enrollment, testCollectorPrivateKey, receiptID, snapshot.GeneratedAt)
	if err != nil {
		panic(err)
	}
	return receipt
}

func newImportReceipt(
	snapshot Snapshot,
	enrollment Enrollment,
	privateKey ed25519.PrivateKey,
	receiptID string,
	emittedAt time.Time,
) (ImportReceipt, error) {
	if err := ValidateSnapshot(snapshot); err != nil {
		return ImportReceipt{}, err
	}
	if err := ValidateEnrollment(enrollment); err != nil {
		return ImportReceipt{}, err
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		return ImportReceipt{}, errors.New("prior-work collector private key is invalid")
	}
	snapshotDigest, err := fingerprintSnapshot(snapshot)
	if err != nil {
		return ImportReceipt{}, err
	}
	enrollmentFingerprint, err := fingerprintEnrollment(enrollment)
	if err != nil {
		return ImportReceipt{}, err
	}
	receipt := ImportReceipt{
		SchemaVersion: 1, ReceiptID: receiptID, EvidenceClass: "adapter_command",
		Capability: "sharepoint_work_collection", ProducerRuntime: "claude",
		Outcome: "succeeded", EmittedAt: emittedAt.UTC(),
		TenantRef: snapshot.TenantRef, Roots: append([]RootRef(nil), snapshot.Roots...),
		PolicyVersion: enrollment.PolicyVersion, EnrollmentFingerprint: enrollmentFingerprint,
		CollectionSequence: snapshot.CollectionSequence, Watermark: snapshot.Watermark,
		SnapshotDigest: snapshotDigest, KeyID: enrollment.CollectorKeyID, TriggerRef: "manual-test",
	}
	body, err := receiptSigningBody(receipt)
	if err != nil {
		return ImportReceipt{}, err
	}
	receipt.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, body))
	return receipt, ValidateImportReceipt(receipt, snapshot)
}

func testAccess() AccessContext {
	return AccessContext{ActorRef: "actor-partner", Purpose: "prior_work_retrieval"}
}

func newTestStore(root string, now *time.Time) Store {
	return Store{
		Root:  root,
		clock: func() time.Time { return *now },
	}
}

func TestParseSnapshotRejectsUnknownDuplicateAndCodexCollection(t *testing.T) {
	valid := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	body, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	document["unexpected"] = true
	unknown, _ := json.Marshal(document)
	if _, err := ParseSnapshot(bytes.NewReader(unknown)); err == nil {
		t.Fatal("expected unknown field to fail closed")
	}

	duplicate := strings.Replace(string(body), `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1)
	if _, err := ParseSnapshot(strings.NewReader(duplicate)); err == nil {
		t.Fatal("expected duplicate JSON key to fail closed")
	}

	valid.AdapterRuntime = "codex"
	body, _ = json.Marshal(valid)
	if _, err := ParseSnapshot(bytes.NewReader(body)); err == nil || !strings.Contains(err.Error(), "Claude") {
		t.Fatalf("expected Codex collection to be unavailable, got %v", err)
	}
}

func TestParseSnapshotRequiresCompleteRootResultsAndArrays(t *testing.T) {
	valid := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	body, _ := json.Marshal(valid)
	missing := strings.Replace(string(body), `,"tombstones":[]`, "", 1)
	if _, err := ParseSnapshot(strings.NewReader(missing)); err == nil {
		t.Fatal("expected missing required tombstones array to fail")
	}

	valid.RootResults[0].State = "partial"
	if err := ValidateSnapshot(valid); err == nil {
		t.Fatal("expected partial root enumeration to fail")
	}
	valid.RootResults = []RootResult{}
	if err := ValidateSnapshot(valid); err == nil {
		t.Fatal("expected missing root completion to fail")
	}
}

func TestImportReceiptMustBindExactSnapshot(t *testing.T) {
	snapshot := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	receipt := testReceipt(snapshot, testEnrollment())
	receipt.SnapshotDigest = strings.Repeat("0", 64)
	if err := ValidateImportReceipt(receipt, snapshot); err == nil {
		t.Fatal("expected a mismatched adapter-command receipt to fail")
	}
}

func TestStoreRejectsUnauthorizedActorAndForgedReceipt(t *testing.T) {
	now := testTime.Add(time.Hour)
	enrollment := testEnrollment()
	store := newTestStore(t.TempDir(), &now)
	if err := store.Enroll(enrollment); err != nil {
		t.Fatal(err)
	}
	snapshot := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	receipt := testReceipt(snapshot, enrollment)
	if _, err := store.Apply(snapshot, receipt, AccessContext{
		ActorRef: "actor-unknown", Purpose: "prior_work_retrieval",
	}); err == nil {
		t.Fatal("expected unknown actor to fail authorization")
	}
	receipt.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	if _, err := store.Apply(snapshot, receipt, testAccess()); err == nil ||
		!strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("expected forged receipt to fail, got %v", err)
	}
}

func TestJSONSchemasAndGoValidatorAgreeOnContractCorpus(t *testing.T) {
	catalogSchema, receiptSchema := compilePriorWorkSchemas(t)
	valid := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	validDocument := jsonDocument(t, valid)
	if err := catalogSchema.Validate(validDocument); err != nil {
		t.Fatalf("catalog schema rejected valid snapshot: %v", err)
	}
	if err := ValidateSnapshot(valid); err != nil {
		t.Fatalf("Go validator rejected valid snapshot: %v", err)
	}
	receipt := testReceipt(valid, testEnrollment())
	if err := receiptSchema.Validate(jsonDocument(t, receipt)); err != nil {
		t.Fatalf("receipt schema rejected valid receipt: %v", err)
	}

	invalid := valid
	invalid.RootResults[0].State = "partial"
	if err := catalogSchema.Validate(jsonDocument(t, invalid)); err == nil {
		t.Fatal("catalog schema accepted partial root result")
	}
	if err := ValidateSnapshot(invalid); err == nil {
		t.Fatal("Go validator accepted partial root result")
	}
	for name, mutate := range map[string]func(*Snapshot){
		"external URL":    func(value *Snapshot) { value.Items[0].SourceURL = "https://example.com/deck.pptx" },
		"untrimmed label": func(value *Snapshot) { value.Items[0].Name = " deck.pptx" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
			mutate(&candidate)
			if err := catalogSchema.Validate(jsonDocument(t, candidate)); err == nil {
				t.Fatal("catalog schema accepted invalid snapshot")
			}
			if err := ValidateSnapshot(candidate); err == nil {
				t.Fatal("Go validator accepted invalid snapshot")
			}
		})
	}
}

func TestValidateSnapshotRejectsWrongRootAndUnsafeURL(t *testing.T) {
	item := suzanoDeck()
	item.Root.FolderRef = "folder-not-enrolled"
	if err := ValidateSnapshot(testSnapshot("full", "", "watermark-1", []Item{item}, nil)); err == nil {
		t.Fatal("expected out-of-scope item to fail")
	}

	item = suzanoDeck()
	item.SourceURL = "https://example.com/copied-deck.pptx"
	if err := ValidateSnapshot(testSnapshot("full", "", "watermark-1", []Item{item}, nil)); err == nil {
		t.Fatal("expected non-SharePoint URL to fail")
	}
}

func TestStoreFullDeltaIdempotencyAndExplicitQuery(t *testing.T) {
	now := testTime.Add(time.Hour)
	enrollment := testEnrollment()
	store := newTestStore(t.TempDir(), &now)
	if err := store.Enroll(enrollment); err != nil {
		t.Fatal(err)
	}

	full := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	first, err := store.Apply(full, testReceipt(full, enrollment), testAccess())
	if err != nil {
		t.Fatal(err)
	}
	if first.State != "published" || first.Items != 1 {
		t.Fatalf("unexpected first report: %#v", first)
	}
	if err := store.VerifyPublication(first, testAccess()); err != nil {
		t.Fatalf("active publication proof did not verify: %v", err)
	}
	forged := first
	forged.Fingerprint = strings.Repeat("0", 64)
	if err := store.VerifyPublication(forged, testAccess()); err == nil {
		t.Fatal("forged publication proof matched no active manifest")
	}

	now = testTime.Add(2 * time.Hour)
	retry := full
	second, err := store.Apply(retry, testReceipt(retry, enrollment), testAccess())
	if err != nil {
		t.Fatal(err)
	}
	if second.State != "unchanged" || second.Fingerprint != first.Fingerprint {
		t.Fatalf("retry was not idempotent: %#v", second)
	}

	if _, err := store.Find(Query{Text: "quero o deck que apresentei pro CEO da Suzano em 2023 sobre PLANTIO"}); !errors.Is(err, ErrExplicitIntentRequired) {
		t.Fatalf("expected explicit-intent gate, got %v", err)
	}
	results, err := store.Find(Query{
		Text:                    "quero o deck que apresentei pro CEO da Suzano em 2023 sobre PLANTIO",
		ExplicitPriorWorkIntent: true,
		Limit:                   5,
		Access:                  testAccess(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Results) != 1 || results.Results[0].ItemRef != suzanoDeck().ItemRef {
		t.Fatalf("expected the Suzano deck first, got %#v", results)
	}
	if results.Results[0].AuthorizationNote == "" || results.Results[0].Score <= 0 {
		t.Fatalf("missing bounded result evidence: %#v", results.Results[0])
	}

	delta := testSnapshot("delta", "watermark-1", "watermark-2", nil, []Tombstone{{
		ItemRef: suzanoDeck().ItemRef, Root: testRoot(), Reason: "access_revoked", ObservedAt: testTime.Add(4 * time.Hour),
	}})
	delta.CollectionSequence = 2
	now = testTime.Add(4 * time.Hour)
	report, err := store.Apply(delta, testReceipt(delta, enrollment), testAccess())
	if err != nil {
		t.Fatal(err)
	}
	if report.Removed != 1 {
		t.Fatalf("expected one removal, got %#v", report)
	}
	now = testTime.Add(5 * time.Hour)
	results, err = store.Find(Query{Text: "Suzano plantio 2023 CEO", ExplicitPriorWorkIntent: true, Access: testAccess()})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Results) != 0 {
		t.Fatalf("revoked item leaked through query: %#v", results)
	}
}

func TestCompilerIsDeterministicAndKeepsLogsMetadataOnly(t *testing.T) {
	snapshot := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	digest, err := fingerprintSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	catalog := Catalog{
		SchemaVersion: 1, TenantRef: "tenant-br", PolicyVersion: "spwk-v1",
		CollectionSequence: 1, Watermark: "watermark-1", SnapshotDigest: digest, GeneratedAt: testTime,
		Roots: []RootRef{testRoot()}, Items: []Item{suzanoDeck()},
	}
	left, right := t.TempDir(), t.TempDir()
	if err := Compile(left, catalog); err != nil {
		t.Fatal(err)
	}
	if err := Compile(right, catalog); err != nil {
		t.Fatal(err)
	}
	leftFiles := readTree(t, left)
	rightFiles := readTree(t, right)
	if len(leftFiles) != len(rightFiles) {
		t.Fatalf("non-deterministic file count: %d != %d", len(leftFiles), len(rightFiles))
	}
	for path, leftBody := range leftFiles {
		if !bytes.Equal(leftBody, rightFiles[path]) {
			t.Fatalf("non-deterministic compiler output at %s", path)
		}
	}
	for _, name := range []string{"log.md", "diagnostics.json"} {
		body := string(leftFiles[name])
		if strings.Contains(body, "Suzano") || strings.Contains(body, "sharepoint.com") {
			t.Fatalf("%s leaked item metadata into operational output", name)
		}
	}
}

func TestCompositeIdentityCanonicalizationIsOrderIndependent(t *testing.T) {
	secondRoot := RootRef{SiteRef: "site-consulting", DriveRef: "drive-archive", FolderRef: "folder-enrolled"}
	first := suzanoDeck()
	first.Facets.Themes = []string{"Silvicultura", "Plantio"}
	second := suzanoDeck()
	second.Root = secondRoot
	second.Name = "Second copy.pptx"
	second.Facets.Themes = []string{"Plantio", "Silvicultura"}
	left := testSnapshot("full", "", "watermark-1", []Item{first, second}, nil)
	left.Roots = []RootRef{testRoot(), secondRoot}
	left.RootResults = []RootResult{{Root: testRoot(), State: "complete"}, {Root: secondRoot, State: "complete"}}
	right := left
	right.Roots = []RootRef{secondRoot, testRoot()}
	right.RootResults = []RootResult{{Root: secondRoot, State: "complete"}, {Root: testRoot(), State: "complete"}}
	right.Items = []Item{second, first}
	right.Items[1].Facets.Themes = []string{"Plantio", "Silvicultura"}

	leftDigest, err := fingerprintSnapshot(left)
	if err != nil {
		t.Fatal(err)
	}
	rightDigest, err := fingerprintSnapshot(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftDigest != rightDigest {
		t.Fatalf("equivalent snapshots produced different digests: %s != %s", leftDigest, rightDigest)
	}
	leftCatalog := Catalog{
		SchemaVersion: 1, TenantRef: left.TenantRef, PolicyVersion: "spwk-v1",
		CollectionSequence: 1, Watermark: left.Watermark, SnapshotDigest: leftDigest,
		GeneratedAt: left.GeneratedAt, Roots: left.Roots, Items: left.Items,
	}
	rightCatalog := leftCatalog
	rightCatalog.Roots, rightCatalog.Items, rightCatalog.SnapshotDigest = right.Roots, right.Items, rightDigest
	leftFingerprint, _ := catalogFingerprint(leftCatalog)
	rightFingerprint, _ := catalogFingerprint(rightCatalog)
	if leftFingerprint != rightFingerprint {
		t.Fatalf("equivalent catalogs produced different fingerprints: %s != %s", leftFingerprint, rightFingerprint)
	}
}

func TestStoreRejectsForkedDeltaAndMutableWatermark(t *testing.T) {
	now := testTime.Add(time.Hour)
	enrollment := testEnrollment()
	store := newTestStore(t.TempDir(), &now)
	if err := store.Enroll(enrollment); err != nil {
		t.Fatal(err)
	}
	full := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	if _, err := store.Apply(full, testReceipt(full, enrollment), testAccess()); err != nil {
		t.Fatal(err)
	}

	forked := testSnapshot("delta", "wrong-watermark", "watermark-2", nil, []Tombstone{})
	forked.CollectionSequence = 2
	now = testTime.Add(2 * time.Hour)
	if _, err := store.Apply(forked, testReceipt(forked, enrollment), testAccess()); !errors.Is(err, ErrWatermarkConflict) {
		t.Fatalf("expected watermark conflict, got %v", err)
	}

	mutated := full
	mutated.Items[0].Name = "Changed under same watermark.pptx"
	if _, err := store.Apply(mutated, testReceipt(mutated, enrollment), testAccess()); !errors.Is(err, ErrImmutableWatermark) {
		t.Fatalf("expected immutable-watermark rejection, got %v", err)
	}
	changedProvenance := full
	changedProvenance.GeneratedAt = changedProvenance.GeneratedAt.Add(time.Minute)
	if _, err := store.Apply(changedProvenance, testReceipt(changedProvenance, enrollment), testAccess()); !errors.Is(err, ErrImmutableWatermark) {
		t.Fatalf("expected changed provenance under same watermark to fail, got %v", err)
	}
}

func TestSnapshotLimitCountsItemsAndTombstones(t *testing.T) {
	now := testTime.Add(time.Hour)
	enrollment := testEnrollment()
	enrollment.MaxSnapshotItems = 1
	store := newTestStore(t.TempDir(), &now)
	if err := store.Enroll(enrollment); err != nil {
		t.Fatal(err)
	}
	full := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	if _, err := store.Apply(full, testReceipt(full, enrollment), testAccess()); err != nil {
		t.Fatal(err)
	}
	delta := testSnapshot("delta", "watermark-1", "watermark-2", nil, []Tombstone{
		{Root: testRoot(), ItemRef: suzanoDeck().ItemRef, Reason: "deleted", ObservedAt: testTime},
		{Root: testRoot(), ItemRef: "item-other", Reason: "deleted", ObservedAt: testTime},
	})
	delta.CollectionSequence = 2
	now = testTime.Add(2 * time.Hour)
	if _, err := store.Apply(delta, testReceipt(delta, enrollment), testAccess()); err == nil ||
		!strings.Contains(err.Error(), "item limit") {
		t.Fatalf("expected combined snapshot limit to fail, got %v", err)
	}
}

func TestRevocationBarrierPrecedesFailedCompilation(t *testing.T) {
	now := testTime.Add(time.Hour)
	enrollment := testEnrollment()
	store := newTestStore(t.TempDir(), &now)
	if err := store.Enroll(enrollment); err != nil {
		t.Fatal(err)
	}
	full := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	if _, err := store.Apply(full, testReceipt(full, enrollment), testAccess()); err != nil {
		t.Fatal(err)
	}

	store.compile = func(*os.Root, Catalog) error { return errors.New("simulated compiler failure") }
	revoked := testSnapshot("delta", "watermark-1", "watermark-2", nil, []Tombstone{{
		ItemRef: suzanoDeck().ItemRef, Root: testRoot(), Reason: "deleted", ObservedAt: testTime.Add(2 * time.Hour),
	}})
	revoked.CollectionSequence = 2
	now = testTime.Add(2 * time.Hour)
	_, err := store.Apply(revoked, testReceipt(revoked, enrollment), testAccess())
	if err == nil {
		t.Fatal("expected compiler failure")
	}

	store.compile = nil
	now = testTime.Add(3 * time.Hour)
	if _, err := store.Apply(full, testReceipt(full, enrollment), testAccess()); err != nil {
		t.Fatal(err)
	}
	results, err := store.Find(Query{Text: "Suzano plantio", ExplicitPriorWorkIntent: true, Access: testAccess()})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Results) != 0 {
		t.Fatal("last-known-good catalog served a synchronously blocked item")
	}
}

func TestPartialRevocationBatchFencesAllQueries(t *testing.T) {
	now := testTime.Add(time.Hour)
	enrollment := testEnrollment()
	store := newTestStore(t.TempDir(), &now)
	if err := store.Enroll(enrollment); err != nil {
		t.Fatal(err)
	}
	first := suzanoDeck()
	second := suzanoDeck()
	second.ItemRef = "item-suzano-plantio-copy"
	second.Name = "Suzano CEO - Plantio 2023 copy.pptx"
	second.SourceURL = "https://bcgbr.sharepoint.com/sites/consulting/Shared%20Documents/Suzano-Plantio-2023-copy.pptx"
	full := testSnapshot("full", "", "watermark-1", []Item{first, second}, nil)
	if _, err := store.Apply(full, testReceipt(full, enrollment), testAccess()); err != nil {
		t.Fatal(err)
	}
	delta := testSnapshot("delta", "watermark-1", "watermark-2", nil, []Tombstone{
		{Root: testRoot(), ItemRef: first.ItemRef, Reason: "access_revoked", ObservedAt: testTime.Add(2 * time.Hour)},
		{Root: testRoot(), ItemRef: second.ItemRef, Reason: "access_revoked", ObservedAt: testTime.Add(2 * time.Hour)},
	})
	delta.CollectionSequence = 2
	blockingPath := filepath.Join(store.Root, "barriers", opaqueFilename(delta.Tombstones[1].key())+".json")
	if err := os.Mkdir(blockingPath, 0o700); err != nil {
		t.Fatal(err)
	}
	now = testTime.Add(2 * time.Hour)
	if _, err := store.Apply(delta, testReceipt(delta, enrollment), testAccess()); err == nil {
		t.Fatal("expected partial barrier persistence to fail")
	}
	if _, err := store.Find(Query{
		Text: "Suzano Plantio", ExplicitPriorWorkIntent: true, Access: testAccess(),
	}); !errors.Is(err, ErrRevocationFence) {
		t.Fatalf("expected incomplete revocation batch to fence every query, got %v", err)
	}
	if err := os.Remove(blockingPath); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Apply(delta, testReceipt(delta, enrollment), testAccess()); err != nil {
		t.Fatal(err)
	}
	response, err := store.Find(Query{
		Text: "Suzano Plantio", ExplicitPriorWorkIntent: true, Access: testAccess(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 0 {
		t.Fatalf("revoked items remained queryable: %#v", response.Results)
	}
}

func TestStoreStatusFreshDueAndStale(t *testing.T) {
	now := testTime
	enrollment := testEnrollment()
	store := newTestStore(t.TempDir(), &now)
	if err := store.Enroll(enrollment); err != nil {
		t.Fatal(err)
	}
	full := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	if _, err := store.Apply(full, testReceipt(full, enrollment), testAccess()); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		now   time.Time
		state string
		due   bool
		stale bool
	}{
		{testTime.Add(12 * time.Hour), "fresh", false, false},
		{testTime.Add(25 * time.Hour), "due", true, false},
		{testTime.Add(73 * time.Hour), "stale", true, true},
	} {
		now = tc.now
		status, err := store.Status(testAccess())
		if err != nil {
			t.Fatal(err)
		}
		if status.State != tc.state || status.Due != tc.due || status.Stale != tc.stale {
			t.Fatalf("at %s got %#v", tc.now, status)
		}
	}
	now = enrollment.AuthorizationExpiresAt
	status, err := store.Status(testAccess())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "authorization_expired" || !status.Due || !status.Stale {
		t.Fatalf("expired enrollment status = %#v", status)
	}
	if _, err := store.Find(Query{
		Text: "Suzano", ExplicitPriorWorkIntent: true, Access: testAccess(),
	}); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired query to fail, got %v", err)
	}
}

func TestEnrollmentCannotBeExpandedByOverwrite(t *testing.T) {
	now := testTime
	store := newTestStore(t.TempDir(), &now)
	enrollment := testEnrollment()
	if err := store.Enroll(enrollment); err != nil {
		t.Fatal(err)
	}
	enrollment.Roots = append(enrollment.Roots, RootRef{SiteRef: "other", DriveRef: "other", FolderRef: "other"})
	if err := store.Enroll(enrollment); !errors.Is(err, ErrAlreadyEnrolled) {
		t.Fatalf("expected overwrite to be blocked, got %v", err)
	}
	body, err := os.ReadFile(filepath.Join(store.Root, "enrollment.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"other"`) {
		t.Fatal("enrollment expansion was persisted")
	}
}

func TestCompositeIdentityRevokesOnlyOneDriveItem(t *testing.T) {
	now := testTime.Add(time.Hour)
	enrollment := testEnrollment()
	store := newTestStore(t.TempDir(), &now)
	secondRoot := RootRef{SiteRef: "site-consulting", DriveRef: "drive-archive", FolderRef: "folder-enrolled"}
	enrollment.Roots = append(enrollment.Roots, secondRoot)
	if err := store.Enroll(enrollment); err != nil {
		t.Fatal(err)
	}
	first := suzanoDeck()
	second := suzanoDeck()
	second.Root = secondRoot
	second.Name = "Suzano Plantio archive copy.pptx"
	second.SourceURL = "https://bcgbr.sharepoint.com/sites/consulting/Archive/Suzano-Plantio-2023.pptx"
	full := testSnapshot("full", "", "watermark-1", []Item{first, second}, nil)
	full.Roots = enrollment.Roots
	full.RootResults = []RootResult{{Root: testRoot(), State: "complete"}, {Root: secondRoot, State: "complete"}}
	before, _ := json.Marshal(full)
	if _, err := store.Apply(full, testReceipt(full, enrollment), testAccess()); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(full)
	if !bytes.Equal(before, after) {
		t.Fatal("catalog canonicalization mutated the caller snapshot")
	}

	delta := testSnapshot("delta", "watermark-1", "watermark-2", nil, []Tombstone{{
		Root: testRoot(), ItemRef: first.ItemRef, Reason: "access_revoked", ObservedAt: testTime.Add(2 * time.Hour),
	}})
	delta.CollectionSequence = 2
	delta.Roots = enrollment.Roots
	delta.RootResults = full.RootResults
	now = testTime.Add(2 * time.Hour)
	if _, err := store.Apply(delta, testReceipt(delta, enrollment), testAccess()); err != nil {
		t.Fatal(err)
	}
	now = testTime.Add(3 * time.Hour)
	response, err := store.Find(Query{Text: "Suzano Plantio 2023", ExplicitPriorWorkIntent: true, Access: testAccess()})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 || response.Results[0].Root.key() != secondRoot.key() {
		t.Fatalf("composite revocation affected the wrong item: %#v", response.Results)
	}
}

func TestImportLockSerializesSuccessors(t *testing.T) {
	now := testTime.Add(time.Hour)
	enrollment := testEnrollment()
	store := newTestStore(t.TempDir(), &now)
	if err := store.Enroll(enrollment); err != nil {
		t.Fatal(err)
	}
	snapshot := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	entered := make(chan struct{})
	release := make(chan struct{})
	store.compile = func(destination *os.Root, catalog Catalog) error {
		close(entered)
		<-release
		return compileAt(destination, catalog)
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := store.Apply(snapshot, testReceipt(snapshot, enrollment), testAccess())
		firstDone <- err
	}()
	<-entered
	_, secondErr := store.Apply(snapshot, testReceipt(snapshot, enrollment), testAccess())
	if !errors.Is(secondErr, ErrImportLocked) {
		t.Fatalf("expected concurrent import to be locked, got %v", secondErr)
	}
	if _, err := store.Find(Query{
		Text: "Suzano", ExplicitPriorWorkIntent: true, Access: testAccess(),
	}); !errors.Is(err, ErrImportLocked) {
		t.Fatalf("expected query to linearize behind import lock, got %v", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestPublisherRejectsCompilerSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	now := testTime.Add(time.Hour)
	enrollment := testEnrollment()
	store := newTestStore(t.TempDir(), &now)
	if err := store.Enroll(enrollment); err != nil {
		t.Fatal(err)
	}
	store.compile = func(destination *os.Root, catalog Catalog) error {
		if err := compileAt(destination, catalog); err != nil {
			return err
		}
		return destination.Symlink("index.md", "escape")
	}
	snapshot := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	if _, err := store.Apply(snapshot, testReceipt(snapshot, enrollment), testAccess()); err == nil ||
		!strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected compiler symlink to fail closed, got %v", err)
	}
	if _, _, err := store.loadActive(); !errors.Is(err, ErrNoActiveCatalog) {
		t.Fatalf("failed publication must preserve absent active catalog, got %v", err)
	}
}

func TestFullReplayAndSharingURLFailClosed(t *testing.T) {
	now := testTime.Add(time.Hour)
	enrollment := testEnrollment()
	store := newTestStore(t.TempDir(), &now)
	if err := store.Enroll(enrollment); err != nil {
		t.Fatal(err)
	}
	full := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	if _, err := store.Apply(full, testReceipt(full, enrollment), testAccess()); err != nil {
		t.Fatal(err)
	}
	next := testSnapshot("full", "watermark-1", "watermark-2", []Item{suzanoDeck()}, nil)
	next.CollectionSequence = 2
	now = testTime.Add(2 * time.Hour)
	if _, err := store.Apply(next, testReceipt(next, enrollment), testAccess()); err != nil {
		t.Fatal(err)
	}
	now = testTime.Add(3 * time.Hour)
	if _, err := store.Apply(full, testReceipt(full, enrollment), testAccess()); !errors.Is(err, ErrCollectionReplay) {
		t.Fatalf("expected stale full replay to fail, got %v", err)
	}

	item := suzanoDeck()
	item.SourceURL += "?share=secret"
	unsafe := testSnapshot("full", "", "watermark-unsafe", []Item{item}, nil)
	otherNow := testTime.Add(time.Hour)
	otherEnrollment := testEnrollment()
	otherStore := newTestStore(t.TempDir(), &otherNow)
	if err := otherStore.Enroll(otherEnrollment); err != nil {
		t.Fatal(err)
	}
	if _, err := otherStore.Apply(unsafe, ImportReceipt{}, testAccess()); err == nil {
		t.Fatal("expected sharing-token URL to fail")
	}
}

func TestActiveManifestTraversalAndSymlinkFailClosed(t *testing.T) {
	now := testTime.Add(time.Hour)
	enrollment := testEnrollment()
	store := newTestStore(t.TempDir(), &now)
	if err := store.Enroll(enrollment); err != nil {
		t.Fatal(err)
	}
	activePath := filepath.Join(store.Root, "active.json")
	traversal := Manifest{
		SchemaVersion: 1, Version: "v-/../../outside", TenantRef: enrollment.TenantRef,
		CollectionSequence: 1, Watermark: "watermark-1", Fingerprint: strings.Repeat("a", 64),
		PolicyVersion: enrollment.PolicyVersion, EnrollmentFingerprint: strings.Repeat("b", 64),
		SnapshotDigest: strings.Repeat("c", 64), CompilerVersion: compilerVersion,
		PublishedAt: now, ItemCount: 1,
	}
	if err := atomicWriteAt(store.Root, "active.json", traversal); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.loadActive(); err == nil {
		t.Fatal("expected traversal manifest version to fail")
	}

	if runtime.GOOS == "windows" {
		return
	}
	external := filepath.Join(t.TempDir(), "external.json")
	if err := os.WriteFile(external, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(activePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, activePath); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.loadActive(); err == nil {
		t.Fatal("expected active-manifest symlink to fail")
	}
}

func TestImmutableSnapshotRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	root := t.TempDir()
	store := Store{Root: root}
	if err := store.prepareRoot(); err != nil {
		t.Fatal(err)
	}
	snapshot := testSnapshot("full", "", "watermark-1", []Item{suzanoDeck()}, nil)
	fingerprint := strings.Repeat("d", 64)
	path := filepath.Join(root, "snapshots", fingerprint+".json")
	external := filepath.Join(t.TempDir(), "external.json")
	if err := os.WriteFile(external, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, path); err != nil {
		t.Fatal(err)
	}
	if err := writeSnapshot(root, fingerprint, snapshot); err == nil {
		t.Fatal("expected immutable snapshot symlink to fail closed")
	}
}

func compilePriorWorkSchemas(t *testing.T) (*jsonschema.Schema, *jsonschema.Schema) {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	resources := map[string]string{
		"sharepoint-work-catalog.schema.json":        "urn:bcg-brasil-agentic-os:schema:sharepoint-work-catalog:v1",
		"sharepoint-work-import-receipt.schema.json": "urn:bcg-brasil-agentic-os:schema:sharepoint-work-import-receipt:v1",
	}
	for name, identifier := range resources {
		body, err := os.ReadFile(filepath.Join("..", "..", "schemas", name))
		if err != nil {
			t.Fatal(err)
		}
		var document any
		if err := json.Unmarshal(body, &document); err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource(identifier, document); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := compiler.Compile(resources["sharepoint-work-catalog.schema.json"])
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := compiler.Compile(resources["sharepoint-work-import-receipt.schema.json"])
	if err != nil {
		t.Fatal(err)
	}
	return catalog, receipt
}

func jsonDocument(t *testing.T, value any) any {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func readTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = body
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}
