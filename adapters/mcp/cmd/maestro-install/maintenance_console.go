package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

func manageClientMaintenance(ctx context.Context, o installer.ClientOptions, prompt func(string) (string, error), confirm func(any) error, output io.Writer) error {
	action, err := prompt("Maintenance: choose status, register-model, qualify, enroll, pause, resume, revoke, wake-status, wake-opt-in, wake-pause or wake-revoke: ")
	if err != nil {
		return err
	}
	if action == "register-model" {
		return manageClientModelRegistration(ctx, o, prompt, confirm, output)
	}
	if action == "wake-status" {
		status, err := setup.InspectClientWake(ctx, o)
		if err == nil {
			return json.NewEncoder(output).Encode(status)
		}
		return err
	}
	if strings.HasPrefix(action, "wake-") {
		selected := strings.TrimPrefix(action, "wake-")
		review, err := setup.ReviewClientWake(ctx, o, selected)
		if err != nil {
			return err
		}
		if err := confirm(review); err != nil {
			return err
		}
		status, err := setup.ApplyClientWake(ctx, o, selected, review.ReviewDigest, true)
		if status.State != "" {
			if e := json.NewEncoder(output).Encode(status); e != nil {
				return e
			}
		}
		return err
	}
	status, err := setup.InspectClientMaintenance(ctx, o)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(output).Encode(status); err != nil {
		return err
	}
	if action == "status" {
		return nil
	}
	switch action {
	case "qualify", "enroll", "pause", "resume", "revoke":
	default:
		return errors.New("choose a listed maintenance action")
	}
	id, err := prompt("Select one job_id from the actual status above: ")
	if err != nil {
		return err
	}
	var selected *setup.ClientMaintenanceJob
	for i := range status.Jobs {
		if status.Jobs[i].JobID == id {
			if selected != nil {
				return errors.New("ambiguous job selection")
			}
			selected = &status.Jobs[i]
		}
	}
	if selected == nil {
		return errors.New("job is not present in the current maintenance status")
	}
	request := product.MaintenanceMutation{Operation: "maintenance." + action, JobID: id, ExpectedRevision: selected.Revision}
	if action == "enroll" {
		request.Timezone, err = prompt("Choose the job timezone (IANA name; Enter for UTC): ")
		if err != nil {
			return err
		}
		if request.Timezone == "" {
			request.Timezone = "UTC"
		}
	}
	review, err := setup.ReviewClientMaintenanceTransition(ctx, o, request)
	if err != nil {
		return err
	}
	if err := confirm(review); err != nil {
		return err
	}
	result, err := setup.ApplyClientMaintenanceTransition(ctx, o, request, review.ReviewDigest, true)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(result)
}
