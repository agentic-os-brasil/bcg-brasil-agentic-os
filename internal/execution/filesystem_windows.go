//go:build windows

package execution

import (
	"os"
	"path/filepath"
)

// Legacy Store callers retain their Windows contract. Product entry requires
// OpenConfinedStore, which stays unavailable until handle confinement qualifies.
var ledgerOpen = os.OpenFile
var ledgerMkdirAll = os.MkdirAll
var ledgerMkdir = os.Mkdir
var ledgerMkdirTemp = os.MkdirTemp
var ledgerReadDir = os.ReadDir
var ledgerStatDirectory = os.Stat
var ledgerRemove = os.Remove
var ledgerRemoveAll = os.RemoveAll

func OpenConfinedStore(root string) (Store, error) {
	return Store{}, ErrConfinementUnavailable
}

func ledgerPublishJSON(path string, body []byte, immutable bool) error {
	if !immutable {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".publish-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(body)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if immutable {
		return durablePublishNoClobber(file.Name(), path)
	}
	return durableReplace(file.Name(), path)
}
