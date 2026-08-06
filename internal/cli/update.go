package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateplan"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
)

var AuthorityRegistrySHA256 = ""

var updatePlanIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var authorityRegistryDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

var (
	errReleaseUnavailable     = errors.New("private release capability is unavailable")
	errAuthenticationRequired = errors.New("private release authentication is required")
	errNoPendingUpdate        = errors.New("no matching pending update exists")
)

type releaseUpdateService interface {
	Check(context.Context) (updateservice.Pending, error)
	Confirm(string) (updateservice.Pending, error)
}

type releaseUpdateResult struct {
	SchemaVersion        int                       `json:"schema_version"`
	Capability           string                    `json:"capability"`
	State                string                    `json:"state"`
	Reason               string                    `json:"reason,omitempty"`
	ConfirmationRequired bool                      `json:"confirmation_required"`
	PlanID               string                    `json:"plan_id,omitempty"`
	Plan                 *updateplan.Plan          `json:"plan,omitempty"`
	WorkspaceMigration   *workspaceMigrationResult `json:"workspace_migration,omitempty"`
	NextAction           string                    `json:"next_action,omitempty"`
}

type workspaceMigrationResult struct {
	Required  bool   `json:"required"`
	Version   int    `json:"version"`
	State     string `json:"state"`
	Execution string `json:"execution"`
	Reason    string `json:"reason"`
}

func workspaceMigrationForPlan(plan updateplan.Plan) *workspaceMigrationResult {
	return &workspaceMigrationResult{
		Required: plan.WorkspaceMigrationRequired, Version: plan.WorkspaceMigrationVersion,
		State: plan.WorkspaceMigrationState, Execution: plan.WorkspaceMigrationExecution,
		Reason: "post-bootstrap workspace target and trusted core-activation verifier are not wired into bcgos update",
	}
}

type releaseCapabilityInspection struct {
	State  string
	Reason string
}

type unavailableReleaseUpdateService struct {
	reason string
}

func (service unavailableReleaseUpdateService) Check(context.Context) (updateservice.Pending, error) {
	return updateservice.Pending{}, fmt.Errorf("%w: %s", errReleaseUnavailable, service.reason)
}

func (service unavailableReleaseUpdateService) Confirm(string) (updateservice.Pending, error) {
	return updateservice.Pending{}, fmt.Errorf("%w: %s", errReleaseUnavailable, service.reason)
}

type configuredReleaseUpdateService struct {
	config          releaseprovider.Config
	auth            releaseprovider.AuthService
	dataRoot        func() (string, error)
	executable      func() (string, error)
	targetOS        string
	targetArch      string
	processID       func() int
	now             func() time.Time
	registrySHA256  string
	loadRegistry    func(string, string, func() time.Time) (releaseverify.KeyRegistry, error)
	provider        func(releaseprovider.Config, string) updateservice.Provider
	check           func(context.Context, updateservice.CheckOptions) (updateservice.CheckResult, error)
	stage           func(string, installtx.State, updateservice.CheckResult, time.Time) (updateservice.Pending, error)
	loadPending     func(string) (updateservice.Pending, error)
	validatePending func(string, string, string, releaseverify.KeyRegistry) (updateservice.Pending, error)
	startActivation func(string, string, string, int, time.Time) error
}

func defaultReleaseUpdateService() releaseUpdateService {
	config, err := releaseProviderConfig()
	if err != nil {
		return unavailableReleaseUpdateService{reason: "embedded release-provider configuration is invalid"}
	}
	if !config.Approved() {
		return unavailableReleaseUpdateService{reason: config.Reason}
	}
	return configuredReleaseUpdateService{
		config: config,
		auth:   config.AuthService(releaseprovider.NewNativeSecureStore),
		dataRoot: func() (string, error) {
			return defaultDataRoot()
		},
		executable:     os.Executable,
		targetOS:       runtime.GOOS,
		targetArch:     runtime.GOARCH,
		processID:      os.Getpid,
		now:            time.Now,
		registrySHA256: AuthorityRegistrySHA256,
		loadRegistry: func(path, digest string, clock func() time.Time) (releaseverify.KeyRegistry, error) {
			return releaseverify.LoadPinnedAuthorityRegistry(path, digest, clock)
		},
		provider: func(config releaseprovider.Config, token string) updateservice.Provider {
			return releaseprovider.GitHubProvider{
				APIBase: config.APIBase, Owner: config.Owner, Repository: config.Repository, Token: token,
			}
		},
		check:           updateservice.Check,
		stage:           updateservice.StagePending,
		loadPending:     updateservice.LoadPending,
		validatePending: updateservice.ValidatePendingLaunch,
		startActivation: startDetachedActivation,
	}
}

