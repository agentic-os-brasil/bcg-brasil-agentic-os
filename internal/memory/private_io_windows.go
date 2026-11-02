//go:build windows

package memory

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"os"
	"path/filepath"
)

func ownedIOAvailable() error { return ErrConfinementUnavailable }

var memoryMkdirAll = os.MkdirAll
var memoryMkdir = os.Mkdir
var memoryMkdirTemp = os.MkdirTemp
var memoryReadDir = os.ReadDir
var memoryRemoveAll = os.RemoveAll
var memoryRename = durableRename

func memoryOpen(path string, flags int) (*os.File, error) { return os.OpenFile(path, flags, 0600) }
func memoryReadFile(path string, maximum int64) ([]byte, error) {
	return scheduler.ReadPrivateFile(path, maximum)
}
func memoryRandomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func memoryPublish(path string, b []byte, immutable bool) error {
	if immutable {
		return scheduler.WriteNewPrivateFile(path, b)
	}
	return scheduler.ReplacePrivateFile(path, b)
}

func memoryPruneVisibilityHistory(path string, through uint64, fault func() error) error {
	for revision := uint64(1); revision <= through; revision++ {
		if err := memoryRemoveAll(filepath.Join(path, fmt.Sprintf("%020d.json", revision))); err != nil {
			return err
		}
		if err := fault(); err != nil {
			return err
		}
	}
	return nil
}
