package installer

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

// LifecycleClientCapability describes the finite protocol, not native execution.
type LifecycleClientCapability struct {
	SchemaVersion   int      `json:"schema_version"`
	Product         string   `json:"product"`
	Component       string   `json:"component"`
	Version         string   `json:"version"`
	RegistrySHA256  string   `json:"authority_registry_sha256"`
	Profile         string   `json:"profile"`
	Events          []string `json:"events"`
	Enforcement     string   `json:"enforcement"`
	NativeExecution string   `json:"native_execution"`
}

func LifecycleClientCapabilities(component, version, pin string) LifecycleClientCapability {
	return LifecycleClientCapability{1, "maestro", component, version, pin, "mcp_lifecycle", []string{"SessionStart", "UserPromptSubmit", "Stop"}, "unavailable", "not_observed"}
}
func PassiveLifecycleEvent(event string) bool {
	return event == "SessionStart" || event == "UserPromptSubmit" || event == "Stop"
}

// RequireLifecycleClientCapabilities probes an already admitted component. It
// performs no setup, event, provider call or native qualification.
func RequireLifecycleClientCapabilities(ctx context.Context, path, component, version, pin string) error {
	deadline, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	body, err := execCommand(deadline, path, "lifecycle-capabilities")
	if err != nil || len(body) > 64<<10 {
		return errors.New("actual admitted component lacks passive lifecycle protocol")
	}
	var actual LifecycleClientCapability
	if err = releasecontract.DecodeStrictJSON(body, &actual); err != nil {
		return err
	}
	want, err := releasecontract.CanonicalJSON(LifecycleClientCapabilities(component, version, pin))
	if err != nil {
		return err
	}
	got, err := releasecontract.CanonicalJSON(actual)
	if err != nil || !bytes.Equal(got, want) {
		return errors.New("actual passive lifecycle protocol differs from admitted component")
	}
	return nil
}

// PrepareStableLifecycleHook uses exactly the current source/client launch
// checks. It cannot select a binary, grant native authority or initialize state.
func PrepareStableLifecycleHook(ctx context.Context, o ClientOptions, event string) (QualifiedLaunch, error) {
	if !PassiveLifecycleEvent(event) {
		return QualifiedLaunch{}, errors.New("unsupported passive lifecycle event")
	}
	return prepareStableLaunch(ctx, o, event)
}
