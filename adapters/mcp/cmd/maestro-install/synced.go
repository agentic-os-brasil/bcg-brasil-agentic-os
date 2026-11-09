package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
)

type lifecycleFlags struct{ ManagedRoot, UserHome, Workspace, ManifestSHA256, ClientInputsSHA256, PlanID, CarrierID, PackageSHA256, NativeContractSHA256, ClientCodeRoot, ClientDesktopPath, ClientCodeExecutable, ClientScope string }
type lifecycleRequest struct {
	ManagedRoot string                      `json:"managed_root"`
	UserHome    string                      `json:"user_home"`
	SourceRoot  string                      `json:"source_root,omitempty"`
	Source      releasecontract.SourceTuple `json:"source,omitempty"`
}

func runLifecycle(ctx context.Context, command, self, registryPath string, flags lifecycleFlags, digest string, confirmed bool) (bool, error) {
	switch command {
	case "launch", "source-review", "source-check", "stage", "update-review", "update", "reconcile", "rollback-review", "rollback":
	default:
		return false, nil
	}
	operation := installer.SeededOperationOptions{Self: self, Version: Version, AuthorityRegistrySHA256: AuthorityRegistrySHA256, UserHome: flags.UserHome, ManagedRoot: flags.ManagedRoot, DataRoot: filepath.Join(flags.UserHome, ".maestro-install")}
	if command == "launch" {
		launch, err := installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: operation, ExpectedBinding: installtx.ReleaseBinding{ManifestSHA256: flags.ManifestSHA256, ClientInputsSHA256: flags.ClientInputsSHA256}, Workspace: flags.Workspace})
		if err != nil {
			return true, err
		}
		child := exec.CommandContext(ctx, launch.Executable, launch.Args...)
		child.Stdin = os.Stdin
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		return true, child.Run()
	}
	body, err := io.ReadAll(io.LimitReader(os.Stdin, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return true, errors.New("bounded local lifecycle request required")
	}
	var request lifecycleRequest
	if err := releasecontract.DecodeStrictJSON(body, &request); err != nil {
		return true, err
	}
	if flags.UserHome != "" && flags.UserHome != request.UserHome || flags.ManagedRoot != "" && flags.ManagedRoot != request.ManagedRoot {
		return true, errors.New("fixed lifecycle binding differs from local request")
	}
	operation.UserHome = request.UserHome
	operation.ManagedRoot = request.ManagedRoot
	operation.DataRoot = filepath.Join(request.UserHome, ".maestro-install")
	encode := func(value any, err error) (bool, error) {
		if value != nil {
			if e := json.NewEncoder(os.Stdout).Encode(value); e != nil {
				return true, e
			}
		}
		return true, err
	}
	sourceOptions := installer.SeededSourceOptions{SeededOperationOptions: operation, SourceRoot: request.SourceRoot, ExpectedSource: request.Source, ReviewDigest: digest}
	if command == "source-review" || command == "source-check" || command == "stage" {
		if sourceOptions.SourceRoot == "" {
			state, e := installtx.ReadSourceState(operation.DataRoot, operation.ManagedRoot)
			if e != nil {
				return true, e
			}
			if state.Source == nil {
				return true, errors.New("first source review requires an explicit local folder and exact source tuple")
			}
			sourceOptions.SourceRoot = state.Source.Locator
			sourceOptions.ExpectedSource = state.Source.Tuple
		}
		if command == "source-review" {
			review, e := installer.ReviewSourceSeeded(ctx, sourceOptions)
			return encode(review, e)
		}
		if !confirmed {
			return true, errors.New("source effects require exact review digest and explicit local confirmation")
		}
		checked, e := installer.CheckSourceSeeded(ctx, sourceOptions)
		out := map[string]any{"state": "source_observation_unavailable"}
		if checked.Current.Source != nil {
			out["state"] = "source_observed_candidate_unavailable"
			out["sequence"] = checked.Current.Source.Sequence
			out["generation_sha256"] = checked.Current.Source.GenerationSHA256
		}
		if errors.Is(e, updateservice.ErrCurrent) {
			out["state"] = "current_signed_release"
			return encode(out, nil)
		}
		if e != nil {
			return encode(out, e)
		}
		if checked.Plan.ID != "" {
			pending, e := updateservice.StagePending(operation.DataRoot, checked.Current, checked, checked.Current.Source.ObservedAt)
			if e != nil {
				return encode(out, e)
			}
			out["state"] = "candidate_staged_activation_unconfirmed"
			out["plan_id"] = pending.Plan.ID
			out["plan"] = pending.Plan
		}
		return encode(out, nil)
	}
	if command == "rollback-review" {
		review, e := installer.ReviewRollbackSeeded(ctx, operation)
		return encode(review, e)
	}
	if command == "update-review" {
		review, e := installer.ReviewPendingSeeded(ctx, installer.SeededUpdateOptions{SeededOperationOptions: operation, PlanID: flags.PlanID})
		return encode(review, e)
	}
	if !confirmed {
		return true, errors.New("installation mutation requires the exact reviewed plan or rollback digest and explicit confirmation")
	}
	switch command {
	case "update":
		state, e := installer.UpdateSeeded(ctx, installer.SeededUpdateOptions{SeededOperationOptions: operation, PlanID: flags.PlanID})
		return encode(map[string]any{"release": state.Release, "schema_version": state.SchemaVersion, "state": "activation_readback"}, e)
	case "reconcile":
		state, applied, e := installer.ReconcileSeeded(ctx, installer.SeededUpdateOptions{SeededOperationOptions: operation, PlanID: flags.PlanID})
		return encode(map[string]any{"release": state.Release, "applied": applied, "state": "reconciliation_readback"}, e)
	case "rollback":
		state, e := installer.RollbackSeeded(ctx, installer.SeededRollbackOptions{SeededOperationOptions: operation, ReviewDigest: digest})
		return encode(map[string]any{"release": state.Release, "state": "rollback_readback"}, e)
	}
	return true, errors.New("unsupported lifecycle operation")
}
