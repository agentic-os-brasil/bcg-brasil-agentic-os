// Package installtx prepares and activates recoverable Maestro installations.
package installtx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

const PlanName = "activation-plan.json"

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type PrepareOptions struct {
	TargetOS    string
	TargetArch  string
	ManagedRoot string
	DataRoot    string
}

type ActivationPlan struct {
	SchemaVersion int    `json:"schema_version"`
	TransactionID string `json:"transaction_id"`
	Release       string `json:"release"`
	Channel       string `json:"channel"`
	CLIVersion    string `json:"cli_version"`
	BundleVersion string `json:"bundle_version"`
	TargetOS      string `json:"target_os"`
	TargetArch    string `json:"target_arch"`
	ManagedRoot   string `json:"managed_root"`
	DataRoot      string `json:"data_root"`
	StagedCLI     string `json:"staged_cli"`
	StagedBundle  string `json:"staged_bundle"`
}

type ActivateOptions struct {
	CheckCLI func(path, version string) error
}

type State struct {
	SchemaVersion int       `json:"schema_version"`
	Release       string    `json:"release"`
	Channel       string    `json:"channel"`
	CLIVersion    string    `json:"cli_version"`
	BundleVersion string    `json:"bundle_version"`
	TargetOS      string    `json:"target_os"`
	TargetArch    string    `json:"target_arch"`
	ActivatedAt   time.Time `json:"activated_at"`
	Previous      *Snapshot `json:"previous,omitempty"`
}

type Snapshot struct {
	Release       string `json:"release"`
	Channel       string `json:"channel"`
	CLIVersion    string `json:"cli_version"`
	BundleVersion string `json:"bundle_version"`
	TargetOS      string `json:"target_os"`
	TargetArch    string `json:"target_arch"`
	CLIBackup     string `json:"cli_backup"`
}

func Prepare(verified releaseverify.VerifiedRelease, options PrepareOptions) (string, error) {
	managedRoot, dataRoot, err := normalizedRoots(options.ManagedRoot, options.DataRoot)
	if err != nil {
		return "", err
	}
	if !supportedTarget(options.TargetOS, options.TargetArch) {
		return "", fmt.Errorf("unsupported install target %s/%s", options.TargetOS, options.TargetArch)
	}
	var cliArtifact, bundleArtifact *releasecontract.Artifact
	for index := range verified.Manifest.Artifacts {
		artifact := &verified.Manifest.Artifacts[index]
		switch {
		case artifact.Kind == "cli" && artifact.OS == options.TargetOS && artifact.Arch == options.TargetArch:
			if cliArtifact != nil {
				return "", errors.New("release has multiple CLI artifacts for the target")
			}
			cliArtifact = artifact
		case artifact.Kind == "bundle":
			bundleArtifact = artifact
		}
	}
	if cliArtifact == nil || bundleArtifact == nil {
		return "", errors.New("release does not contain the requested CLI and base bundle")
	}
	updatesRoot := filepath.Join(dataRoot, "updates")
	if err := os.MkdirAll(updatesRoot, 0o700); err != nil {
		return "", err
	}
	transaction, err := os.MkdirTemp(updatesRoot, "tx-")
	if err != nil {
		return "", err
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(transaction)
		}
	}()
	stagedCLI := filepath.Join(transaction, "bin", executableName(options.TargetOS))
	if err := copyRegular(filepath.Join(verified.Directory, cliArtifact.Name), stagedCLI, 0o755); err != nil {
		return "", err
	}
	stagedBundle := filepath.Join(transaction, "bundle")
	if err := os.MkdirAll(stagedBundle, 0o700); err != nil {
		return "", err
	}
	bundle, err := os.Open(filepath.Join(verified.Directory, bundleArtifact.Name))
	if err != nil {
		return "", err
	}
	extractErr := extractBundle(bundle, stagedBundle)
	closeErr := bundle.Close()
	if extractErr != nil {
		return "", extractErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	plan := ActivationPlan{
		SchemaVersion: 1,
		TransactionID: filepath.Base(transaction),
		Release:       verified.Manifest.Release,
		Channel:       verified.Manifest.Channel,
		CLIVersion:    verified.Manifest.CLI.Version,
		BundleVersion: verified.Manifest.Bundle.Version,
		TargetOS:      options.TargetOS,
		TargetArch:    options.TargetArch,
		ManagedRoot:   managedRoot,
		DataRoot:      dataRoot,
		StagedCLI:     stagedCLI,
		StagedBundle:  stagedBundle,
	}
	planPath := filepath.Join(transaction, PlanName)
	if err := WritePlan(planPath, plan); err != nil {
		return "", err
	}
	success = true
	return planPath, nil
}

