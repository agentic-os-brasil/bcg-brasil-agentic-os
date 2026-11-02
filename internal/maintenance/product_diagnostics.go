package maintenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installreadiness"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimecap"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimeprojection"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

type SourceObservation struct {
	Source string `json:"source"`
	State  string `json:"state"`
	SHA256 string `json:"sha256,omitempty"`
}
type RuntimeDiagnosis struct {
	Runtime              string                        `json:"runtime"`
	NativeObservation    string                        `json:"native_observation"`
	Observations         []SourceObservation           `json:"observations"`
	Projection           *runtimeprojection.Status     `json:"projection,omitempty"`
	Readiness            *installreadiness.Report      `json:"readiness,omitempty"`
	CapabilityManifest   runtimecap.Manifest           `json:"capability_manifest"`
	Capabilities         []runtimecap.CapabilityReport `json:"capabilities"`
	InstallationCoverage string                        `json:"installation_coverage"`
}
type DiagnosticReport struct {
	SchemaVersion int                    `json:"schema_version"`
	WorkspaceID   string                 `json:"workspace_id"`
	JobID         string                 `json:"job_id"`
	SourceSHA256  string                 `json:"source_sha256"`
	ObservedAt    time.Time              `json:"observed_at"`
	Finding       string                 `json:"finding"`
	Runtime       *RuntimeDiagnosis      `json:"runtime,omitempty"`
	Retention     *memory.RetentionState `json:"retention,omitempty"`
}

