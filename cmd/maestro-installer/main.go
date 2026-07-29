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
	"strings"
	"sync"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

type options struct {
	releaseDir        string
	bootstrapper      string
	authorityRegistry string
	managedRoot       string
	dataRoot          string
	wizardDir         string
	headless          bool
	preview           bool
	simulate          bool
	simulationRoot    string
	sessionToken      string
	origin            string
	shutdown          func()
	shutdownGraceful  func(context.Context) error
	lifecycle         *wizardLifecycle
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
		writeHTTPJSON(writer, map[string]any{
			"platform": runtime.GOOS, "architecture": runtime.GOARCH,
			"release_dir": options.releaseDir, "managed_root": options.managedRoot,
			"data_root": options.dataRoot, "trust": map[bool]string{true: "simulation", false: "pending"}[options.simulate],
			"mode": map[bool]string{true: "simulation", false: "runtime"}[options.simulate],
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
