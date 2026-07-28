// Package lifecycleprobe reports environment blockers without manufacturing
// native lifecycle evidence. It is development-only acceptance support.
package lifecycleprobe

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const ClaudeMinimumVersion = "2.1.177"

type Result struct {
	SchemaVersion      int    `json:"schema_version"`
	Runtime            string `json:"runtime"`
	ExecutableDetected bool   `json:"executable_detected"`
	RuntimeVersion     string `json:"runtime_version,omitempty"`
	State              string `json:"state"`
	EvidenceClass      string `json:"evidence_class"`
	NativeObservation  string `json:"native_observation"`
	CapabilityState    string `json:"capability_state"`
	Blocker            string `json:"blocker"`
}

func Probe(runtime string, lookPath func(string) (string, error), version func(string) (string, error)) (Result, error) {
	if runtime != "claude" && runtime != "codex" {
		return Result{}, fmt.Errorf("unsupported runtime %q", runtime)
	}
	result := Result{SchemaVersion: 1, Runtime: runtime, State: "blocked", EvidenceClass: "environment_probe", NativeObservation: "not_observed", CapabilityState: "unavailable"}
	executable, err := lookPath(runtime)
	if err != nil {
		result.Blocker = runtime + " executable was not found"
		return result, nil
	}
	result.ExecutableDetected = true
	output, err := version(executable)
	if err != nil {
		result.Blocker = runtime + " version could not be read: " + err.Error()
		return result, nil
	}
	result.RuntimeVersion = strings.TrimSpace(output)
	if runtime == "codex" {
		result.Blocker = "Codex has only a SessionStart configuration seam; context injection, guard, post-action and stop bindings are not implemented"
		return result, nil
	}
	if belowClaudeMinimum(result.RuntimeVersion) {
		result.Blocker = "Claude Code " + result.RuntimeVersion + " is below the required " + ClaudeMinimumVersion + " lifecycle-hook contract version"
		return result, nil
	}
	result.State = "not_observed"
	result.Blocker = "Claude version satisfies the local minimum, but no fresh native-session observation has been captured"
	return result, nil
}

func SystemProbe(runtime string) (Result, error) {
	return Probe(runtime, exec.LookPath, func(executable string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, executable, "--version").CombinedOutput()
		if ctx.Err() != nil {
			return "", fmt.Errorf("version command timed out")
		}
		if err != nil {
			return "", fmt.Errorf("version command failed: %w", err)
		}
		return string(output), nil
	})
}

var versionPattern = regexp.MustCompile(`([0-9]+)\.([0-9]+)\.([0-9]+)`)

func belowClaudeMinimum(value string) bool {
	match := versionPattern.FindStringSubmatch(value)
	if len(match) != 4 {
		return true
	}
	actual := [3]int{}
	minimum := [3]int{2, 1, 177}
	for index := range actual {
		parsed, err := strconv.Atoi(match[index+1])
		if err != nil {
			return true
		}
		actual[index] = parsed
		if actual[index] != minimum[index] {
			return actual[index] < minimum[index]
		}
	}
	return false
}
