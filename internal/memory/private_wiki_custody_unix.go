//go:build !windows

package memory

func privateWikiHold(string) (func(), error) { return func() {}, nil }

func privateWikiHoldParents(string, []string) (func(), error) { return func() {}, nil }

func privateWikiRetire(path string) error { return memoryRemoveAll(path) }

func privateWikiFinishStaging(path string) { _ = memoryRemoveAll(path) }
