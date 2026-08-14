package markitdown

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
)

const onDemandSchemaVersion = 1

// PinnedMarkItDownVersion and PinnedPythonVersion are the exact versions the
// PYUV on-demand environment installs. Bump deliberately and never resolve
// "latest": decision PYUV requires pinned, reproducible versions rather than
// an ambient install.
const (
	PinnedMarkItDownVersion = "0.1.7"
	PinnedPythonVersion     = "3.12"
)

//go:embed runtime/adapter.py
var onDemandAdapterScript []byte

// OnDemandReceipt records the provenance of the PYUV on-demand Python
// environment so a later session can verify it is still the exact pinned
// environment before reusing it.
type OnDemandReceipt struct {
	SchemaVersion     int    `json:"schema_version"`
	MarkItDownVersion string `json:"markitdown_version"`
	PythonVersion     string `json:"python_version"`
	Platform          string `json:"platform"`
	CreatedAt         string `json:"created_at"`
}

// OnDemandVenvDir returns the fixed, single on-demand environment location
// under the workspace data root, per decision PYUV.
func OnDemandVenvDir(dataRoot string) string {
	return filepath.Join(dataRoot, "runtime", "venv")
}

// OnDemandReceiptPath returns where the on-demand environment's provenance
// receipt lives.
func OnDemandReceiptPath(dataRoot string) string {
	return filepath.Join(dataRoot, "runtime", "python-env.json")
}

func onDemandAdapterScriptPath(dataRoot string) string {
	return filepath.Join(OnDemandVenvDir(dataRoot), "markitdown_adapter.py")
}

func onDemandVenvPython(dataRoot string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(OnDemandVenvDir(dataRoot), "Scripts", "python.exe")
	}
	return filepath.Join(OnDemandVenvDir(dataRoot), "bin", "python")
}

// ResolveOnDemandPack reports whether the PYUV on-demand environment is
// ready to run. It never creates or mutates the environment: absence is
// reported as unavailable with a reason telling the caller that user
// confirmation is required, never as a silent failure.
func ResolveOnDemandPack(dataRoot string) (Pack, error) {
	if strings.TrimSpace(dataRoot) == "" {
		return Pack{State: "unavailable", Reason: "local data root is not configured"}, nil
	}
	receipt, err := loadOnDemandReceipt(dataRoot)
	if errors.Is(err, os.ErrNotExist) {
		return Pack{State: "unavailable", Reason: "on-demand Python environment has not been created; ask the user to confirm creating one"}, nil
	}
	if err != nil {
		return Pack{State: "unavailable", Reason: "on-demand Python environment receipt cannot be inspected"}, nil
	}
	if receipt.SchemaVersion != onDemandSchemaVersion || receipt.MarkItDownVersion != PinnedMarkItDownVersion || receipt.PythonVersion != PinnedPythonVersion {
		return Pack{State: "unavailable", Reason: "on-demand Python environment is out of date and must be recreated"}, nil
	}
	pythonPath := onDemandVenvPython(dataRoot)
	if err := regularFile(pythonPath); err != nil {
		return Pack{State: "unavailable", Reason: "on-demand Python environment interpreter is missing"}, nil
	}
	scriptPath := onDemandAdapterScriptPath(dataRoot)
	body, err := os.ReadFile(scriptPath)
	if err != nil || !bytes.Equal(body, onDemandAdapterScript) {
		return Pack{State: "unavailable", Reason: "on-demand Python environment adapter script is missing or modified"}, nil
	}
	return Pack{State: "ready", Command: []string{pythonPath, scriptPath, "--request-stdin"}}, nil
}

func loadOnDemandReceipt(dataRoot string) (OnDemandReceipt, error) {
	var receipt OnDemandReceipt
	body, err := os.ReadFile(OnDemandReceiptPath(dataRoot))
	if err != nil {
		return receipt, err
	}
	if err := json.Unmarshal(body, &receipt); err != nil {
		return receipt, err
	}
	return receipt, nil
}

// CommandRunner executes one external command. Production code runs a real
// process; tests inject a recording double so environment creation is
// exercised without a real uv install or network access.
type CommandRunner func(name string, args []string) error

