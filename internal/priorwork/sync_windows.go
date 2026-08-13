//go:build windows

package priorwork

import "os"

// syncRootDirectory is a no-op on Windows. A directory handle opened with
// plain os.Open (including through os.Root) does not support Sync there
// (FlushFileBuffers on a directory requires FILE_FLAG_BACKUP_SEMANTICS and
// Windows returns "Access is denied" without it); NTFS's own metadata
// journal already makes the preceding write durable, so there is nothing
// safe to flush here.
func syncRootDirectory(root *os.Root, relative string) error {
	return nil
}
