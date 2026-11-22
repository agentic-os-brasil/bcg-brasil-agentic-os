//go:build windows

package darwin

import "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"

func holdProductPrivateFiles(root string) (*productPrivateFiles, func() error, error) {
	if err := scheduler.EnsurePrivateDirectory(root); err != nil {
		return nil, nil, err
	}
	held, err := scheduler.HoldDirectory(root, nil)
	if err != nil {
		return nil, nil, err
	}
	files := &productPrivateFiles{root: root, directories: map[string]*scheduler.HeldDirectory{root: held}, order: []*scheduler.HeldDirectory{held}}
	return files, files.close, nil
}
