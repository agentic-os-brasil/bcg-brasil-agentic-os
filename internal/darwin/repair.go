package darwin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const managedStateRelativePath = "managed-state/current.json"

type managedState struct {
	SchemaVersion int    `json:"schema_version"`
	State         string `json:"state"`
	Generation    int    `json:"generation"`
	WindowID      string `json:"window_id,omitempty"`
	ProposalID    string `json:"proposal_id,omitempty"`
}

func (state managedState) validate() error {
	if state.SchemaVersion != SchemaVersion || (state.State != "stale" && state.State != "healthy") || state.Generation < 0 || state.Generation > 1_000_000 {
		return errors.New("Darwin managed state is invalid")
	}
	if (state.WindowID == "") != (state.ProposalID == "") {
		return errors.New("Darwin managed state repair binding is incomplete")
	}
	if state.WindowID != "" && (!idPattern.MatchString(state.WindowID) || !idPattern.MatchString(state.ProposalID)) {
		return errors.New("Darwin managed state repair binding is invalid")
	}
	return nil
}

func encodeManagedState(state managedState) ([]byte, error) {
	if err := state.validate(); err != nil {
		return nil, err
	}
	body, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func readManagedState(root string) (managedState, error) {
	var state managedState
	if strings.TrimSpace(root) == "" {
		return state, errors.New("Darwin managed state root is required")
	}
	if err := readStrictJSON(filepath.Join(root, managedStateRelativePath), &state); err != nil {
		return managedState{}, err
	}
	return state, state.validate()
}

func managedStateRepairCall() ToolCall {
	return ToolCall{Tool: "filesystem", Operation: "edit", Resource: "bcgos://health/maestro-system/managed-state/current.json"}
}

// ManagedStateRepairInvoker applies only the closed, reversible repair that
// refreshes Darwin-owned derived state. It validates both sides of the atomic
// replacement and restores the prior bytes if post-validation fails.
type ManagedStateRepairInvoker struct {
	Root         string
	PostValidate func(managedState) error
}

func (invoker ManagedStateRepairInvoker) Invoke(ctx context.Context, call ToolCall, artifact Artifact) (ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolResult{}, err
	}
	if call != managedStateRepairCall() || artifact.SchemaVersion != SchemaVersion || artifact.AgentID != AgentID || artifact.Finding != ObservationStateStale || artifact.Action != ActionRefreshDerivedState || !idPattern.MatchString(artifact.WindowID) || !idPattern.MatchString(artifact.ProposalID) {
		return ToolResult{}, errors.New("Darwin managed state repair request is outside the allowlist")
	}
	var result ToolResult
	err := withManagedStateLock(ctx, invoker.Root, func() error {
		current, err := readManagedState(invoker.Root)
		if err != nil {
			return err
		}
		if current.State == "healthy" && current.WindowID == artifact.WindowID && current.ProposalID == artifact.ProposalID {
			result = ToolResult{Outcome: OutcomeSucceeded}
			return nil
		}
		if current.State != "stale" {
			return errors.New("Darwin managed state repair requires validated stale state")
		}
		previous, err := encodeManagedState(current)
		if err != nil {
			return err
		}
		desired := managedState{SchemaVersion: SchemaVersion, State: "healthy", Generation: current.Generation + 1, WindowID: artifact.WindowID, ProposalID: artifact.ProposalID}
		next, err := encodeManagedState(desired)
		if err != nil {
			return err
		}
		if err := replaceManagedState(invoker.Root, next); err != nil {
			return err
		}
		validated, validateErr := readManagedState(invoker.Root)
		if validateErr == nil && validated != desired {
			validateErr = errors.New("Darwin managed state post-validation mismatch")
		}
		if validateErr == nil && invoker.PostValidate != nil {
			validateErr = invoker.PostValidate(validated)
		}
		if validateErr != nil {
			if rollbackErr := replaceManagedState(invoker.Root, previous); rollbackErr != nil {
				return errors.Join(validateErr, rollbackErr)
			}
			return validateErr
		}
		result = ToolResult{Outcome: OutcomeSucceeded}
		return nil
	})
	return result, err
}