func boundedSource(path string) ([]byte, error) {
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > 2<<20 {
		return nil, errors.New("source exceeds observation bound")
	}
	body, err := io.ReadAll(io.LimitReader(f, 2<<20+1))
	if len(body) > 2<<20 {
		return nil, errors.New("source exceeds observation bound")
	}
	return body, err
}
func rawDigest(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }
func observeFile(root, relative, label string) (SourceObservation, []byte, error) {
	body, err := boundedSource(filepath.Join(root, relative))
	if errors.Is(err, os.ErrNotExist) {
		return SourceObservation{Source: label, State: "missing"}, nil, nil
	}
	if err != nil {
		return SourceObservation{}, nil, err
	}
	return SourceObservation{Source: label, State: "observed", SHA256: rawDigest(body)}, body, nil
}
func (s *Service) runtimeDiagnosis() (RuntimeDiagnosis, error) {
	out := RuntimeDiagnosis{Runtime: "unknown", NativeObservation: "not_observed", InstallationCoverage: "unavailable"}
	// These are real admitted workspace bytes. An absent native projection does
	// not erase the observed identity and cannot become positive native evidence.
	for _, file := range []struct{ root, relative, label string }{
		{s.options.WorkspacePath, ".bcgos/workspace.json", "workspace_registration"},
		{s.home, "config/install-state.json", "installation_state"},
	} {
		observation, _, err := observeFile(file.root, file.relative, file.label)
		if err != nil {
			return out, err
		}
		out.Observations = append(out.Observations, observation)
	}
	out.Observations = append(out.Observations, SourceObservation{"running_executable", "observed", s.build})
	observation, body, err := observeFile(s.options.WorkspacePath, runtimeprojection.ManifestRelativePath, "runtime_projection")
	if err != nil {
		return out, err
	}
	out.Observations = append(out.Observations, observation)
	if observation.State == "observed" {
		var selected struct {
			Runtime string `json:"runtime"`
		}
		if err = json.Unmarshal(body, &selected); err != nil {
			return out, err
		}
		if selected.Runtime != "claude" && selected.Runtime != "codex" {
			return out, errors.New("invalid projection runtime")
		}
		out.Runtime = selected.Runtime
		status, err := runtimeprojection.Inspect(selected.Runtime, s.options.WorkspacePath)
		if err != nil {
			return out, err
		}
		out.Projection = &status
		relative := ".codex/hooks.json"
		if selected.Runtime == "claude" {
			relative = ".claude/settings.local.json"
		}
		config, _, err := observeFile(s.options.WorkspacePath, relative, "runtime_configuration")
		if err != nil {
			return out, err
		}
		out.Observations = append(out.Observations, config)
		readiness, _ := installreadiness.Verify(installreadiness.Options{Runtime: selected.Runtime, WorkspacePath: s.options.WorkspacePath, DataRoot: s.home, ExecutablePath: s.executable, CLIVersion: s.options.CLIVersion})
		out.Readiness = &readiness
		// Readiness is configuration evidence only, even when Ready is true.
		if readiness.Ready {
			out.InstallationCoverage = "configured"
		}
	}
	manifest, err := baseruntime.Manifest()
	if err != nil {
		return out, err
	}
	out.CapabilityManifest = manifest
	for _, capability := range manifest.Capabilities {
		declared := capability.Runtimes[out.Runtime]
		item := runtimecap.CapabilityReport{ID: capability.ID, Criticality: capability.Criticality, SemanticEvent: capability.SemanticEvent, State: "unavailable", Mechanism: declared.Mechanism, Reason: "native observation unavailable"}
		if out.Readiness != nil {
			for _, binding := range out.Readiness.Lifecycle {
				if binding.CapabilityID == capability.ID {
					item.Configured = binding.Configured
				}
			}
		}
		out.Capabilities = append(out.Capabilities, item)
	}
	return out, nil
}
func (s *Service) indexSource() (skillsindex.RuntimeIndexSource, error) {
	observation, body, err := observeFile(s.options.WorkspacePath, runtimeprojection.ManifestRelativePath, "runtime_projection")
	if err != nil {
		return skillsindex.RuntimeIndexSource{}, err
	}
	if observation.State != "observed" {
		return skillsindex.RuntimeIndexSource{}, scheduler.ErrCapabilityUnavailable
	}
	var selected struct {
		Runtime string `json:"runtime"`
	}
	if err = json.Unmarshal(body, &selected); err != nil {
		return skillsindex.RuntimeIndexSource{}, err
	}
	catalog, _, installed, err := runtimeprojection.RoutingInputs(selected.Runtime, s.options.WorkspacePath)
	if err != nil {
		return skillsindex.RuntimeIndexSource{}, err
	}
	source := skillsindex.RuntimeIndexSource{WorkspaceID: s.workspaceID, Runtime: selected.Runtime, ProjectionSHA256: observation.SHA256, Catalog: catalog}
	for _, skill := range installed {
		source.Pointers = append(source.Pointers, skillsindex.RuntimePointer{ID: skill.ID, Pointer: skill.Pointer})
	}
	skillsindex.SortRuntimePointers(source.Pointers)
	after, _, err := observeFile(s.options.WorkspacePath, runtimeprojection.ManifestRelativePath, "runtime_projection")
	if err != nil {
		return source, err
	}
	if after != observation {
		return source, ErrProductConflict
	}
	return source, nil
}
func (s *Service) diagnosticSource(job string) (DiagnosticReport, error) {
	report := DiagnosticReport{SchemaVersion: 1, WorkspaceID: s.workspaceID, JobID: job}
	if job == "memory-retention-check" {
		snapshot, err := s.engine.RetentionSnapshot(s.workspaceID)
		if err != nil {
			return report, err
		}
		report.Retention = &snapshot
		report.SourceSHA256 = productDigest(snapshot)
		report.Finding = "sources_preserved"
	} else {
		snapshot, err := s.runtimeDiagnosis()
		if err != nil {
			return report, err
		}
		report.Runtime = &snapshot
		report.SourceSHA256 = productDigest(snapshot)
		report.Finding = "unavailable"
		if job == "runtime-drift-check" {
			report.Finding = "incomplete"
			if snapshot.Projection != nil && len(snapshot.Projection.Conflicts) > 0 {
				report.Finding = "drift_detected"
			}
			if snapshot.Projection != nil && snapshot.Projection.State == "installed" && snapshot.InstallationCoverage == "configured" {
				report.Finding = "configured_sources_match_native_unobserved"
			}
		}
	}
	return report, nil
}
func (r DiagnosticReport) validate() error {
	if r.SchemaVersion != 1 || !commandIDPattern.MatchString(r.WorkspaceID) || !diagnosticJob(r.JobID) || !digestPattern.MatchString(r.SourceSHA256) || r.ObservedAt.IsZero() {
		return errors.New("invalid diagnostic report identity")
	}
	if r.JobID == "memory-retention-check" {
		if r.Retention == nil || r.Runtime != nil || r.Retention.WorkspaceID != r.WorkspaceID || !r.Retention.PreserveSources || r.Retention.LiveCaptures+r.Retention.DeniedCaptures+r.Retention.ActiveArtifacts < 1 || productDigest(r.Retention) != r.SourceSHA256 {
			return errors.New("retention diagnosis lacks actual canonical state")
		}
	} else {
		if r.Runtime == nil || r.Retention != nil || r.Runtime.NativeObservation != "not_observed" || productDigest(r.Runtime) != r.SourceSHA256 || r.Runtime.CapabilityManifest.Validate() != nil {
			return errors.New("invalid runtime diagnosis")
		}
		observed := 0
		for _, o := range r.Runtime.Observations {
			if o.State == "observed" && digestPattern.MatchString(o.SHA256) {
				observed++
			}
		}
		if observed < 2 || len(r.Runtime.Capabilities) != len(r.Runtime.CapabilityManifest.Capabilities) {
			return errors.New("diagnosis lacks actual observed input")
		}
		for _, c := range r.Runtime.Capabilities {
			if c.State != "unavailable" || c.AdapterObserved || c.NativeQualified {
				return errors.New("diagnosis cannot qualify native capability")
			}
		}
	}
	return nil
}
func diagnosticJob(id string) bool {
	return id == "memory-retention-check" || id == "runtime-health-check" || id == "capability-recheck" || id == "runtime-drift-check"
}

