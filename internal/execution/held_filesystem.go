package execution

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

func (store Store) ledgerOpen(path string, flags int, mode os.FileMode) (*os.File, error) {
	if store.DataDirectory == nil {
		return ledgerOpen(path, flags, mode)
	}
	rel, e := store.DataDirectory.Relative(path)
	if e != nil {
		return nil, e
	}
	return store.DataDirectory.OpenFile(rel, flags)
}
func (store Store) ledgerMkdirAll(path string, mode os.FileMode) error {
	if store.DataDirectory == nil {
		return ledgerMkdirAll(path, mode)
	}
	rel, e := store.DataDirectory.Relative(path)
	if e != nil {
		return e
	}
	return store.DataDirectory.EnsureDirectory(rel)
}
func (store Store) ledgerMkdir(path string, mode os.FileMode) error {
	if store.DataDirectory == nil {
		return ledgerMkdir(path, mode)
	}
	rel, e := store.DataDirectory.Relative(path)
	if e != nil {
		return e
	}
	return store.DataDirectory.Mkdir(rel)
}
func (store Store) ledgerMkdirTemp(path, prefix string) (string, error) {
	name := filepath.Join(path, prefix+hex.EncodeToString(randomBytes(12)))
	return name, store.ledgerMkdir(name, 0700)
}
func (store Store) ledgerReadDir(path string) ([]os.DirEntry, error) {
	if store.DataDirectory == nil {
		return ledgerReadDir(path)
	}
	rel, e := store.DataDirectory.Relative(path)
	if e != nil {
		return nil, e
	}
	entries, e := store.DataDirectory.ReadDir(rel, -1)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, e
}
func (store Store) ledgerStatDirectory(path string) (os.FileInfo, error) {
	if store.DataDirectory == nil {
		return ledgerStatDirectory(path)
	}
	rel, e := store.DataDirectory.Relative(path)
	if e != nil {
		return nil, e
	}
	return store.DataDirectory.StatDirectory(rel)
}
func (store Store) ledgerRemove(path string) error {
	if store.DataDirectory == nil {
		return ledgerRemove(path)
	}
	rel, e := store.DataDirectory.Relative(path)
	if e != nil {
		return e
	}
	return store.DataDirectory.Remove(rel)
}
func (store Store) ledgerRemoveAll(path string) error {
	if store.DataDirectory == nil {
		return ledgerRemoveAll(path)
	}
	// Reserved failed stages remain non-authoritative orphans; recursive name-based cleanup cannot preserve custody.
	return ErrConfinementUnavailable
}
func (store Store) ledgerPublishJSON(path string, body []byte, immutable bool) error {
	if store.DataDirectory == nil {
		return ledgerPublishJSON(path, body, immutable)
	}
	rel, e := store.DataDirectory.Relative(path)
	if e != nil {
		return e
	}
	if immutable {
		return store.DataDirectory.PublishNewFile(rel, body)
	}
	return store.DataDirectory.UpdatePrivateFile(rel, maximumLedgerJSONBytes, func([]byte) ([]byte, error) { return body, nil })
}
func (store Store) durableRename(source, target string) error {
	if store.DataDirectory == nil {
		return durableRename(source, target)
	}
	from, e := store.DataDirectory.Relative(source)
	if e != nil {
		return e
	}
	to, e := store.DataDirectory.Relative(target)
	if e != nil {
		return e
	}
	return store.DataDirectory.Rename(from, to, runtime.GOOS != "windows")
}
func (store Store) publishSetupItem(source, target string) error {
	if store.DataDirectory == nil {
		return publishSetupItem(source, target)
	}
	from, e := store.DataDirectory.Relative(source)
	if e != nil {
		return e
	}
	to, e := store.DataDirectory.Relative(target)
	if e != nil {
		return e
	}
	return store.DataDirectory.Rename(from, to, false)
}
