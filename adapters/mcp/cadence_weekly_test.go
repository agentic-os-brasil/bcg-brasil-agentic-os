package product_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestWeeklyRegistrationMutationAcceptsOnlyClosedIntent(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"weekly", `{"operation":"maintenance.register","job_id":"yoda-self-review-weekly","expected_revision":0}`, true},
		{"legacy", `{"operation":"maintenance.qualify","job_id":"memory-light-dream","expected_revision":0}`, true},
		{"other-job", `{"operation":"maintenance.register","job_id":"darwin-daily","expected_revision":0}`, false},
		{"model-selected-declaration", `{"operation":"maintenance.register","job_id":"yoda-self-review-weekly","expected_revision":0,"registration":{}}`, false},
		{"negative-revision", `{"operation":"maintenance.register","job_id":"yoda-self-review-weekly","expected_revision":-1}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := product.DecodeMaintenanceMutation([]byte(tc.body))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}

func TestWeeklyStartupDeclarationReachesReviewWithoutQualification(t *testing.T) {
	f := newOwnerFixture(t, false)
	request := product.MaintenanceMutation{Operation: "maintenance.register", JobID: maintenance.YodaSelfReviewWeeklyJobID}
	bound, err := product.Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bound.MaintenanceReview(request); err == nil {
		t.Fatal("registration without trusted startup declaration accepted")
	}
	candidate := weeklyMCPRegistrationFixture(f)
	f.options.WeeklyCandidate = &candidate
	bound, err = product.Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	review, err := bound.MaintenanceReview(request)
	if err != nil {
		t.Fatal(err)
	}
	if review.Review.WeeklyRegistration == nil || review.Review.WeeklyRegistration.Digest() != candidate.Digest() || review.Review.Scope != "owner" {
		t.Fatal("startup declaration did not reach exact owner review")
	}
	if _, err := bound.ApplyMaintenanceReview(context.Background(), request, review.ReviewDigest, false); err == nil {
		t.Fatal("declined declaration accepted")
	}
	result, err := bound.ApplyMaintenanceReview(context.Background(), request, review.ReviewDigest, true)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		State              string `json:"state"`
		QualificationState string `json:"qualification_state"`
		Revision           uint64 `json:"revision"`
	}
	if err := json.Unmarshal(body, &state); err != nil || state.State != "inactive" || state.QualificationState != "unqualified" || state.Revision != 1 {
		t.Fatalf("declaration qualified or activated work: %s %v", body, err)
	}
	if _, err := bound.ApplyMaintenanceReview(context.Background(), request, review.ReviewDigest, true); err == nil {
		t.Fatal("stale registration review replay accepted")
	}
}

func weeklyMCPRegistrationFixture(f ownerFixture) maintenance.WeeklyModelRegistration {
	digest := strings.Repeat("a", 64)
	return maintenance.WeeklyModelRegistration{
		SchemaVersion: 1,
		Descriptor: maintenance.WeeklyModelDescriptor{
			SchemaVersion: 1, JobID: maintenance.YodaSelfReviewWeeklyJobID,
			Runtime: "claude_code_cli", RuntimeVersion: "synthetic-version",
			Executable: filepath.Join(filepath.Dir(f.root), "synthetic-not-executed"), ExecutableSHA256: digest,
			ConfigurationSHA256: digest, ManagedPolicySHA256: digest, ProfileReferenceSHA256: digest, TransportSHA256: digest,
			Provider: "synthetic-provider", Model: "synthetic-model", CredentialMode: "runtime_owned_session", ValidatorVersion: "weekly-self-v1",
			Limits: maintenance.WeeklyModelLimits{MaxInputBytes: 32768, MaxOutputBytes: 32768, MaxStderrBytes: 4096, MaxOutputTokens: 128, TimeoutSeconds: 5, Attempts: 1, AgenticTurns: 1},
		},
		Policy: ownerctx.WeeklySelfOptions{OwnerID: "synthetic-owner", Facets: []string{"communication-style"}, Limits: ownerctx.DefaultWeeklySelfLimits(), WorkingLanguage: "en", TranslatorID: "identity", TranslatorVersion: "1"},
	}
}

func TestWeeklyNativeFormShowsCompleteOwnerRegistration(t *testing.T) {
	for _, accept := range []bool{false, true} {
		t.Run(map[bool]string{false: "decline", true: "accept"}[accept], func(t *testing.T) {
			f := newOwnerFixture(t, false)
			candidate := weeklyMCPRegistrationFixture(f)
			f.options.WeeklyCandidate = &candidate
			before := cadenceTree(t, f)
			seen := 0
			client := f.connect(t, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				seen++
				start := strings.IndexByte(req.Params.Message, '{')
				if start < 0 {
					t.Fatal("native form omitted review JSON")
				}
				var shown struct {
					Registration *maintenance.WeeklyModelRegistration `json:"weekly_registration"`
					ScopeKind    string                               `json:"scope_kind"`
					OwnerScopeID string                               `json:"owner_scope_id"`
					Scope        string                               `json:"scope"`
				}
				if err := json.Unmarshal([]byte(req.Params.Message[start:]), &shown); err != nil {
					t.Fatal(err)
				}
				if shown.Registration == nil || shown.Registration.Digest() != candidate.Digest() {
					t.Error("native form hid or changed the exact executable/provider/model/configuration/policy/limits")
				}
				if shown.ScopeKind != "owner" || shown.OwnerScopeID == "" || !strings.Contains(shown.Scope, "across admitted workspaces") {
					t.Error("native form did not disclose owner-wide effect")
				}
				if !accept {
					return &mcp.ElicitResult{Action: "decline"}, nil
				}
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			})
			out := cadenceCall(t, client, "register", map[string]any{"job_id": maintenance.YodaSelfReviewWeeklyJobID, "expected_revision": 0}, !accept)
			if seen != 1 {
				t.Fatalf("native reviews=%d, expected one", seen)
			}
			if !accept && cadenceTree(t, f) != before {
				t.Fatal("declined native review changed owner state")
			}
			if accept && (out["scope"] != "owner" || out["state"] != "inactive" || out["qualification_state"] != "unqualified") {
				t.Fatal("registration activated or qualified work", out)
			}
		})
	}
}
