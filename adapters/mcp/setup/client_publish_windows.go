//go:build windows

package setup

import (
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"os"
	"path/filepath"
)

func publishClientCandidate(source, target string) error {
	if filepath.Dir(source) != filepath.Dir(target) {
		return errors.New("client publication requires the selected same parent")
	}
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		held, err := winprivate.OpenMovableDirectory(source)
		if err != nil {
			return err
		}
		defer held.Close()
		moved, err := held.MoveTo(filepath.Base(target))
		if err != nil {
			return err
		}
		if !moved {
			return errors.New("client directory publication did not move")
		}
		return nil
	}
	return winprivate.Rename(source, target, false)
}
