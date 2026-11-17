package maintenance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

type WeeklyReportMetadata struct {
	TerminalState      string                      `json:"terminal_state,omitempty"`
	RegistrationSHA256 string                      `json:"registration_sha256"`
	QualificationID    string                      `json:"qualification_id,omitempty"`
	Working            *ownerctx.WeeklySelfReceipt `json:"working,omitempty"`
	ScheduledFor       time.Time                   `json:"scheduled_for,omitempty"`
	ExpiresAt          time.Time                   `json:"expires_at,omitempty"`
}

func (s *Service) weeklyReport() (ProductDomainReport, error) {
	out := ProductDomainReport{JobID: YodaSelfReviewWeeklyJobID, WorkspaceID: s.workspaceID, Scope: "owner", Finding: "no_report", NativeObservation: "not_observed"}
	if err := s.validate(); err != nil {
		return out, err
	}
	c, err := s.loadControl(YodaSelfReviewWeeklyJobID)
	if err != nil {
		return out, err
	}
	if c.Weekly == nil {
		return out, nil
	}
	out.Weekly = &WeeklyReportMetadata{RegistrationSHA256: c.Weekly.Registration.Digest()}
	qualified := c.Qualification != nil && s.verifyQualification(c.Qualification) == nil
	if qualified {
		out.Weekly.QualificationID = c.Qualification.ID
		out.Finding = "historical_native_probe_only"
		out.NativeObservation = "verified_historical_probe"
		out.ObservedAt = c.Qualification.VerifiedAt
	}
	options := c.Weekly.Registration.Policy
	options.OccurrenceID = strings.Repeat("0", 64)
	options.ScheduledFor = s.now().UTC()
	working, err := ownerctx.ReadWeeklySelf(filepath.Join(s.home, "brain"), options, s.now().UTC())
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Finding = "weekly_working_projection_unattested"
	out.NativeObservation = "not_observed"
	out.ObservedAt = time.Time{}
	out.Weekly.Working = &working.Receipt
	out.Weekly.ExpiresAt = working.ExpiresAt
	out.Weekly.ScheduledFor = working.Source.Options.ScheduledFor
	out.Counts = map[string]int{"facets": len(working.Facets)}
	if execution := working.Execution; execution != nil {
		out.Weekly.TerminalState = "pending"
		complete, e := s.weeklyTerminalComplete(weeklyReceiptFromExecution(YodaSelfReviewWeeklyJobID, execution), working.Source.Options.ScheduledFor)
		if e != nil {
			return out, e
		}
		if complete {
			out.Weekly.TerminalState = "complete"
		}

		if execution.RegistrationSHA256 != c.Weekly.Registration.Digest() || execution.OwnerScope != s.workspaceID {
			return out, ErrProductConflict
		}
		out.Finding = "historical_weekly_working_projection"
		out.NativeObservation = "historical_execution_record"
		if qualified && execution.QualificationSHA256 == c.Qualification.ID && execution.ImplementationSHA256 == s.build {
			out.Finding = "weekly_working_projection_committed"
			out.NativeObservation = "weekly_native_effect_record_verified"
		}
	}
	return out, nil
}
