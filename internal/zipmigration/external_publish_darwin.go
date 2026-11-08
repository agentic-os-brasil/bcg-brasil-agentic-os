//go:build darwin

package zipmigration

import "golang.org/x/sys/unix"

func externalRenameExclusive(fromFD int, from string, toFD int, to string) error {
	return unix.RenameatxNp(fromFD, from, toFD, to, unix.RENAME_EXCL)
}
