//go:build maestro_experimental_installed_seed && windows

package main

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/pilotdocument"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"os"
)

func withPilotDocumentRoot(path string, action func(func(int) (*os.File, error)) error) error {
	held, err := winprivate.OpenPinnedTree(path)
	if err != nil {
		return err
	}
	defer held.Close()
	if err := verifyPilotDocumentDirectory(path, held.File); err != nil {
		return err
	}
	err = action(func(flags int) (*os.File, error) { return held.OpenFile(pilotdocument.Name, flags) })
	if err != nil {
		return err
	}
	return verifyPilotDocumentDirectory(path, held.File)
}
