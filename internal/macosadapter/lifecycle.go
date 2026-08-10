package macosadapter

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"os/user"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const maxRunnerOutput = 8192

const (
	nativeKickstartAttempts = 3
	nativeKickstartBackoff  = 250 * time.Millisecond
)

// launchctlPath is deliberately absolute. A Finder-launched macOS app and a
// restricted/headless shell may have different PATH values, but the native
// lifecycle must not become environment-dependent.
const launchctlPath = "/bin/launchctl"

var uidPattern = regexp.MustCompile(`^[0-9]+$`)

type CommandResult struct {
	ExitCode int
	Output   string
}

type CommandRunner interface {
	Run(context.Context, string, []string) (CommandResult, error)
}

type ExecCommandRunner struct{}

func (ExecCommandRunner) Run(ctx context.Context, name string, args []string) (CommandResult, error) {
	if name != launchctlPath {
		return CommandResult{}, errors.New("only launchctl is permitted")
	}
	command := exec.CommandContext(ctx, name, args...)
	var output boundedBuffer
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	result := CommandResult{Output: redactRunnerOutput(output.String())}
	if err == nil {
		return result, nil
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitError.ExitCode()
	}
	return result, err
}

type boundedBuffer struct{ bytes.Buffer }

func (buffer *boundedBuffer) Write(value []byte) (int, error) {
	remaining := maxRunnerOutput - buffer.Len()
	if remaining <= 0 {
		return len(value), nil
	}
	if len(value) > remaining {
		value = value[:remaining]
	}
	return buffer.Buffer.Write(value)
}

type LaunchAgentStatus struct {
	State           string `json:"state"`
	Path            string `json:"path"`
	Label           string `json:"label"`
	FilePresent     bool   `json:"file_present"`
	Loaded          bool   `json:"loaded"`
	Enabled         bool   `json:"enabled"`
	NativeQualified bool   `json:"native_qualified"`
	Disabled        bool   `json:"disabled"`
	Diagnostic      string `json:"diagnostic,omitempty"`
}

type Lifecycle struct {
	Runner      CommandRunner
	UID         string
	CurrentHome string
	Timeout     time.Duration
	Native      bool
}

func CurrentUID() (string, error) {
	current, err := user.Current()
	if err != nil {
		return "", err
	}
	if !uidPattern.MatchString(current.Uid) {
		return "", errors.New("current user UID is not numeric")
	}
	return current.Uid, nil
}

func NewCurrentLifecycle(home string, runner CommandRunner) (Lifecycle, error) {
	uid, err := CurrentUID()
	if err != nil {
		return Lifecycle{}, err
	}
	currentHome, err := osUserHome()
	if err != nil {
		return Lifecycle{}, err
	}
	if home == "" {
		home = currentHome
	}
	return Lifecycle{Runner: runner, UID: uid, CurrentHome: currentHome, Timeout: 15 * time.Second, Native: runtime.GOOS == "darwin" && samePath(home, currentHome)}, nil
}