func defaultReleaseCapability() releaseCapabilityInspection {
	config, err := releaseProviderConfig()
	if err != nil {
		return releaseCapabilityInspection{
			State: "unavailable", Reason: "embedded release-provider configuration is invalid",
		}
	}
	return inspectReleaseCapability(
		config,
		AuthorityRegistrySHA256,
		releaseprovider.NewNativeSecureStore(),
	)
}

func inspectReleaseCapability(
	config releaseprovider.Config,
	registrySHA256 string,
	store releaseprovider.SecureStore,
) releaseCapabilityInspection {
	if !config.Approved() {
		return releaseCapabilityInspection{State: "unavailable", Reason: config.Reason}
	}
	if !authorityRegistryDigestPattern.MatchString(registrySHA256) {
		return releaseCapabilityInspection{
			State: "unavailable", Reason: "approved release-authority seed is not embedded in this build",
		}
	}
	if store == nil {
		return releaseCapabilityInspection{
			State: "unavailable", Reason: "operating-system credential store is unavailable",
		}
	}
	if err := store.Available(); err != nil {
		return releaseCapabilityInspection{
			State: "unavailable", Reason: "operating-system credential store is unavailable",
		}
	}
	return releaseCapabilityInspection{
		State:  "configured",
		Reason: "private release provider, authority seed and native credential store are configured",
	}
}

