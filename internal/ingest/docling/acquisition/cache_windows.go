//go:build windows

package acquisition

import (
	"io"
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
)

func directoryOwner(name string) error {
	held, err := winprivate.OpenPinnedTree(name)
	if err != nil {
		return err
	}
	return held.Close()
}
func mkdirPrivateNew(name string) error {
	held, err := winprivate.Fresh(name)
	if err != nil {
		return err
	}
	return held.Close()
}
func openPrivateNew(name string) (*os.File, error) {
	return winprivate.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
}
func holdPrivateDirectory(name string) (io.Closer, error) { return winprivate.OpenPinnedTree(name) }
