//go:build !windows

package priorwork

// Unix rename replaces a file atomically within one filesystem. Keep the
// existing durable writer on Unix, where that property is part of the
// platform contract.
func writeActiveManifest(rootPath string, manifest Manifest) error {
	return atomicWriteAt(rootPath, "active.json", manifest)
}
