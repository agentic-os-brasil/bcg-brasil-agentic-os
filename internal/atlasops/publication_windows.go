//go:build windows

package atlasops

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
)

// Preserve the existing owner constructor and behavior; new workspace effects
// require a separately qualified native ownership/confinement publisher.
func workspaceMutationAvailable() error {
	return errors.New("workspace atlas mutation is unavailable on Windows pending native confinement qualification")
}

// Native read/publication retain the admitted parent through the complete
// effect and cleanup. The public availability gate above stays closed until the
// dedicated ordinary-account kernel evidence qualifies this implementation.
func publishAtlasFile(path string, body []byte, exclusive, _ bool) error {
	d, err := winprivate.OpenPinned(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Publish(filepath.Base(path), body, exclusive)
}
func readAtlasFile(path string, maximum int64, _ bool) ([]byte, error) {
	d, err := winprivate.OpenPinned(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer d.Close()
	file, err := d.OpenFile(filepath.Base(path), os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return winprivate.Read(file, maximum)
}
