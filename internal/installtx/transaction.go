// Package installtx prepares and activates recoverable Maestro installations.
package installtx

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processwait"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/rolemigration"
)

const (
	PlanName    = "activation-plan.json"
	ReceiptName = "activation-receipt.json"
)

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var confirmationPlanIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type PrepareOptions struct {
	Transition         string
	ConfirmationPlanID string
	FromRelease        string
	FromChannel        string
	FromCLIVersion     string
	FromBundleVersion  string
	TargetOS           string
	TargetArch         string
	ManagedRoot        string
	DataRoot           string
}

type ActivationPlan struct {
	SchemaVersion           int    `json:"schema_version"`
	TransactionID           string `json:"transaction_id"`
	Transition              string `json:"transition"`
	ConfirmationPlanID      string `json:"confirmation_plan_id"`
	FromRelease             string `json:"from_release"`
	FromChannel             string `json:"from_channel"`
	FromCLIVersion          string `json:"from_cli_version"`
	FromBundleVersion       string `json:"from_bundle_version"`
	Release                 string `json:"release"`
	Channel                 string `json:"channel"`
	CLIVersion              string `json:"cli_version"`
	BundleVersion           string `json:"bundle_version"`
	TargetOS                string `json:"target_os"`
	TargetArch              string `json:"target_arch"`
	ManagedRoot             string `json:"managed_root"`
	DataRoot                string `json:"data_root"`
	ManifestSHA256          string `json:"manifest_sha256"`
	CLIArtifactName         string `json:"cli_artifact_name"`
	CLISHA256               string `json:"cli_sha256"`
	CLISize                 int64  `json:"cli_size"`
	BundleArtifactName      string `json:"bundle_artifact_name"`
	BundleSHA256            string `json:"bundle_sha256"`
	BundleSize              int64  `json:"bundle_size"`
	RoleMigrationID         string `json:"role_migration_id,omitempty"`
	CatalogSHA256           string `json:"catalog_sha256,omitempty"`
	PolicySHA256            string `json:"policy_sha256,omitempty"`
	StagedCLI               string `json:"staged_cli"`
	StagedBundleArchive     string `json:"staged_bundle_archive"`
	RuntimePackArtifactName string `json:"runtime_pack_artifact_name,omitempty"`
	RuntimePackSHA256       string `json:"runtime_pack_sha256,omitempty"`
	RuntimePackSize         int64  `json:"runtime_pack_size,omitempty"`
	StagedRuntimePack       string `json:"staged_runtime_pack,omitempty"`
}

type ActivateOptions struct {
	PrepareOptions
	CheckCLI        func(path, version string) error
	beforeSelfCheck func()
	afterPayload    func()
}

type State struct {
	SchemaVersion int       `json:"schema_version"`
	ManagedRoot   string    `json:"managed_root"`
	Release       string    `json:"release"`
	Channel       string    `json:"channel"`
	CLIVersion    string    `json:"cli_version"`
	BundleVersion string    `json:"bundle_version"`
	TargetOS      string    `json:"target_os"`
	TargetArch    string    `json:"target_arch"`
	RoleAuthority string    `json:"role_authority,omitempty"`
	MigrationID   string    `json:"migration_id,omitempty"`
	CatalogSHA256 string    `json:"catalog_sha256,omitempty"`
	PolicySHA256  string    `json:"policy_sha256,omitempty"`
	ActivatedAt   time.Time `json:"activated_at"`
	Previous      *Snapshot `json:"previous,omitempty"`
}

type ActivationReceipt struct {
	SchemaVersion      int       `json:"schema_version"`
	ConfirmationPlanID string    `json:"confirmation_plan_id"`
	TransactionID      string    `json:"transaction_id"`
	Release            string    `json:"release"`
	Channel            string    `json:"channel"`
	CLIVersion         string    `json:"cli_version"`
	BundleVersion      string    `json:"bundle_version"`
	TargetOS           string    `json:"target_os"`
	TargetArch         string    `json:"target_arch"`
	ManagedRoot        string    `json:"managed_root"`
	DataRoot           string    `json:"data_root"`
	ManifestSHA256     string    `json:"manifest_sha256"`
	CLISHA256          string    `json:"cli_sha256"`
	CLISize            int64     `json:"cli_size"`
	BundleSHA256       string    `json:"bundle_sha256"`
	BundleSize         int64     `json:"bundle_size"`
	RoleMigrationID    string    `json:"role_migration_id,omitempty"`
	CatalogSHA256      string    `json:"catalog_sha256,omitempty"`
	PolicySHA256       string    `json:"policy_sha256,omitempty"`
	SourceCLIBackup    string    `json:"source_cli_backup"`
	CommittedAt        time.Time `json:"committed_at"`
}

