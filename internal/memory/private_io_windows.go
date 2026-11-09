//go:build windows

package memory

import (
	"errors"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"os"
)

func ownedIOAvailable() error                             { _, e := winprivate.CurrentUser(); return e }
func memoryMkdirAll(path string, _ os.FileMode) error     { return winprivate.MkdirAll(path) }
func memoryMkdir(path string, _ os.FileMode) error        { return winprivate.Mkdir(path) }
func memoryMkdirTemp(path, prefix string) (string, error) { return winprivate.MkdirTemp(path, prefix) }
func memoryReadDir(path string) ([]os.DirEntry, error)    { return winprivate.ReadDir(path) }
func memoryRemoveAll(path string) error                   { return winprivate.Remove(path, true) }
func memoryRename(source, target string) error            { return winprivate.Rename(source, target, false) }
func memoryOpen(path string, flags int) (*os.File, error) { return winprivate.OpenFile(path, flags) }
func memoryReadFile(path string, maximum int64) ([]byte, error) {
	return winprivate.ReadFile(path, maximum)
}
func memoryRandomID() string { return winprivate.Random() }
func memoryPublish(path string, b []byte, immutable bool) error {
	return winprivate.Publish(path, b, immutable)
}
func memoryPruneVisibilityHistory(path string, through uint64, fault func() error) error {
	d, e := winprivate.Open(path, false)
	if e != nil {
		return e
	}
	defer d.Close()
	for revision := uint64(1); revision <= through; revision++ {
		name := fmt.Sprintf("%020d.json", revision)
		if e = d.Remove(name, false); errors.Is(e, os.ErrNotExist) {
			continue
		} else if e != nil {
			return e
		}
		if e = fault(); e != nil {
			return errors.Join(e, d.Sync())
		}
	}
	return d.Sync()
}
