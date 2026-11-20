package setup

import (
	"context"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"strings"
	"testing"
)

func TestModelRegistrationRequiresExplicitConfirmationBeforeSourceAccess(t *testing.T) {
	if _, err := ApplyModelRegistration(context.Background(), installer.ClientOptions{}, ModelRegistrationReview{}, "", false); err == nil || !strings.Contains(err.Error(), "explicit exact") {
		t.Fatalf("registration requires its exact operator review before source access: %v", err)
	}
	if _, err := ApplyModelRegistration(context.Background(), installer.ClientOptions{}, ModelRegistrationReview{}, "", true); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("missing review digest must fail before source access: %v", err)
	}
}
func TestModelProfilePreparationAndLoginRequireSeparateConfirmation(t *testing.T) {
	if _, err := ApplyModelProfile(context.Background(), installer.ClientOptions{}, ModelProfileReview{}, "", false); err == nil || !strings.Contains(err.Error(), "preparation review") {
		t.Fatalf("preparation approval required: %v", err)
	}
	if _, err := LoginModelProfile(context.Background(), installer.ClientOptions{}, ModelProfileLoginReview{}, "", false); err == nil || !strings.Contains(err.Error(), "official model profile login action") {
		t.Fatalf("distinct official login approval required: %v", err)
	}
}
func TestModelRegistrationRefusesOwnerPolicyForWorkspaceBeforeSourceAccess(t *testing.T) {
	selection := ModelRegistrationSelection{JobID: maintenance.MemoryModelJobID, Provider: "anthropic", Model: "claude-test", OwnerPolicy: ownerctx.WeeklySelfOptions{WorkingLanguage: "pt-BR"}}
	if _, err := ReviewModelRegistration(context.Background(), installer.ClientOptions{}, selection); err == nil || !strings.Contains(err.Error(), "cannot borrow owner self policy") {
		t.Fatalf("workspace model registration must reject owner policy, not discard it: %v", err)
	}
	selection.OwnerPolicy = ownerctx.WeeklySelfOptions{}
	selection.Model = "--arbitrary-option"
	if _, err := ReviewModelRegistration(context.Background(), installer.ClientOptions{}, selection); err == nil || !strings.Contains(err.Error(), "provider and model") {
		t.Fatalf("model selection must remain bounded data: %v", err)
	}
}

func TestModelRegistrationReviewUsesActualSelectedJobScope(t *testing.T) {
	for _, tc := range []struct {
		name, jobID, jobScope, jobScopeID, reviewScope, reviewID string
		wantErr                                                  bool
	}{
		{"owner_review", maintenance.YodaSelfReviewWeeklyJobID, "owner", "owner-a", "owner", "owner-a", false},
		{"foreign_owner", maintenance.YodaSelfReviewWeeklyJobID, "owner", "owner-a", "owner", "owner-b", true},
		{"workspace_cannot_replace_owner", maintenance.YodaSelfReviewWeeklyJobID, "owner", "owner-a", "workspace", "workspace-a", true},
		{"owner_id_requires_owner_scope", maintenance.YodaSelfReviewWeeklyJobID, "", "owner-a", "owner", "owner-a", true},
		{"owner_scope_id_required", maintenance.YodaSelfReviewWeeklyJobID, "owner", "", "owner", "", true},
		{"workspace_review", maintenance.MemoryModelJobID, "", "", "workspace", "workspace-a", false},
		{"explicit_workspace_metadata", maintenance.MemoryModelJobID, "workspace", "workspace-a", "workspace", "workspace-a", false},
		{"memory_cannot_use_owner_review", maintenance.MemoryModelJobID, "", "", "owner", "workspace-a", true},
		{"memory_cannot_use_owner_status", maintenance.MemoryModelJobID, "owner", "owner-a", "workspace", "workspace-a", true},
		{"foreign_workspace_status", maintenance.MemoryModelJobID, "workspace", "workspace-b", "workspace", "workspace-a", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job := ClientMaintenanceJob{JobID: tc.jobID, Scope: tc.jobScope, ScopeID: tc.jobScopeID, Revision: 3}
			status := ClientMaintenanceStatus{WorkspaceID: "workspace-a", Jobs: []ClientMaintenanceJob{job}}
			registration := maintenance.WeeklyModelRegistration{Descriptor: maintenance.WeeklyModelDescriptor{JobID: tc.jobID}}
			original := product.LocalMaintenanceReview{Review: maintenance.ProductReview{
				Scope: tc.reviewScope, WorkspaceID: tc.reviewID,
				Operation:          maintenance.Operation{Kind: "register", JobID: tc.jobID, ExpectedRevision: 3},
				WeeklyRegistration: &registration,
			}, ReviewDigest: "measured-original-review"}
			if err := validateMeasuredModelReview(status, job, registration, original); (err != nil) != tc.wantErr {
				t.Fatalf("selected job scope mismatch: got error %v, want error %v", err, tc.wantErr)
			}
		})
	}
}
