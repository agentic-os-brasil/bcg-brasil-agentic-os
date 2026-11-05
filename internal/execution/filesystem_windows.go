//go:build windows

package execution

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"os"
	"path/filepath"
)

func ledgerOpen(path string, flags int, _ os.FileMode) (*os.File, error) {
	return winprivate.OpenFile(path, flags)
}
func openEvidenceFile(path string) (*os.File, error)       { return winprivate.OpenRegular(path) }
func ledgerMkdirAll(path string, _ os.FileMode) error      { return winprivate.MkdirAll(path) }
func ledgerMkdir(path string, _ os.FileMode) error         { return winprivate.Mkdir(path) }
func ledgerMkdirTemp(path, prefix string) (string, error)  { return winprivate.MkdirTemp(path, prefix) }
func ledgerReadDir(path string) ([]os.DirEntry, error)     { return winprivate.ReadDir(path) }
func ledgerStatDirectory(path string) (os.FileInfo, error) { return winprivate.StatDirectory(path) }
func ledgerRemove(path string) error                       { return winprivate.Remove(path, false) }
func ledgerRemoveAll(path string) error                    { return winprivate.Remove(path, true) }
func OpenConfinedStore(root string) (Store, error) {
	if e := validateStoreRoot(root); e != nil {
		return Store{}, e
	}
	d, e := winprivate.Open(root, false)
	if e != nil {
		return Store{}, e
	}
	defer d.Close()
	return Store{Root: d.File.Name()}, nil
}
func ledgerPublishJSON(path string, body []byte, immutable bool) error {
	d, e := winprivate.Open(filepath.Dir(path), !immutable)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Publish(filepath.Base(path), body, immutable)
}