type stateWire struct {
	SchemaVersion int       `json:"schema_version"`
	ManagedRoot   *string   `json:"managed_root,omitempty"`
	Release       string    `json:"release"`
	Channel       string    `json:"channel"`
	CLIVersion    string    `json:"cli_version"`
	BundleVersion string    `json:"bundle_version"`
	TargetOS      string    `json:"target_os"`
	TargetArch    string    `json:"target_arch"`
	RoleAuthority string    `json:"role_authority,omitempty"`
	MigrationID   string    `json:"migration_id,omitempty"`
	CatalogSHA256 string    `json:"catalog_sha256,omitempty"`
	PolicySHA256  string    `json:"policy_sha256,omitempty"`
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
	RoleAuthority string `json:"role_authority,omitempty"`
	MigrationID   string `json:"migration_id,omitempty"`
	CatalogSHA256 string `json:"catalog_sha256,omitempty"`
	PolicySHA256  string `json:"policy_sha256,omitempty"`
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
	if err := validateTransitionOptions(options); err != nil {
		return "", err
	}
	roleBinding, err := roleBindingForTransition(options, verified.Manifest)
	if err != nil {
		return "", err
	}
	cliArtifact, bundleArtifact, runtimePack, err := releaseArtifacts(verified, options.TargetOS, options.TargetArch)
	if err != nil {
		return "", err
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
	if err := copyVerifiedRegular(
		filepath.Join(verified.Directory, cliArtifact.Name),
		stagedCLI,
		0o755,
		cliArtifact.Size,
		cliArtifact.SHA256,
	); err != nil {
		return "", err
	}
	stagedRuntimePack := ""
	if runtimePack != nil {
		stagedRuntimePack = filepath.Join(transaction, "runtime-pack.tar.gz")
		if err := copyVerifiedRegular(filepath.Join(verified.Directory, runtimePack.Name), stagedRuntimePack, 0o600, runtimePack.Size, runtimePack.SHA256); err != nil {
			return "", err
		}
	}
	stagedBundleArchive := filepath.Join(transaction, "bundle.tar.gz")
	if err := copyVerifiedRegular(
		filepath.Join(verified.Directory, bundleArtifact.Name),
		stagedBundleArchive,
		0o600,
		bundleArtifact.Size,
		bundleArtifact.SHA256,
	); err != nil {
		return "", err
	}
	plan := ActivationPlan{
		SchemaVersion:       2,
		TransactionID:       filepath.Base(transaction),
		Transition:          options.Transition,
		ConfirmationPlanID:  options.ConfirmationPlanID,
		FromRelease:         options.FromRelease,
		FromChannel:         options.FromChannel,
		FromCLIVersion:      options.FromCLIVersion,
		FromBundleVersion:   options.FromBundleVersion,
		Release:             verified.Manifest.Release,
		Channel:             verified.Manifest.Channel,
		CLIVersion:          verified.Manifest.CLI.Version,
		BundleVersion:       verified.Manifest.Bundle.Version,
		TargetOS:            options.TargetOS,
		TargetArch:          options.TargetArch,
		ManagedRoot:         managedRoot,
		DataRoot:            dataRoot,
		ManifestSHA256:      verified.ManifestSHA256,
		CLIArtifactName:     cliArtifact.Name,
		CLISHA256:           cliArtifact.SHA256,
		CLISize:             cliArtifact.Size,
		BundleArtifactName:  bundleArtifact.Name,
		BundleSHA256:        bundleArtifact.SHA256,
		BundleSize:          bundleArtifact.Size,
		RoleMigrationID:     roleBinding.ID,
		CatalogSHA256:       roleBinding.CatalogSHA256,
		PolicySHA256:        roleBinding.PolicySHA256,
		StagedCLI:           stagedCLI,
		StagedBundleArchive: stagedBundleArchive,
		StagedRuntimePack:   stagedRuntimePack,
	}
	if runtimePack != nil {
		plan.RuntimePackArtifactName = runtimePack.Name
		plan.RuntimePackSHA256 = runtimePack.SHA256
		plan.RuntimePackSize = runtimePack.Size
	}
	planPath := filepath.Join(transaction, PlanName)
	if err := WritePlan(planPath, plan); err != nil {
		return "", err
	}
	success = true
	return planPath, nil
}

func ValidatePrepared(
	planPath string,
	verified releaseverify.VerifiedRelease,
	options PrepareOptions,
) (ActivationPlan, error) {
	plan, err := ReadPlan(planPath)
	if err != nil {
		return ActivationPlan{}, err
	}
	if err := validatePlan(planPath, plan); err != nil {
		return ActivationPlan{}, err
	}
	managedRoot, dataRoot, err := normalizedRoots(options.ManagedRoot, options.DataRoot)
	if err != nil {
		return ActivationPlan{}, err
	}
	if err := validateTransitionOptions(options); err != nil {
		return ActivationPlan{}, err
	}
	cliArtifact, bundleArtifact, runtimePack, err := releaseArtifacts(verified, options.TargetOS, options.TargetArch)
	if err != nil {
		return ActivationPlan{}, err
	}
	if plan.Transition != options.Transition ||
		plan.ConfirmationPlanID != options.ConfirmationPlanID ||
		plan.FromRelease != options.FromRelease ||
		plan.FromChannel != options.FromChannel ||
		plan.FromCLIVersion != options.FromCLIVersion ||
		plan.FromBundleVersion != options.FromBundleVersion ||
		plan.Release != verified.Manifest.Release ||
		plan.Channel != verified.Manifest.Channel ||
		plan.CLIVersion != verified.Manifest.CLI.Version ||
		plan.BundleVersion != verified.Manifest.Bundle.Version ||
		plan.TargetOS != options.TargetOS ||
		plan.TargetArch != options.TargetArch ||
		plan.ManagedRoot != managedRoot ||
		plan.DataRoot != dataRoot ||
		plan.ManifestSHA256 != verified.ManifestSHA256 ||
		plan.CLIArtifactName != cliArtifact.Name ||
		plan.CLISHA256 != cliArtifact.SHA256 ||
		plan.CLISize != cliArtifact.Size ||
		plan.BundleArtifactName != bundleArtifact.Name ||
		plan.BundleSHA256 != bundleArtifact.SHA256 ||
		plan.BundleSize != bundleArtifact.Size ||
		(runtimePack != nil && (plan.RuntimePackArtifactName != runtimePack.Name || plan.RuntimePackSHA256 != runtimePack.SHA256 || plan.RuntimePackSize != runtimePack.Size || plan.StagedRuntimePack == "")) ||
		(runtimePack == nil && (plan.RuntimePackArtifactName != "" || plan.RuntimePackSHA256 != "" || plan.RuntimePackSize != 0 || plan.StagedRuntimePack != "")) {
		return ActivationPlan{}, errors.New("activation plan does not match the verified release and install target")
	}
	roleBinding, err := roleBindingForTransition(options, verified.Manifest)
	if err != nil {
		return ActivationPlan{}, err
	}
	hasRoleMigration := roleBinding.ID != ""
	if (plan.RoleMigrationID != "") != hasRoleMigration {
		return ActivationPlan{}, errors.New("activation plan role migration binding does not match the verified release")
	}
	if hasRoleMigration && (plan.RoleMigrationID != roleBinding.ID || plan.CatalogSHA256 != roleBinding.CatalogSHA256 || plan.PolicySHA256 != roleBinding.PolicySHA256) {
		return ActivationPlan{}, errors.New("activation plan role migration identity does not match the verified release")
	}
	if err := ensureSafePath(dataRoot, plan.StagedCLI); err != nil {
		return ActivationPlan{}, fmt.Errorf("staged CLI path is outside the private data root: %w", err)
	}
	if err := ensureSafePath(dataRoot, plan.StagedBundleArchive); err != nil {
		return ActivationPlan{}, fmt.Errorf("staged bundle path is outside the private data root: %w", err)
	}
	if err := verifyRegularFile(plan.StagedCLI, plan.CLISize, plan.CLISHA256); err != nil {
		return ActivationPlan{}, fmt.Errorf("verify staged CLI: %w", err)
	}
	if err := verifyRegularFile(plan.StagedBundleArchive, plan.BundleSize, plan.BundleSHA256); err != nil {
		return ActivationPlan{}, fmt.Errorf("verify staged bundle archive: %w", err)
	}
	if runtimePack != nil {
		if err := ensureSafePath(dataRoot, plan.StagedRuntimePack); err != nil {
			return ActivationPlan{}, fmt.Errorf("staged runtime pack path is outside the private data root: %w", err)
		}
		if err := verifyRegularFile(plan.StagedRuntimePack, plan.RuntimePackSize, plan.RuntimePackSHA256); err != nil {
			return ActivationPlan{}, fmt.Errorf("verify staged runtime pack: %w", err)
		}
	}
	return plan, nil
}

func Activate(
	planPath string,
	verified releaseverify.VerifiedRelease,
	options ActivateOptions,
) error {
	plan, err := ValidatePrepared(planPath, verified, options.PrepareOptions)
	if err != nil {
		return err
	}
	checker := options.CheckCLI
	if checker == nil {
		checker = commandSelfCheck
	}
	if plan.Transition == "install" {
		if err := os.MkdirAll(plan.ManagedRoot, 0o755); err != nil {
			return err
		}
	} else {
		info, err := os.Stat(plan.ManagedRoot)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("managed root is not a directory")
		}
	}
	unlock, err := acquireLock(plan.ManagedRoot)
	if err != nil {
		return err
	}
	defer unlock()

	activeCLI := filepath.Join(plan.ManagedRoot, "bin", executableName(plan.TargetOS))
	bundleTarget := filepath.Join(plan.ManagedRoot, "bundles", plan.BundleVersion)
	runtimeTarget := ""
	if plan.RuntimePackArtifactName != "" {
		runtimeTarget = filepath.Join(plan.ManagedRoot, "runtimes", "pack", plan.Release)
	}
	if err := ensureSafePath(plan.ManagedRoot, activeCLI); err != nil {
		return err
	}
	if err := ensureSafePath(plan.ManagedRoot, bundleTarget); err != nil {
		return err
	}
	if runtimeTarget != "" {
		if err := ensureSafePath(plan.ManagedRoot, runtimeTarget); err != nil {
			return err
		}
	}
	if _, err := os.Stat(bundleTarget); err == nil {
		return fmt.Errorf("bundle version already exists: %s", plan.BundleVersion)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	current, stateErr := ReadStateForManagedRoot(plan.DataRoot, plan.ManagedRoot)
	hadState := stateErr == nil
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return stateErr
	}
	switch plan.Transition {
	case "install":
		if hadState {
			return errors.New("first-install activation found an existing installed state")
		}
		if _, err := os.Lstat(activeCLI); err == nil {
			return errors.New("first-install activation found an existing active CLI")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	case "update":
		if !hadState || !stateMatchesSource(current, plan) {
			return errors.New("installed state changed after update confirmation")
		}
		info, err := os.Lstat(activeCLI)
		if err != nil {
			return fmt.Errorf("active CLI is unavailable before update: %w", err)
		}
		if !info.Mode().IsRegular() {
			return errors.New("active CLI must be a regular file before update")
		}
	}
	activatedAt := time.Now().UTC()
	receiptWritten := false
	stateCommitted := false
	if plan.Transition == "update" {
		if err := writeActivationReceipt(planPath, receiptForPlan(plan, activatedAt)); err != nil {
			return fmt.Errorf("write activation intent: %w", err)
		}
		receiptWritten = true
	}
	defer func() {
		if receiptWritten && !stateCommitted {
			_ = os.Remove(filepath.Join(filepath.Dir(planPath), ReceiptName))
		}
	}()
	if err := os.MkdirAll(filepath.Dir(activeCLI), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(bundleTarget), 0o755); err != nil {
		return err
	}
	if runtimeTarget != "" {
		if err := os.MkdirAll(filepath.Dir(runtimeTarget), 0o755); err != nil {
			return err
		}
	}
	pendingCLI := activeCLI + ".pending-" + plan.TransactionID
	if err := ensureSafePath(plan.ManagedRoot, pendingCLI); err != nil {
		return err
	}
	if err := copyVerifiedRegular(plan.StagedCLI, pendingCLI, 0o755, plan.CLISize, plan.CLISHA256); err != nil {
		return err
	}
	defer os.Remove(pendingCLI)
	pendingBundle := filepath.Join(plan.ManagedRoot, "bundles", ".pending-"+plan.TransactionID)
	if err := ensureSafePath(plan.ManagedRoot, pendingBundle); err != nil {
		return err
	}
	if err := extractVerifiedBundleArchive(
		plan.StagedBundleArchive,
		pendingBundle,
		plan.BundleSize,
		plan.BundleSHA256,
	); err != nil {
		return err
	}
	defer os.RemoveAll(pendingBundle)
	pendingRuntime := ""
	if runtimeTarget != "" {
		pendingRuntime = filepath.Join(plan.ManagedRoot, "runtimes", "pack", ".pending-"+plan.TransactionID)
		if err := ensureSafePath(plan.ManagedRoot, pendingRuntime); err != nil {
			return err
		}
		if err := extractVerifiedBundleArchive(plan.StagedRuntimePack, pendingRuntime, plan.RuntimePackSize, plan.RuntimePackSHA256); err != nil {
			return err
		}
		defer os.RemoveAll(pendingRuntime)
	}

	var backup string
	if _, err := os.Stat(activeCLI); err == nil {
		backup = filepath.Join(plan.ManagedRoot, "recovery", "cli", "unknown-"+plan.TransactionID, executableName(plan.TargetOS))
		if plan.Transition == "update" {
			backup = expectedSourceCLIBackup(plan)
		}
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
		if runtimeTarget != "" {
			_ = os.RemoveAll(runtimeTarget)
		}
		return cause
	}
	if err := os.Rename(pendingBundle, bundleTarget); err != nil {
		return restore(fmt.Errorf("activate bundle: %w", err))
	}
	if pendingRuntime != "" {
		if err := os.Rename(pendingRuntime, runtimeTarget); err != nil {
			return restore(fmt.Errorf("activate runtime pack: %w", err))
		}
	}
	if err := os.Rename(pendingCLI, activeCLI); err != nil {
		return restore(fmt.Errorf("activate CLI: %w", err))
	}
	if options.beforeSelfCheck != nil {
		options.beforeSelfCheck()
	}
	if err := checker(activeCLI, plan.CLIVersion); err != nil {
		return restore(fmt.Errorf("activated CLI self-check failed: %w", err))
	}
	if options.afterPayload != nil {
		options.afterPayload()
	}
	next := State{
		SchemaVersion: 2,
		ManagedRoot:   plan.ManagedRoot,
		Release:       plan.Release,
		Channel:       plan.Channel,
		CLIVersion:    plan.CLIVersion,
		BundleVersion: plan.BundleVersion,
		TargetOS:      plan.TargetOS,
		TargetArch:    plan.TargetArch,
		RoleAuthority: roleAuthorityForPlan(plan),
		MigrationID:   plan.RoleMigrationID,
		CatalogSHA256: plan.CatalogSHA256,
		PolicySHA256:  plan.PolicySHA256,
		ActivatedAt:   activatedAt,
	}
	if hadState {
		next.Previous = &Snapshot{
			Release: current.Release, Channel: current.Channel, CLIVersion: current.CLIVersion,
			BundleVersion: current.BundleVersion, TargetOS: current.TargetOS, TargetArch: current.TargetArch,
			RoleAuthority: current.RoleAuthority, MigrationID: current.MigrationID,
			CatalogSHA256: current.CatalogSHA256, PolicySHA256: current.PolicySHA256,
			CLIBackup: backup,
		}
	}
	if err := WriteState(plan.DataRoot, next); err != nil {
		return restore(fmt.Errorf("commit install state: %w", err))
	}
	stateCommitted = true
	return nil
}

func stateMatchesSource(state State, plan ActivationPlan) bool {
	return state.Release == plan.FromRelease &&
		state.Channel == plan.FromChannel &&
		state.CLIVersion == plan.FromCLIVersion &&
		state.BundleVersion == plan.FromBundleVersion &&
		state.TargetOS == plan.TargetOS &&
		state.TargetArch == plan.TargetArch
}

func Rollback(managedRoot, dataRoot string, checker func(path, version string) error) error {
	managedRoot, dataRoot, err := normalizedRoots(managedRoot, dataRoot)
	if err != nil {
		return err
	}
	current, err := ReadStateForManagedRoot(dataRoot, managedRoot)
	if err != nil {
		return err
	}
	if current.Previous == nil {
		return errors.New("no previous Maestro installation is available")
	}
	currentExpired, err := rolemigration.IsExpired(current.Release)
	if err != nil {
		return fmt.Errorf("validate rollback release authority: %w", err)
	}
	previousExpired, err := rolemigration.IsExpired(current.Previous.Release)
	if err != nil {
		return fmt.Errorf("validate previous rollback release authority: %w", err)
	}
	if (currentExpired || current.RoleAuthority == rolemigration.CanonicalRole) &&
		(!previousExpired || current.Previous.RoleAuthority != rolemigration.CanonicalRole) {
		return errors.New("rollback would reactivate a legacy agent authority after alias expiry")
	}
	if err := ensureSafePath(managedRoot, current.Previous.CLIBackup); err != nil {
		return fmt.Errorf("previous CLI backup is outside the managed root: %w", err)
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
	if err := ensureSafePath(managedRoot, activeCLI); err != nil {
		return err
	}
	if _, err := os.Lstat(current.Previous.CLIBackup); err != nil {
		return fmt.Errorf("previous CLI backup unavailable: %w", err)
	}
	revertedBackup := filepath.Join(managedRoot, "recovery", "cli", current.CLIVersion+"-rollback-"+fmt.Sprint(time.Now().UnixNano()), executableName(current.TargetOS))
	if err := ensureSafePath(managedRoot, revertedBackup); err != nil {
		return err
	}
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
		SchemaVersion: 2,
		ManagedRoot:   managedRoot,
		Release:       current.Previous.Release,
		Channel:       current.Previous.Channel,
		CLIVersion:    current.Previous.CLIVersion,
		BundleVersion: current.Previous.BundleVersion,
		TargetOS:      current.Previous.TargetOS,
		TargetArch:    current.Previous.TargetArch,
		RoleAuthority: current.Previous.RoleAuthority,
		MigrationID:   current.Previous.MigrationID,
		CatalogSHA256: current.Previous.CatalogSHA256,
		PolicySHA256:  current.Previous.PolicySHA256,
		ActivatedAt:   time.Now().UTC(),
		Previous: &Snapshot{
			Release: current.Release, Channel: current.Channel, CLIVersion: current.CLIVersion,
			BundleVersion: current.BundleVersion, TargetOS: current.TargetOS, TargetArch: current.TargetArch,
			RoleAuthority: current.RoleAuthority, MigrationID: current.MigrationID,
			CatalogSHA256: current.CatalogSHA256, PolicySHA256: current.PolicySHA256,
			CLIBackup: revertedBackup,
		},
	}
	if err := WriteState(dataRoot, next); err != nil {
		// The durable state is still current, so compensate the filesystem back
		// to the current CLI rather than leaving role authority and bytes split.
		backupErr := os.Rename(activeCLI, current.Previous.CLIBackup)
		restoreErr := os.Rename(revertedBackup, activeCLI)
		if backupErr != nil || restoreErr != nil {
			return fmt.Errorf("commit rollback state: %w (filesystem compensation failed: backup=%v restore=%v)", err, backupErr, restoreErr)
		}
		return fmt.Errorf("commit rollback state: %w (filesystem restored)", err)
	}
	return nil
}

func roleBindingForTransition(options PrepareOptions, manifest releasecontract.Manifest) (rolemigration.Binding, error) {
	if options.Transition == "update" {
		return rolemigration.EnsureUpdateAllowed(options.FromRelease, manifest.Release, manifest)
	}
	// A fresh install validates any advertised migration but never applies or
	// persists a legacy-source binding.
	if _, _, err := rolemigration.FromManifest(manifest); err != nil {
		return rolemigration.Binding{}, err
	}
	return rolemigration.Binding{}, nil
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

func ValidateReconciliation(planPath string, options PrepareOptions) (bool, error) {
	plan, err := validateReconciliationPlan(planPath, options)
	if err != nil {
		return false, err
	}
	receipt, err := readActivationReceipt(planPath, plan)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	current, err := ReadStateForManagedRoot(options.DataRoot, options.ManagedRoot)
	if err != nil {
		return true, err
	}
	if stateMatchesSource(current, plan) ||
		stateMatchesCommittedTarget(current, plan, receipt) {
		return true, nil
	}
	return true, errors.New("installed state matches neither the update source nor its committed target")
}

func ReconcileActivated(planPath string, options ActivateOptions) (bool, error) {
	plan, err := validateReconciliationPlan(planPath, options.PrepareOptions)
	if err != nil {
		return false, err
	}
	checker := options.CheckCLI
	if checker == nil {
		checker = commandSelfCheck
	}
	receipt, err := readActivationReceipt(planPath, plan)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			unlock, lockErr := acquireReconciliationLock(plan.ManagedRoot)
			if lockErr != nil {
				return false, lockErr
			}
			unlock()
			return false, nil
		}
		return false, err
	}
	unlock, err := acquireReconciliationLock(plan.ManagedRoot)
	if err != nil {
		return false, err
	}
	defer unlock()
	current, err := ReadStateForManagedRoot(options.PrepareOptions.DataRoot, options.PrepareOptions.ManagedRoot)
	if err != nil {
		return false, err
	}
	if stateMatchesSource(current, plan) {
		payloadErr := verifyActivatedPayload(plan)
		backupInfo, backupErr := os.Lstat(receipt.SourceCLIBackup)
		backupReady := backupErr == nil && backupInfo.Mode().IsRegular()
		if payloadErr == nil && backupReady {
			activeCLI := filepath.Join(plan.ManagedRoot, "bin", executableName(plan.TargetOS))
			if err := checker(activeCLI, plan.CLIVersion); err != nil {
				if restoreErr := restoreInterruptedActivation(plan, receipt); restoreErr != nil {
					return false, fmt.Errorf(
						"interrupted activation failed its CLI self-check and could not be restored: %w",
						errors.Join(err, restoreErr),
					)
				}
				return false, nil
			}
			if err := WriteState(
				plan.DataRoot,
				committedTargetState(plan, current, receipt.CommittedAt),
			); err != nil {
				return false, fmt.Errorf("complete interrupted activation state: %w", err)
			}
			return true, nil
		}
		if err := restoreInterruptedActivation(plan, receipt); err != nil {
			return false, fmt.Errorf(
				"interrupted activation could not be restored (payload=%v backup=%v): %w",
				payloadErr,
				backupErr,
				err,
			)
		}
		return false, nil
	}
	if !stateMatchesCommittedTarget(current, plan, receipt) {
		return false, errors.New("installed state matches neither the update source nor its committed target")
	}
	if err := verifyActivatedPayload(plan); err != nil {
		return false, err
	}
	return true, nil
}