func (lifecycle Lifecycle) Install(ctx context.Context, home string, spec Spec, optIn bool) (LaunchAgentStatus, error) {
	if !optIn {
		return LaunchAgentStatus{}, ErrOptInRequired
	}
	if lifecycle.Timeout <= 0 || lifecycle.Timeout > time.Minute {
		lifecycle.Timeout = 15 * time.Second
	}
	if lifecycle.Native {
		if err := validateNativeLabel(spec.Label); err != nil {
			return LaunchAgentStatus{State: "native_label_rejected", Label: spec.Label}, err
		}
	}
	// Reconcile an already loaded service before replacing its plist. This
	// makes reinstall idempotent and avoids bootstrapping a second instance.
	if lifecycle.Native && lifecycle.Runner != nil {
		existing, statusErr := lifecycle.Status(ctx, home, spec.Label)
		if statusErr != nil {
			return existing, statusErr
		}
		if existing.Loaded {
			if existing.State == "loaded_identity_mismatch" {
				return existing, errors.New("managed LaunchAgent label has a mismatched native identity")
			}
			if err := lifecycle.removeNative(ctx, spec.Label); err != nil {
				return existing, err
			}
		}
	}
	filesystemStatus, err := Install(home, spec, true)
	if err != nil {
		return LaunchAgentStatus{State: "partial_file_error", Path: filesystemStatus.Path, Label: spec.Label}, err
	}
	if !lifecycle.Native {
		return lifecycle.fileStatus(home, spec.Label, "file_present_native_qualification_pending")
	}
	if lifecycle.Runner == nil {
		_ = Uninstall(home, spec.Label)
		return LaunchAgentStatus{State: "partial_native_runner_unavailable", Path: filesystemStatus.Path, Label: spec.Label}, errors.New("launchctl runner is unavailable")
	}
	// launchd retains a disabled label even after a prior service is booted out.
	// Re-enable the allow-listed label before bootstrap; otherwise launchctl
	// rejects a valid replacement plist with exit status 5.
	if err := lifecycle.run(ctx, []string{"enable", "gui/" + lifecycle.UID + "/" + spec.Label}); err != nil {
		_ = Uninstall(home, spec.Label)
		return LaunchAgentStatus{State: "partial_enable_failed", Path: filesystemStatus.Path, Label: spec.Label, Diagnostic: "launchctl enable failed"}, err
	}
	if err := lifecycle.run(ctx, []string{"bootstrap", "gui/" + lifecycle.UID, filesystemStatus.Path}); err != nil {
		_ = Uninstall(home, spec.Label)
		return LaunchAgentStatus{State: "partial_bootstrap_failed", Path: filesystemStatus.Path, Label: spec.Label, Diagnostic: "launchctl bootstrap failed"}, err
	}
	if err := lifecycle.kickstartWithRecovery(ctx, home, spec.Label); err != nil {
		_ = lifecycle.run(ctx, []string{"bootout", "gui/" + lifecycle.UID + "/" + spec.Label})
		_ = Uninstall(home, spec.Label)
		return LaunchAgentStatus{State: "partial_kickstart_failed", Path: filesystemStatus.Path, Label: spec.Label, Diagnostic: "launchctl kickstart failed"}, err
	}
	return lifecycle.Status(ctx, home, spec.Label)
}

// kickstartWithRecovery tolerates the short interval in which launchd has
// accepted a plist but has not finished publishing the service. On macOS,
// launchctl can return signal: killed during that handoff even though the
// service is already loaded, or become ready on the next kickstart. Confirm
// the native state before rolling back and retry only the idempotent kickstart
// operation with a bounded backoff.
func (lifecycle Lifecycle) kickstartWithRecovery(ctx context.Context, home, label string) error {
	var lastErr error
	for attempt := 0; attempt < nativeKickstartAttempts; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(nativeKickstartBackoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		lastErr = lifecycle.run(ctx, []string{"kickstart", "-k", "gui/" + lifecycle.UID + "/" + label})
		if lastErr == nil {
			return nil
		}
		status, statusErr := lifecycle.Status(ctx, home, label)
		if statusErr == nil && status.Loaded && status.Enabled {
			return nil
		}
	}
	return lastErr
}

func (lifecycle Lifecycle) Status(ctx context.Context, home, label string) (LaunchAgentStatus, error) {
	fileStatus, err := ReadStatus(home, label)
	if err != nil {
		return LaunchAgentStatus{}, err
	}
	status := LaunchAgentStatus{State: "file_present", Path: fileStatus.Path, Label: label, FilePresent: fileStatus.State != "not_installed", Disabled: fileStatus.Disabled}
	if !lifecycle.Native || lifecycle.Runner == nil {
		if !status.FilePresent {
			status.State = "not_present"
			return status, nil
		}
		status.State = "file_present_native_qualification_pending"
		return status, nil
	}
	printResult, printErr := lifecycle.runResult(ctx, []string{"print", "gui/" + lifecycle.UID + "/" + label})
	status.Loaded = printErr == nil && printResult.ExitCode == 0
	disabledResult, disabledErr := lifecycle.runResult(ctx, []string{"print-disabled", "gui/" + lifecycle.UID})
	status.Enabled = status.Loaded && !status.Disabled
	if disabledErr == nil {
		status.Enabled = status.Loaded && !disabledLabel(disabledResult.Output, label)
	}
	if !status.Loaded {
		if !status.FilePresent {
			status.State = "not_present"
		} else {
			status.State = "file_present_not_loaded"
		}
		return status, nil
	}
	if !status.FilePresent {
		status.State = "orphan_loaded"
		status.Diagnostic = "loaded managed label has no managed plist"
		return status, nil
	}
	expectedProgram, identityErr := launchAgentProgram(home, label)
	if identityErr != nil || strings.TrimSpace(printResult.Output) == "" {
		status.State = "loaded_identity_mismatch"
		status.Diagnostic = "loaded LaunchAgent identity could not be bound"
		return status, nil
	}
	nativeProgram, nativePath, parseErr := parseLaunchctlIdentity(printResult.Output, label)
	if parseErr != nil || nativeProgram != expectedProgram || !samePath(nativePath, status.Path) {
		status.State = "loaded_identity_mismatch"
		status.Diagnostic = "loaded LaunchAgent identity does not match managed plist"
		return status, nil
	}
	status.NativeQualified = true
	if status.Loaded && status.Enabled {
		status.State = "active_loaded_enabled"
	} else if status.Loaded {
		status.State = "loaded_disabled"
	} else {
		status.State = "file_present_not_loaded"
	}
	return status, nil
}

