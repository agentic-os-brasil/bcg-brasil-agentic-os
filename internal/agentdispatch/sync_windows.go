//go:build windows

package agentdispatch

// syncDirectory is a no-op on Windows. A directory handle opened with plain
// os.Open does not support Sync there (FlushFileBuffers on a directory
// requires FILE_FLAG_BACKUP_SEMANTICS and Windows returns "Access is
// denied" without it); NTFS's own metadata journal already makes the
// preceding rename durable, so there is nothing safe to flush here.
func syncDirectory(path string) error {
	return nil
}
