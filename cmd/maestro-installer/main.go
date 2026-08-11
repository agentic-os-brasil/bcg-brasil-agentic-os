// Command maestro-installer is the visual, user-space entry point for a
// signed release package. Release content always remains Ed25519 verified;
// native trust is either strict, an owner-directed canary-simple diagnostic
// profile, or a factory-pinned Windows Canary profile.
package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceagent"
)

// These values are immutable package metadata. Release factories may replace
// them with -ldflags; there is deliberately no runtime flag that weakens trust.
var (
	BuildTrustProfile                = "strict"
	BuildLocalBetaIssuer             = ""
	BuildLocalBetaKeyID              = ""
	BuildLocalBetaRegistrySHA256     = ""
	BuildLocalBetaBootstrapperSHA256 = ""
)

type options struct {
	releaseDir             string
	bootstrapper           string
	authorityRegistry      string
	managedRoot            string
	dataRoot               string
	wizardDir              string
	headless               bool
	preview                bool
	simulate               bool
	simulationRoot         string
	primaryRuntime         string
	sessionToken           string
	origin                 string
	shutdown               func()
	shutdownGraceful       func(context.Context) error
	lifecycle              *wizardLifecycle
	chooseWorkspace        func() (string, error)
	launchRuntime          func(runtimeID, workspacePath string) error
	runtimeTargets         func() []runtimeTarget
	chooseImportSource     func() (string, error)
	chooseWorkspaceSource  func(workspaceFlowMode) (string, error)
	workspacePath          func() (string, error)
	configureWorkspace     func(options, string) (workspaceActivation, error)
	authorizeSetup         func(options, string) (workspaceSetupAuthorization, error)
	commandRunner          commandRunner
	workspaceFlow          workspaceFlowBackend
	nativeTrustMode        installer.NativeTrustMode
	localBetaPins          installer.LocalBetaPins
	allowUnqualifiedNative bool
}

type runtimeTarget struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type workspaceHandoff struct {
	WorkspacePath    string            `json:"workspace_path"`
	WorkspaceID      string            `json:"workspace_id"`
	Prompt           string            `json:"prompt"`
	DeepLinks        map[string]string `json:"deeplinks"`
	RuntimePaths     map[string]string `json:"runtime_paths"`
	RuntimeAvailable map[string]bool   `json:"runtime_available"`
	Diagnostics      []string          `json:"diagnostics"`
}

type commandRunner interface {
	Run(context.Context, string, []string) ([]byte, error)
}

type commandRunnerFunc func(context.Context, string, []string) ([]byte, error)

func (function commandRunnerFunc) Run(ctx context.Context, executable string, arguments []string) ([]byte, error) {
	return function(ctx, executable, arguments)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, executable string, arguments []string) ([]byte, error) {
	return exec.CommandContext(ctx, executable, arguments...).CombinedOutput()
}

type workspaceActivation struct {
	State       string                `json:"state"`
	WorkspaceID string                `json:"workspace_id"`
	Lifecycle   lifecycleActivation   `json:"lifecycle"`
	Maintenance maintenanceActivation `json:"maintenance"`
}

type workspaceSetupAuthorization struct {
	State       string `json:"state"`
	GrantDigest string `json:"grant_digest,omitempty"`
}

type lifecycleActivation struct {
	Runtime        string   `json:"runtime"`
	State          string   `json:"state"`
	Events         []string `json:"events"`
	StartSession   string   `json:"start_session"`
	HookReview     string   `json:"hook_review"`
	NativeObserved string   `json:"native_observed"`
}

type maintenanceActivation struct {
	State           string `json:"state"`
	Schedule        string `json:"schedule"`
	NativeObserved  bool   `json:"native_observed"`
	NativeQualified bool   `json:"native_qualified"`
	ModelBacked     string `json:"model_backed"`
}

type wizardLifecycle struct {
	mu            sync.Mutex
	closing       bool
	active        int
	drained       chan struct{}
	drainedClosed bool
}

func newWizardLifecycle() *wizardLifecycle {
	return &wizardLifecycle{drained: make(chan struct{})}
}

func (lifecycle *wizardLifecycle) beginMutation() bool {
	if lifecycle == nil {
		return true
	}
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if lifecycle.closing {
		return false
	}
	lifecycle.active++
	return true
}

func (lifecycle *wizardLifecycle) endMutation() {
	if lifecycle == nil {
		return
	}
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if lifecycle.active > 0 {
		lifecycle.active--
	}
	if lifecycle.closing && lifecycle.active == 0 && !lifecycle.drainedClosed {
		close(lifecycle.drained)
		lifecycle.drainedClosed = true
	}
}

func (lifecycle *wizardLifecycle) requestClose() bool {
	if lifecycle == nil {
		return true
	}
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if lifecycle.closing {
		return false
	}
	lifecycle.closing = true
	if lifecycle.active == 0 && !lifecycle.drainedClosed {
		close(lifecycle.drained)
		lifecycle.drainedClosed = true
	}
	return true
}

func (lifecycle *wizardLifecycle) waitDrained(ctx context.Context) error {
	if lifecycle == nil {
		return nil
	}
	select {
	case <-lifecycle.drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func main() {
	options := parseOptions()
	mode, pins, err := resolveBuildTrustProfile(
		BuildTrustProfile, BuildLocalBetaIssuer, BuildLocalBetaKeyID,
		BuildLocalBetaRegistrySHA256, BuildLocalBetaBootstrapperSHA256,
	)
	if err != nil {
		writeError(err)
		os.Exit(2)
	}
	options.nativeTrustMode = mode
	options.localBetaPins = pins
	// Only the explicitly compiled canary-simple macOS profile may activate
	// before hook evidence promotes the runtime to native-qualified. Strict
	// builds keep the qualification gate unchanged.
	options.allowUnqualifiedNative = BuildTrustProfile == string(installer.NativeTrustCanarySimple) && runtime.GOOS == "darwin"
	if options.preview {
		if err := resolvePreviewDefaults(&options); err != nil {
			writeError(err)
			os.Exit(2)
		}
		servePreview(options.wizardDir)
		return
	}
	if options.simulate {
		if err := resolveSimulationDefaults(&options); err != nil {
			writeError(err)
			os.Exit(2)
		}
		serveWizard(options)
		return
	}
	if err := resolveDefaults(&options); err != nil {
		writeError(err)
		os.Exit(2)
	}
	if options.headless {
		result, err := installer.Install(context.Background(), installerOptions(options))
		if err != nil {
			writeError(err)
			os.Exit(1)
		}
		writeJSON(result)
		return
	}
	if options.wizardDir == "" {
		writeError(fmt.Errorf("--wizard-dir is required for visual mode"))
		os.Exit(2)
	}
	serveWizard(options)
}

func resolveBuildTrustProfile(profile, issuer, keyID, registrySHA256, bootstrapperSHA256 string) (installer.NativeTrustMode, installer.LocalBetaPins, error) {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		profile = string(installer.NativeTrustStrict)
	}
	pins := installer.LocalBetaPins{
		AuthorityRegistrySHA256: registrySHA256,
		BootstrapperSHA256:      bootstrapperSHA256,
		Issuer:                  issuer,
		KeyID:                   keyID,
	}
	switch installer.NativeTrustMode(profile) {
	case installer.NativeTrustCanarySimple:
		if issuer != "" || keyID != "" || registrySHA256 != "" || bootstrapperSHA256 != "" {
			return "", installer.LocalBetaPins{}, errors.New("canary-simple build trust profile must not carry local-beta pins")
		}
		return installer.NativeTrustCanarySimple, installer.LocalBetaPins{}, nil
	case installer.NativeTrustStrict:
		if issuer != "" || keyID != "" || registrySHA256 != "" || bootstrapperSHA256 != "" {
			return "", installer.LocalBetaPins{}, errors.New("strict build trust profile must not carry local-beta pins")
		}
		return installer.NativeTrustStrict, installer.LocalBetaPins{}, nil
	case installer.NativeTrustWindowsLocalBeta:
		if strings.TrimSpace(issuer) == "" || strings.TrimSpace(keyID) == "" ||
			!isCanonicalSHA256(registrySHA256) || !isCanonicalSHA256(bootstrapperSHA256) {
			return "", installer.LocalBetaPins{}, errors.New("windows-local-beta build trust profile requires issuer, key ID and exact lowercase registry/bootstrapper SHA-256 pins")
		}
		return installer.NativeTrustWindowsLocalBeta, pins, nil
	default:
		return "", installer.LocalBetaPins{}, fmt.Errorf("unsupported build trust profile %q", profile)
	}
}

func isCanonicalSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func parseOptions() options {
	flags := flag.NewFlagSet("maestro-installer", flag.ExitOnError)
	var result options
	flags.StringVar(&result.releaseDir, "release-dir", "", "exact signed release directory")
	flags.StringVar(&result.bootstrapper, "bootstrapper", "", "native-signed seeded bootstrapper")
	flags.StringVar(&result.authorityRegistry, "authority-registry", "", "approved public authority registry")
	flags.StringVar(&result.managedRoot, "managed-root", "", "optional managed root override")
	flags.StringVar(&result.dataRoot, "data-root", "", "optional owner-data root override")
	flags.StringVar(&result.wizardDir, "wizard-dir", "", "directory containing the Maestro wizard assets")
	flags.BoolVar(&result.headless, "headless", false, "install without opening the visual wizard")
	flags.BoolVar(&result.preview, "preview", false, "open the non-mutating visual preview without release inputs")
	flags.BoolVar(&result.simulate, "simulate", false, "run a local technical installation rehearsal in an isolated user-space sandbox")
	flags.StringVar(&result.simulationRoot, "simulation-root", "", "optional empty root for the technical installation rehearsal")
	flags.StringVar(&result.primaryRuntime, "runtime", "claude", "primary workspace runtime: claude or codex")
	flags.Parse(os.Args[1:])
	if flags.NArg() != 0 {
		writeError(fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " ")))
		os.Exit(2)
	}
	return result
}