func (lifecycle Lifecycle) Pause(ctx context.Context, home, label string) (LaunchAgentStatus, error) {
	status, err := lifecycle.Status(ctx, home, label)
	if err != nil {
		return status, err
	}
	if status.State == "orphan_loaded" {
		if err := lifecycle.removeNative(ctx, label); err != nil {
			return status, err
		}
		return lifecycle.Status(ctx, home, label)
	}
	if status.State == "loaded_identity_mismatch" {
		return status, errors.New("refusing to mutate a mismatched loaded LaunchAgent")
	}
	if !status.FilePresent {
		return status, nil
	}
	if lifecycle.Native {
		if err := validateNativeLabel(label); err != nil {
			return status, err
		}
	}
	fileStatus := status
	if fileStatus.Disabled && (!lifecycle.Native || !status.Loaded) {
		return status, nil
	}
	if lifecycle.Native && status.Loaded {
		if err := lifecycle.removeNative(ctx, label); err != nil {
			return status, err
		}
	}
	if !fileStatus.Disabled {
		if _, err := setDisabled(home, label, true); err != nil {
			return status, err
		}
	}
	return lifecycle.Status(ctx, home, label)
}

func (lifecycle Lifecycle) Resume(ctx context.Context, home, label string) (LaunchAgentStatus, error) {
	status, err := lifecycle.Status(ctx, home, label)
	if err != nil {
		return status, err
	}
	if status.State == "orphan_loaded" || status.State == "loaded_identity_mismatch" {
		return status, errors.New("cannot resume an unbound LaunchAgent identity")
	}
	if !status.FilePresent {
		return status, nil
	}
	if lifecycle.Native {
		if err := validateNativeLabel(label); err != nil {
			return status, err
		}
	}
	if !status.Disabled && (!lifecycle.Native || (status.Loaded && status.Enabled)) {
		return status, nil
	}
	if _, err := setDisabled(home, label, false); err != nil {
		return status, err
	}
	if !lifecycle.Native {
		return lifecycle.Status(ctx, home, label)
	}
	if err := lifecycle.run(ctx, []string{"enable", "gui/" + lifecycle.UID + "/" + label}); err != nil {
		return status, err
	}
	path := status.Path
	if !status.Loaded {
		if err := lifecycle.run(ctx, []string{"bootstrap", "gui/" + lifecycle.UID, path}); err != nil {
			return status, err
		}
	}
	if err := lifecycle.run(ctx, []string{"kickstart", "-k", "gui/" + lifecycle.UID + "/" + label}); err != nil {
		return status, err
	}
	return lifecycle.Status(ctx, home, label)
}

func (lifecycle Lifecycle) Uninstall(ctx context.Context, home, label string) error {
	status, err := lifecycle.Status(ctx, home, label)
	if err != nil {
		return err
	}
	if status.State == "orphan_loaded" {
		if err := lifecycle.removeNative(ctx, label); err != nil {
			return err
		}
		return nil
	}
	if status.State == "loaded_identity_mismatch" {
		return errors.New("refusing to uninstall a mismatched loaded LaunchAgent")
	}
	if !status.FilePresent {
		return nil
	}
	if lifecycle.Native {
		if err := validateNativeLabel(label); err != nil {
			return err
		}
	}
	if lifecycle.Native && status.Loaded {
		if err := lifecycle.removeNative(ctx, label); err != nil {
			return err
		}
	} else if lifecycle.Native {
		if err := lifecycle.run(ctx, []string{"disable", "gui/" + lifecycle.UID + "/" + label}); err != nil {
			return err
		}
	}
	return Uninstall(home, label)
}

