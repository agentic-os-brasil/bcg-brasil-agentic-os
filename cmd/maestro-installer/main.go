// Command maestro-installer is the visual, user-space entry point for a
// signed release package. It never installs unsigned bytes; all trust-bearing
// work is delegated to internal/installer and the seeded bootstrapper.
package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
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
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceagent"
)

type options struct {
	releaseDir         string
	bootstrapper       string
	authorityRegistry  string
	managedRoot        string
	dataRoot           string
	wizardDir          string
	headless           bool
	preview            bool
	simulate           bool
	simulationRoot     string
	primaryRuntime     string
	sessionToken       string
	origin             string
	shutdown           func()
	shutdownGraceful   func(context.Context) error
	lifecycle          *wizardLifecycle
	chooseWorkspace    func() (string, error)
	launchRuntime      func(runtimeID, workspacePath string) error
	runtimeTargets     func() []runtimeTarget
	chooseImportSource func() (string, error)
	workspacePath      func() (string, error)
	configureWorkspace func(options, string) (workspaceActivation, error)
	commandRunner      commandRunner
}

type runtimeTarget struct {
	ID    string `json:"id"`
	Label string `json:"label"`
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

type lifecycleActivation struct {
	Runtime        string   `json:"runtime"`
	State          string   `json:"state"`
	Events         []string `json:"events"`
	StartSession   string   `json:"start_session"`
	HookReview     string   `json:"hook_review"`
	NativeObserved string   `json:"native_observed"`
}

type maintenanceActivation struct {
	State          string `json:"state"`
	Schedule       string `json:"schedule"`
	NativeObserved bool   `json:"native_observed"`
	ModelBacked    string `json:"model_backed"`
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
		result, err := installer.Install(context.Background(), installer.Options{
			ReleaseDir: options.releaseDir, Bootstrapper: options.bootstrapper,
			AuthorityRegistry: options.authorityRegistry, ManagedRoot: options.managedRoot,
			DataRoot: options.dataRoot, TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH,
		})
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

func installerOptions(options options) installer.Options {
	return installer.Options{
		ReleaseDir:        options.releaseDir,
		Bootstrapper:      options.bootstrapper,
		AuthorityRegistry: options.authorityRegistry,
		ManagedRoot:       options.managedRoot,
		DataRoot:          options.dataRoot,
		TargetOS:          runtime.GOOS,
		TargetArch:        runtime.GOARCH,
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
	mux.Handle("/", fileServer)
	mux.HandleFunc("/api/state", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		installedCLI := installedCLIPath(options)
		writeHTTPJSON(writer, map[string]any{
			"platform": runtime.GOOS, "architecture": runtime.GOARCH,
			"release_dir": options.releaseDir, "managed_root": options.managedRoot,
			"data_root": options.dataRoot, "trust": map[bool]string{true: "simulation", false: "pending"}[options.simulate],
			"mode":      map[bool]string{true: "simulation", false: "runtime"}[options.simulate],
			"installed": installedCLI != "", "cli_path": installedCLI,
			"runtimes":          availableRuntimeTargets(options),
			"workspace_default": defaultWorkspaceFor(options),
		})
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
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": "não foi possível entender sua escolha de memória"})
			return
		}
		workspacePath, err := defaultWorkspacePath(options)
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		var sourcePath string
		if payload.ImportExisting {
			chooser := options.chooseImportSource
			if chooser == nil {
				chooser = chooseImportSource
			}
			sourcePath, err = chooser()
			if err != nil {
				writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("não foi possível escolher a pasta de memórias: %v", err)})
				return
			}
			if err := validateMemorySource(sourcePath, workspacePath); err != nil {
				writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
				return
			}
		}
		result, activation, err := initializeDefaultWorkspace(options, workspacePath)
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if sourcePath != "" {
			if err := writeImportIntent(workspacePath, sourcePath); err != nil {
				writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("o workspace foi criado, mas não foi possível registrar a fonte: %v", err)})
				return
			}
		}
		writeHTTPJSON(writer, map[string]any{
			"status": activation.State, "workspace_path": result.WorkspacePath, "workspace_id": result.WorkspaceID,
			"source_registered": sourcePath != "", "ingestion_state": map[bool]string{true: "pending_verified_pack", false: "not_requested"}[sourcePath != ""],
			"adapter_state": activation.Lifecycle.State, "readiness_state": activation.State, "scheduler_state": activation.Maintenance.State,
			"ready_for_runtime": activation.State == "ready", "diagnostic_command": workspaceDiagnosticCommand(options, result.WorkspacePath),
			"activation": activation,
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
	if runtimeID == "claude" && runtime.GOOS == "darwin" {
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

func claudeDesktopAvailable() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	for _, root := range []string{"/Applications", filepath.Join(os.Getenv("HOME"), "Applications")} {
		if info, err := os.Stat(filepath.Join(root, "Claude.app")); err == nil && info.IsDir() {
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
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("a seleção gráfica de workspace ainda está disponível apenas no macOS")
	}
	output, err := exec.Command("osascript", "-e", `POSIX path of (choose folder with prompt "Escolha um workspace Maestro")`).Output()
	if err != nil {
		return "", fmt.Errorf("a seleção foi cancelada ou não pôde ser aberta")
	}
	path := strings.TrimSpace(string(output))
	if path == "" {
		return "", fmt.Errorf("nenhum workspace foi selecionado")
	}
	return path, nil
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
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("a seleção gráfica de memórias ainda está disponível apenas no macOS")
	}
	output, err := exec.Command("osascript", "-e", `POSIX path of (choose folder with prompt "Escolha a pasta de memórias que o Maestro deve preparar para ingestão")`).Output()
	if err != nil {
		return "", fmt.Errorf("a seleção foi cancelada ou não pôde ser aberta")
	}
	path := strings.TrimSpace(string(output))
	if path == "" {
		return "", fmt.Errorf("nenhuma pasta de memórias foi selecionada")
	}
	return path, nil
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

func initializeDefaultWorkspace(options options, workspacePath string) (workspace.Result, workspaceActivation, error) {
	if strings.TrimSpace(options.dataRoot) == "" {
		return workspace.Result{}, workspaceActivation{}, fmt.Errorf("a área de dados do Maestro não está configurada")
	}
	if info, err := os.Stat(workspacePath); err == nil && info.IsDir() {
		inspection, inspectErr := workspace.Inspect(workspacePath, options.dataRoot)
		if inspectErr != nil {
			return workspace.Result{}, workspaceActivation{}, inspectErr
		}
		if inspection.State != "ready" && inspection.State != "warning" {
			entries, readErr := os.ReadDir(workspacePath)
			if readErr != nil {
				return workspace.Result{}, workspaceActivation{}, readErr
			}
			if len(entries) > 0 {
				return workspace.Result{}, workspaceActivation{}, fmt.Errorf("%s já existe e não é um workspace Maestro; escolha um novo local ou renomeie essa pasta", workspacePath)
			}
		}
	} else if err != nil && !os.IsNotExist(err) {
		return workspace.Result{}, workspaceActivation{}, err
	}
	result, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: options.dataRoot})
	if err != nil {
		return workspace.Result{}, workspaceActivation{}, err
	}
	if _, err := workspaceagent.Initialize(options.dataRoot, result.WorkspaceID); err != nil {
		return workspace.Result{}, workspaceActivation{}, err
	}
	if _, err := agentscaffold.Scaffold(options.dataRoot, agentscaffold.WorkspaceRequest(result.WorkspaceID)); err != nil {
		return workspace.Result{}, workspaceActivation{}, err
	}
	configure := options.configureWorkspace
	if configure == nil {
		configure = configureWorkspaceRuntime
	}
	activation, err := configure(options, workspacePath)
	if err != nil {
		return workspace.Result{}, workspaceActivation{}, err
	}
	return result, activation, nil
}

func configureWorkspaceRuntime(options options, workspacePath string) (workspaceActivation, error) {
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
	if maintenanceStatus.State != "enrolled" || maintenanceStatus.Enrollment.WorkspaceID != status.Workspace.WorkspaceID || maintenanceStatus.LaunchAgent.State != "active_loaded_enabled" || !maintenanceStatus.LaunchAgent.FilePresent || !maintenanceStatus.LaunchAgent.Loaded || !maintenanceStatus.LaunchAgent.Enabled || !maintenanceStatus.LaunchAgent.NativeQualified {
		return workspaceActivation{}, readinessError(cliPath, maintenanceArguments, "o launchd não ficou ativo, carregado e vinculado a este workspace")
	}
	return workspaceActivation{
		State:       "ready",
		WorkspaceID: status.Workspace.WorkspaceID,
		Lifecycle: lifecycleActivation{
			Runtime:        runtimeName,
			State:          "configured",
			Events:         []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"},
			StartSession:   "configured",
			HookReview:     "owner_review_required",
			NativeObserved: "unavailable_pending_first_session",
		},
		Maintenance: maintenanceActivation{
			State:          maintenanceStatus.LaunchAgent.State,
			Schedule:       "run_at_load_and_every_15_minutes",
			NativeObserved: true,
			ModelBacked:    "unavailable",
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
		"state":          "pending_verified_pack",
		"notice":         "Source was selected by the owner. No files have been read, copied or uploaded yet.",
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
	if runtime.GOOS == "darwin" && runtimeID == "claude" {
		return openPath(claudeCodeWorkspaceLink(workspacePath))
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

func claudeCodeWorkspaceLink(workspacePath string) string {
	deepLink := url.URL{Scheme: "claude", Host: "code", Path: "/new"}
	query := deepLink.Query()
	query.Set("folder", workspacePath)
	query.Set("q", claudeDesktopKickoffPrompt)
	deepLink.RawQuery = query.Encode()
	return deepLink.String()
}

const claudeDesktopKickoffPrompt = `INÍCIO GUIADO DO MAESTRO

Você está no workspace Maestro recém-criado. Depois que o owner confirmar este diretório no Claude Desktop, responda em português do Brasil com a estrutura abaixo — acolhedora, objetiva e com energia de produto. Não se apresente como Kowalski.

# 🎼 Bem-vindo ao Maestro
Abra com uma frase curta explicando que Maestro é o segundo cérebro profissional que organiza contexto, execução e evidência neste workspace.

## ✨ O que já está pronto
- Workspace local e separado dos projetos existentes
- Estrutura inicial para contexto, decisões, pessoas e tarefas
- Hooks locais do Maestro; explique que o runtime pedirá a revisão de confiança quando aplicável
- Manutenção local preparada; não alegue que uma sessão nativa já foi observada

## 🧭 Como vamos começar
Explique em três passos: entender o owner, dar nome e papel aos agentes principais, e escolher a primeira frente de trabalho. Diga que a entrevista acontece uma pergunta por vez e que nada de memórias externas será ingerido sem autorização explícita.

## 🛠️ Atalhos que podemos usar depois do onboarding
Sugira apenas estas skills instaladas, sem executá-las automaticamente:
- /interaction-profile — calibrar como o Maestro deve comunicar e decidir
- /agent-identity-setup — definir nomes, emojis e ownership dos agentes
- /workspace-agent-setup — estruturar um novo workspace ou frente de projeto
- /case-kickoff — transformar um escopo aprovado em plano inicial
- /ingest-content — trazer conteúdo local de forma verificada
- /meeting-to-work-items — converter notas em decisões, tarefas e próximos passos

## 🚀 Primeiro passo
Faça somente a primeira pergunta da entrevista: “Qual é o seu papel, contexto profissional e tipo de trabalho que você quer que o Maestro ajude a conduzir?”

Antes de responder, leia AGENTS.md. Não inicie tarefa profissional, não execute skill, não acesse memória externa e não conceda confiança global; aguarde a resposta do owner após essa primeira pergunta.`

func codexWorkspaceLink(workspacePath string) string {
	deepLink := url.URL{Scheme: "codex", Host: "new"}
	query := deepLink.Query()
	query.Set("path", workspacePath)
	query.Set("prompt", "Você está no workspace Maestro recém-criado. Antes da primeira tarefa, revise os cinco hooks locais do Maestro quando o Codex solicitar a confiança deles; essa revisão pertence ao owner e não é burlada pelo instalador. Em seguida, leia AGENTS.md e proponha a primeira tarefa segura.")
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
