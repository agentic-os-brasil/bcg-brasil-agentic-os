// Command maestro-installer is the visual, user-space entry point for a
// signed release package. It never installs unsigned bytes; all trust-bearing
// work is delegated to internal/installer and the seeded bootstrapper.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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
	mux := wizardHandler(options)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		writeError(err)
		os.Exit(1)
	}
	url := "http://" + listener.Addr().String() + "/"
	fmt.Println("Maestro installer wizard: " + url)
	openBrowser(url)
	if err := http.Serve(listener, mux); err != nil && !strings.Contains(err.Error(), "Server closed") {
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
	mux.Handle("/", fileServer)
	mux.HandleFunc("/api/state", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeHTTPJSON(writer, map[string]any{
			"platform": runtime.GOOS, "architecture": runtime.GOARCH,
			"release_dir": options.releaseDir, "managed_root": options.managedRoot,
			"data_root": options.dataRoot, "trust": "pending",
		})
	})
	mux.HandleFunc("/api/verify", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		plan, _, err := installer.Prepare(installerOptions(options))
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeHTTPJSON(writer, plan)
	})
	mux.HandleFunc("/api/install", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		result, err := installer.Install(request.Context(), installerOptions(options))
		if err != nil {
			writeHTTPJSONStatus(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeHTTPJSON(writer, result)
	})
	mux.HandleFunc("/api/open-data", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
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
	return mux
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