func withManagedStateLock(ctx context.Context, root string, operation func() error) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("Darwin managed state root is required")
	}
	if err := ensurePrivateDirectory(root); err != nil {
		return err
	}
	directory := filepath.Join(root, "managed-state")
	if err := ensurePrivateDirectory(directory); err != nil {
		return err
	}
	lockPath := filepath.Join(directory, "repair.lock")
	deadline := time.Now().Add(3 * time.Second)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, statErr := os.Lstat(lockPath)
		if statErr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
			return errors.New("Darwin managed state lock is not a regular file")
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		file, openErr := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
		if openErr == nil {
			lockErr := tryLockManagedStateFile(file)
			if lockErr == nil {
				err := operation()
				unlockErr := unlockManagedStateFile(file)
				closeErr := file.Close()
				if err != nil {
					return err
				}
				if unlockErr != nil {
					return unlockErr
				}
				return closeErr
			}
			_ = file.Close()
			if !errors.Is(lockErr, errManagedStateLockBusy) {
				return lockErr
			}
		} else if !errors.Is(openErr, os.ErrNotExist) {
			return openErr
		}
		if time.Now().After(deadline) {
			return errManagedStateLockBusy
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func replaceManagedState(root string, body []byte) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("Darwin managed state root is required")
	}
	if err := ensurePrivateDirectory(root); err != nil {
		return err
	}
	if err := rejectSymlinkPath(root, managedStateRelativePath); err != nil {
		return err
	}
	directory := filepath.Join(root, "managed-state")
	if err := ensurePrivateDirectory(directory); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".current-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceManagedStateFile(temporaryPath, filepath.Join(root, managedStateRelativePath)); err != nil {
		return err
	}
	actual, err := os.ReadFile(filepath.Join(root, managedStateRelativePath))
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, body) {
		return errors.New("Darwin managed state atomic replacement was not durable")
	}
	return nil
}

// OperationalInvoker keeps diagnostic artifacts and applied repairs separate.
// A diagnostic write returns no_action and therefore cannot become repair
// success at the receipt boundary.
type OperationalInvoker struct {
	Diagnostics FilesystemInvoker
	Repairs     ManagedStateRepairInvoker
	Guard       ToolGuard
}

func (invoker OperationalInvoker) Invoke(ctx context.Context, call ToolCall, artifact Artifact) (ToolResult, error) {
	if artifact.Action == ActionRefreshDerivedState && artifact.Finding == ObservationStateStale {
		expected := ToolCall{Tool: "filesystem", Operation: "write", Resource: "bcgos://health/maestro-system/derived/refresh_derived_state-" + artifact.ProposalID + ".json"}
		if call != expected {
			return ToolResult{}, errors.New("Darwin operational repair call is not bound to its deterministic plan")
		}
		if invoker.Guard == nil {
			return ToolResult{}, errors.New("Darwin operational repair requires a tool guard")
		}
		if err := invoker.Guard.Authorize(managedStateRepairCall()); err != nil {
			return ToolResult{}, err
		}
		return invoker.Repairs.Invoke(ctx, managedStateRepairCall(), artifact)
	}
	return invoker.Diagnostics.Invoke(ctx, call, artifact)
}

func managedStateSurface(root string) ProductSurface {
	if strings.TrimSpace(root) == "" {
		return ProductSurface{State: "healthy"}
	}
	state, err := readManagedState(root)
	if errors.Is(err, os.ErrNotExist) {
		return ProductSurface{State: "healthy"}
	}
	if err != nil {
		return ProductSurface{State: "failed", Count: 1}
	}
	if state.State == "stale" {
		return ProductSurface{State: "stale", Count: 1}
	}
	return ProductSurface{State: "healthy"}
}