type DiagnosticStore struct{ Root string }

func (s DiagnosticStore) root(workspace, job string) string {
	return filepath.Join(s.Root, "workspaces", workspace, "diagnostics", job)
}
func (s DiagnosticStore) Load(workspace, job, generation string) (DiagnosticReport, string, error) {
	var report DiagnosticReport
	if !commandIDPattern.MatchString(workspace) || !diagnosticJob(job) {
		return report, "", errors.New("invalid diagnostic scope")
	}
	root := s.root(workspace, job)
	if generation == "" {
		body, err := scheduler.ReadPrivateFile(filepath.Join(root, "current.json"), 1024)
		if err != nil {
			return report, "", err
		}
		var pointer struct {
			Generation string `json:"generation"`
		}
		if decodeProduct(body, &pointer) != nil {
			return report, "", errors.New("invalid diagnostic pointer")
		}
		generation = pointer.Generation
	}
	if !digestPattern.MatchString(generation) {
		return report, "", errors.New("invalid diagnostic generation")
	}
	path := filepath.Join(root, "versions", generation+".json")
	body, err := scheduler.ReadPrivateFile(path, 2<<20)
	if err != nil {
		return report, "", err
	}
	if rawDigest(body) != generation || decodeProduct(body, &report) != nil || report.WorkspaceID != workspace || report.JobID != job {
		return report, "", errors.New("diagnostic readback mismatch")
	}
	if err = report.validate(); err != nil {
		return report, "", err
	}
	return report, path, nil
}
func (s DiagnosticStore) Publish(report DiagnosticReport, guard func(func() error) error) (bool, error) {
	if err := report.validate(); err != nil {
		return false, err
	}
	if guard == nil {
		return false, errors.New("diagnostic publication guard required")
	}
	changed := false
	err := scheduler.WithPrivateLock(filepath.Join(s.root(report.WorkspaceID, report.JobID), "publication.guard"), func() error {
		previous, _, err := s.Load(report.WorkspaceID, report.JobID, "")
		if err == nil && previous.SourceSHA256 == report.SourceSHA256 {
			return nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		body, err := json.Marshal(report)
		if err != nil {
			return err
		}
		generation := rawDigest(body)
		root := s.root(report.WorkspaceID, report.JobID)
		if err = scheduler.EnsurePrivateDirectory(filepath.Join(root, "versions")); err != nil {
			return err
		}
		if err = scheduler.WritePrivateFileAtomic(filepath.Join(root, "versions", generation+".json"), body); err != nil {
			return err
		}
		if _, _, err = s.Load(report.WorkspaceID, report.JobID, generation); err != nil {
			return err
		}
		pointer, _ := json.Marshal(struct {
			Generation string `json:"generation"`
		}{generation})
		if err = guard(func() error { return scheduler.WritePrivateFileAtomic(filepath.Join(root, "current.json"), pointer) }); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return changed, err
}

type productDiagnosticHandler struct {
	service  *Service
	expected string
	job      string
}

func (h productDiagnosticHandler) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if err := ValidateExecutionGrant(grant, command); err != nil {
		return HandlerResult{}, err
	}
	if command.JobID != h.job || command.WorkspaceID != h.service.workspaceID {
		return HandlerResult{}, ErrProductConflict
	}
	execute := func(snapshot *memory.RetentionState) (HandlerResult, error) {
		report, err := h.service.diagnosticSource(h.job)
		if err != nil {
			return HandlerResult{}, err
		}
		if snapshot != nil && productDigest(snapshot) != report.SourceSHA256 {
			return HandlerResult{}, ErrProductConflict
		}
		if h.expected != "" && h.expected != report.SourceSHA256 {
			return HandlerResult{}, ErrProductConflict
		}
		report.ObservedAt = command.RequestedAt
		changed, err := (DiagnosticStore{Root: h.service.receipts.Root}).Publish(report, func(publish func() error) error {
			return publishWithGrant(ctx, grant, command, func() error {
				current, err := h.service.diagnosticSource(h.job)
				if err != nil {
					return err
				}
				if current.SourceSHA256 != report.SourceSHA256 {
					return ErrProductConflict
				}
				return publish()
			})
		})
		if err != nil {
			return HandlerResult{}, err
		}
		if !changed {
			return HandlerResult{State: ReceiptReviewedNoChange, ReasonCode: ReasonReviewedNoChange}, nil
		}
		return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
	}
	if h.job == "memory-retention-check" {
		var result HandlerResult
		err := h.service.engine.WithRetentionSnapshot(command.WorkspaceID, func(snapshot memory.RetentionState) error {
			var err error
			result, err = execute(&snapshot)
			return err
		})
		return result, err
	}
	return execute(nil)
}

type productIndexHandler struct {
	service  *Service
	expected string
}

func (h productIndexHandler) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if err := ValidateExecutionGrant(grant, command); err != nil {
		return HandlerResult{}, err
	}
	if command.JobID != "skills-index-refresh" || command.WorkspaceID != h.service.workspaceID {
		return HandlerResult{}, ErrProductConflict
	}
	source, err := h.service.indexSource()
	if err != nil {
		return HandlerResult{}, err
	}
	identity := productDigest(source)
	if h.expected != "" && h.expected != identity {
		return HandlerResult{}, ErrProductConflict
	}
	_, changed, err := skillsindex.PublishRuntimeIndex(h.service.indexRoot(), source, func(publish func() error) error {
		return publishWithGrant(ctx, grant, command, func() error {
			current, err := h.service.indexSource()
			if err != nil {
				return err
			}
			if productDigest(current) != identity {
				return ErrProductConflict
			}
			return publish()
		})
	})
	if err != nil {
		return HandlerResult{}, err
	}
	if !changed {
		return HandlerResult{State: ReceiptReviewedNoChange, ReasonCode: ReasonReviewedNoChange}, nil
	}
	return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
}
func (s *Service) indexRoot() string {
	return filepath.Join(s.home, "brain", "runtime", "workspaces", s.workspaceID, "skills-index")
}