func validateReconciliationPlan(planPath string, options PrepareOptions) (ActivationPlan, error) {
	plan, err := ReadPlan(planPath)
	if err != nil {
		return ActivationPlan{}, err
	}
	if err := validatePlan(planPath, plan); err != nil {
		return ActivationPlan{}, err
	}
	managedRoot, dataRoot, err := normalizedRoots(options.ManagedRoot, options.DataRoot)
	if err != nil {
		return ActivationPlan{}, err
	}
	if options.Transition != "update" ||
		plan.Transition != options.Transition ||
		plan.ConfirmationPlanID != options.ConfirmationPlanID ||
		plan.FromRelease != options.FromRelease ||
		plan.FromChannel != options.FromChannel ||
		plan.FromCLIVersion != options.FromCLIVersion ||
		plan.FromBundleVersion != options.FromBundleVersion ||
		plan.TargetOS != options.TargetOS ||
		plan.TargetArch != options.TargetArch ||
		plan.ManagedRoot != managedRoot ||
		plan.DataRoot != dataRoot {
		return ActivationPlan{}, errors.New("activation reconciliation options do not match the confirmed plan")
	}
	return plan, nil
}

func readActivationReceipt(planPath string, plan ActivationPlan) (ActivationReceipt, error) {
	var receipt ActivationReceipt
	if err := readJSONStrict(filepath.Join(filepath.Dir(planPath), ReceiptName), &receipt); err != nil {
		return ActivationReceipt{}, err
	}
	if receipt != receiptForPlan(plan, receipt.CommittedAt) ||
		receipt.CommittedAt.IsZero() ||
		receipt.CommittedAt.Location() != time.UTC {
		return ActivationReceipt{}, errors.New("activation receipt does not match the confirmed plan")
	}
	return receipt, nil
}

