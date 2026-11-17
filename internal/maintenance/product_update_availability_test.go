package maintenance

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func prepareAvailabilitySource(t *testing.T, s *Service) string {
	t.Helper()
	now := s.now().UTC()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(filepath.Dir(s.options.ProductRoot), "synced-source")
	registry := []byte(fmt.Sprintf(`{"schema_version":1,"product":"maestro","authorities":[{"issuer":"engineering","key_id":"fixture","algorithm":"ed25519","public_key":%q,"status":"active","valid_from":%q,"valid_until":%q}]}`, base64.StdEncoding.EncodeToString(pub), now.Add(-time.Hour).Format(time.RFC3339), now.Add(14*24*time.Hour).Format(time.RFC3339)))
	s.options.AuthorityRegistryPath = filepath.Join(s.options.ProductRoot, "trust", "release-authority-registry.json")
	s.options.AuthorityRegistrySHA256 = fmt.Sprintf("%x", sha256.Sum256(registry))
	if err := os.MkdirAll(filepath.Dir(s.options.AuthorityRegistryPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.options.AuthorityRegistryPath, registry, 0600); err != nil {
		t.Fatal(err)
	}
	g := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: releasecontract.Issuer{ID: "engineering", KeyID: "fixture"}, Source: releasecontract.SourceTuple{SourceID: "fixture", Channel: "canary", OS: "darwin", Arch: "arm64"}, Sequence: 1, PublishedAt: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(7 * 24 * time.Hour).Format(time.RFC3339), Release: "0.2.0", ManifestSHA256: strings.Repeat("a", 64), ClientInputsSHA256: strings.Repeat("b", 64), Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}
	body, _ := json.Marshal(g)
	sum := fmt.Sprintf("%x", sha256.Sum256(body))
	signature := ed25519.Sign(key, body)
	dir := filepath.Join(source, "generations", g.GenerationDirectory(sum))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for p, b := range map[string][]byte{filepath.Join(source, releaseprovider.CurrentGenerationName): body, filepath.Join(source, releaseprovider.CurrentGenerationSignatureName): signature, filepath.Join(dir, releaseprovider.GenerationName): body, filepath.Join(dir, releaseprovider.GenerationSignatureName): signature} {
		if err := os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	data := filepath.Join(s.options.UserHome, ".maestro-install")
	state := installtx.State{SchemaVersion: 4, Profile: releasecontract.ModernProfile, ManagedRoot: s.options.ProductRoot, Release: g.Release, Channel: g.Source.Channel, CLIVersion: "0.2.0", BundleVersion: "0.2.0", TargetOS: g.Source.OS, TargetArch: g.Source.Arch, ActivatedAt: now,
		Server:  installtx.ExecutableIdentity{Version: "0.2.0", Protocol: releasecontract.ServerProtocol, ExecutionContract: releasecontract.SyncedSourceContract, CompatibleBundle: ">=0.2.0 <0.3.0", CompatibleServer: ">=0.2.0 <0.3.0", EmbeddedInputsSHA256: strings.Repeat("a", 64), ArtifactName: "server", SHA256: strings.Repeat("b", 64), Size: 1},
		Binding: installtx.ReleaseBinding{ManifestSHA256: g.ManifestSHA256, ClientInputsSHA256: g.ClientInputsSHA256, ManifestJSON: "{}", ManifestSignature: strings.Repeat("a", 88), BundleSHA256: strings.Repeat("c", 64), BundleSize: 1, BundleArchive: filepath.Join(data, "updates", "fixture.tar.gz")},
		Source:  &installtx.SourceControl{Locator: source, SourceBinding: installtx.SourceBinding{Tuple: g.Source, Sequence: g.Sequence, GenerationSHA256: sum}, Body: body, Signature: signature, Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: g.MinimumReaderContract, ObservedAt: now}}
	if err := installtx.WriteState(data, state); err != nil {
		t.Fatal(err)
	}
	return source
}

func TestUpdateAvailabilityDefaultAndMissingSourceAreReadOnly(t *testing.T) {
	s := productFixture(t)
	before := productSnapshot(t, s.home)
	report, err := s.RunUpdateAvailability(context.Background())
	if err != nil || report.State != "no_due_work" {
		t.Fatalf("fresh report %+v %v", report, err)
	}
	if _, err := s.Review(Operation{Kind: "qualify", JobID: UpdateAvailabilityJobID}); err == nil {
		t.Fatal("unconfigured source qualified")
	}
	if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
		t.Fatal("unconfigured source initialized metadata or owner state")
	}
}