func resolvePreviewDefaults(options *options) error {
	if options == nil {
		return fmt.Errorf("installer options are required")
	}
	if options.wizardDir != "" {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve installer location: %w", err)
	}
	options.wizardDir = filepath.Join(filepath.Dir(executable), "wizard")
	return nil
}

func resolveDefaults(options *options) error {
	if options == nil {
		return fmt.Errorf("installer options are required")
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve installer location: %w", err)
	}
	packageRoot := filepath.Dir(executable)
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home: %w", err)
	}
	return resolveDefaultsAt(options, packageRoot, runtime.GOOS, runtime.GOARCH, home, os.Getenv("LOCALAPPDATA"))
}

func resolveSimulationDefaults(options *options) error {
	if options == nil {
		return fmt.Errorf("installer options are required")
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve installer location: %w", err)
	}
	if options.wizardDir == "" {
		options.wizardDir = filepath.Join(filepath.Dir(executable), "wizard")
	}
	if options.simulationRoot == "" {
		options.simulationRoot, err = os.MkdirTemp("", "maestro-install-rehearsal-")
		if err != nil {
			return fmt.Errorf("create rehearsal sandbox: %w", err)
		}
	} else {
		options.simulationRoot, err = filepath.Abs(options.simulationRoot)
		if err != nil {
			return fmt.Errorf("normalize rehearsal sandbox: %w", err)
		}
		if err := os.MkdirAll(options.simulationRoot, 0o700); err != nil {
			return fmt.Errorf("create rehearsal sandbox: %w", err)
		}
	}
	if options.managedRoot == "" {
		options.managedRoot = filepath.Join(options.simulationRoot, "managed")
	}
	if options.dataRoot == "" {
		options.dataRoot = filepath.Join(options.simulationRoot, "data")
	}
	return nil
}

func resolveDefaultsAt(options *options, packageRoot, platform, architecture, home, localAppData string) error {
	if options.wizardDir == "" {
		options.wizardDir = filepath.Join(packageRoot, "wizard")
	}
	if options.releaseDir == "" {
		options.releaseDir = filepath.Join(packageRoot, "release")
	}
	if options.authorityRegistry == "" {
		options.authorityRegistry = filepath.Join(packageRoot, "authority-registry.json")
	}
	if options.bootstrapper == "" {
		pattern := filepath.Join(packageRoot, "bcgos-bootstrap_*_"+platform+"_"+architecture)
		if platform == "windows" {
			pattern += ".exe"
		}
		matches, globErr := filepath.Glob(pattern)
		if globErr != nil {
			return fmt.Errorf("resolve bootstrapper package input: %w", globErr)
		}
		if len(matches) != 1 {
			return fmt.Errorf("package must contain exactly one native bootstrapper matching %s", pattern)
		}
		options.bootstrapper = matches[0]
	}
	if err := validateInstallerPackageInputs(options); err != nil {
		return err
	}
	if options.managedRoot == "" || options.dataRoot == "" {
		paths, pathErr := installer.DefaultPaths(platform, home, localAppData)
		if pathErr != nil {
			return pathErr
		}
		if options.managedRoot == "" {
			options.managedRoot = paths.ManagedRoot
		}
		if options.dataRoot == "" {
			options.dataRoot = paths.DataRoot
		}
	}
	return nil
}

func validateInstallerPackageInputs(options *options) error {
	if options == nil {
		return fmt.Errorf("installer options are required")
	}
	for _, input := range []struct {
		path      string
		label     string
		directory bool
	}{
		{options.wizardDir, "wizard directory", true},
		{options.releaseDir, "release directory", true},
		{options.authorityRegistry, "authority registry", false},
		{options.bootstrapper, "native bootstrapper", false},
	} {
		info, err := os.Lstat(input.path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("installer_package_incomplete: %s is missing", input.label)
			}
			return fmt.Errorf("installer_package_incomplete: inspect %s: %w", input.label, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("installer_package_incomplete: %s must not be a symlink", input.label)
		}
		if input.directory && !info.IsDir() {
			return fmt.Errorf("installer_package_incomplete: %s is not a directory", input.label)
		}
		if !input.directory && !info.Mode().IsRegular() {
			return fmt.Errorf("installer_package_incomplete: %s is not a regular file", input.label)
		}
	}
	return nil
}

func installerOptions(options options) installer.Options {
	return installer.Options{
		ReleaseDir:        options.releaseDir,
		Bootstrapper:      options.bootstrapper,
		AuthorityRegistry: options.authorityRegistry,
		ManagedRoot:       options.managedRoot,
		DataRoot:          options.dataRoot,
		TargetOS:          runtime.GOOS,
		TargetArch:        runtime.GOARCH,
		NativeTrustMode:   options.nativeTrustMode,
		LocalBetaPins:     options.localBetaPins,
	}
}

func serveWizard(options options) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		writeError(err)
		os.Exit(1)
	}
	origin := "http://" + listener.Addr().String()
	sessionToken, err := newSessionToken()
	if err != nil {
		writeError(err)
		os.Exit(2)
	}
	options.origin = origin
	options.sessionToken = sessionToken
	options.lifecycle = newWizardLifecycle()
	server := &http.Server{}
	options.shutdown = func() {
		_ = server.Close()
	}
	options.shutdownGraceful = server.Shutdown
	mux := wizardHandler(options)
	server.Handler = mux
	launchURL := origin + "/?session=" + url.QueryEscape(sessionToken)
	label := "Maestro installer wizard"
	if options.simulate {
		label = "Maestro installer rehearsal"
	}
	fmt.Println(label + ": " + launchURL)
	openBrowser(launchURL)
	if err := server.Serve(listener); err != nil && !strings.Contains(err.Error(), "Server closed") {
		writeError(err)
		os.Exit(1)
	}
}

func servePreview(wizardDir string) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		writeError(err)
		os.Exit(1)
	}
	url := "http://" + listener.Addr().String() + "/"
	fmt.Println("Maestro installer preview: " + url)
	openBrowser(url)
	if err := http.Serve(listener, http.FileServer(http.Dir(wizardDir))); err != nil && !strings.Contains(err.Error(), "Server closed") {
		writeError(err)
		os.Exit(1)
	}
}

