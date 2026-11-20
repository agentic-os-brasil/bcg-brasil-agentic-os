package product_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestWeeklyReportMCPNoReportIsReadOnly(t *testing.T) {
	f := newOwnerFixture(t, false)
	before := cadenceTree(t, f)
	cs := f.connect(t, nil)
	report := cadenceCall(t, cs, "report", map[string]any{"job_id": maintenance.YodaSelfReviewWeeklyJobID}, false)
	if report["finding"] != "no_report" || report["scope"] != "owner" {
		t.Fatal("missing report was not truthful owner state", report)
	}
	if cadenceTree(t, f) != before {
		t.Fatal("report initialized or changed owner state")
	}
}

func TestWeeklyReportMCPReadsCanonicalWorkingAcrossWorkspaces(t *testing.T) {
	// This fixture publishes owned canonical test data, not a native model run or qualification.
	f := newOwnerFixture(t, false)
	digest := strings.Repeat("a", 64)
	registration := maintenance.WeeklyModelRegistration{SchemaVersion: 1, Descriptor: maintenance.WeeklyModelDescriptor{SchemaVersion: 1, JobID: maintenance.YodaSelfReviewWeeklyJobID, Runtime: "claude_code_cli", RuntimeVersion: "synthetic", Executable: filepath.Join(filepath.Dir(f.root), "not-executed"), ExecutableSHA256: digest, ConfigurationSHA256: digest, ManagedPolicySHA256: digest, ProfileReferenceSHA256: digest, TransportSHA256: digest, Provider: "synthetic", Model: "synthetic", CredentialMode: "runtime_owned_session", ValidatorVersion: "weekly-self-v1", Limits: maintenance.WeeklyModelLimits{MaxInputBytes: 32768, MaxOutputBytes: 32768, MaxStderrBytes: 4096, MaxOutputTokens: 128, TimeoutSeconds: 5, Attempts: 1, AgenticTurns: 1}}, Policy: ownerctx.WeeklySelfOptions{OwnerID: "synthetic-owner", Facets: []string{"communication-style"}, Limits: ownerctx.DefaultWeeklySelfLimits(), WorkingLanguage: "en", TranslatorID: "identity", TranslatorVersion: "1"}}
	f.options.WeeklyCandidate = &registration
	b, e := product.Open(f.options)
	if e != nil {
		t.Fatal(e)
	}
	mutation := product.MaintenanceMutation{Operation: "maintenance.register", JobID: maintenance.YodaSelfReviewWeeklyJobID}
	review, e := b.MaintenanceReview(mutation)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = b.ApplyMaintenanceReview(context.Background(), mutation, review.ReviewDigest, true); e != nil {
		t.Fatal(e)
	}
	canonical := filepath.Join(f.root, "owner/self/communication-style.md")
	if e = os.WriteFile(canonical, []byte("# Communication style\n\n## Current\n\nprivate-weekly-source-body\n"), 0600); e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	options := registration.Policy
	options.OccurrenceID = digest
	options.ScheduledFor = now
	input, e := ownerctx.FreezeWeeklySelf(f.root, options, nil, now)
	if e != nil {
		t.Fatal(e)
	}
	candidate := ownerctx.WeeklySelfCandidate{SchemaVersion: 1, InputSHA256: input.InputSHA256, CanonicalSHA256: input.Facets.CanonicalSourceDigest, PromptHistorySHA256: input.PromptHistorySHA256, PolicySHA256: input.PolicySHA256, TranslationSHA256: input.TranslationSHA256, Facets: map[string]string{"communication-style": "private-weekly-working-body"}}
	if _, e = ownerctx.PublishWeeklySelf(context.Background(), f.root, input.InputSHA256, candidate, input.ExpectedRevision, func(write func() error) error { return write() }, now); e != nil {
		t.Fatal(e)
	}
	other := f
	other.options.WorkspacePath = filepath.Join(filepath.Dir(f.options.WorkspacePath), "workspace-other")
	if _, e = workspace.Initialize(workspace.Options{WorkspacePath: other.options.WorkspacePath, DataRoot: filepath.Join(filepath.Dir(f.root), "state")}); e != nil {
		t.Fatal(e)
	}
	before := cadenceTree(t, f)
	first := cadenceCall(t, f.connect(t, nil), "report", map[string]any{"job_id": maintenance.YodaSelfReviewWeeklyJobID}, false)
	second := cadenceCall(t, other.connect(t, nil), "report", map[string]any{"job_id": maintenance.YodaSelfReviewWeeklyJobID}, false)
	if first["workspace_id"] != second["workspace_id"] || first["scope"] != "owner" {
		t.Fatal("report used workspace namespace", first, second)
	}
	if first["finding"] != "weekly_working_projection_unattested" || first["native_observation"] != "not_observed" {
		t.Fatal("test canonical data claimed native execution", first)
	}
	if counts, ok := first["counts"].(map[string]any); !ok || counts["facets"] != float64(1) {
		t.Fatal("verified current working metadata missing", first)
	}
	if cadenceTree(t, f) != before {
		t.Fatal("report changed owner state")
	}
	encoded, _ := json.Marshal(first)
	for _, private := range []string{"private-weekly-source-body", "private-weekly-working-body"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("weekly report leaked body")
		}
	}

	if e = os.WriteFile(canonical, []byte("# Current\n\nCorrection after working publication.\n"), 0600); e != nil {
		t.Fatal(e)
	}
	cadenceCall(t, f.connect(t, nil), "report", map[string]any{"job_id": maintenance.YodaSelfReviewWeeklyJobID}, true)
}