func verifyActivatedPayload(plan ActivationPlan) error {
	activeCLI := filepath.Join(plan.ManagedRoot, "bin", executableName(plan.TargetOS))
	if err := verifyRegularFile(activeCLI, plan.CLISize, plan.CLISHA256); err != nil {
		return fmt.Errorf("reconcile active CLI: %w", err)
	}
	expectedBundle, err := os.MkdirTemp(filepath.Join(plan.DataRoot, "updates"), ".reconcile-bundle-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(expectedBundle)
	if err := extractVerifiedBundleArchive(
		plan.StagedBundleArchive,
		expectedBundle,
		plan.BundleSize,
		plan.BundleSHA256,
	); err != nil {
		return fmt.Errorf("reconcile signed bundle: %w", err)
	}
	bundleTarget := filepath.Join(plan.ManagedRoot, "bundles", plan.BundleVersion)
	equal, err := equalTrees(expectedBundle, bundleTarget)
	if err != nil {
		return err
	}
	if !equal {
		return errors.New("installed bundle does not match the committed activation receipt")
	}
	return nil
}

func restoreInterruptedActivation(plan ActivationPlan, receipt ActivationReceipt) error {
	activeCLI := filepath.Join(plan.ManagedRoot, "bin", executableName(plan.TargetOS))
	backupInfo, backupErr := os.Lstat(receipt.SourceCLIBackup)
	switch {
	case backupErr == nil:
		if !backupInfo.Mode().IsRegular() {
			return errors.New("interrupted activation backup is not a regular file")
		}
		if err := os.Remove(activeCLI); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(receipt.SourceCLIBackup, activeCLI); err != nil {
			return err
		}
	case errors.Is(backupErr, os.ErrNotExist):
		info, err := os.Lstat(activeCLI)
		if err != nil {
			return errors.New("interrupted activation has neither source CLI nor its backup")
		}
		if !info.Mode().IsRegular() {
			return errors.New("interrupted activation source CLI is not a regular file")
		}
	default:
		return backupErr
	}
	if err := os.RemoveAll(filepath.Join(plan.ManagedRoot, "bundles", plan.BundleVersion)); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(plan.ManagedRoot, "bundles", ".pending-"+plan.TransactionID)); err != nil {
		return err
	}
	if err := os.Remove(activeCLI + ".pending-" + plan.TransactionID); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(filepath.Join(filepath.Dir(filepath.Dir(plan.StagedCLI)), ReceiptName)); err != nil &&
		!errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func committedTargetState(plan ActivationPlan, source State, activatedAt time.Time) State {
	return State{
		SchemaVersion: 2,
		ManagedRoot:   plan.ManagedRoot,
		Release:       plan.Release,
		Channel:       plan.Channel,
		CLIVersion:    plan.CLIVersion,
		BundleVersion: plan.BundleVersion,
		TargetOS:      plan.TargetOS,
		TargetArch:    plan.TargetArch,
		RoleAuthority: roleAuthorityForPlan(plan),
		MigrationID:   plan.RoleMigrationID,
		CatalogSHA256: plan.CatalogSHA256,
		PolicySHA256:  plan.PolicySHA256,
		ActivatedAt:   activatedAt,
		Previous: &Snapshot{
			Release: source.Release, Channel: source.Channel, CLIVersion: source.CLIVersion,
			BundleVersion: source.BundleVersion, TargetOS: source.TargetOS, TargetArch: source.TargetArch,
			RoleAuthority: source.RoleAuthority, MigrationID: source.MigrationID,
			CatalogSHA256: source.CatalogSHA256, PolicySHA256: source.PolicySHA256,
			CLIBackup: expectedSourceCLIBackup(plan),
		},
	}
}

func stateMatchesCommittedTarget(
	state State,
	plan ActivationPlan,
	receipt ActivationReceipt,
) bool {
	return state.Release == plan.Release &&
		state.Channel == plan.Channel &&
		state.CLIVersion == plan.CLIVersion &&
		state.BundleVersion == plan.BundleVersion &&
		state.TargetOS == plan.TargetOS &&
		state.TargetArch == plan.TargetArch &&
		state.RoleAuthority == roleAuthorityForPlan(plan) &&
		state.MigrationID == plan.RoleMigrationID &&
		state.CatalogSHA256 == plan.CatalogSHA256 &&
		state.PolicySHA256 == plan.PolicySHA256 &&
		state.Previous != nil &&
		state.Previous.Release == plan.FromRelease &&
		state.Previous.Channel == plan.FromChannel &&
		state.Previous.CLIVersion == plan.FromCLIVersion &&
		state.Previous.BundleVersion == plan.FromBundleVersion &&
		state.Previous.TargetOS == plan.TargetOS &&
		state.Previous.TargetArch == plan.TargetArch &&
		state.Previous.CLIBackup == receipt.SourceCLIBackup
}

func receiptForPlan(plan ActivationPlan, committedAt time.Time) ActivationReceipt {
	return ActivationReceipt{
		SchemaVersion:      1,
		ConfirmationPlanID: plan.ConfirmationPlanID,
		TransactionID:      plan.TransactionID,
		Release:            plan.Release,
		Channel:            plan.Channel,
		CLIVersion:         plan.CLIVersion,
		BundleVersion:      plan.BundleVersion,
		TargetOS:           plan.TargetOS,
		TargetArch:         plan.TargetArch,
		ManagedRoot:        plan.ManagedRoot,
		DataRoot:           plan.DataRoot,
		ManifestSHA256:     plan.ManifestSHA256,
		CLISHA256:          plan.CLISHA256,
		CLISize:            plan.CLISize,
		BundleSHA256:       plan.BundleSHA256,
		BundleSize:         plan.BundleSize,
		RoleMigrationID:    plan.RoleMigrationID,
		CatalogSHA256:      plan.CatalogSHA256,
		PolicySHA256:       plan.PolicySHA256,
		SourceCLIBackup:    expectedSourceCLIBackup(plan),
		CommittedAt:        committedAt,
	}
}

func roleAuthorityForPlan(plan ActivationPlan) string {
	if plan.RoleMigrationID == rolemigration.MigrationID {
		return rolemigration.CanonicalRole
	}
	if expired, err := rolemigration.IsExpired(plan.Release); err == nil && expired {
		return rolemigration.CanonicalRole
	}
	return ""
}

func expectedSourceCLIBackup(plan ActivationPlan) string {
	if plan.Transition != "update" {
		return ""
	}
	return filepath.Join(
		plan.ManagedRoot,
		"recovery",
		"cli",
		plan.FromCLIVersion+"-"+plan.TransactionID,
		executableName(plan.TargetOS),
	)
}

func writeActivationReceipt(planPath string, receipt ActivationReceipt) error {
	return writeJSONAtomic(filepath.Join(filepath.Dir(planPath), ReceiptName), receipt, 0o600)
}

func WriteState(dataRoot string, state State) error {
	if state.SchemaVersion != 2 || state.ManagedRoot == "" {
		return errors.New("new install state must use schema version 2 with a managed root")
	}
	if !validRoleState(state.RoleAuthority, state.MigrationID, state.CatalogSHA256, state.PolicySHA256) {
		return errors.New("install state role migration binding is invalid")
	}
	if state.Previous != nil && !validRoleState(state.Previous.RoleAuthority, state.Previous.MigrationID, state.Previous.CatalogSHA256, state.Previous.PolicySHA256) {
		return errors.New("previous install state role migration binding is invalid")
	}
	return writeJSONAtomic(statePath(dataRoot), state, 0o600)
}

func ReadState(dataRoot string) (State, error) {
	return readState(dataRoot, "", false)
}

func ReadStateForManagedRoot(dataRoot, expectedManagedRoot string) (State, error) {
	managedRoot, normalizedDataRoot, err := normalizedRoots(expectedManagedRoot, dataRoot)
	if err != nil {
		return State{}, err
	}
	state, err := readState(normalizedDataRoot, managedRoot, true)
	if err != nil {
		return State{}, err
	}
	return state, nil
}

func readState(dataRoot, expectedManagedRoot string, allowV1Migration bool) (State, error) {
	var wire stateWire
	if err := readJSONStrict(statePath(dataRoot), &wire); err != nil {
		return State{}, err
	}
	if !validStateCore(wire) {
		return State{}, errors.New("invalid install state")
	}
	switch wire.SchemaVersion {
	case 1:
		if wire.ManagedRoot != nil {
			return State{}, errors.New("schema version 1 install state must not contain managed_root")
		}
		if !allowV1Migration {
			return State{}, errors.New("schema version 1 install state requires an authoritative managed root for migration")
		}
		state := stateFromWire(wire, expectedManagedRoot)
		state.SchemaVersion = 2
		if err := WriteState(dataRoot, state); err != nil {
			return State{}, fmt.Errorf("migrate install state to schema version 2: %w", err)
		}
		return state, nil
	case 2:
		if wire.ManagedRoot == nil || *wire.ManagedRoot == "" {
			return State{}, errors.New("schema version 2 install state requires managed_root")
		}
	default:
		return State{}, errors.New("unsupported install-state schema version")
	}
	state := stateFromWire(wire, *wire.ManagedRoot)
	managedRoot, normalizedDataRoot, err := normalizedRoots(state.ManagedRoot, dataRoot)
	requestedDataRoot, absoluteErr := filepath.Abs(dataRoot)
	if err != nil || absoluteErr != nil ||
		managedRoot != state.ManagedRoot ||
		normalizedDataRoot != filepath.Clean(requestedDataRoot) ||
		(allowV1Migration && managedRoot != expectedManagedRoot) {
		return State{}, errors.New("invalid install-state roots")
	}
	return state, nil
}

func validStateCore(state stateWire) bool {
	if !versionPattern.MatchString(state.Release) ||
		!versionPattern.MatchString(state.CLIVersion) ||
		!versionPattern.MatchString(state.BundleVersion) ||
		(state.Channel != "canary" && state.Channel != "beta" && state.Channel != "stable") ||
		!supportedTarget(state.TargetOS, state.TargetArch) ||
		state.ActivatedAt.IsZero() {
		return false
	}
	if !validRoleState(state.RoleAuthority, state.MigrationID, state.CatalogSHA256, state.PolicySHA256) {
		return false
	}
	if state.Previous == nil {
		return true
	}
	return validRoleState(state.Previous.RoleAuthority, state.Previous.MigrationID, state.Previous.CatalogSHA256, state.Previous.PolicySHA256) &&
		versionPattern.MatchString(state.Previous.Release) &&
		versionPattern.MatchString(state.Previous.CLIVersion) &&
		versionPattern.MatchString(state.Previous.BundleVersion) &&
		(state.Previous.Channel == "canary" || state.Previous.Channel == "beta" || state.Previous.Channel == "stable") &&
		supportedTarget(state.Previous.TargetOS, state.Previous.TargetArch) &&
		filepath.IsAbs(state.Previous.CLIBackup)
}

func validRoleState(authority, migrationID, catalogSHA256, policySHA256 string) bool {
	if authority != "" && authority != rolemigration.CanonicalRole {
		return false
	}
	if migrationID == "" {
		return catalogSHA256 == "" && policySHA256 == ""
	}
	return migrationID == rolemigration.MigrationID && authority == rolemigration.CanonicalRole &&
		sha256Pattern.MatchString(catalogSHA256) && sha256Pattern.MatchString(policySHA256)
}

func stateFromWire(wire stateWire, managedRoot string) State {
	return State{
		SchemaVersion: wire.SchemaVersion,
		ManagedRoot:   managedRoot,
		Release:       wire.Release,
		Channel:       wire.Channel,
		CLIVersion:    wire.CLIVersion,
		BundleVersion: wire.BundleVersion,
		TargetOS:      wire.TargetOS,
		TargetArch:    wire.TargetArch,
		RoleAuthority: wire.RoleAuthority,
		MigrationID:   wire.MigrationID,
		CatalogSHA256: wire.CatalogSHA256,
		PolicySHA256:  wire.PolicySHA256,
		ActivatedAt:   wire.ActivatedAt,
		Previous:      wire.Previous,
	}
}

func statePath(dataRoot string) string {
	return filepath.Join(dataRoot, "config", "install-state.json")
}

func validatePlan(path string, plan ActivationPlan) error {
	if plan.SchemaVersion != 2 || !versionPattern.MatchString(plan.Release) ||
		!versionPattern.MatchString(plan.CLIVersion) || !versionPattern.MatchString(plan.BundleVersion) ||
		!sha256Pattern.MatchString(plan.ManifestSHA256) ||
		!sha256Pattern.MatchString(plan.CLISHA256) ||
		!sha256Pattern.MatchString(plan.BundleSHA256) ||
		plan.CLISize <= 0 || plan.BundleSize <= 0 ||
		filepath.Base(plan.CLIArtifactName) != plan.CLIArtifactName ||
		filepath.Base(plan.BundleArtifactName) != plan.BundleArtifactName {
		return errors.New("invalid activation plan versions")
	}
	if plan.RoleMigrationID != "" && (plan.RoleMigrationID != rolemigration.MigrationID ||
		!sha256Pattern.MatchString(plan.CatalogSHA256) || !sha256Pattern.MatchString(plan.PolicySHA256)) {
		return errors.New("invalid activation plan role migration binding")
	}
	if err := validateTransitionOptions(PrepareOptions{
		Transition:         plan.Transition,
		ConfirmationPlanID: plan.ConfirmationPlanID,
		FromRelease:        plan.FromRelease,
		FromChannel:        plan.FromChannel,
		FromCLIVersion:     plan.FromCLIVersion,
		FromBundleVersion:  plan.FromBundleVersion,
	}); err != nil {
		return fmt.Errorf("invalid activation transition: %w", err)
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
	if !within(transaction, plan.StagedCLI) || !within(transaction, plan.StagedBundleArchive) {
		return errors.New("staged activation paths escape their transaction")
	}
	if filepath.Clean(plan.StagedCLI) != filepath.Join(transaction, "bin", executableName(plan.TargetOS)) {
		return errors.New("staged CLI has an unexpected name")
	}
	if filepath.Clean(plan.StagedBundleArchive) != filepath.Join(transaction, "bundle.tar.gz") {
		return errors.New("staged bundle archive has an unexpected path")
	}
	return nil
}

func validateTransitionOptions(options PrepareOptions) error {
	switch options.Transition {
	case "install":
		if options.ConfirmationPlanID != "" ||
			options.FromRelease != "" ||
			options.FromChannel != "" ||
			options.FromCLIVersion != "" ||
			options.FromBundleVersion != "" {
			return errors.New("first installation cannot carry an update source binding")
		}
	case "update":
		if !confirmationPlanIDPattern.MatchString(options.ConfirmationPlanID) ||
			!versionPattern.MatchString(options.FromRelease) ||
			!versionPattern.MatchString(options.FromCLIVersion) ||
			!versionPattern.MatchString(options.FromBundleVersion) ||
			(options.FromChannel != "canary" &&
				options.FromChannel != "beta" &&
				options.FromChannel != "stable") {
			return errors.New("update transition requires an exact confirmed source-state binding")
		}
	default:
		return errors.New("activation transition must be install or update")
	}
	return nil
}

func releaseArtifacts(
	verified releaseverify.VerifiedRelease,
	targetOS, targetArch string,
) (*releasecontract.Artifact, *releasecontract.Artifact, *releasecontract.Artifact, error) {
	if !sha256Pattern.MatchString(verified.ManifestSHA256) {
		return nil, nil, nil, errors.New("verified release is missing its authenticated manifest digest")
	}
	var cliArtifact, bundleArtifact, runtimePack *releasecontract.Artifact
	for index := range verified.Manifest.Artifacts {
		artifact := &verified.Manifest.Artifacts[index]
		switch {
		case artifact.Kind == "cli" && artifact.OS == targetOS && artifact.Arch == targetArch:
			if cliArtifact != nil {
				return nil, nil, nil, errors.New("release has multiple CLI artifacts for the target")
			}
			cliArtifact = artifact
		case artifact.Kind == "bundle":
			if bundleArtifact != nil {
				return nil, nil, nil, errors.New("release has multiple base bundle artifacts")
			}
			bundleArtifact = artifact
		case artifact.Kind == "runtime_pack":
			if runtimePack != nil {
				return nil, nil, nil, errors.New("release has multiple runtime packs")
			}
			runtimePack = artifact
		}
	}
	if cliArtifact == nil || bundleArtifact == nil {
		return nil, nil, nil, errors.New("release does not contain the requested CLI and base bundle")
	}
	for _, artifact := range []*releasecontract.Artifact{cliArtifact, bundleArtifact} {
		if filepath.Base(artifact.Name) != artifact.Name ||
			!sha256Pattern.MatchString(artifact.SHA256) ||
			artifact.Size <= 0 {
			return nil, nil, nil, errors.New("release artifact has invalid authenticated metadata")
		}
	}
	if runtimePack != nil && (filepath.Base(runtimePack.Name) != runtimePack.Name || !sha256Pattern.MatchString(runtimePack.SHA256) || runtimePack.Size <= 0) {
		return nil, nil, nil, errors.New("runtime pack has invalid authenticated metadata")
	}
	return cliArtifact, bundleArtifact, runtimePack, nil
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
	if err := ensureSafePath(managed, managed); err != nil {
		return "", "", fmt.Errorf("managed root is unsafe: %w", err)
	}
	if err := ensureSafePath(data, data); err != nil {
		return "", "", fmt.Errorf("owner-data root is unsafe: %w", err)
	}
	return managed, data, nil
}

// ensureSafePath enforces the physical boundary of a trusted root. Lexical
// containment alone is insufficient when a pre-existing symlink or junction
// can redirect an install operation after validation.
func ensureSafePath(root, candidate string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return err
	}
	root, candidate = filepath.Clean(root), filepath.Clean(candidate)
	if !within(root, candidate) {
		return errors.New("path escapes its trusted root")
	}
	// A root may not exist yet. Walk to the nearest existing parent before
	// validating the lexical components; otherwise a symlinked parent can make
	// MkdirAll create the eventual root outside the physical boundary.
	nearest := filepath.Dir(candidate)
	for {
		info, statErr := os.Lstat(nearest)
		if statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("path ancestor is a symlink or junction: %s", nearest)
			}
			if !info.IsDir() {
				return fmt.Errorf("path ancestor is not a directory: %s", nearest)
			}
			break
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		parent := filepath.Dir(nearest)
		if parent == nearest {
			return errors.New("path has no existing trusted ancestor")
		}
		nearest = parent
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return err
	}
	current := root
	rootInfo, statErr := os.Lstat(current)
	if statErr == nil {
		if rootInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("trusted root is a symlink or junction: %s", current)
		}
		if !rootInfo.IsDir() {
			return fmt.Errorf("trusted root is not a directory: %s", current)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path component is a symlink or junction: %s", current)
		}
		if current != candidate && !info.IsDir() {
			return fmt.Errorf("path component is not a directory: %s", current)
		}
	}
	return nil
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

