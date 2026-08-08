// Command maestro-installer-singlefile is the user-facing Windows wrapper.
// It carries a verified package payload, extracts it into a private temporary
// directory and delegates the actual installation to the existing bridge.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installerbundle"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(arguments []string) int {
	executable, err := os.Executable()
	if err != nil {
		return reportFailure(fmt.Errorf("resolve self-contained installer location: %w", err))
	}
	packageRoot, cleanup, err := installerbundle.ExtractExecutable(executable)
	if err != nil {
		return reportFailure(err)
	}
	defer cleanup()

	bridge := filepath.Join(packageRoot, "maestro-installer.exe")
	command := exec.Command(bridge, arguments...)
	command.Dir = packageRoot
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ProcessState != nil {
			return exitError.ProcessState.ExitCode()
		}
		return reportFailure(fmt.Errorf("run Maestro installer bridge: %w", err))
	}
	if command.ProcessState == nil {
		return 0
	}
	return command.ProcessState.ExitCode()
}

func reportFailure(err error) int {
	_, _ = fmt.Fprintf(os.Stderr, "Maestro installer: %v\n", err)
	return 1
}