func (lifecycle Lifecycle) removeNative(ctx context.Context, label string) error {
	if err := validateNativeLabel(label); err != nil {
		return err
	}
	if err := lifecycle.run(ctx, []string{"disable", "gui/" + lifecycle.UID + "/" + label}); err != nil {
		return err
	}
	return lifecycle.run(ctx, []string{"bootout", "gui/" + lifecycle.UID + "/" + label})
}

func validateNativeLabel(label string) error {
	if label != "com.bcg.maestro.maintenance" {
		return errors.New("refusing native mutation for an unknown managed label")
	}
	return nil
}

func (lifecycle Lifecycle) fileStatus(home, label, state string) (LaunchAgentStatus, error) {
	status, err := ReadStatus(home, label)
	if err != nil {
		return LaunchAgentStatus{}, err
	}
	return LaunchAgentStatus{State: state, Path: status.Path, Label: label, FilePresent: status.State != "not_installed", Disabled: status.Disabled}, nil
}
func (lifecycle Lifecycle) run(ctx context.Context, args []string) error {
	_, err := lifecycle.runResult(ctx, args)
	return err
}
func (lifecycle Lifecycle) runResult(ctx context.Context, args []string) (CommandResult, error) {
	if lifecycle.Runner == nil {
		return CommandResult{}, errors.New("launchctl runner is unavailable")
	}
	callCtx, cancel := context.WithTimeout(ctx, lifecycle.Timeout)
	defer cancel()
	result, err := lifecycle.Runner.Run(callCtx, launchctlPath, args)
	if err == nil && result.ExitCode != 0 {
		return result, errors.New("launchctl returned a non-zero status")
	}
	return result, err
}

func disabledLabel(output, label string) bool {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=>", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.Trim(strings.TrimSpace(parts[0]), "\"")
		if name == label && strings.EqualFold(strings.TrimSpace(parts[1]), "true") {
			return true
		}
	}
	return false
}

func parseLaunchctlIdentity(output, expectedLabel string) (program, path string, err error) {
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 256), maxRunnerOutput)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if key != "program" && key != "path" && key != "label" {
			continue
		}
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"")
		if value == "" || len(value) > 1024 || strings.ContainsRune(value, '\x00') {
			return "", "", errors.New("launchctl identity field is invalid")
		}
		if _, duplicate := values[key]; duplicate {
			return "", "", errors.New("launchctl identity field is duplicated")
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return "", "", err
	}
	if label, present := values["label"]; present && label != expectedLabel {
		return "", "", fmt.Errorf("launchctl identity label mismatch")
	}
	if values["program"] == "" || values["path"] == "" || !pathpkg.IsAbs(values["program"]) || !filepath.IsAbs(values["path"]) || strings.Contains(values["program"], `\`) {
		return "", "", errors.New("launchctl identity is incomplete")
	}
	return values["program"], values["path"], nil
}
func samePath(left, right string) bool {
	left, _ = filepath.Abs(left)
	right, _ = filepath.Abs(right)
	return filepath.Clean(left) == filepath.Clean(right)
}
func osUserHome() (string, error) {
	current, err := user.Current()
	if err != nil {
		return "", err
	}
	return current.HomeDir, nil
}
func redactRunnerOutput(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > maxRunnerOutput {
		value = value[:maxRunnerOutput]
	}
	for _, key := range []string{"password", "token", "credential", "secret"} {
		value = redactKey(value, key)
	}
	return value
}
func redactKey(value, key string) string {
	lower := strings.ToLower(value)
	index := strings.Index(lower, key+"=")
	if index < 0 {
		return value
	}
	end := strings.IndexAny(value[index:], " \n\r")
	if end < 0 {
		end = len(value) - index
	}
	return value[:index+len(key)+1] + "[redacted]" + value[index+end:]
}