func runUpdate(args []string, out, errOut io.Writer, service releaseUpdateService) int {
	switch {
	case len(args) == 1 && args[0] == "--check":
		pending, err := service.Check(context.Background())
		if err != nil {
			return writeUpdateError(out, errOut, err, "")
		}
		plan := pending.Plan
		return writeJSON(out, releaseUpdateResult{
			SchemaVersion: 1, Capability: "private_release_update", State: "available",
			ConfirmationRequired: true, PlanID: plan.ID, Plan: &plan,
			WorkspaceMigration: workspaceMigrationForPlan(plan),
			NextAction:         "review the exact plan, then run bcgos update --confirm " + plan.ID,
		}, errOut)
	case len(args) == 2 && args[0] == "--confirm" && updatePlanIDPattern.MatchString(args[1]):
		pending, err := service.Confirm(args[1])
		if err != nil {
			return writeUpdateError(out, errOut, err, args[1])
		}
		return writeJSON(out, releaseUpdateResult{
			SchemaVersion: 1, Capability: "private_release_update", State: "activation_started",
			PlanID: pending.Plan.ID, WorkspaceMigration: workspaceMigrationForPlan(pending.Plan),
			NextAction: "the stable bootstrapper will activate the confirmed update after this CLI exits",
		}, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos update <--check|--confirm PLAN_ID>")
		return ExitUsage
	}
}

func writeUpdateError(out, errOut io.Writer, err error, planID string) int {
	result := releaseUpdateResult{
		SchemaVersion: 1, Capability: "private_release_update",
		State: "error", Reason: "update operation failed", PlanID: planID,
		NextAction: "run bcgos doctor and retry only after resolving the reported condition",
	}
	code := ExitFailure
	switch {
	case errors.Is(err, errReleaseUnavailable),
		errors.Is(err, releaseprovider.ErrSecureStoreUnavailable):
		result.State = "unavailable"
		result.Reason = "approved release provider, release authority or operating-system credential store is not configured"
		result.NextAction = "complete the approved release configuration before checking or applying updates"
		code = ExitUnavailable
	case errors.Is(err, errAuthenticationRequired),
		errors.Is(err, releaseprovider.ErrCredentialNotFound):
		result.State = "authentication_required"
		result.Reason = "private release authentication is required"
		result.NextAction = "run bcgos auth login, then retry the update check"
		code = ExitUnavailable
	case errors.Is(err, updateservice.ErrCurrent):
		result.State = "current"
		result.Reason = "the installed Maestro release is current"
		result.NextAction = ""
		code = ExitOK
	case errors.Is(err, errNoPendingUpdate):
		result.State = "no_pending_update"
		result.Reason = "the requested update plan is not pending"
		result.NextAction = "run bcgos update --check to create a new exact confirmation plan"
		code = ExitUnavailable
	default:
		fmt.Fprintln(errOut, "error:", err)
	}
	if writeCode := writeJSON(out, result, errOut); writeCode != ExitOK {
		return writeCode
	}
	return code
}

func (service configuredReleaseUpdateService) Check(ctx context.Context) (updateservice.Pending, error) {
	if !service.config.Approved() {
		return updateservice.Pending{}, fmt.Errorf("%w: %s", errReleaseUnavailable, service.config.Reason)
	}
	managedRoot, dataRoot, current, err := service.installedContext()
	if err != nil {
		return updateservice.Pending{}, err
	}
	if !authorityRegistryDigestPattern.MatchString(service.registrySHA256) {
		return updateservice.Pending{}, fmt.Errorf("%w: release authority seed is unavailable", errReleaseUnavailable)
	}
	registry, err := service.loadRegistry(
		filepath.Join(managedRoot, "trust", "release-authority-registry.json"),
		service.registrySHA256,
		service.now,
	)
	if err != nil {
		return updateservice.Pending{}, err
	}
	existing, err := service.loadPending(dataRoot)
	switch {
	case err == nil:
		validated, validateErr := service.validatePending(
			dataRoot,
			managedRoot,
			existing.Plan.ID,
			registry,
		)
		if validateErr != nil {
			return updateservice.Pending{}, validateErr
		}
		return validated, nil
	case !errors.Is(err, os.ErrNotExist):
		return updateservice.Pending{}, err
	}
	token, err := service.auth.AccessToken(ctx)
	if errors.Is(err, releaseprovider.ErrCredentialNotFound) {
		return updateservice.Pending{}, errAuthenticationRequired
	}
	if err != nil {
		return updateservice.Pending{}, err
	}
	result, err := service.check(ctx, updateservice.CheckOptions{
		Current: current, TargetOS: service.targetOS, TargetArch: service.targetArch,
		StagingRoot: filepath.Join(dataRoot, "updates"),
		Provider:    service.provider(service.config, token),
		Registry:    registry,
	})
	if err != nil {
		return updateservice.Pending{}, err
	}
	pending, err := service.stage(dataRoot, current, result, service.now().UTC())
	if err != nil {
		if !pathWithin(filepath.Join(dataRoot, "updates"), result.Verified.Directory) {
			return updateservice.Pending{}, err
		}
		if cleanupErr := os.RemoveAll(result.Verified.Directory); cleanupErr != nil {
			return updateservice.Pending{}, fmt.Errorf("%w; clean checked release: %v", err, cleanupErr)
		}
		return updateservice.Pending{}, err
	}
	return pending, nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != "." && relative != ".." &&
		!filepath.IsAbs(relative) &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (service configuredReleaseUpdateService) Confirm(planID string) (updateservice.Pending, error) {
	if !updatePlanIDPattern.MatchString(planID) {
		return updateservice.Pending{}, errNoPendingUpdate
	}
	managedRoot, dataRoot, current, err := service.installedContext()
	if err != nil {
		return updateservice.Pending{}, err
	}
	pending, err := service.loadPending(dataRoot)
	if errors.Is(err, os.ErrNotExist) {
		return updateservice.Pending{}, errNoPendingUpdate
	}
	if err != nil {
		return updateservice.Pending{}, err
	}
	if pending.Plan.ID != planID ||
		pending.Plan.TargetOS != current.TargetOS ||
		pending.Plan.TargetArch != current.TargetArch {
		return updateservice.Pending{}, errNoPendingUpdate
	}
	if !authorityRegistryDigestPattern.MatchString(service.registrySHA256) {
		return updateservice.Pending{}, fmt.Errorf("%w: release authority seed is unavailable", errReleaseUnavailable)
	}
	registry, err := service.loadRegistry(
		filepath.Join(managedRoot, "trust", "release-authority-registry.json"),
		service.registrySHA256,
		service.now,
	)
	if err != nil {
		return updateservice.Pending{}, err
	}
	validated, err := service.validatePending(dataRoot, managedRoot, planID, registry)
	if err != nil {
		return updateservice.Pending{}, err
	}
	if err := service.startActivation(
		bootstrapperPath(managedRoot, current.TargetOS),
		dataRoot,
		planID,
		service.processID(),
		service.now().UTC(),
	); err != nil {
		return updateservice.Pending{}, err
	}
	return validated, nil
}

func (service configuredReleaseUpdateService) installedContext() (string, string, installtx.State, error) {
	executable, err := service.executable()
	if err != nil {
		return "", "", installtx.State{}, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", "", installtx.State{}, err
	}
	managedRoot, err := managedRootFromCLIExecutablePath(executable)
	if err != nil {
		return "", "", installtx.State{}, err
	}
	dataRoot, err := service.dataRoot()
	if err != nil {
		return "", "", installtx.State{}, err
	}
	dataRoot, err = filepath.Abs(dataRoot)
	if err != nil {
		return "", "", installtx.State{}, err
	}
	dataRoot = filepath.Clean(dataRoot)
	current, err := installtx.ReadStateForManagedRoot(dataRoot, managedRoot)
	if err != nil {
		return "", "", installtx.State{}, err
	}
	if current.TargetOS != service.targetOS || current.TargetArch != service.targetArch {
		return "", "", installtx.State{}, errors.New("installed state does not match the running platform")
	}
	return managedRoot, dataRoot, current, nil
}

func managedRootFromCLIExecutablePath(executable string) (string, error) {
	name := filepath.Base(executable)
	if name != "bcgos" && name != "bcgos.exe" {
		return "", errors.New("CLI executable does not have its protected installed name")
	}
	bin := filepath.Dir(executable)
	if filepath.Base(bin) != "bin" {
		return "", errors.New("CLI executable is outside its protected bin directory")
	}
	root := filepath.Dir(bin)
	if !filepath.IsAbs(root) ||
		filepath.Clean(root) == filepath.Clean(bin) ||
		filepath.Dir(filepath.Clean(root)) == filepath.Clean(root) {
		return "", errors.New("CLI managed root is invalid")
	}
	return filepath.Clean(root), nil
}

func bootstrapperPath(managedRoot, targetOS string) string {
	name := "bcgos-bootstrap"
	if targetOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(managedRoot, name)
}

func startDetachedActivation(
	bootstrapper, dataRoot, planID string,
	waitPID int,
	startedAt time.Time,
) error {
	if waitPID <= 0 || !updatePlanIDPattern.MatchString(planID) || startedAt.IsZero() {
		return errors.New("detached activation requires exact process and plan identities")
	}
	info, err := os.Lstat(bootstrapper)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("stable bootstrapper must be a regular file")
	}
	logRoot := filepath.Join(dataRoot, "logs")
	if err := os.MkdirAll(logRoot, 0o700); err != nil {
		return err
	}
	logPath := filepath.Join(
		logRoot,
		"update-"+planID+"-"+strconv.FormatInt(startedAt.UnixNano(), 10)+".log",
	)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	command := exec.Command(
		bootstrapper,
		"activate",
		"--plan-id", planID,
		"--data-root", dataRoot,
		"--wait-pid", strconv.Itoa(waitPID),
	)
	command.Stdout = logFile
	command.Stderr = logFile
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		_ = os.Remove(logPath)
		return err
	}
	if err := logFile.Close(); err != nil {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
		return err
	}
	if err := command.Process.Release(); err != nil {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
		return err
	}
	return nil
}
