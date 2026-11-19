package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

// This is the existing backend's metadata-only CLI projection. The opaque
// qualification object is presence metadata; it is never imported as proof.
type ClientMaintenanceJob struct {
	Scope              string                      `json:"scope,omitempty"`
	ScopeID            string                      `json:"scope_id,omitempty"`
	Diagnosis          *maintenance.DarwinSource   `json:"diagnosis,omitempty"`
	JobID              string                      `json:"job_id"`
	Revision           uint64                      `json:"revision"`
	State              string                      `json:"state"`
	Schedule           maintenance.ProductSchedule `json:"schedule"`
	Boundary           time.Time                   `json:"no_backfill_before,omitempty"`
	Writes             []string                    `json:"writes"`
	QualificationState string                      `json:"qualification_state"`
	Qualification      json.RawMessage             `json:"qualification,omitempty"`
	Reason             string                      `json:"reason,omitempty"`
}

type ClientMaintenanceStatus struct {
	WorkspaceID string                 `json:"workspace_id"`
	Jobs        []ClientMaintenanceJob `json:"jobs"`
}

// MaintenanceRefreshCandidates never enrolls a new job or resumes a paused
// one. Executable/configuration-bound proof is renewed by the original job
// producer, with real inputs and the original review/confirmation owner.
func MaintenanceRefreshCandidates(status ClientMaintenanceStatus) []ClientMaintenanceJob {
	var jobs []ClientMaintenanceJob
	for _, job := range status.Jobs {
		if len(job.Qualification) > 0 && !bytes.Equal(job.Qualification, []byte("null")) && job.QualificationState == "stale" {
			jobs = append(jobs, job)
		}
	}
	return jobs
}

func currentMaintenanceCommand(ctx context.Context, o installer.ClientOptions, command string, input any, digest string) ([]byte, error) {
	s, err := installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now())
	if err != nil {
		return nil, err
	}
	name := "maestro-mcp"
	if s.TargetOS == "windows" {
		name += ".exe"
	}
	server := filepath.Join(o.ManagedRoot, "bin", name)
	launch, err := installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: o.SeededOperationOptions, ExpectedBinding: s.Binding, Workspace: o.Request.Workspace})
	if err != nil || launch.Executable != server {
		return nil, errors.New("independently admitted maintenance launch required")
	}
	before, err := installer.ClientFileDigest(server)
	if err != nil || before != s.Server.SHA256 {
		return nil, errors.New("current maintenance owner changed")
	}
	args, err := maintenanceLaunchArguments(launch.Args, command, digest)
	if err != nil {
		return nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, server, args...)
	cmd.Dir = o.Request.Workspace
	cmd.WaitDelay = 2 * time.Second
	if input != nil {
		body, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		cmd.Stdin = bytes.NewReader(body)
	}
	var output boundedClientOutput
	cmd.Stdout = &output
	cmd.Stderr = &boundedClientOutput{}
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	after, err := installer.ClientFileDigest(server)
	if err != nil || after != before {
		return nil, errors.New("current maintenance owner changed during operation")
	}
	if _, err := installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, s.Binding, time.Now()); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// Keep the original qualified launch's complete release/experimental tail.
func maintenanceLaunchArguments(qualified []string, command, digest string) ([]string, error) {
	if len(qualified) == 0 || qualified[0] != "serve" || command != "maintenance-status" && command != "maintenance-review" && command != "maintenance-apply" {
		return nil, errors.New("closed maintenance command requires a qualified launch")
	}
	args := append([]string{command}, qualified[1:]...)
	if digest != "" {
		args = append(args, "--review-digest", digest, "--confirm")
	}
	return args, nil
}

// ReviewClientMaintenanceTransition uses the same original domain review for
// enrollment/pause/resume/revoke. A wake never supplies any of these grants.
func ReviewClientMaintenanceTransition(ctx context.Context, o installer.ClientOptions, request product.MaintenanceMutation) (product.LocalMaintenanceReview, error) {
	body, err := currentMaintenanceCommand(ctx, o, "maintenance-review", request, "")
	if err != nil {
		return product.LocalMaintenanceReview{}, err
	}
	var review product.LocalMaintenanceReview
	err = releasecontract.DecodeStrictJSON(body, &review)
	return review, err
}

func ApplyClientMaintenanceTransition(ctx context.Context, o installer.ClientOptions, request product.MaintenanceMutation, digest string, confirmed bool) (ClientMaintenanceJob, error) {
	if !confirmed {
		return ClientMaintenanceJob{}, errors.New("explicit per-job review confirmation required")
	}
	body, err := currentMaintenanceCommand(ctx, o, "maintenance-apply", request, digest)
	if err != nil {
		return ClientMaintenanceJob{}, err
	}
	var job ClientMaintenanceJob
	err = releasecontract.DecodeStrictJSON(body, &job)
	return job, err
}

func InspectClientMaintenance(ctx context.Context, o installer.ClientOptions) (ClientMaintenanceStatus, error) {
	body, err := currentMaintenanceCommand(ctx, o, "maintenance-status", nil, "")
	if err != nil {
		return ClientMaintenanceStatus{}, err
	}
	var status ClientMaintenanceStatus
	err = releasecontract.DecodeStrictJSON(body, &status)
	return status, err
}

func ReviewClientMaintenance(ctx context.Context, o installer.ClientOptions, job ClientMaintenanceJob) (product.LocalMaintenanceReview, error) {
	request := product.MaintenanceMutation{Operation: "maintenance.qualify", JobID: job.JobID, ExpectedRevision: job.Revision}
	body, err := currentMaintenanceCommand(ctx, o, "maintenance-review", request, "")
	if err != nil {
		return product.LocalMaintenanceReview{}, err
	}
	var review product.LocalMaintenanceReview
	err = releasecontract.DecodeStrictJSON(body, &review)
	return review, err
}

func ApplyClientMaintenance(ctx context.Context, o installer.ClientOptions, job ClientMaintenanceJob, digest string, confirmed bool) (ClientMaintenanceJob, error) {
	if !confirmed {
		return ClientMaintenanceJob{}, errors.New("explicit genuine per-job qualification review required")
	}
	request := product.MaintenanceMutation{Operation: "maintenance.qualify", JobID: job.JobID, ExpectedRevision: job.Revision}
	body, err := currentMaintenanceCommand(ctx, o, "maintenance-apply", request, digest)
	if err != nil {
		return ClientMaintenanceJob{}, err
	}
	var status ClientMaintenanceJob
	err = releasecontract.DecodeStrictJSON(body, &status)
	return status, err
}