func runCommand(name string, args []string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ensureNotElevated and lookPath are package-level indirections so tests can
// exercise CreateOnDemandEnv's guards without an elevated test runner or a
// real uv install, matching the pattern in internal/userlevel.
var ensureNotElevated = userlevel.EnsureNotElevated
var lookPath = exec.LookPath

// CreateOnDemandEnv provisions the PYUV on-demand Python environment: a
// pinned Python interpreter and pinned MarkItDown version installed through
// uv, plus the bundled adapter script and a provenance receipt. Callers must
// have already obtained explicit user confirmation before invoking this; it
// performs the actual download/install and must never run silently.
func CreateOnDemandEnv(dataRoot string, run CommandRunner) (OnDemandReceipt, error) {
	var receipt OnDemandReceipt
	if strings.TrimSpace(dataRoot) == "" {
		return receipt, errors.New("local data root is not configured")
	}
	if err := ensureNotElevated(); err != nil {
		return receipt, fmt.Errorf("on-demand Python environment setup refused: %w", err)
	}
	uvPath, err := lookPath("uv")
	if err != nil {
		return receipt, errors.New("uv is not installed; install it from https://docs.astral.sh/uv/ before retrying")
	}
	if run == nil {
		run = runCommand
	}

	venvDir := OnDemandVenvDir(dataRoot)
	if err := os.MkdirAll(filepath.Dir(venvDir), 0o700); err != nil {
		return receipt, fmt.Errorf("create runtime directory: %w", err)
	}
	if err := run(uvPath, []string{"venv", venvDir, "--python", PinnedPythonVersion}); err != nil {
		return receipt, fmt.Errorf("create pinned Python environment: %w", err)
	}
	pythonPath := onDemandVenvPython(dataRoot)
	if err := run(uvPath, []string{"pip", "install", "--python", pythonPath, fmt.Sprintf("markitdown==%s", PinnedMarkItDownVersion)}); err != nil {
		return receipt, fmt.Errorf("install pinned markitdown: %w", err)
	}
	if err := os.WriteFile(onDemandAdapterScriptPath(dataRoot), onDemandAdapterScript, 0o600); err != nil {
		return receipt, fmt.Errorf("write markitdown adapter script: %w", err)
	}

	receipt = OnDemandReceipt{
		SchemaVersion:     onDemandSchemaVersion,
		MarkItDownVersion: PinnedMarkItDownVersion,
		PythonVersion:     PinnedPythonVersion,
		Platform:          runtime.GOOS,
		CreatedAt:         time.Now().UTC().Format(time.RFC3339),
	}
	body, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return receipt, err
	}
	if err := os.WriteFile(OnDemandReceiptPath(dataRoot), body, 0o600); err != nil {
		return receipt, fmt.Errorf("write on-demand environment receipt: %w", err)
	}
	return receipt, nil
}

// uvInstallCommand returns the exact, unmodified official astral.sh installer
// invocation for the given GOOS, per decision UVIN: install.sh on macOS/Linux,
// install.ps1 on Windows, never a mirrored or re-hosted copy.
func uvInstallCommand(goos string) (string, []string) {
	if goos == "windows" {
		return "powershell", []string{"-ExecutionPolicy", "ByPass", "-c", "irm https://astral.sh/uv/install.ps1 | iex"}
	}
	return "sh", []string{"-c", "curl -LsSf https://astral.sh/uv/install.sh | sh"}
}

// InstallUV downloads and runs the official astral.sh uv installer for the
// current platform, per decision UVIN. Callers must have already obtained
// explicit user confirmation before invoking this; it performs the actual
// download/install and must never run silently. It never requires or grants
// elevated privileges, matching the on-demand Python environment it unblocks.
func InstallUV(run CommandRunner) error {
	if err := ensureNotElevated(); err != nil {
		return fmt.Errorf("uv installer refused: %w", err)
	}
	if run == nil {
		run = runCommand
	}
	name, args := uvInstallCommand(runtime.GOOS)
	if err := run(name, args); err != nil {
		return fmt.Errorf("install uv: %w", err)
	}
	return nil
}