func TestUpdateAvailabilityPublishedContractsKeepObservationSeparate(t *testing.T) {
	if err := ValidateSchemaAndCatalog("../../schemas/maintenance-jobs.schema.json", "../../bundles/base/runtime/maintenance.json"); err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := loadSchema("../../schemas/maintenance-jobs.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	for i, job := range catalog.Jobs {
		if job.ID != UpdateAvailabilityJobID {
			continue
		}
		catalog.Jobs[i].Writes = []string{"source_custody"}
		if catalog.Validate() == nil {
			t.Fatal("metadata job gained source custody in semantic contract")
		}
		body, _ := json.Marshal(catalog)
		var raw any
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatal(err)
		}
		if schema.Validate(raw) == nil {
			t.Fatal("published catalog schema allowed source custody")
		}
	}
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	command := testCommand(now)
	command.JobID = UpdateAvailabilityJobID
	command.Trigger = TriggerPresence
	if err := command.Validate(now); err != nil {
		t.Fatal(err)
	}
	if err := validatePublishedSchemaDocument(t, "maintenance-command.schema.json", command); err != nil {
		t.Fatal(err)
	}
	receipt := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: strings.Repeat("a", 32), OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: ReceiptSucceeded, RecordedAt: now, Deadline: command.Deadline, ReasonCode: ReasonCompleted}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := validatePublishedSchemaDocument(t, "maintenance-receipt.schema.json", receipt); err != nil {
		t.Fatal(err)
	}
	command.ProposalOnly = true
	if command.Validate(now) == nil || validatePublishedSchemaDocument(t, "maintenance-command.schema.json", command) == nil {
		t.Fatal("metadata job became proposal execution")
	}
}

func TestUpdateAvailabilityConcurrentFirstUseSharesOccurrence(t *testing.T) {
	s := productFixture(t)
	prepareAvailabilitySource(t, s)
	clock := s.now()
	s.now = func() time.Time { return clock }
	productApply(t, s.updateAvailabilityService(), "qualify", UpdateAvailabilityJobID, "")
	productApply(t, s.updateAvailabilityService(), "enroll", UpdateAvailabilityJobID, "UTC")
	second, err := NewService(s.options)
	if err != nil {
		t.Fatal(err)
	}
	second.now = s.now
	clock = clock.Add(25 * time.Hour)
	start := make(chan struct{})
	type outcome struct {
		report WakeReport
		err    error
	}
	out := make(chan outcome, 2)
	for _, service := range []*Service{s, second} {
		go func(service *Service) {
			<-start
			report, err := service.RunUpdateAvailability(context.Background())
			out <- outcome{report, err}
		}(service)
	}
	close(start)
	for range 2 {
		got := <-out
		if got.err != nil && !errors.Is(got.err, scheduler.ErrLeaseBusy) {
			t.Fatal(got.err)
		}
	}
	// A normal next wake recovers a busy occurrence without moving the clock.
	if _, err := s.RunUpdateAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}
	owner := s.updateAvailabilityService()
	receipts, err := owner.scheduler.Receipts(owner.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, receipt := range receipts {
		if receipt.JobID == UpdateAvailabilityJobID && receipt.State == scheduler.Succeeded && receipt.AttemptedAt.Equal(clock) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("concurrent first-use successes=%d, want one", count)
	}
}

