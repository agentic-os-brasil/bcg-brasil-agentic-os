//go:build !windows && !darwin && !linux

package scheduler

import "errors"

func renamePrivateExclusiveBetween(_ int, _ string, _ int, _ string) error {
	return errors.New("atomic exclusive private rename is unavailable on this platform")
}
