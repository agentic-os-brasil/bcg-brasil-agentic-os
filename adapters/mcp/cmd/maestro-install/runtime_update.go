package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

// The existing foreground manager owns the exact local human review. No MCP
// tool can confirm or invoke placement through this composition.
func reconcileRuntime(ctx context.Context, o installer.ClientOptions, confirm func(any) error, output io.Writer) error {
	review, err := setup.ReviewRuntimeUpdate(ctx, o)
	if err != nil {
		original := err
		recovery, recoveryErr := setup.ReviewRuntimeRecoverySource(ctx, o)
		if recoveryErr != nil {
			return fmt.Errorf("source-updated/runtime-pending; owner data and prior converter retained; retry manage reconcile after inspection: runtime review: %v; snapshot recovery: %w", original, recoveryErr)
		}
		if err = confirm(recovery); err != nil {
			return fmt.Errorf("source-updated/runtime-pending; snapshot staging declined; retry manage reconcile: %w", err)
		}
		staged, stageErr := setup.ApplyRuntimeRecoverySource(ctx, o, recovery.Digest, true)
		if e := json.NewEncoder(output).Encode(staged); e != nil {
			return e
		}
		if stageErr != nil {
			return stageErr
		}
		review, err = setup.ReviewRuntimeUpdate(ctx, o)
		if err != nil {
			return fmt.Errorf("source-updated/runtime-pending; reviewed snapshot retained; retry manage reconcile: %w", err)
		}
	}
	if len(review.Effects) == 0 {
		return json.NewEncoder(output).Encode(review.Availability)
	}
	if err = confirm(review); err != nil {
		return fmt.Errorf("source-updated/runtime-pending; converter review declined or interrupted; retry manage reconcile: %w", err)
	}
	result, err := setup.ApplyRuntimeUpdate(ctx, o, review.Digest, true)
	if e := json.NewEncoder(output).Encode(result); e != nil {
		return e
	}
	return err
}
