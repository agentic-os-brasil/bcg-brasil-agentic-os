package scheduler

import "golang.org/x/sys/unix"

func renamePrivateExclusive(directory int, source, target string) error {
	return unix.RenameatxNp(directory, source, directory, target, unix.RENAME_EXCL)
}
