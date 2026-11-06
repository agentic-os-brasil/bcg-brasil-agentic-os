//go:build windows

package memory

import (
	"path/filepath"
	"sort"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
)

func privateWikiHold(path string) (func(), error) {
	directory, err := winprivate.OpenPinnedTree(path)
	if err != nil {
		return nil, err
	}
	return func() { _ = directory.Close() }, nil
}

// Every candidate parent stays named until immutable readback and pointer
// publication finish; leaf-only custody would permit moving its ancestor.
func privateWikiHoldParents(root string, files []string) (func(), error) {
	parents := map[string]bool{root: true}
	for _, name := range files {
		if !privateWikiSafeName(name) || !filepath.IsLocal(filepath.FromSlash(name)) {
			return nil, ErrPrivateWikiUnavailable
		}
		for directory := filepath.Dir(filepath.Join(root, filepath.FromSlash(name))); directory != root; directory = filepath.Dir(directory) {
			parents[directory] = true
		}
	}
	var paths []string
	for directory := range parents {
		paths = append(paths, directory)
	}
	sort.Slice(paths, func(i, j int) bool {
		if len(paths[i]) != len(paths[j]) {
			return len(paths[i]) < len(paths[j])
		}
		return paths[i] < paths[j]
	})
	var held []func()
	release := func() {
		for i := len(held) - 1; i >= 0; i-- {
			held[i]()
		}
	}
	for _, directory := range paths {
		if err := memoryMkdirAll(directory, 0700); err != nil {
			release()
			return nil, err
		}
		custody, err := privateWikiHold(directory)
		if err != nil {
			release()
			return nil, err
		}
		held = append(held, custody)
	}
	return release, nil
}

func privateWikiRetire(path string) error { return winprivate.RemovePinned(path, true) }
