package maintenance

import (
	"errors"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
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
		if report.Retention != nil {
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
