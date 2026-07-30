// Package lifecycleprobe reports environment blockers without manufacturing
// native lifecycle evidence. It is development-only acceptance support.
package lifecycleprobe

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/codexadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
)

const ClaudeMinimumVersion = lifecycle.ClaudeMinimumVersion

var versionLinePattern = regexp.MustCompile(`(?m)(?:^|\s)(?:claude|codex(?:-cli)?)?\s*v?([0-9]+\.[0-9]+\.[0-9]+)(?:\s|$)`)

type Result struct {
	SchemaVersion      int                 `json:"schema_version"`
	Runtime            string              `json:"runtime"`
	ExecutableDetected bool                `json:"executable_detected"`
	RuntimeVersion     string              `json:"runtime_version,omitempty"`
	State              string              `json:"state"`
	EvidenceClass      string              `json:"evidence_class"`
	NativeObservation  string              `json:"native_observation"`
	CapabilityState    string              `json:"capability_state"`
	Blocker            string              `json:"blocker"`
	Surfaces           []lifecycle.Surface `json:"surfaces"`
}

func Probe(runtime string, lookPath func(string) (string, error), version func(string) (string, error)) (Result, error) {
	if runtime != "claude" && runtime != "codex" {
		return Result{}, fmt.Errorf("unsupported runtime %q", runtime)
	}
	result := Result{SchemaVersion: 1, Runtime: runtime, State: "blocked", EvidenceClass: "environment_probe", NativeObservation: "not_observed", CapabilityState: "unavailable"}
	if runtime == "claude" {
		result.Surfaces = claudeSurfaces("runtime native observation is not available")
	} else {
		result.Surfaces = codexadapter.Surfaces()
	}
	executable, err := lookPath(runtime)
	if err != nil {
		result.Blocker = runtime + " executable was not found"
		setSurfaceBlocker(result.Surfaces, result.Blocker)
		return result, nil
	}
	result.ExecutableDetected = true
	output, err := version(executable)
	if err != nil {
		result.Blocker = runtime + " version could not be read: " + err.Error()
		return result, nil
	}
	result.RuntimeVersion = parseRuntimeVersion(output)
	if result.RuntimeVersion == "" {
		result.Blocker = runtime + " version output did not contain a parseable semantic version"
		return result, nil
	}
	if runtime == "codex" {
		result.State = "not_observed"
		result.Blocker = "Codex lifecycle hooks are configured, but no fresh native-session observation has been captured"
		return result, nil
	}
	if !lifecycle.MeetsClaudeMinimum(result.RuntimeVersion) {
		result.Blocker = "Claude Code " + result.RuntimeVersion + " is below the required " + ClaudeMinimumVersion + " lifecycle-hook contract version"
		setSurfaceBlocker(result.Surfaces, result.Blocker)
		return result, nil
	}
	result.State = "not_observed"
	result.Blocker = "Claude version satisfies the local minimum, but no fresh native-session observation has been captured"
	setSurfaceBlocker(result.Surfaces, result.Blocker)
	return result, nil
}

func SystemProbe(runtime string) (Result, error) {
	return Probe(runtime, exec.LookPath, func(executable string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		// Keep stderr out of the evidence receipt. Several CLIs emit local
		// warnings there; mixing it into runtime_version makes the probe
		// non-deterministic and can hide the actual version.
		output, err := exec.CommandContext(ctx, executable, "--version").Output()
		if ctx.Err() != nil {
			return "", fmt.Errorf("version command timed out")
		}
		if err != nil {
			return "", fmt.Errorf("version command failed: %w", err)
		}
		return string(output), nil
	})
}

func parseRuntimeVersion(output string) string {
	matches := versionLinePattern.FindStringSubmatch(output)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func claudeSurfaces(defaultBlocker string) []lifecycle.Surface {
	bindings := []struct {
		event   string
		binding string
	}{
		{lifecycle.SessionStart, "SessionStart"},
		{lifecycle.ContextInject, "UserPromptSubmit"},
		{lifecycle.PreActionGuard, "PreToolUse"},
		{lifecycle.PostActionObserve, "PostToolUse"},
		{lifecycle.StopFinalize, "Stop"},
	}
	surfaces := make([]lifecycle.Surface, 0, len(bindings))
	for _, value := range bindings {
		surfaces = append(surfaces, lifecycle.Surface{
			SemanticEvent: value.event, NativeBinding: value.binding,
			Implementation: "configured", EvidenceClass: lifecycle.EvidenceContractTested,
			NativeObservation: "not_observed", CapabilityState: "unavailable",
			Blocker: defaultBlocker,
		})
	}
	return surfaces
}

func setSurfaceBlocker(surfaces []lifecycle.Surface, blocker string) {
	for index := range surfaces {
		surfaces[index].Blocker = blocker
		if surfaces[index].NativeObservation == "not_observed" && strings.Contains(blocker, "below the required") {
			surfaces[index].NativeObservation = "blocked"
		}
	}
}
