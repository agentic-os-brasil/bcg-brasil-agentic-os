//go:build windows

package docling

import (
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
)

func openPlacementDirectory(path string) (*os.File, error) {
	directory, err := winprivate.Open(path, false)
	if err != nil {
		return nil, err
	}
	return directory.File, nil
}

func openPlacementLeaf(parent *os.File, name string) (*os.File, error) {
	directory := &winprivate.Directory{File: parent}
	leaf, err := directory.Child(name, false, false)
	if err != nil {
		return nil, err
	}
	return leaf.File, nil
}

func syncPlacementDirectory(directory *os.File) error {
	return (&winprivate.Directory{File: directory}).Sync()
}