func wizardHandler(options options) http.Handler {
	fileServer := http.FileServer(http.Dir(options.wizardDir))
	mux := http.NewServeMux()
	var stateMu sync.Mutex
	var verifiedPlan *installer.Plan
	workspaceFlow := workspaceFlowBackendFor(options)
	workspaceSelections := make(map[string]workspaceFlowSelection)
	workspaceAnalyses := make(map[string]workspaceFlowAnalysis)
	mux.Handle("/", fileServer)
	mux.HandleFunc("/api/state", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		installedCLI := installedCLIPath(options)
		trust := "pending"
		if options.simulate {
			trust = "simulation"
		} else if options.nativeTrustMode == installer.NativeTrustWindowsLocalBeta {
			trust = "windows_local_beta"
		}
		configuredRuntime, runtimeErr := primaryRuntime(options)
		if runtimeErr != nil {
			writeHTTPJSONStatus(writer, http.StatusInternalServerError, map[string]string{"error": runtimeErr.Error()})
			return
		}
		writeHTTPJSON(writer, map[string]any{
			"platform": runtime.GOOS, "architecture": runtime.GOARCH,
			"release_dir": options.releaseDir, "managed_root": options.managedRoot,
			"data_root": options.dataRoot, "trust": trust,
			"local_beta": options.nativeTrustMode == installer.NativeTrustWindowsLocalBeta,
			"mode":       map[bool]string{true: "simulation", false: "runtime"}[options.simulate],
			"installed":  installedCLI != "", "cli_path": installedCLI, "installed_version": installedReleaseVersion(options),
			"primary_runtime":   configuredRuntime,
			"runtimes":          availableRuntimeTargets(options),
			"workspace_default": defaultWorkspaceFor(options),
			"workspace_flow": map[string]any{
				"backend": workspaceFlowBackendName(options),
				"modes": []workspaceFlowMode{
					workspaceFlowModeUpdate,
					workspaceFlowModeWorkspaceMigration,
					workspaceFlowModeExternalImport,
				},
			},
		})
	})
	mux.HandleFunc("/api/workspace-flow/select", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !authorizeMutation(writer, request, options) {
			return
		}
		if options.lifecycle != nil && !options.lifecycle.beginMutation() {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": "o instalador está sendo encerrado"})
			return
		}
		defer options.lifecycle.endMutation()
		request.Body = http.MaxBytesReader(writer, request.Body, 4<<10)
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		var payload struct {
			Mode workspaceFlowMode `json:"mode"`
		}
		if err := decoder.Decode(&payload); err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": "não foi possível entender o caminho escolhido"})
			return
		}
		if err := validateWorkspaceFlowMode(payload.Mode); err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		var sourcePath string
		if payload.Mode != workspaceFlowModeUpdate {
			if options.simulate && options.chooseWorkspaceSource == nil && options.chooseImportSource == nil {
				sourcePath = filepath.Join(options.simulationRoot, "fixture-external-source")
			} else {
				chooser := options.chooseWorkspaceSource
				if chooser == nil {
					chooser = func(workspaceFlowMode) (string, error) {
						if options.chooseImportSource != nil {
							return options.chooseImportSource()
						}
						return chooseImportSource()
					}
				}
				var err error
				sourcePath, err = chooser(payload.Mode)
				if err != nil {
					writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("não foi possível selecionar a fonte: %v", err)})
					return
				}
			}
			if strings.TrimSpace(sourcePath) == "" {
				writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": "nenhuma fonte foi selecionada"})
				return
			}
		}
		source, err := workspaceFlowSourceFor(payload.Mode, sourcePath)
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		flowID, err := newSessionToken()
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusInternalServerError, map[string]any{"error": "não foi possível criar a sessão de análise"})
			return
		}
		selection := workspaceFlowSelection{SchemaVersion: workspaceFlowSchemaVersion, FlowID: flowID, Mode: payload.Mode, Source: source}
		stateMu.Lock()
		workspaceSelections[flowID] = selection
		stateMu.Unlock()
		writeHTTPJSON(writer, workspaceFlowSelectionResponse{workspaceFlowSelection: selection, Backend: workspaceFlowBackendName(options)})
	})
	mux.HandleFunc("/api/workspace-flow/analyze", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !authorizeMutation(writer, request, options) {
			return
		}
		if options.lifecycle != nil && !options.lifecycle.beginMutation() {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": "o instalador está sendo encerrado"})
			return
		}
		defer options.lifecycle.endMutation()
		flowID, err := decodeWorkspaceFlowID(writer, request)
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		stateMu.Lock()
		selection, ok := workspaceSelections[flowID]
		stateMu.Unlock()
		if !ok {
			writeHTTPJSONStatus(writer, http.StatusNotFound, map[string]any{"error": "a seleção do workspace expirou; escolha a fonte novamente"})
			return
		}
		analysis, err := workspaceFlow.Analyze(request.Context(), selection)
		if err != nil {
			writeWorkspaceFlowError(writer, err)
			return
		}
		if err := validateWorkspaceFlowAnalysis(analysis, selection); err != nil {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": err.Error(), "ready": false})
			return
		}
		stateMu.Lock()
		workspaceAnalyses[flowID] = analysis
		stateMu.Unlock()
		if analysis.State == "blocked" {
			status := http.StatusConflict
			for _, blocker := range analysis.Blockers {
				if strings.Contains(blocker.Code, "unavailable") {
					status = http.StatusServiceUnavailable
					break
				}
			}
			writeHTTPJSONStatus(writer, status, analysis)
			return
		}
		writeHTTPJSON(writer, analysis)
	})
	mux.HandleFunc("/api/workspace-flow/confirm", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !authorizeMutation(writer, request, options) {
			return
		}
		if options.lifecycle != nil && !options.lifecycle.beginMutation() {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": "o instalador está sendo encerrado"})
			return
		}
		defer options.lifecycle.endMutation()
		flowID, planDigest, action, err := decodeWorkspaceFlowConfirmation(writer, request)
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		stateMu.Lock()
		selection, selected := workspaceSelections[flowID]
		analysis, analyzed := workspaceAnalyses[flowID]
		stateMu.Unlock()
		if !selected || !analyzed {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": "analise a fonte antes de confirmar", "ready": false})
			return
		}
		if !analysis.CanConfirm || analysis.ApprovalAction != workspaceFlowApprovalImport {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": "este plano está bloqueado e não pode ser confirmado", "code": "blocked", "ready": false})
			return
		}
		if planDigest != analysis.PlanDigest {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": "o plano mudou; execute a análise novamente", "ready": false})
			return
		}
		receipt, err := workspaceFlow.Confirm(request.Context(), selection, planDigest, action)
		if err != nil {
			writeWorkspaceFlowError(writer, err)
			return
		}
		if err := validateWorkspaceFlowReceipt(receipt, selection, planDigest); err != nil {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": err.Error(), "code": "invalid_receipt", "ready": false})
			return
		}
		writeHTTPJSON(writer, receipt)
	})
	mux.HandleFunc("/api/workspace-flow/rollback", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !authorizeMutation(writer, request, options) {
			return
		}
		if options.lifecycle != nil && !options.lifecycle.beginMutation() {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": "o instalador está sendo encerrado"})
			return
		}
		defer options.lifecycle.endMutation()
		flowID, planDigest, receiptID, action, err := decodeWorkspaceFlowRollback(writer, request)
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		stateMu.Lock()
		selection, selected := workspaceSelections[flowID]
		stateMu.Unlock()
		if !selected {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": "a seleção do workspace expirou; escolha a fonte novamente", "ready": false})
			return
		}
		receipt, err := workspaceFlow.Rollback(request.Context(), selection, planDigest, receiptID, action)
		if err != nil {
			writeWorkspaceFlowError(writer, err)
			return
		}
		if err := validateWorkspaceFlowRollbackReceipt(receipt, selection, planDigest, receiptID); err != nil {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": err.Error(), "code": "invalid_receipt", "ready": false})
			return
		}
		writeHTTPJSON(writer, receipt)
	})
	mux.HandleFunc("/api/verify", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !authorizeMutation(writer, request, options) {
			return
		}
		if options.lifecycle != nil && !options.lifecycle.beginMutation() {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": "o instalador está sendo encerrado"})
			return
		}
		defer options.lifecycle.endMutation()
		if options.simulate {
			plan := simulationPlan(options)
			stateMu.Lock()
			verifiedPlan = &plan
			stateMu.Unlock()
			writeHTTPJSON(writer, plan)
			return
		}
		plan, _, err := installer.Prepare(installerOptions(options))
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		stateMu.Lock()
		verifiedPlan = &plan
		stateMu.Unlock()
		writeHTTPJSON(writer, plan)
	})
	mux.HandleFunc("/api/install", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !authorizeMutation(writer, request, options) {
			return
		}
		if options.lifecycle != nil && !options.lifecycle.beginMutation() {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": "o instalador está sendo encerrado"})
			return
		}
		defer options.lifecycle.endMutation()
		requestedDigest, err := decodePlanDigest(writer, request)
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		stateMu.Lock()
		plan := verifiedPlan
		stateMu.Unlock()
		if plan == nil || requestedDigest != plan.PlanDigest {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": "verifique o release novamente antes de instalar"})
			return
		}
		if options.simulate {
			result, err := installSimulation(options, *plan)
			if err != nil {
				writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
				return
			}
			stateMu.Lock()
			verifiedPlan = nil
			stateMu.Unlock()
			writeHTTPJSON(writer, result)
			return
		}
		installOptions := installerOptions(options)
		installOptions.ExpectedPlanDigest = requestedDigest
		result, err := installer.Install(request.Context(), installOptions)
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		stateMu.Lock()
		verifiedPlan = nil
		stateMu.Unlock()
		writeHTTPJSON(writer, result)
	})
	mux.HandleFunc("/api/open-data", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !authorizeMutation(writer, request, options) {
			return
		}
		if options.lifecycle != nil && !options.lifecycle.beginMutation() {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": "o instalador está sendo encerrado"})
			return
		}
		defer options.lifecycle.endMutation()
		if options.dataRoot == "" {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": "a pasta de dados do Maestro não está configurada"})
			return
		}
		info, err := os.Stat(options.dataRoot)
		if err != nil || !info.IsDir() {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": "a pasta de dados ainda não existe; execute bcgos doctor após a instalação"})
			return
		}
		if err := openPath(options.dataRoot); err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("não foi possível abrir a pasta de dados: %v", err)})
			return
		}
		writeHTTPJSON(writer, map[string]any{"path": options.dataRoot, "status": "opened"})
	})
	mux.HandleFunc("/api/create-workspace", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !authorizeMutation(writer, request, options) {
			return
		}
		if options.lifecycle != nil && !options.lifecycle.beginMutation() {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": "o instalador está sendo encerrado"})
			return
		}
		defer options.lifecycle.endMutation()
		var payload struct {
			ImportExisting bool `json:"import_existing"`
			AuthorizeSetup bool `json:"authorize_setup"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": "não foi possível entender sua escolha de memória"})
			return
		}
		if !payload.AuthorizeSetup {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": "confirme uma vez que o Maestro pode preparar, diagnosticar e reparar o setup local reversível"})
			return
		}
		workspacePath, err := defaultWorkspacePath(options)
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		result, err := initializeDefaultWorkspace(options, workspacePath)
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		authorize := options.authorizeSetup
		if authorize == nil {
			authorize = authorizeWorkspaceSetup
		}
		setupAuthorization, err := authorize(options, workspacePath)
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{
				"error":         fmt.Sprintf("o bootstrap local foi preparado, mas a autorização one-and-done não foi registrada: %v", err),
				"setup_state":   "authorization_pending",
				"retry_command": workspaceDiagnosticCommand(options, workspacePath),
			})
			return
		}
		configure := options.configureWorkspace
		if configure == nil {
			configure = configureWorkspaceRuntime
		}
		activation, err := configure(options, workspacePath)
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{
				"error":               fmt.Sprintf("a autorização foi registrada, mas a configuração runtime ficou pendente: %v", err),
				"setup_state":         "runtime_configuration_pending",
				"retry_command":       workspaceDiagnosticCommand(options, workspacePath),
				"setup_authorization": setupAuthorization,
			})
			return
		}
		var sourcePath string
		if payload.ImportExisting {
			// Source selection is deliberately post-bootstrap. Any selected
			// material must be interpreted and ingested from the workspace that
			// owns it; the installer never asks for a source before that workspace
			// exists.
			chooser := options.chooseImportSource
			if chooser == nil {
				chooser = chooseImportSource
			}
			sourcePath, err = chooser()
			if err != nil {
				writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("o workspace foi criado, mas não foi possível escolher a fonte de trabalho: %v", err)})
				return
			}
			if err := validateMemorySource(sourcePath, workspacePath); err != nil {
				writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
				return
			}
		}
		if sourcePath != "" {
			if err := writeImportIntent(workspacePath, sourcePath); err != nil {
				writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("o workspace foi criado, mas não foi possível registrar a fonte: %v", err)})
				return
			}
		}
		receiptID, receiptErr := newSessionToken()
		if receiptErr != nil {
			writeHTTPJSONStatus(writer, http.StatusInternalServerError, map[string]any{"error": "o workspace foi criado, mas não foi possível emitir um receipt"})
			return
		}
		workspaceReceipt := workspaceFlowReceipt{
			SchemaVersion: workspaceFlowSchemaVersion, ReceiptID: receiptID, Operation: "new_workspace",
			Status: activation.State, Valid: activation.State == "ready" && result.WorkspaceID != "", Ready: activation.State == "ready" && result.WorkspaceID != "",
			WorkspacePath: result.WorkspacePath, SourceEffect: workspaceFlowSourcePreserved, TargetEffect: map[bool]string{true: "workspace_created_and_pointer_recorded", false: "workspace_created"}[sourcePath != ""], RollbackEffect: "available_from_workspace_receipt",
			Stages: []workspaceFlowStage{
				{ID: "staging", Status: "completed", Detail: "estrutura do workspace preparada no destino escolhido"},
				{ID: "validation", Status: map[bool]string{true: "completed", false: "failed"}[activation.State == "ready"], Detail: "readiness do workspace conferido pelo installer"},
				{ID: "rollback", Status: "available", Detail: "o workspace e a origem permanecem fora da atualização do core"},
			},
		}
		handoff := workspaceHandoffFor(result.WorkspacePath, result.WorkspaceID)
		writeHTTPJSON(writer, map[string]any{
			"status": activation.State, "workspace_path": result.WorkspacePath, "workspace_id": result.WorkspaceID,
			"prompt": handoff.Prompt, "deeplinks": handoff.DeepLinks, "handoff": handoff,
			"source_registered": sourcePath != "", "source_state": map[bool]string{true: "pointer_recorded_pending_analysis", false: "not_requested"}[sourcePath != ""],
			"ingestion_state": map[bool]string{true: "not_ingested_pointer_only", false: "not_requested"}[sourcePath != ""],
			"adapter_state":   activation.Lifecycle.State, "readiness_state": activation.State, "scheduler_state": activation.Maintenance.State,
			"ready_for_runtime": activation.State == "ready", "diagnostic_command": workspaceDiagnosticCommand(options, result.WorkspacePath),
			"activation": activation, "setup_authorization": setupAuthorization, "receipt": workspaceReceipt,
		})
	})
	mux.HandleFunc("/api/launch-runtime", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !authorizeMutation(writer, request, options) {
			return
		}
		if options.lifecycle != nil && !options.lifecycle.beginMutation() {
			writeHTTPJSONStatus(writer, http.StatusConflict, map[string]any{"error": "o instalador está sendo encerrado"})
			return
		}
		defer options.lifecycle.endMutation()
		var payload struct {
			Runtime string `json:"runtime"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": "não foi possível entender o runtime selecionado"})
			return
		}
		if !runtimeIsAvailable(options, payload.Runtime) {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": "esse runtime não está disponível neste computador"})
			return
		}
		workspacePath, err := defaultWorkspacePath(options)
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("não foi possível localizar o workspace padrão: %v", err)})
			return
		}
		inspection, err := workspace.Inspect(workspacePath, options.dataRoot)
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("não foi possível conferir o workspace: %v", err)})
			return
		}
		if inspection.State != "ready" && inspection.State != "warning" {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": "a pasta escolhida ainda não é um workspace Maestro pronto; escolha uma pasta com .bcgos/workspace.json e brain/README.md"})
			return
		}
		launcher := options.launchRuntime
		if launcher == nil {
			launcher = launchRuntime
		}
		if err := launcher(payload.Runtime, inspection.WorkspacePath); err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("não foi possível abrir o runtime: %v", err)})
			return
		}
		writeHTTPJSON(writer, map[string]any{"status": "launched", "runtime": payload.Runtime, "workspace_path": inspection.WorkspacePath})
	})
	mux.HandleFunc("/api/close", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !authorizeMutation(writer, request, options) {
			return
		}
		firstClose := options.lifecycle == nil || options.lifecycle.requestClose()
		writeHTTPJSON(writer, map[string]any{"status": "closing"})
		if firstClose {
			if options.lifecycle == nil {
				if options.shutdown != nil {
					options.shutdown()
				}
				return
			}
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if options.lifecycle != nil {
					_ = options.lifecycle.waitDrained(ctx)
				}
				if options.shutdownGraceful != nil {
					_ = options.shutdownGraceful(ctx)
				} else if options.shutdown != nil {
					options.shutdown()
				}
			}()
		}
	})
	return mux
}