type digestCounter struct {
	hash hash.Hash
	size int64
}

func (counter *digestCounter) Write(body []byte) (int, error) {
	written, err := counter.hash.Write(body)
	counter.size += int64(written)
	return written, err
}

func copyVerifiedRegular(source, target string, mode os.FileMode, expectedSize int64, expectedSHA256 string) error {
	input, err := openRegular(source, expectedSize)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	success := false
	defer func() {
		if !success {
			_ = os.Remove(target)
		}
	}()
	counter := &digestCounter{hash: sha256.New()}
	written, copyErr := io.Copy(io.MultiWriter(output, counter), input)
	if copyErr != nil {
		output.Close()
		return copyErr
	}
	if written != expectedSize ||
		counter.size != expectedSize ||
		fmt.Sprintf("%x", counter.hash.Sum(nil)) != expectedSHA256 {
		output.Close()
		return errors.New("staged artifact does not match its authenticated size and digest")
	}
	if err := output.Sync(); err != nil {
		output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	success = true
	return nil
}

func verifyRegularFile(path string, expectedSize int64, expectedSHA256 string) error {
	file, err := openRegular(path, expectedSize)
	if err != nil {
		return err
	}
	defer file.Close()
	counter := &digestCounter{hash: sha256.New()}
	if _, err := io.Copy(counter, file); err != nil {
		return err
	}
	if counter.size != expectedSize || fmt.Sprintf("%x", counter.hash.Sum(nil)) != expectedSHA256 {
		return errors.New("file does not match its authenticated size and digest")
	}
	return nil
}

func extractVerifiedBundleArchive(
	source, target string,
	expectedSize int64,
	expectedSHA256 string,
) error {
	file, err := openRegular(source, expectedSize)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		return err
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(target)
		}
	}()
	counter := &digestCounter{hash: sha256.New()}
	authenticatedReader := io.TeeReader(file, counter)
	if err := extractBundle(authenticatedReader, target); err != nil {
		return err
	}
	if _, err := io.Copy(io.Discard, authenticatedReader); err != nil {
		return err
	}
	if counter.size != expectedSize || fmt.Sprintf("%x", counter.hash.Sum(nil)) != expectedSHA256 {
		return errors.New("bundle archive does not match its authenticated size and digest")
	}
	success = true
	return nil
}