func TestUpdateAvailabilityDailyOwnerScopeOfflineAndRevocation(t *testing.T) {
	s := productFixture(t)
	source := prepareAvailabilitySource(t, s)
	clock := s.now()
	s.now = func() time.Time { return clock }
	brain := filepath.Join(s.home, "brain", "retained.md")
	if err := os.MkdirAll(filepath.Dir(brain), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(brain, []byte("retained private knowledge\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dataRoot := filepath.Join(s.options.UserHome, ".maestro-install")
	beforeInstall, err := os.ReadFile(filepath.Join(dataRoot, "config", "install-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	qualified := productApply(t, s.updateAvailabilityService(), "qualify", UpdateAvailabilityJobID, "")
	if qualified.State != "inactive" || qualified.QualificationState != "qualified" {
		t.Fatalf("qualification %+v", qualified)
	}
	productApply(t, s.updateAvailabilityService(), "enroll", UpdateAvailabilityJobID, "UTC")
	otherWorkspace := filepath.Join(filepath.Dir(s.options.WorkspacePath), "second-workspace")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: otherWorkspace, DataRoot: filepath.Join(s.home, "state")}); err != nil {
		t.Fatal(err)
	}
	o := s.options
	o.WorkspacePath = otherWorkspace
	second, err := NewService(o)
	if err != nil {
		t.Fatal(err)
	}
	second.now = s.now
	clock = clock.Add(73 * time.Hour)
	if unrelated, err := s.selfObservationService().RunDue(context.Background()); err != nil || len(unrelated.Receipts) != 0 {
		t.Fatalf("another owner job executed update availability: %+v %v", unrelated, err)
	}
	idleCalls := 0
	s.options.EligibilityGuard = func() error { idleCalls++; return errors.New("user active") }
	report, err := s.RunUpdateAvailability(context.Background())
	if err != nil || len(report.Receipts) != 1 || report.Receipts[0].State != ReceiptSucceeded || idleCalls != 0 {
		t.Fatalf("first use %+v %v idle=%d", report, err, idleCalls)
	}
	view, err := second.Report(UpdateAvailabilityJobID)
	if err != nil || view.Availability == nil || view.Availability.Outcome != "current_signed_metadata" {
		t.Fatalf("report %+v %v", view, err)
	}
	if err := os.Remove(filepath.Join(source, releaseprovider.CurrentGenerationName)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		again, err := second.RunUpdateAvailability(context.Background())
		if err != nil || len(again.Receipts) != 0 {
			t.Fatalf("same-day/reconnect duplicate %+v %v", again, err)
		}
	}
	clock = clock.Add(24 * time.Hour)
	report, err = second.RunUpdateAvailability(context.Background())
	if err != nil || len(report.Receipts) != 1 || report.Receipts[0].State != ReceiptSucceeded {
		t.Fatalf("offline observation %+v %v", report, err)
	}
	view, err = second.Report(UpdateAvailabilityJobID)
	if err != nil || view.Availability.Outcome != "source_unavailable" {
		t.Fatalf("offline reported current: %+v %v", view, err)
	}
	changedOptions := second.options
	changedOptions.CLIVersion += "-changed"
	changed, err := NewService(changedOptions)
	if err != nil {
		t.Fatal(err)
	}
	changed.now = s.now
	status, err := changed.updateAvailabilityService().Status()
	if err != nil || len(status.Jobs) != 1 || status.Jobs[0].QualificationState != "stale" || status.Jobs[0].Reason != "qualification_requires_refresh" {
		t.Fatalf("changed implementation reused qualification: %+v %v", status, err)
	}
	if r, err := changed.RunUpdateAvailability(context.Background()); err == nil || len(r.Receipts) != 0 {
		t.Fatalf("stale update observer ran: %+v %v", r, err)
	}
	productApply(t, second.updateAvailabilityService(), "revoke", UpdateAvailabilityJobID, "")
	clock = clock.Add(24 * time.Hour)
	if r, err := s.RunUpdateAvailability(context.Background()); err != nil || len(r.Receipts) != 0 {
		t.Fatalf("revoked source checked %+v %v", r, err)
	}
	afterInstall, _ := os.ReadFile(filepath.Join(dataRoot, "config", "install-state.json"))
	knowledge, _ := os.ReadFile(brain)
	if string(beforeInstall) != string(afterInstall) || string(knowledge) != "retained private knowledge\n" {
		t.Fatal("availability changed source custody or brain")
	}
	if _, err := os.Lstat(filepath.Join(dataRoot, "config", "pending-update.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("availability staged a pending update")
	}
}