func Activate(planPath string, options ActivateOptions) error {
	plan, err := ReadPlan(planPath)
	if err != nil {
		return err
	}
	if err := validatePlan(planPath, plan); err != nil {
		return err
	}
	checker := options.CheckCLI
	if checker == nil {
		checker = commandSelfCheck
	}
	if err := os.MkdirAll(plan.ManagedRoot, 0o755); err != nil {
		return err
	}
	unlock, err := acquireLock(plan.ManagedRoot)
	if err != nil {
		return err
	}
	defer unlock()

	activeCLI := filepath.Join(plan.ManagedRoot, "bin", executableName(plan.TargetOS))
	bundleTarget := filepath.Join(plan.ManagedRoot, "bundles", plan.BundleVersion)
	if _, err := os.Stat(bundleTarget); err == nil {
		return fmt.Errorf("bundle version already exists: %s", plan.BundleVersion)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	current, stateErr := ReadState(plan.DataRoot)
	hadState := stateErr == nil
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return stateErr
	}
	if err := os.MkdirAll(filepath.Dir(activeCLI), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(bundleTarget), 0o755); err != nil {
		return err
	}
	pendingCLI := activeCLI + ".pending-" + plan.TransactionID
	if err := copyRegular(plan.StagedCLI, pendingCLI, 0o755); err != nil {
		return err
	}
	defer os.Remove(pendingCLI)
	pendingBundle := filepath.Join(plan.ManagedRoot, "bundles", ".pending-"+plan.TransactionID)
	if err := copyTree(plan.StagedBundle, pendingBundle); err != nil {
		return err
	}
	defer os.RemoveAll(pendingBundle)

	var backup string
	if _, err := os.Stat(activeCLI); err == nil {
		previousVersion := "unknown"
		if hadState {
			previousVersion = current.CLIVersion
		}
		backup = filepath.Join(plan.ManagedRoot, "recovery", "cli", previousVersion+"-"+plan.TransactionID, executableName(plan.TargetOS))
		if err := os.MkdirAll(filepath.Dir(backup), 0o700); err != nil {
			return err
		}
		if err := os.Rename(activeCLI, backup); err != nil {
			return fmt.Errorf("backup active CLI: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	restore := func(cause error) error {
		_ = os.Remove(activeCLI)
		if backup != "" {
			_ = os.Rename(backup, activeCLI)
		}
		_ = os.RemoveAll(bundleTarget)
		return cause
	}
	if err := os.Rename(pendingBundle, bundleTarget); err != nil {
		return restore(fmt.Errorf("activate bundle: %w", err))
	}
	if err := os.Rename(pendingCLI, activeCLI); err != nil {
		return restore(fmt.Errorf("activate CLI: %w", err))
	}
	if err := checker(activeCLI, plan.CLIVersion); err != nil {
		return restore(fmt.Errorf("activated CLI self-check failed: %w", err))
	}
	next := State{
		SchemaVersion: 1,
		Release:       plan.Release,
		Channel:       plan.Channel,
		CLIVersion:    plan.CLIVersion,
		BundleVersion: plan.BundleVersion,
		TargetOS:      plan.TargetOS,
		TargetArch:    plan.TargetArch,
		ActivatedAt:   time.Now().UTC(),
	}
	if hadState {
		next.Previous = &Snapshot{
			Release: current.Release, Channel: current.Channel, CLIVersion: current.CLIVersion,
			BundleVersion: current.BundleVersion, TargetOS: current.TargetOS, TargetArch: current.TargetArch,
			CLIBackup: backup,
		}
	}
	if err := WriteState(plan.DataRoot, next); err != nil {
		return restore(fmt.Errorf("commit install state: %w", err))
	}
	return nil
}

func Rollback(managedRoot, dataRoot string, checker func(path, version string) error) error {
	managedRoot, dataRoot, err := normalizedRoots(managedRoot, dataRoot)
	if err != nil {
		return err
	}
	current, err := ReadState(dataRoot)
	if err != nil {
		return err
	}
	if current.Previous == nil {
		return errors.New("no previous Maestro installation is available")
	}
	if checker == nil {
		checker = commandSelfCheck
	}
	unlock, err := acquireLock(managedRoot)
	if err != nil {
		return err
	}
	defer unlock()
	activeCLI := filepath.Join(managedRoot, "bin", executableName(current.TargetOS))
	if _, err := os.Lstat(current.Previous.CLIBackup); err != nil {
		return fmt.Errorf("previous CLI backup unavailable: %w", err)
	}
	revertedBackup := filepath.Join(managedRoot, "recovery", "cli", current.CLIVersion+"-rollback-"+fmt.Sprint(time.Now().UnixNano()), executableName(current.TargetOS))
	if err := os.MkdirAll(filepath.Dir(revertedBackup), 0o700); err != nil {
		return err
	}
	if err := os.Rename(activeCLI, revertedBackup); err != nil {
		return err
	}
	if err := os.Rename(current.Previous.CLIBackup, activeCLI); err != nil {
		_ = os.Rename(revertedBackup, activeCLI)
		return err
	}
	if err := checker(activeCLI, current.Previous.CLIVersion); err != nil {
		_ = os.Remove(activeCLI)
		_ = os.Rename(revertedBackup, activeCLI)
		return fmt.Errorf("rolled-back CLI self-check failed: %w", err)
	}
	next := State{
		SchemaVersion: 1,
		Release:       current.Previous.Release,
		Channel:       current.Previous.Channel,
		CLIVersion:    current.Previous.CLIVersion,
		BundleVersion: current.Previous.BundleVersion,
		TargetOS:      current.Previous.TargetOS,
		TargetArch:    current.Previous.TargetArch,
		ActivatedAt:   time.Now().UTC(),
		Previous: &Snapshot{
			Release: current.Release, Channel: current.Channel, CLIVersion: current.CLIVersion,
			BundleVersion: current.BundleVersion, TargetOS: current.TargetOS, TargetArch: current.TargetArch,
			CLIBackup: revertedBackup,
		},
	}
	return WriteState(dataRoot, next)
}

func WritePlan(path string, plan ActivationPlan) error {
	return writeJSONAtomic(path, plan, 0o600)
}

func ReadPlan(path string) (ActivationPlan, error) {
	var plan ActivationPlan
	if err := readJSONStrict(path, &plan); err != nil {
		return ActivationPlan{}, err
	}
	return plan, nil
}

func WriteState(dataRoot string, state State) error {
	return writeJSONAtomic(statePath(dataRoot), state, 0o600)
}

func ReadState(dataRoot string) (State, error) {
	var state State
	if err := readJSONStrict(statePath(dataRoot), &state); err != nil {
		return State{}, err
	}
	if state.SchemaVersion != 1 || !versionPattern.MatchString(state.Release) ||
		!versionPattern.MatchString(state.CLIVersion) || !versionPattern.MatchString(state.BundleVersion) {
		return State{}, errors.New("invalid install state")
	}
	return state, nil
}

func statePath(dataRoot string) string {
	return filepath.Join(dataRoot, "config", "install-state.json")
}

func validatePlan(path string, plan ActivationPlan) error {
	if plan.SchemaVersion != 1 || !versionPattern.MatchString(plan.Release) ||
		!versionPattern.MatchString(plan.CLIVersion) || !versionPattern.MatchString(plan.BundleVersion) {
		return errors.New("invalid activation plan versions")
	}
	if plan.Channel != "canary" && plan.Channel != "beta" && plan.Channel != "stable" {
		return errors.New("invalid activation plan channel")
	}
	if !supportedTarget(plan.TargetOS, plan.TargetArch) {
		return errors.New("invalid activation plan target")
	}
	managedRoot, dataRoot, err := normalizedRoots(plan.ManagedRoot, plan.DataRoot)
	if err != nil || managedRoot != plan.ManagedRoot || dataRoot != plan.DataRoot {
		return errors.New("activation plan roots are not canonical")
	}
	transaction := filepath.Dir(filepath.Clean(path))
	if filepath.Base(transaction) != plan.TransactionID || !within(filepath.Join(plan.DataRoot, "updates"), transaction) {
		return errors.New("activation plan is outside its data-root transaction")
	}
	if !within(transaction, plan.StagedCLI) || !within(transaction, plan.StagedBundle) {
		return errors.New("staged activation paths escape their transaction")
	}
	if filepath.Clean(plan.StagedCLI) != filepath.Join(transaction, "bin", executableName(plan.TargetOS)) {
		return errors.New("staged CLI has an unexpected name")
	}
	if filepath.Clean(plan.StagedBundle) != filepath.Join(transaction, "bundle") {
		return errors.New("staged bundle has an unexpected path")
	}
	return nil
}

func normalizedRoots(managedRoot, dataRoot string) (string, string, error) {
	if managedRoot == "" || dataRoot == "" {
		return "", "", errors.New("managed and data roots are required")
	}
	managed, err := filepath.Abs(managedRoot)
	if err != nil {
		return "", "", err
	}
	data, err := filepath.Abs(dataRoot)
	if err != nil {
		return "", "", err
	}
	managed = filepath.Clean(managed)
	data = filepath.Clean(data)
	if within(managed, data) || within(data, managed) {
		return "", "", errors.New("managed and owner-data roots must be separate")
	}
	return managed, data, nil
}

func within(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func supportedTarget(osName, arch string) bool {
	return (osName == "windows" || osName == "darwin") && (arch == "amd64" || arch == "arm64")
}

func executableName(osName string) string {
	if osName == "windows" {
		return "bcgos.exe"
	}
	return "bcgos"
}

func copyRegular(source, target string, mode os.FileMode) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("install source must be a regular file: %s", source)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("staged bundle symlink is forbidden: %s", relative)
		}
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		return copyRegular(path, destination, 0o644)
	})
}

func commandSelfCheck(path, version string) error {
	command := exec.Command(path, "version")
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(string(output)) != "bcgos "+version {
		return fmt.Errorf("unexpected version output %q", strings.TrimSpace(string(output)))
	}
	return nil
}

func acquireLock(managedRoot string) (func(), error) {
	path := filepath.Join(managedRoot, ".activation.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("another activation may be running: %w", err)
	}
	if _, err := fmt.Fprintf(file, "%d\n", os.Getpid()); err != nil {
		file.Close()
		os.Remove(path)
		return nil, err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return nil, err
	}
	return func() { _ = os.Remove(path) }, nil
}

func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".pending-")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(body); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func readJSONStrict(path string, target any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(body) > 1<<20 {
		return errors.New("JSON state exceeds 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
