package maintenance

import (
	"errors"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
)

type CapabilityFinding struct {
	ID              string `json:"id"`
	State           string `json:"state"`
	Configured      bool   `json:"configured"`
	NativeQualified bool   `json:"native_qualified"`
}
type SourceCoverage struct {
	Source string `json:"source"`
	State  string `json:"state"`
}
type ProductDomainReport struct {
	CurrentPending       *UpdatePendingReadback       `json:"current_pending,omitempty"`
	Update               *UpdateDiagnosis             `json:"update,omitempty"`
	Wiki                 *WikiDiagnostic              `json:"wiki,omitempty"`
	Period               string                       `json:"period,omitempty"`
	LifetimeStatus       string                       `json:"lifetime_status,omitempty"`
	JobID                string                       `json:"job_id"`
	WorkspaceID          string                       `json:"workspace_id"`
	Finding              string                       `json:"finding"`
	ObservedAt           time.Time                    `json:"observed_at,omitempty"`
	NativeObservation    string                       `json:"native_observation,omitempty"`
	InstallationCoverage string                       `json:"installation_coverage,omitempty"`
	Counts               map[string]int               `json:"counts,omitempty"`
	Sources              []SourceCoverage             `json:"sources,omitempty"`
	Capabilities         []CapabilityFinding          `json:"capabilities,omitempty"`
	Skills               []skillsindex.RuntimePointer `json:"skills,omitempty"`
}

// Report is a closed, read-only projection of independently verified domain
// output. It never opens an argument-selected path or returns captured text.
func (s *Service) Report(job string) (ProductDomainReport, error) {
	out := ProductDomainReport{JobID: job, WorkspaceID: s.workspaceID}
	if err := s.validate(); err != nil {
		return out, err
	}
	switch {
	case diagnosticJob(job):
		report, _, err := (DiagnosticStore{Root: s.receipts.Root}).Load(s.workspaceID, job, "")
		if err != nil {
			return out, err
		}
		out.Finding = report.Finding
		out.ObservedAt = report.ObservedAt
		if report.Update != nil {
			out.Update = report.Update
			out.CurrentPending = &UpdatePendingReadback{State: "unavailable"}
			if report.Update.Outcome == "candidate_staged_activation_unconfirmed" {
				// Keep the stored report/plan's actual historical provenance.
				// A current actionable plan is separately reauthenticated.
				out.Finding = "historical_update_check"
				options, e := s.updateOptions()
				if e == nil {
					pending, e := updateservice.LoadPending(options.DataRoot)
					if e == nil {
						verified, _, e := updateservice.ConfirmPending(options.DataRoot, options.ManagedRoot, pending.Plan.ID, options.Registry)
						if e == nil && verified.Plan.ToRelease == report.Update.CandidateRelease && verified.Plan.ManifestSHA256 == report.Update.ManifestSHA256 && verified.Plan.ClientInputsSHA256 == report.Update.ClientInputsSHA256 && verified.Plan.Source.GenerationSHA256 == report.Update.GenerationSHA256 {
							out.CurrentPending = &UpdatePendingReadback{State: "verified_unconfirmed", PlanID: verified.Plan.ID, Source: &verified.Plan.Source.Tuple, Sequence: verified.Plan.Source.Sequence, GenerationSHA256: verified.Plan.Source.GenerationSHA256, CandidateRelease: verified.Plan.ToRelease, ManifestSHA256: verified.Plan.ManifestSHA256, ClientInputsSHA256: verified.Plan.ClientInputsSHA256, TargetOS: verified.Plan.TargetOS, TargetArch: verified.Plan.TargetArch}
							if verified.Plan.ID == report.Update.PlanID {
								out.Finding = report.Finding
							}
						}
					}
				}
			}
		} else if report.Wiki != nil {
			out.Wiki = report.Wiki
			out.Counts = map[string]int{"concepts": report.Wiki.Concepts, "files": report.Wiki.Files}
			current, err := s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
			if err != nil || current.State != "current" || current.GraphEvidence != report.Wiki.GraphEvidence {
				out.Finding = "historical_private_graph_verification"
			}
		} else if report.Retention != nil {
			snapshot := report.Retention
			out.Counts = map[string]int{"live_captures": snapshot.LiveCaptures, "denied_captures": snapshot.DeniedCaptures, "active_artifacts": snapshot.ActiveArtifacts}
		} else {
			snapshot := report.Runtime
			out.NativeObservation = snapshot.NativeObservation
			out.InstallationCoverage = snapshot.InstallationCoverage
			for _, source := range snapshot.Observations {
				out.Sources = append(out.Sources, SourceCoverage{source.Source, source.State})
			}
			if snapshot.Projection != nil {
				out.Counts = map[string]int{"projection_conflicts": len(snapshot.Projection.Conflicts), "installed_skills": snapshot.Projection.SkillCount}
			}
			if job == "capability-recheck" {
				for _, capability := range snapshot.Capabilities {
					out.Capabilities = append(out.Capabilities, CapabilityFinding{capability.ID, capability.State, capability.Configured, false})
				}
			}
		}
	case job == "skills-index-refresh":
		index, err := skillsindex.LoadRuntimeIndex(s.indexRoot(), "")
		if err != nil {
			return out, err
		}
		current, err := s.indexSource()
		if err != nil {
			return out, err
		}
		if productDigest(current) != productDigest(index.Metadata.Source) {
			return out, ErrProductConflict
		}
		out.Finding = "verified_installed_index"
		out.Skills = index.Metadata.Source.Pointers
		out.Counts = map[string]int{"skills": len(out.Skills)}
	case job == MemoryDeepDreamJobID:
		output, err := s.weekly.WeeklyOutput(s.workspaceID)
		if err != nil {
			return out, err
		}
		out.Finding = "weekly_layers_published"
		out.Counts = map[string]int{"layers": 2}
		out.ObservedAt = output.GeneratedAt
		out.Period = output.Period
		out.LifetimeStatus = output.LifetimeStatus
	default:
		return out, errors.New("domain report unavailable for job")
	}
	return out, nil
}
