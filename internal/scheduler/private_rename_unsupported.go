//go:build !windows && !darwin && !linux

package scheduler

import "errors"

func renamePrivateExclusive(_ int, _, _ string) error {
	return errors.New("atomic exclusive private rename is unavailable on this platform")
}