func openRegular(path string, expectedSize int64) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != expectedSize {
		return nil, fmt.Errorf("install source must be a regular file with authenticated size: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !openedInfo.Mode().IsRegular() ||
		openedInfo.Size() != expectedSize ||
		!os.SameFile(info, openedInfo) {
		file.Close()
		return nil, fmt.Errorf("install source changed while opening: %s", path)
	}
	return file, nil
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

type treeEntryIdentity struct {
	Kind   string
	Size   int64
	SHA256 string
}

func equalTrees(left, right string) (bool, error) {
	leftEntries, err := treeIdentity(left)
	if err != nil {
		return false, err
	}
	rightEntries, err := treeIdentity(right)
	if err != nil {
		return false, err
	}
	if len(leftEntries) != len(rightEntries) {
		return false, nil
	}
	for path, leftEntry := range leftEntries {
		if rightEntries[path] != leftEntry {
			return false, nil
		}
	}
	return true, nil
}

func treeIdentity(root string) (map[string]treeEntryIdentity, error) {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("bundle identity root must be a directory")
	}
	entries := map[string]treeEntryIdentity{}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("bundle identity rejects symlink %s", relative)
		}
		if entry.IsDir() {
			entries[relative] = treeEntryIdentity{Kind: "directory"}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("bundle identity rejects special entry %s", relative)
		}
		file, err := openRegular(path, info.Size())
		if err != nil {
			return err
		}
		counter := &digestCounter{hash: sha256.New()}
		_, copyErr := io.Copy(counter, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		entries[relative] = treeEntryIdentity{
			Kind: "file", Size: counter.size,
			SHA256: fmt.Sprintf("%x", counter.hash.Sum(nil)),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
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

func acquireReconciliationLock(managedRoot string) (func(), error) {
	unlock, err := acquireLock(managedRoot)
	if err == nil {
		return unlock, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	path := filepath.Join(managedRoot, ".activation.lock")
	body, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, fmt.Errorf("read interrupted activation lock: %w", readErr)
	}
	if len(body) == 0 || len(body) > 32 || !strings.HasSuffix(string(body), "\n") {
		return nil, errors.New("interrupted activation lock is malformed")
	}
	pid, parseErr := strconv.Atoi(strings.TrimSuffix(string(body), "\n"))
	if parseErr != nil || pid <= 0 || pid == os.Getpid() {
		return nil, errors.New("interrupted activation lock has an invalid owner")
	}
	if waitErr := processwait.UntilExit(pid, 100*time.Millisecond); waitErr != nil {
		return nil, fmt.Errorf("activation lock owner may still be running: %w", waitErr)
	}
	currentBody, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(currentBody, body) {
		return nil, errors.New("activation lock changed during stale-owner verification")
	}
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("remove stale activation lock: %w", err)
	}
	return acquireLock(managedRoot)
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
