//go:build windows

package priorwork

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// Windows os.Rename does not promise atomic replacement when the destination
// already exists. Stage the complete manifest in the same private root and
// use MoveFileEx with replace+write-through so readers see either the previous
// complete manifest or the next complete manifest. A crash before the replace
// leaves the old active pointer intact; a failed replace is fail-closed.
func writeActiveManifest(rootPath string, manifest Manifest) error {
	stageName, err := randomStageName(".", ".active-")
	if err != nil {
		return err
	}
	if err := atomicWriteAt(rootPath, stageName, manifest); err != nil {
		return err
	}
	defer func() { _ = removePrivatePath(rootPath, stageName) }()

	rootAbsolute, err := filepath.Abs(rootPath)
	if err != nil {
		return err
	}
	from, err := filepath.Abs(filepath.Join(rootAbsolute, stageName))
	if err != nil {
		return err
	}
	to := filepath.Join(rootAbsolute, "active.json")
	fromUTF16, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	toUTF16, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(fromUTF16, toUTF16, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return errors.New("prior-work active manifest replacement failed")
	}
	return nil
}

func removePrivatePath(rootPath, relative string) error {
	root, err := openAnchoredRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Remove(relative)
}
