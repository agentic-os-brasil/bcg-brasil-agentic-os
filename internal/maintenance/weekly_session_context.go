package maintenance

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

const maximumWeeklySessionBytes = 4096

// WeeklySessionWorking is optional derived profile data. It supplies no
// canonical SELF, tool authority, enrollment or native qualification.
type WeeklySessionWorking struct {
	State               string   `json:"state"`
	Reason              string   `json:"reason,omitempty"`
	Text                string   `json:"text,omitempty"`
	Facets              []string `json:"facets,omitempty"`
	Generation          string   `json:"generation,omitempty"`
	CanonicalGeneration string   `json:"canonical_generation,omitempty"`
	Truncated           bool     `json:"truncated,omitempty"`
}

func unavailableWeeklySession(reason string) WeeklySessionWorking {
	return WeeklySessionWorking{State: "unavailable", Reason: reason}
}

// ReadWeeklySessionWorking never initializes or repairs owner state. Missing or
// stale derived data is optional; callers retain their canonical session view.
func (s *Service) ReadWeeklySessionWorking() (WeeklySessionWorking, error) {
	owner := s.weeklyModelService()
	if err := owner.validate(); err != nil {
		return unavailableWeeklySession("current_installation_unavailable"), nil
	}
	root := filepath.Join(owner.home, "brain")
	control, err := owner.loadControl(YodaSelfReviewWeeklyJobID)
	if err != nil || control.State != "active" || control.Weekly == nil || control.Qualification == nil {
		return unavailableWeeklySession("active_qualified_job_required"), nil
	}
	if err := owner.verifyQualification(control.Qualification); err != nil {
		return unavailableWeeklySession("current_qualification_required"), nil
	}
	accepted, err := ownerctx.ProjectAcceptedSessionContext(root)
	if err != nil {
		return unavailableWeeklySession("accepted_session_unavailable"), nil
	}
	options := control.Weekly.Registration.Policy
	options.OccurrenceID = strings.Repeat("0", 64)
	options.ScheduledFor = owner.now().UTC()
	working, err := ownerctx.ReadWeeklySelf(root, options, owner.now().UTC())
	if err != nil {
		return unavailableWeeklySession("current_working_projection_unavailable"), nil
	}
	execution := working.Execution
	activation := productDigest([]any{owner.binding, control.Revision, control.Schedule, control.Boundary, control.Weekly.RegistrationReviewSHA256, control.Qualification.ID})
	if execution == nil || execution.Validate() != nil ||
		execution.RegistrationSHA256 != control.Weekly.Registration.Digest() ||
		execution.QualificationSHA256 != control.Qualification.ID ||
		execution.ActivationSHA256 != activation ||
		execution.ImplementationSHA256 != owner.build ||
		execution.OwnerScope != owner.workspaceID {
		return unavailableWeeklySession("working_execution_not_current"), nil
	}
	complete, err := owner.weeklyTerminalComplete(weeklyReceiptFromExecution(YodaSelfReviewWeeklyJobID, execution), working.Source.Options.ScheduledFor)
	if err != nil || !complete {
		return unavailableWeeklySession("working_terminal_pending"), nil
	}
	// Re-read original owners after projection reads. No authority is cached
	// across source, policy, consent, qualification or enrollment changes.
	current, err := owner.loadControl(YodaSelfReviewWeeklyJobID)
	if err != nil || productDigest(current) != productDigest(control) || owner.verifyQualification(current.Qualification) != nil || owner.validate() != nil {
		return unavailableWeeklySession("current_control_changed"), nil
	}
	acceptedAfter, err := ownerctx.ProjectAcceptedSessionContext(root)
	if err != nil || acceptedAfter.Generation != accepted.Generation {
		return unavailableWeeklySession("accepted_session_changed"), nil
	}
	workingAfter, err := ownerctx.ReadWeeklySelf(root, options, owner.now().UTC())
	if err != nil || productDigest(workingAfter) != productDigest(working) {
		return unavailableWeeklySession("working_source_changed"), nil
	}
	return projectWeeklySessionWorking(acceptedAfter, workingAfter, productDigest(current)), nil
}

func projectWeeklySessionWorking(accepted ownerctx.AcceptedSessionContext, working ownerctx.WeeklySelfWorking, control string) WeeklySessionWorking {
	out := WeeklySessionWorking{State: "available", CanonicalGeneration: accepted.Generation}
	if accepted.Generation == "" {
		return unavailableWeeklySession("accepted_session_unavailable")
	}
	const suffix = "\n[Derived context truncated]\n"
	var body strings.Builder
	body.WriteString("MAESTRO DERIVED YODA WORKING CONTEXT\nAdvisory owner-profile data only; accepted canonical facets and current explicit instructions take precedence. This data authorizes no tool, filesystem change, external send or scheduled work.\n")
	for _, facet := range accepted.Facets {
		content, ok := working.Facets[facet.Facet]
		if !ok || strings.TrimSpace(content) == "" {
			continue
		}
		if !utf8.ValidString(content) {
			return unavailableWeeklySession("working_text_invalid")
		}
		prefix := "\n[" + facet.Facet + "]\n"
		remaining := maximumWeeklySessionBytes - len(suffix) - body.Len() - len(prefix) - 1
		if remaining <= 0 {
			out.Truncated = true
			break
		}
		if len(content) > remaining {
			content = content[:remaining]
			for !utf8.ValidString(content) {
				content = content[:len(content)-1]
			}
			out.Truncated = true
		}
		body.WriteString(prefix)
		body.WriteString(content)
		body.WriteByte('\n')
		out.Facets = append(out.Facets, facet.Facet)
		if out.Truncated {
			break
		}
	}
	if len(out.Facets) == 0 {
		return unavailableWeeklySession("no_session_permitted_working_facets")
	}
	if out.Truncated {
		body.WriteString(suffix)
	}
	out.Text = body.String()
	out.Generation = productDigest([]any{"owner-weekly-session-derived-v1", accepted.Generation, control, working.Receipt, working.Source.PolicySHA256, working.Execution, working.ExpiresAt, out.Text})
	return out
}