func workspaceDiagnosticCommand(options options, workspacePath string) string {
	cliPath := installedCLIPath(options)
	if cliPath == "" || strings.TrimSpace(workspacePath) == "" {
		return ""
	}
	if runtime.GOOS == "windows" {
		return "& " + strconv.Quote(cliPath) + " doctor " + strconv.Quote(workspacePath)
	}
	return shellQuote(cliPath) + " doctor " + shellQuote(workspacePath)
}

// installedCLIPath is intentionally a narrow, read-only UX hint. It never
// treats an arbitrary managed root as installed: the exact regular CLI path
// must exist. Activation remains exclusively owned by installer.Install.
func installedCLIPath(options options) string {
	name := "bcgos"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(options.managedRoot, "bin", name)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return path
}

func installedReleaseVersion(options options) string {
	if options.dataRoot == "" {
		return ""
	}
	body, err := os.ReadFile(filepath.Join(options.dataRoot, "config", "install-state.json"))
	if err != nil {
		return ""
	}
	var state struct {
		CLIVersion string `json:"cli_version"`
	}
	if err := json.Unmarshal(body, &state); err != nil {
		return ""
	}
	return strings.TrimSpace(state.CLIVersion)
}

func authorizeMutation(writer http.ResponseWriter, request *http.Request, options options) bool {
	presented := request.Header.Get("X-Maestro-Session")
	if options.sessionToken == "" || subtle.ConstantTimeCompare([]byte(presented), []byte(options.sessionToken)) != 1 {
		writeHTTPJSONStatus(writer, http.StatusForbidden, map[string]any{"error": "sessão do instalador inválida"})
		return false
	}
	if origin := request.Header.Get("Origin"); origin != "" && options.origin != "" && origin != options.origin {
		writeHTTPJSONStatus(writer, http.StatusForbidden, map[string]any{"error": "origem da sessão do instalador inválida"})
		return false
	}
	return true
}

