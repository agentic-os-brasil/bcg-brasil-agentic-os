package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
)

// PreflightInitialize inspects only initialization's write footprint without
// creating directories, changing permissions or opening existing file contents.
// Missing roots are allowed; existing links and nonregular targets are refused.
func PreflightInitialize(options Options) error {
	if err := userlevel.EnsureNotElevated(); err != nil {
		return err
	}
	ws, data, err := normalize(options)
	if err != nil {
		return err
	}
	if IsSynchronizedPath(ws) && !options.AllowSynchronizedRoot {
		return ErrSynchronizedWorkspace
	}
	id := workspaceID(ws)
	dirs := append([]string{ws, filepath.Join(ws, ".bcgos")}, bootstrapDirectories(data, id)...)
	dirs = append(dirs, bindingDirectories(data, id)...)
	files := []string{filepath.Join(ws, "brain", "README.md"), filepath.Join(ws, ".bcgos", "workspace.json"), filepath.Join(ws, ".bcgos", "maestro-orchestration-state.json"), filepath.Join(ws, ".bcgos", "maestro-orchestration-state.json.lock"), filepath.Join(data, "workspaces", id, "binding.json")}
	for _, entry := range visibleSurface {
		files = append(files, filepath.Join(ws, filepath.FromSlash(entry.path)))
	}
	for _, path := range dirs {
		if err := validateInitializationTarget(path, true); err != nil {
			return fmt.Errorf("initialization directory unavailable: %w", err)
		}
	}
	for _, path := range files {
		if err := validateInitializationTarget(path, false); err != nil {
			return fmt.Errorf("initialization file unavailable: %w", err)
		}
	}
	return nil
}
func bootstrapDirectories(data, id string) []string {
	return []string{
		filepath.Join(data, "config"), filepath.Join(data, "memory"), filepath.Join(data, "scheduler"), filepath.Join(data, "logs"),
		filepath.Join(data, "memory", "workspaces", id), filepath.Join(data, "maintenance", "scheduler", "workspaces", id), filepath.Join(data, "maintenance", "receipts", "workspaces", id),
	}
}
func bindingDirectories(data, id string) []string {
	return []string{data, filepath.Join(data, "workspaces"), filepath.Join(data, "workspaces", id)}
}
func validateInitializationTarget(path string, directory bool) error {
	path, err := scheduler.CanonicalPrivatePath(path)
	if err != nil {
		return err
	}
	parent := path
	if !directory {
		parent = filepath.Dir(path)
	}
	// The canonical no-follow walk rejects unsafe ancestors, including those of
	// missing targets, while retaining the platform's fixed temporary aliases.
	for {
		err = scheduler.ValidateDirectory(parent)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return err
		}
		parent = next
	}
	if directory {
		return nil
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("initialization target must be a regular non-symlink file")
	}
	return nil
}