func decodePlanDigest(writer http.ResponseWriter, request *http.Request) (string, error) {
	request.Body = http.MaxBytesReader(writer, request.Body, 4<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var body struct {
		PlanDigest string `json:"plan_digest"`
	}
	if err := decoder.Decode(&body); err != nil {
		return "", fmt.Errorf("invalid install confirmation: %w", err)
	}
	if body.PlanDigest == "" {
		return "", fmt.Errorf("install confirmation must include plan_digest")
	}
	return body.PlanDigest, nil
}

func decodeWorkspaceFlowID(writer http.ResponseWriter, request *http.Request) (string, error) {
	request.Body = http.MaxBytesReader(writer, request.Body, 4<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var body struct {
		FlowID string `json:"flow_id"`
	}
	if err := decoder.Decode(&body); err != nil {
		return "", fmt.Errorf("não foi possível entender a sessão de análise: %w", err)
	}
	if strings.TrimSpace(body.FlowID) == "" {
		return "", fmt.Errorf("a sessão de análise é obrigatória")
	}
	return body.FlowID, nil
}

func decodeWorkspaceFlowConfirmation(writer http.ResponseWriter, request *http.Request) (string, string, string, error) {
	request.Body = http.MaxBytesReader(writer, request.Body, 4<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var body struct {
		FlowID     string `json:"flow_id"`
		PlanDigest string `json:"plan_digest"`
		Action     string `json:"action"`
	}
	if err := decoder.Decode(&body); err != nil {
		return "", "", "", fmt.Errorf("não foi possível entender a confirmação do workspace: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return "", "", "", fmt.Errorf("confirmação do workspace inválida: %w", err)
	}
	if strings.TrimSpace(body.FlowID) == "" || strings.TrimSpace(body.PlanDigest) == "" || body.Action != workspaceFlowApprovalImport {
		return "", "", "", fmt.Errorf("a confirmação precisa conter flow_id, plan_digest e action=IMPORT")
	}
	return body.FlowID, body.PlanDigest, body.Action, nil
}

func decodeWorkspaceFlowRollback(writer http.ResponseWriter, request *http.Request) (string, string, string, string, error) {
	request.Body = http.MaxBytesReader(writer, request.Body, 4<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var body struct {
		FlowID     string `json:"flow_id"`
		PlanDigest string `json:"plan_digest"`
		ReceiptID  string `json:"receipt_id"`
		Action     string `json:"action"`
	}
	if err := decoder.Decode(&body); err != nil {
		return "", "", "", "", fmt.Errorf("não foi possível entender o rollback do workspace: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return "", "", "", "", fmt.Errorf("rollback do workspace inválido: %w", err)
	}
	if strings.TrimSpace(body.FlowID) == "" || strings.TrimSpace(body.PlanDigest) == "" || strings.TrimSpace(body.ReceiptID) == "" || body.Action != workspaceFlowApprovalRollback {
		return "", "", "", "", fmt.Errorf("o rollback precisa conter flow_id, plan_digest, receipt_id e action=ROLLBACK")
	}
	return body.FlowID, body.PlanDigest, body.ReceiptID, body.Action, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("apenas um objeto JSON é permitido")
		}
		return fmt.Errorf("dados após o objeto JSON: %w", err)
	}
	return nil
}

func writeWorkspaceFlowError(writer http.ResponseWriter, err error) {
	if capabilityErr, ok := err.(*workspaceFlowCapabilityError); ok {
		writeHTTPJSONStatus(writer, http.StatusServiceUnavailable, map[string]any{
			"error": err.Error(), "code": "capability_unavailable", "capability": capabilityErr.Capability,
			"ready": false, "source_effect": workspaceFlowSourcePreserved, "target_effect": workspaceFlowTargetNone, "rollback_effect": workspaceFlowRollbackNotCreated,
		})
		return
	}
	if backendErr, ok := err.(*workspaceFlowBackendError); ok {
		status := backendErr.Status
		if status == 0 {
			status = http.StatusConflict
		}
		writeHTTPJSONStatus(writer, status, map[string]any{
			"error": backendErr.Message, "code": backendErr.Code, "ready": false,
			"source_effect": workspaceFlowSourcePreserved, "target_effect": workspaceFlowTargetNone, "rollback_effect": workspaceFlowRollbackNotCreated,
		})
		return
	}
	writeHTTPJSONStatus(writer, http.StatusBadGateway, map[string]any{"error": err.Error(), "ready": false})
}

func newSessionToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("create installer session: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func simulationPlan(options options) installer.Plan {
	plan := installer.Plan{
		Release: "0.1.0-simulation", Channel: "canary", TargetOS: runtime.GOOS,
		TargetArch: runtime.GOARCH, ManagedRoot: options.managedRoot, DataRoot: options.dataRoot,
		ReleaseDir: options.simulationRoot, Bootstrapper: "simulation-bootstrapper",
		AuthorityRegistry: "simulation-authority-registry", ManifestSHA256: "simulation",
		RegistrySHA256: "simulation", BootstrapperVersion: "simulation",
	}
	plan.PlanDigest = installer.PlanDigest(plan)
	return plan
}

func installSimulation(options options, plan installer.Plan) (installer.Result, error) {
	if plan.PlanDigest != installer.PlanDigest(plan) {
		return installer.Result{}, fmt.Errorf("technical rehearsal plan changed before install")
	}
	if info, err := os.Stat(plan.ManagedRoot); err == nil {
		if !info.IsDir() {
			return installer.Result{}, fmt.Errorf("rehearsal managed root is not a directory")
		}
		entries, readErr := os.ReadDir(plan.ManagedRoot)
		if readErr != nil {
			return installer.Result{}, fmt.Errorf("inspect rehearsal managed root: %w", readErr)
		}
		if len(entries) != 0 {
			return installer.Result{}, fmt.Errorf("technical rehearsal already ran in this sandbox")
		}
	} else if !os.IsNotExist(err) {
		return installer.Result{}, fmt.Errorf("inspect rehearsal managed root: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(plan.ManagedRoot, "bin"), 0o700); err != nil {
		return installer.Result{}, err
	}
	if err := os.MkdirAll(filepath.Join(plan.DataRoot, "workspaces"), 0o700); err != nil {
		return installer.Result{}, err
	}
	cliPath := filepath.Join(plan.ManagedRoot, "bin", "bcgos")
	if err := os.WriteFile(cliPath, []byte("#!/bin/sh\necho 'bcgos 0.1.0-simulation'\n"), 0o700); err != nil {
		return installer.Result{}, err
	}
	if err := os.WriteFile(filepath.Join(plan.ManagedRoot, "INSTALL-REHEARSAL.txt"), []byte("Maestro technical installation rehearsal. No signed bytes were installed.\n"), 0o600); err != nil {
		return installer.Result{}, err
	}
	if err := os.WriteFile(filepath.Join(plan.DataRoot, "workspaces", "README.md"), []byte("Choose or create a test workspace here after the rehearsal.\n"), 0o600); err != nil {
		return installer.Result{}, err
	}
	receipt, err := json.MarshalIndent(map[string]any{"mode": "simulation", "release": plan.Release, "managed_root": plan.ManagedRoot, "data_root": plan.DataRoot}, "", "  ")
	if err != nil {
		return installer.Result{}, err
	}
	if err := os.WriteFile(filepath.Join(options.simulationRoot, "rehearsal-receipt.json"), append(receipt, '\n'), 0o600); err != nil {
		return installer.Result{}, err
	}
	return installer.Result{Plan: plan, CLIPath: cliPath, Output: "technical installation rehearsal complete"}, nil
}

func openBrowser(url string) {
	_ = openPath(url)
}

func openPath(path string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{path}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", path}
	default:
		command, args = "xdg-open", []string{path}
	}
	return exec.Command(command, args...).Start()
}

func availableRuntimeTargets(options options) []runtimeTarget {
	if options.runtimeTargets != nil {
		return options.runtimeTargets()
	}
	targets := make([]runtimeTarget, 0, 2)
	if runtimeAvailable("claude") {
		targets = append(targets, runtimeTarget{ID: "claude", Label: "Abrir no Claude Code"})
	}
	if runtimeAvailable("codex") {
		label := "Abrir no Codex"
		if chatGPTAppAvailable() {
			label = "Abrir Codex no ChatGPT"
		}
		targets = append(targets, runtimeTarget{ID: "codex", Label: label})
	}
	return targets
}

func workspaceHandoffFor(workspacePath, workspaceID string) workspaceHandoff {
	claudeLink := claudeCodeWorkspaceLink(workspacePath)
	codexLink := codexWorkspaceLink(workspacePath)
	claudePath := claudeDesktopPath()
	available := map[string]bool{
		"claude_desktop":      claudeDesktopAvailable(),
		"claude_code_desktop": claudeDesktopAvailable(),
		"codex":               runtimeAvailable("codex"),
	}
	diagnostics := make([]string, 0, 3)
	if !available["claude_desktop"] {
		diagnostics = append(diagnostics, "Claude Desktop não foi detectado; o link continua disponível para uma tentativa manual.")
	}
	if !available["codex"] {
		diagnostics = append(diagnostics, "Codex não foi detectado; isso não impede concluir o setup.")
	}
	return workspaceHandoff{
		WorkspacePath: workspacePath,
		WorkspaceID:   workspaceID,
		Prompt:        maestroClaudeKickoffPrompt,
		DeepLinks: map[string]string{
			"claude_desktop":      claudeLink,
			"claude_code_desktop": claudeLink,
			"codex":               codexLink,
		},
		RuntimePaths: map[string]string{
			"claude_desktop":      claudePath,
			"claude_code_desktop": claudeLink,
			"codex":               codexLink,
		},
		RuntimeAvailable: available,
		Diagnostics:      diagnostics,
	}
}

func chatGPTAppAvailable() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	for _, root := range []string{"/Applications", filepath.Join(os.Getenv("HOME"), "Applications")} {
		if info, err := os.Stat(filepath.Join(root, "ChatGPT.app")); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func runtimeIsAvailable(options options, runtimeID string) bool {
	for _, target := range availableRuntimeTargets(options) {
		if target.ID == runtimeID {
			return true
		}
	}
	return false
}

func runtimeAvailable(runtimeID string) bool {
	if !runtimeLaunchSupported(runtime.GOOS, runtimeID) {
		return false
	}
	if runtimeID == "claude" {
		return claudeDesktopAvailable()
	}
	if _, ok := runtimeCLIPath(runtimeID); ok {
		return true
	}
	if runtime.GOOS != "darwin" || runtimeID != "codex" {
		return false
	}
	for _, root := range []string{"/Applications", filepath.Join(os.Getenv("HOME"), "Applications")} {
		if info, err := os.Stat(filepath.Join(root, "Codex.app")); err == nil && info.IsDir() {
			return true
		}
	}
	return chatGPTAppAvailable()
}

// runtimeLaunchSupported is deliberately narrower than CLI discovery. The
// wizard advertises a graphical handoff, so a CLI that cannot be opened by the
// current platform must not become an enabled target.
func runtimeLaunchSupported(platform, runtimeID string) bool {
	switch runtimeID {
	case "claude":
		return platform == "darwin" || platform == "windows"
	case "codex":
		return platform == "darwin"
	default:
		return false
	}
}

func claudeDesktopAvailable() bool {
	if claudeDesktopPath() != "" {
		return true
	}
	return runtime.GOOS == "windows" && windowsProtocolRegistered("claude")
}

func claudeDesktopPath() string {
	switch runtime.GOOS {
	case "darwin":
		for _, root := range []string{"/Applications", filepath.Join(os.Getenv("HOME"), "Applications")} {
			path := filepath.Join(root, "Claude.app")
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				return path
			}
		}
	case "windows":
		for _, path := range windowsClaudeDesktopCandidates(os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")) {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path
			}
		}
	}
	return ""
}

func windowsClaudeDesktopCandidates(localAppData, programFiles, programFilesX86 string) []string {
	roots := []string{localAppData, programFiles, programFilesX86}
	candidates := make([]string, 0, len(roots)*2)
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		candidates = append(candidates,
			filepath.Join(root, "Programs", "Claude", "Claude.exe"),
			filepath.Join(root, "Claude", "Claude.exe"),
		)
	}
	return candidates
}

func windowsProtocolRegistered(scheme string) bool {
	if runtime.GOOS != "windows" || strings.TrimSpace(scheme) == "" {
		return false
	}
	for _, key := range []string{
		`HKCU\Software\Classes\` + scheme + `\shell\open\command`,
		`HKLM\Software\Classes\` + scheme + `\shell\open\command`,
	} {
		output, err := exec.Command("reg.exe", "query", key).Output()
		if err == nil && strings.TrimSpace(string(output)) != "" {
			return true
		}
	}
	return false
}

func runtimeCLIPath(runtimeID string) (string, bool) {
	if path, err := exec.LookPath(runtimeID); err == nil {
		return path, true
	}
	if runtime.GOOS != "darwin" {
		return "", false
	}
	for _, directory := range []string{"/opt/homebrew/bin", "/usr/local/bin", filepath.Join(os.Getenv("HOME"), ".local", "bin")} {
		path := filepath.Join(directory, runtimeID)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return path, true
		}
	}
	return "", false
}

func chooseWorkspace() (string, error) {
	return chooseFolder("Escolha um workspace Maestro", "nenhum workspace foi selecionado")
}

func defaultWorkspaceFor(options options) string {
	path, err := defaultWorkspacePath(options)
	if err != nil {
		return "~/Developer/maestro-os"
	}
	return path
}

func defaultWorkspacePath(options options) (string, error) {
	if options.workspacePath != nil {
		return options.workspacePath()
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("não foi possível localizar o seu diretório de usuário")
	}
	return filepath.Join(home, "Developer", "maestro-os"), nil
}

func chooseImportSource() (string, error) {
	return chooseFolder("Escolha a fonte que o Maestro deve analisar", "nenhuma pasta de memórias foi selecionada")
}

func chooseFolder(prompt, emptyMessage string) (string, error) {
	command, arguments, err := folderChooserCommand(runtime.GOOS, prompt)
	if err != nil {
		return "", err
	}
	output, err := exec.Command(command, arguments...).Output()
	if err != nil {
		return "", fmt.Errorf("a seleção foi cancelada ou não pôde ser aberta")
	}
	path := strings.TrimSpace(string(output))
	if path == "" {
		return "", fmt.Errorf("%s", emptyMessage)
	}
	return path, nil
}

func folderChooserCommand(platform, prompt string) (string, []string, error) {
	switch platform {
	case "darwin":
		return "osascript", []string{"-e", `POSIX path of (choose folder with prompt "` + prompt + `")`}, nil
	case "windows":
		// FolderBrowserDialog keeps the source selection local to the user's
		// machine. The installer receives only the chosen path and still does
		// not read, copy, or ingest any source until the governed next step.
		script := "Add-Type -AssemblyName System.Windows.Forms; " +
			"$dialog = New-Object System.Windows.Forms.FolderBrowserDialog; " +
			"$dialog.Description = '" + strings.ReplaceAll(prompt, "'", "''") + "'; " +
			"if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($dialog.SelectedPath) }"
		return "powershell.exe", []string{"-NoProfile", "-STA", "-Command", script}, nil
	default:
		return "", nil, fmt.Errorf("a seleção gráfica de pasta ainda não está disponível neste sistema")
	}
}

func validateMemorySource(sourcePath, workspacePath string) error {
	source, err := filepath.Abs(filepath.Clean(sourcePath))
	if err != nil {
		return fmt.Errorf("não foi possível resolver a pasta de memórias")
	}
	workspace, err := filepath.Abs(filepath.Clean(workspacePath))
	if err != nil {
		return fmt.Errorf("não foi possível resolver o novo workspace")
	}
	if source == workspace || strings.HasPrefix(workspace, source+string(filepath.Separator)) {
		return fmt.Errorf("a pasta de memórias não pode ser o novo workspace nem seu diretório-pai")
	}
	info, err := os.Stat(source)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("a pasta de memórias escolhida não está disponível")
	}
	return nil
}

func initializeDefaultWorkspace(options options, workspacePath string) (workspace.Result, error) {
	if strings.TrimSpace(options.dataRoot) == "" {
		return workspace.Result{}, fmt.Errorf("a área de dados do Maestro não está configurada")
	}
	if info, err := os.Stat(workspacePath); err == nil && info.IsDir() {
		inspection, inspectErr := workspace.Inspect(workspacePath, options.dataRoot)
		if inspectErr != nil {
			return workspace.Result{}, inspectErr
		}
		if inspection.State != "ready" && inspection.State != "warning" {
			entries, readErr := os.ReadDir(workspacePath)
			if readErr != nil {
				return workspace.Result{}, readErr
			}
			if len(entries) > 0 {
				return workspace.Result{}, fmt.Errorf("%s já existe e não é um workspace Maestro; escolha um novo local ou renomeie essa pasta", workspacePath)
			}
		}
	} else if err != nil && !os.IsNotExist(err) {
		return workspace.Result{}, err
	}
	result, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: options.dataRoot})
	if err != nil {
		return workspace.Result{}, err
	}
	if _, err := ownerctx.Initialize(options.dataRoot); err != nil {
		return workspace.Result{}, fmt.Errorf("bootstrap owner context: %w", err)
	}
	if _, err := workspaceagent.Initialize(options.dataRoot, result.WorkspaceID); err != nil {
		return workspace.Result{}, err
	}
	if _, err := agentscaffold.Scaffold(options.dataRoot, agentscaffold.WorkspaceRequest(result.WorkspaceID)); err != nil {
		return workspace.Result{}, err
	}
	return result, nil
}

func configureWorkspaceRuntime(options options, workspacePath string) (workspaceActivation, error) {
	return configureWorkspaceRuntimeForPlatform(options, workspacePath, runtime.GOOS)
}

func authorizeWorkspaceSetup(options options, workspacePath string) (workspaceSetupAuthorization, error) {
	cliPath := installedCLIPath(options)
	if cliPath == "" {
		return workspaceSetupAuthorization{}, fmt.Errorf("o executável instalado do Maestro não foi encontrado")
	}
	runner := options.commandRunner
	if runner == nil {
		runner = execCommandRunner{}
	}
	runtimeName, err := primaryRuntime(options)
	if err != nil {
		return workspaceSetupAuthorization{}, err
	}
	arguments := []string{"setup", "apply", "--workspace", workspacePath, "--runtime", runtimeName, "--executable", cliPath, "--confirm"}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := runner.Run(ctx, cliPath, arguments)
	if err != nil {
		return workspaceSetupAuthorization{}, commandStepError(cliPath, arguments, output, err)
	}
	var report struct {
		State         string                      `json:"state"`
		Authorization workspaceSetupAuthorization `json:"authorization"`
	}
	if err := json.Unmarshal(output, &report); err != nil || (report.State != "complete" && report.State != "complete_with_external_actions_pending") || report.Authorization.State != "active" || strings.TrimSpace(report.Authorization.GrantDigest) == "" {
		return workspaceSetupAuthorization{}, readinessError(cliPath, arguments, "o receipt de autorização one-and-done é inválido")
	}
	return report.Authorization, nil
}

func configureWorkspaceRuntimeForPlatform(options options, workspacePath, platform string) (workspaceActivation, error) {
	runtimeName, err := primaryRuntime(options)
	if err != nil {
		return workspaceActivation{}, err
	}
	cliPath := installedCLIPath(options)
	if cliPath == "" {
		return workspaceActivation{}, fmt.Errorf("o executável instalado do Maestro não foi encontrado")
	}
	runner := options.commandRunner
	if runner == nil {
		runner = execCommandRunner{}
	}
	run := func(arguments []string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		output, err := runner.Run(ctx, cliPath, arguments)
		if err != nil {
			return nil, commandStepError(cliPath, arguments, output, err)
		}
		return output, nil
	}
	if _, err := run([]string{"init", workspacePath}); err != nil {
		return workspaceActivation{}, err
	}
	if _, err := run([]string{"adapter", "install", "--runtime", runtimeName, "--executable", cliPath, workspacePath}); err != nil {
		return workspaceActivation{}, err
	}
	statusOutput, err := run([]string{"status", workspacePath})
	if err != nil {
		return workspaceActivation{}, err
	}
	var status struct {
		Workspace struct {
			State         string `json:"state"`
			WorkspaceID   string `json:"workspace_id"`
			WorkspacePath string `json:"workspace_path"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(statusOutput, &status); err != nil {
		return workspaceActivation{}, readinessError(cliPath, []string{"status", workspacePath}, "a resposta de readiness do workspace não é JSON válido")
	}
	if (status.Workspace.State != "ready" && status.Workspace.State != "warning") || strings.TrimSpace(status.Workspace.WorkspaceID) == "" {
		return workspaceActivation{}, readinessError(cliPath, []string{"status", workspacePath}, "o workspace não ficou pronto")
	}
	if status.Workspace.WorkspacePath != "" && !sameInstallerPath(status.Workspace.WorkspacePath, workspacePath) {
		return workspaceActivation{}, readinessError(cliPath, []string{"status", workspacePath}, "o readiness retornou outro workspace")
	}
	adapterOutput, err := run([]string{"adapter", "status", "--runtime", runtimeName, workspacePath})
	if err != nil {
		return workspaceActivation{}, err
	}
	var adapterStatus struct {
		Runtime    string `json:"runtime"`
		State      string `json:"state"`
		Projection struct {
			State string `json:"state"`
		} `json:"projection"`
	}
	if err := json.Unmarshal(adapterOutput, &adapterStatus); err != nil || adapterStatus.Runtime != runtimeName || adapterStatus.State != "installed" || adapterStatus.Projection.State != "installed" {
		return workspaceActivation{}, readinessError(cliPath, []string{"adapter", "status", "--runtime", runtimeName, workspacePath}, "os cinco hooks e a projeção do runtime primário não ficaram configurados")
	}
	adapterVerifyArguments := []string{"adapter", "verify", "--runtime", runtimeName, workspacePath}
	if _, err := run(adapterVerifyArguments); err != nil {
		return workspaceActivation{}, err
	}
	lifecycle := lifecycleActivation{
		Runtime:        runtimeName,
		State:          "configured",
		Events:         []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"},
		StartSession:   "configured",
		HookReview:     "owner_review_required",
		NativeObserved: "unavailable_pending_first_session",
	}
	if platform == "windows" {
		return workspaceActivation{
			State:       "ready",
			WorkspaceID: status.Workspace.WorkspaceID,
			Lifecycle:   lifecycle,
			Maintenance: maintenanceActivation{
				State:          "unavailable_windows_native_qualification_pending",
				Schedule:       "not_configured",
				NativeObserved: false,
				ModelBacked:    "unavailable",
			},
		}, nil
	}
	if platform != "darwin" {
		return workspaceActivation{}, fmt.Errorf("a manutenção nativa ainda não é suportada em %s", platform)
	}
	maintenanceArguments := []string{"maintenance", "canary", "install-macos", "--workspace-path", workspacePath, "--executable", cliPath, "--confirm", "--launchctl"}
	maintenanceOutput, err := run(maintenanceArguments)
	if err != nil {
		return workspaceActivation{}, err
	}
	var maintenanceStatus struct {
		State      string `json:"state"`
		Enrollment struct {
			WorkspaceID string `json:"workspace_id"`
		} `json:"enrollment"`
		LaunchAgent struct {
			State           string `json:"state"`
			FilePresent     bool   `json:"file_present"`
			Loaded          bool   `json:"loaded"`
			Enabled         bool   `json:"enabled"`
			NativeQualified bool   `json:"native_qualified"`
		} `json:"launch_agent"`
	}
	if err := json.Unmarshal(maintenanceOutput, &maintenanceStatus); err != nil {
		return workspaceActivation{}, readinessError(cliPath, maintenanceArguments, "a resposta da manutenção não é JSON válido")
	}
	launchAgentReady := maintenanceStatus.State == "enrolled" &&
		maintenanceStatus.Enrollment.WorkspaceID == status.Workspace.WorkspaceID &&
		maintenanceStatus.LaunchAgent.State == "active_loaded_enabled" &&
		maintenanceStatus.LaunchAgent.FilePresent &&
		maintenanceStatus.LaunchAgent.Loaded &&
		maintenanceStatus.LaunchAgent.Enabled
	if !launchAgentReady || (!maintenanceStatus.LaunchAgent.NativeQualified && !options.allowUnqualifiedNative) {
		return workspaceActivation{}, readinessError(cliPath, maintenanceArguments, "o launchd não ficou ativo, carregado e vinculado a este workspace")
	}
	return workspaceActivation{
		State:       "ready",
		WorkspaceID: status.Workspace.WorkspaceID,
		Lifecycle:   lifecycle,
		Maintenance: maintenanceActivation{
			State:           maintenanceStatus.LaunchAgent.State,
			Schedule:        "run_at_load_and_every_15_minutes",
			NativeObserved:  true,
			NativeQualified: maintenanceStatus.LaunchAgent.NativeQualified,
			ModelBacked:     "unavailable",
		},
	}, nil
}

func primaryRuntime(options options) (string, error) {
	runtimeName := strings.TrimSpace(options.primaryRuntime)
	if runtimeName == "" {
		runtimeName = "claude"
	}
	if runtimeName != "claude" && runtimeName != "codex" {
		return "", fmt.Errorf("runtime primário deve ser claude ou codex")
	}
	return runtimeName, nil
}

func commandStepError(cliPath string, arguments []string, output []byte, runErr error) error {
	detail := strings.TrimSpace(string(output))
	if len(detail) > 2048 {
		detail = detail[:2048] + "…"
	}
	if detail == "" {
		detail = runErr.Error()
	}
	return fmt.Errorf("ativação falhou em %s: %s. Nada foi marcado como pronto. Corrija o motivo e execute novamente: %s", strings.Join(arguments[:min(3, len(arguments))], " "), detail, installerCommand(cliPath, arguments))
}

func readinessError(cliPath string, arguments []string, reason string) error {
	return fmt.Errorf("readiness falhou: %s. Nada foi marcado como pronto. Execute novamente: %s", reason, installerCommand(cliPath, arguments))
}

func installerCommand(executable string, arguments []string) string {
	parts := []string{installerShellArgument(executable)}
	for _, argument := range arguments {
		parts = append(parts, installerShellArgument(argument))
	}
	return strings.Join(parts, " ")
}

func installerShellArgument(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\r\n'") {
		return value
	}
	return shellQuote(value)
}

func sameInstallerPath(left, right string) bool {
	leftAbsolute, leftErr := filepath.Abs(filepath.Clean(left))
	rightAbsolute, rightErr := filepath.Abs(filepath.Clean(right))
	return leftErr == nil && rightErr == nil && leftAbsolute == rightAbsolute
}

func writeImportIntent(workspacePath, sourcePath string) error {
	value, err := json.MarshalIndent(map[string]any{
		"schema_version": 1,
		"source_path":    sourcePath,
		"state":          workspaceFlowPointerState,
		"notice":         "Source was selected by the owner. No files have been read, copied or uploaded; this is not ingestion.",
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(workspacePath, ".bcgos", "import-intake.json"), append(value, '\n'), 0o600)
}

func launchRuntime(runtimeID, workspacePath string) error {
	if !runtimeAvailable(runtimeID) {
		return fmt.Errorf("%s não está instalado", runtimeID)
	}
	if runtimeID == "claude" {
		link, supported := claudeCodeLaunchLink(runtime.GOOS, workspacePath)
		if !supported {
			return fmt.Errorf("o Claude Code Desktop ainda não possui launcher suportado neste sistema")
		}
		if runtime.GOOS == "darwin" {
			if app := claudeDesktopPath(); app != "" {
				// Passing the deep link to the explicit app bundle both opens the
				// correct workspace and asks macOS to activate Claude in front of
				// the installer window.
				return exec.Command("open", "-a", app, link).Start()
			}
		}
		// On Windows the registered claude:// protocol is the desktop handoff;
		// it keeps the prepared workspace and onboarding prompt together.
		return openPath(link)
	}
	if runtime.GOOS == "darwin" && runtimeID == "codex" && chatGPTAppAvailable() {
		return openPath(codexWorkspaceLink(workspacePath))
	}
	if runtime.GOOS == "darwin" && runtimeID == "codex" {
		for _, root := range []string{"/Applications", filepath.Join(os.Getenv("HOME"), "Applications")} {
			app := filepath.Join(root, "Codex.app")
			if info, err := os.Stat(app); err == nil && info.IsDir() {
				return exec.Command("open", "-a", app, workspacePath).Start()
			}
		}
	}
	cliPath, ok := runtimeCLIPath(runtimeID)
	if !ok {
		return fmt.Errorf("%s não tem um launcher local", runtimeID)
	}
	return openCLIInWorkspace(cliPath, workspacePath)
}

func claudeCodeLaunchLink(platform, workspacePath string) (string, bool) {
	if platform != "darwin" && platform != "windows" {
		return "", false
	}
	return claudeCodeWorkspaceLink(workspacePath), true
}

func claudeCodeWorkspaceLink(workspacePath string) string {
	deepLink := url.URL{Scheme: "claude", Host: "code", Path: "/new"}
	query := deepLink.Query()
	query.Set("folder", workspacePath)
	query.Set("q", maestroClaudeKickoffPrompt)
	deepLink.RawQuery = query.Encode()
	return deepLink.String()
}

const maestroHumanKickoffPrompt = `👋 Olá, Maestro! 🎼

Estou chegando agora e acabei de fazer minha instalação. 🚀
Não vejo a hora de gerar valor ao acionista.

Me direcione pelos próximos passos e me ajude a começar com o pé direito.`

const maestroClaudeKickoffPrompt = maestroHumanKickoffPrompt + `

🧭 Para começar, leia primeiro CLAUDE.md e depois siga o guia instalado de
Maestro Onboarding que ele identifica. Conduza minha entrevista inicial uma
pergunta por vez.

Se um desses arquivos não existir, não crie AGENTS.md, skills ou qualquer
estrutura substituta. Explique brevemente que a preparação local está incompleta
e peça para eu retornar ao instalador para reparar o workspace.`
const maestroCodexKickoffPrompt = maestroHumanKickoffPrompt + `

🧭 Para começar, execute agora a skill $maestro-onboarding e conduza minha entrevista inicial, uma pergunta por vez.`

func codexWorkspaceLink(workspacePath string) string {
	deepLink := url.URL{Scheme: "codex", Host: "new"}
	query := deepLink.Query()
	query.Set("path", workspacePath)
	query.Set("prompt", maestroCodexKickoffPrompt)
	deepLink.RawQuery = query.Encode()
	return deepLink.String()
}

func openCLIInWorkspace(cliPath, workspacePath string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("iniciar o CLI no workspace pelo wizard ainda está disponível apenas no macOS")
	}
	command := "cd -- " + shellQuote(workspacePath) + " && exec " + shellQuote(cliPath)
	activate := `tell application "Terminal" to activate`
	run := "tell application \"Terminal\" to do script " + strconv.Quote(command)
	return exec.Command("osascript", "-e", activate, "-e", run).Start()
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func writeJSON(value any) {
	_ = json.NewEncoder(os.Stdout).Encode(value)
}

func writeError(err error) {
	writeJSON(map[string]string{"error": err.Error()})
}

func writeHTTPJSON(writer http.ResponseWriter, value any) {
	writeHTTPJSONStatus(writer, http.StatusOK, value)
}

func writeHTTPJSONStatus(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
